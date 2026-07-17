package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type playgroundGenerationAPIResponse struct {
	Success bool                       `json:"success"`
	Message string                     `json:"message"`
	Data    model.PlaygroundGeneration `json:"data"`
}

type playgroundGenerationPageAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items    []model.PlaygroundGeneration `json:"items"`
		Total    int64                        `json:"total"`
		Page     int                          `json:"page"`
		PageSize int                          `json:"page_size"`
	} `json:"data"`
}

type playgroundGenerationAssetAPIResponse struct {
	Success bool                                    `json:"success"`
	Message string                                  `json:"message"`
	Data    service.PlaygroundGenerationAssetResult `json:"data"`
}

func setupPlaygroundGenerationControllerTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	oldDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.PlaygroundGeneration{},
		&model.PlaygroundGenerationAsset{},
	))
	require.NoError(t, db.Create(&model.User{
		Id:       101,
		Username: "pg-controller-user",
		Password: "playground-test-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "pg-controller-00000000000000101",
	}).Error)
	t.Setenv(service.PlaygroundAssetDirectoryEnv, t.TempDir())
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func newPlaygroundGenerationContext(t *testing.T, method string, target string, body interface{}, userId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	requestBody := bytes.NewReader(nil)
	if body != nil {
		encoded, err := common.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(encoded)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userId)
	return ctx, recorder
}

func TestPlaygroundGenerationHTTPContract(t *testing.T) {
	setupPlaygroundGenerationControllerTest(t)

	createContext, createRecorder := newPlaygroundGenerationContext(t, http.MethodPost, "/pg/generations", gin.H{
		"operation":        "image",
		"model":            "deepwl/test-image",
		"group":            "default",
		"prompt":           "A glass bottle on a white background",
		"parameters":       gin.H{"resolution": "1024x1024"},
		"status":           "succeeded",
		"outputs":          []string{"https://example.com/forged.png"},
		"error":            "forged result",
		"contract_hash":    "contract-hash",
		"contract_version": 3,
		"pricing_version":  "pricing-version",
		"quoted_quota":     1000,
		"amount":           0.02,
	}, 101)
	CreatePlaygroundGeneration(createContext)

	require.Equal(t, http.StatusOK, createRecorder.Code)
	var createResponse playgroundGenerationAPIResponse
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &createResponse))
	require.True(t, createResponse.Success, createResponse.Message)
	assert.Equal(t, "pending", createResponse.Data.Status)
	assert.Empty(t, createResponse.Data.Outputs)
	assert.Empty(t, createResponse.Data.Error)
	assert.Zero(t, createResponse.Data.CompletedAt)
	assert.Positive(t, createResponse.Data.CreatedAt)
	assert.NotContains(t, createRecorder.Body.String(), "user_id")

	uploadContext, uploadRecorder := newPlaygroundGenerationContext(t, http.MethodPost, "/pg/generations/"+createResponse.Data.Id+"/assets", gin.H{
		"ordinal":  0,
		"data_url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zr1sAAAAASUVORK5CYII=",
	}, 101)
	uploadContext.Params = gin.Params{{Key: "id", Value: createResponse.Data.Id}}
	UploadPlaygroundGenerationAsset(uploadContext)

	var uploadResponse playgroundGenerationAssetAPIResponse
	require.NoError(t, common.Unmarshal(uploadRecorder.Body.Bytes(), &uploadResponse))
	require.True(t, uploadResponse.Success, uploadResponse.Message)
	assert.Equal(t, 0, uploadResponse.Data.Ordinal)
	assert.Equal(t, "image/png", uploadResponse.Data.MimeType)

	assetRouter := gin.New()
	assetRouter.Use(sessions.Sessions("session", cookie.NewStore([]byte("playground-asset-test"))))
	assetRouter.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "playground-user")
		session.Set("role", common.RoleCommonUser)
		session.Set("id", 101)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	assetRouter.GET(
		"/pg/generations/:id/assets/:ordinal",
		middleware.TokenOrUserAuth(),
		GetPlaygroundGenerationAsset,
	)
	loginRecorder := httptest.NewRecorder()
	assetRouter.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)
	assetRecorder := httptest.NewRecorder()
	assetRequest := httptest.NewRequest(http.MethodGet, uploadResponse.Data.URL, nil)
	for _, sessionCookie := range loginRecorder.Result().Cookies() {
		assetRequest.AddCookie(sessionCookie)
	}
	assetRouter.ServeHTTP(assetRecorder, assetRequest)

	require.Equal(t, http.StatusOK, assetRecorder.Code)
	assert.Equal(t, "image/png", assetRecorder.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-store", assetRecorder.Header().Get("Cache-Control"))
	assert.NotEmpty(t, assetRecorder.Body.Bytes())

	outputs := []string{"https://api.carlab.top" + uploadResponse.Data.URL}
	updateContext, updateRecorder := newPlaygroundGenerationContext(t, http.MethodPatch, "/pg/generations/"+createResponse.Data.Id, gin.H{
		"status":  "succeeded",
		"outputs": outputs,
	}, 101)
	updateContext.Params = gin.Params{{Key: "id", Value: createResponse.Data.Id}}
	UpdatePlaygroundGeneration(updateContext)

	var updateResponse playgroundGenerationAPIResponse
	require.NoError(t, common.Unmarshal(updateRecorder.Body.Bytes(), &updateResponse))
	require.True(t, updateResponse.Success, updateResponse.Message)
	assert.Equal(t, outputs, updateResponse.Data.Outputs)
	assert.Positive(t, updateResponse.Data.CompletedAt)

	listContext, listRecorder := newPlaygroundGenerationContext(t, http.MethodGet, "/pg/generations?operation=image&p=1&page_size=10", nil, 101)
	ListPlaygroundGenerations(listContext)

	var listResponse playgroundGenerationPageAPIResponse
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listResponse))
	require.True(t, listResponse.Success)
	assert.EqualValues(t, 1, listResponse.Data.Total)
	assert.Equal(t, 1, listResponse.Data.Page)
	assert.Equal(t, 10, listResponse.Data.PageSize)
	require.Len(t, listResponse.Data.Items, 1)
	assert.Equal(t, createResponse.Data.Id, listResponse.Data.Items[0].Id)

	deleteContext, deleteRecorder := newPlaygroundGenerationContext(t, http.MethodDelete, "/pg/generations/"+createResponse.Data.Id, nil, 101)
	deleteContext.Params = gin.Params{{Key: "id", Value: createResponse.Data.Id}}
	DeletePlaygroundGeneration(deleteContext)

	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	var deleteResponse struct {
		Success bool        `json:"success"`
		Data    interface{} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(deleteRecorder.Body.Bytes(), &deleteResponse))
	assert.True(t, deleteResponse.Success)
	assert.Nil(t, deleteResponse.Data)
}
