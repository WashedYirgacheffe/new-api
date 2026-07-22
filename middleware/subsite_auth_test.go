package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func performSubsiteAuthRequest(t *testing.T, sessionRole int, databaseRole int, databaseStatus int, auth gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	oldDB := model.DB
	dsn := fmt.Sprintf("file:%s-%d-%d-%d?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"), sessionRole, databaseRole, databaseStatus)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, db.Create(&model.User{
		Id:       42,
		Username: "subsite-auth-user",
		Password: "subsite-auth-password",
		Role:     databaseRole,
		Status:   databaseStatus,
		Group:    "gold",
		AffCode:  "subsite-auth-aff",
	}).Error)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("subsite-auth-test-secret"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "subsite-auth-user")
		session.Set("role", sessionRole)
		session.Set("id", 42)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "gold")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", auth, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)
	require.NotEmpty(t, loginRecorder.Result().Cookies())

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(loginRecorder.Result().Cookies()[0])
	request.Header.Set("New-Api-User", "42")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeSubsiteAuthErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	return response.Error.Code
}

func TestSubsiteAdminAuthUsesCurrentDatabaseRole(t *testing.T) {
	tests := []struct {
		name         string
		sessionRole  int
		databaseRole int
		wantStatus   int
		wantCode     string
	}{
		{
			name:         "promotion takes effect immediately",
			sessionRole:  common.RoleCommonUser,
			databaseRole: common.RoleSubsiteAdminUser,
			wantStatus:   http.StatusNoContent,
		},
		{
			name:         "demotion takes effect immediately",
			sessionRole:  common.RoleSubsiteAdminUser,
			databaseRole: common.RoleCommonUser,
			wantStatus:   http.StatusForbidden,
			wantCode:     "subsite_role_required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performSubsiteAuthRequest(t, tt.sessionRole, tt.databaseRole, common.UserStatusEnabled, SubsiteAdminAuth())
			assert.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, decodeSubsiteAuthErrorCode(t, recorder))
			}
		})
	}
}

func TestSubsiteAdminAuthRejectsCurrentlyDisabledUser(t *testing.T) {
	recorder := performSubsiteAuthRequest(
		t,
		common.RoleSubsiteAdminUser,
		common.RoleSubsiteAdminUser,
		common.UserStatusDisabled,
		SubsiteAdminAuth(),
	)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "subsite_user_disabled", decodeSubsiteAuthErrorCode(t, recorder))
}

func TestSubsiteAdminAuthRejectsInvalidAccessTokenWithStructuredStatus(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("subsite-auth-test-secret"))))
	router.GET("/protected", SubsiteAdminAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer invalid-subsite-access-token")
	request.Header.Set("New-Api-User", "42")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, "subsite_access_token_invalid", decodeSubsiteAuthErrorCode(t, recorder))
}

func TestSubsitePlatformAdminAuthRejectsSubsiteAdmin(t *testing.T) {
	recorder := performSubsiteAuthRequest(
		t,
		common.RoleAdminUser,
		common.RoleSubsiteAdminUser,
		common.UserStatusEnabled,
		SubsitePlatformAdminAuth(),
	)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "subsite_platform_admin_required", decodeSubsiteAuthErrorCode(t, recorder))
}

func TestSubsitePlatformAdminAuthAllowsCurrentPlatformAdmins(t *testing.T) {
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		recorder := performSubsiteAuthRequest(
			t,
			common.RoleCommonUser,
			role,
			common.UserStatusEnabled,
			SubsitePlatformAdminAuth(),
		)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	}
}
