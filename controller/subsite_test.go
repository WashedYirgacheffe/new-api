package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subsiteErrorResponse struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

func setupSubsiteControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Subsite{},
		&model.SubsiteAdmin{},
		&model.SubsiteModel{},
		&model.Log{},
		&model.ModelOperationProfile{},
		&model.ModelOperationProfileVersion{},
		&model.ModelOperationBinding{},
	))
	model.InvalidatePricingCache()
	t.Cleanup(model.InvalidatePricingCache)
	return db
}

func invokeSubsiteHandler(
	t *testing.T,
	method string,
	target string,
	body string,
	params gin.Params,
	configure func(*gin.Context),
	handler gin.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = params
	if configure != nil {
		configure(context)
	}
	handler(context)
	return recorder
}

func decodeSubsiteError(t *testing.T, recorder *httptest.ResponseRecorder) subsiteErrorResponse {
	t.Helper()
	var response subsiteErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func createControllerTestSubsite(t *testing.T, code string, domain string, enabled bool) *model.Subsite {
	t.Helper()
	hash, err := common.Password2Hash("test-claim-password")
	require.NoError(t, err)
	site := &model.Subsite{
		Code:              code,
		Name:              "Controller Test Site",
		Domain:            domain,
		Version:           "v1",
		RouteGroup:        "gold",
		ClaimPasswordHash: hash,
		Enabled:           enabled,
	}
	require.NoError(t, model.CreateSubsite(site))
	return site
}

func TestGetSubsiteCatalogByDomainUsesStructuredHTTPFailures(t *testing.T) {
	setupSubsiteControllerTest(t)
	disabled := createControllerTestSubsite(t, "disabled-catalog-site", "disabled-catalog.example.com", false)
	enabled := createControllerTestSubsite(t, "group-catalog-site", "group-catalog.example.com", true)
	t.Cleanup(func() {
		_ = model.DeleteSubsite(disabled.Id)
		_ = model.DeleteSubsite(enabled.Id)
	})

	tests := []struct {
		name       string
		domain     string
		configure  func(*gin.Context)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found",
			domain:     "missing-catalog.example.com",
			wantStatus: http.StatusNotFound,
			wantCode:   "subsite_not_found",
		},
		{
			name:       "disabled",
			domain:     disabled.Domain,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "subsite_disabled",
		},
		{
			name:   "token route group mismatch",
			domain: enabled.Domain,
			configure: func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "silver")
				common.SetContextKey(c, constant.ContextKeyUserGroup, "silver")
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "subsite_route_group_mismatch",
		},
		{
			name:   "auto token group is not a unique site boundary",
			domain: enabled.Domain,
			configure: func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
				common.SetContextKey(c, constant.ContextKeyUserGroup, "gold")
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "subsite_route_group_mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := invokeSubsiteHandler(
				t,
				http.MethodGet,
				"/api/subsites/by-domain/"+tt.domain+"/catalog",
				"",
				gin.Params{{Key: "domain", Value: tt.domain}},
				tt.configure,
				GetSubsiteCatalogByDomain,
			)
			assert.Equal(t, tt.wantStatus, recorder.Code)
			response := decodeSubsiteError(t, recorder)
			assert.False(t, response.Success)
			assert.Equal(t, tt.wantCode, response.Error.Code)
			assert.Equal(t, response.Message, response.Error.Message)
		})
	}
}

func TestUpdateSubsiteModelsRejectsUnsafeModelsWithStructuredErrors(t *testing.T) {
	db := setupSubsiteControllerTest(t)
	site := createControllerTestSubsite(t, "model-gate-site", "model-gate.example.com", true)
	t.Cleanup(func() { _ = model.DeleteSubsite(site.Id) })

	nonRoutableModel := "controller-test-silver-only-model"
	require.NoError(t, db.Create(&model.Model{
		ModelName:   nonRoutableModel,
		DisplayName: "Silver only model",
		ModelType:   "text",
		Status:      1,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     98761,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "Subsite gate test channel",
		Group:  "silver",
		Models: nonRoutableModel,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "silver",
		Model:     nonRoutableModel,
		ChannelId: 98761,
		Enabled:   true,
	}).Error)
	model.InvalidatePricingCache()

	tests := []struct {
		name       string
		modelID    string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "runtime only variant",
			modelID:    "deepwl/grok-video-3-10s",
			wantStatus: http.StatusConflict,
			wantCode:   "model_runtime_only",
		},
		{
			name:       "unknown model",
			modelID:    "missing/subsite-model",
			wantStatus: http.StatusNotFound,
			wantCode:   "model_not_found",
		},
		{
			name:       "different route group",
			modelID:    nonRoutableModel,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "model_not_routable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := invokeSubsiteHandler(
				t,
				http.MethodPut,
				"/api/subsites/"+site.Code+"/models",
				fmt.Sprintf(`{"model_ids":[%q]}`, tt.modelID),
				gin.Params{{Key: "code", Value: site.Code}},
				func(c *gin.Context) {
					c.Set("role", common.RoleAdminUser)
					c.Set("id", 1)
				},
				UpdateSubsiteModels,
			)
			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.Equal(t, tt.wantCode, decodeSubsiteError(t, recorder).Error.Code)
		})
	}
}

func TestClaimableSubsitesWrapsItemsAndNeverReturnsPassword(t *testing.T) {
	setupSubsiteControllerTest(t)
	site := createControllerTestSubsite(t, "claimable-contract-site", "claimable-contract.example.com", true)
	require.NoError(t, model.DB.Create(&model.SubsiteAdmin{
		SubsiteId: site.Id,
		UserId:    5150,
	}).Error)
	t.Cleanup(func() { _ = model.DeleteSubsite(site.Id) })

	recorder := invokeSubsiteHandler(
		t,
		http.MethodGet,
		"/api/subsites/claimable",
		"",
		nil,
		func(c *gin.Context) { c.Set("id", 5150) },
		GetClaimableSubsites,
	)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "test-claim-password")
	assert.NotContains(t, recorder.Body.String(), "claim_password")

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Code    string `json:"code"`
				Claimed bool   `json:"claimed"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotEmpty(t, response.Data.Items)
	assert.Equal(t, site.Code, response.Data.Items[0].Code)
	assert.True(t, response.Data.Items[0].Claimed)
}

func TestBuildSubsiteModelsPayloadExcludesModelsMissingFromCurrentCatalog(t *testing.T) {
	setupSubsiteControllerTest(t)
	site := createControllerTestSubsite(t, "stale-model-site", "stale-model.example.com", true)
	t.Cleanup(func() { _ = model.DeleteSubsite(site.Id) })

	const retiredModel = "retired/subsite-model"
	require.NoError(t, model.ReplaceSubsiteModels(site.Id, []string{retiredModel}))
	payload, err := buildSubsiteModelsPayload(site)
	require.NoError(t, err)

	assert.NotContains(t, payload.EnabledModelIds, retiredModel)
}

func TestAdminSubsiteUpdateAndDeleteUseCodePath(t *testing.T) {
	setupSubsiteControllerTest(t)
	site := createControllerTestSubsite(t, "path-contract-site", "path-contract.example.com", true)
	t.Cleanup(func() {
		if current, err := model.GetSubsiteByCode("renamed-path-contract-site"); err == nil {
			_ = model.DeleteSubsite(current.Id)
		} else if current, err := model.GetSubsiteByCode(site.Code); err == nil {
			_ = model.DeleteSubsite(current.Id)
		}
	})

	updateBody := `{"code":"renamed-path-contract-site","name":"Renamed Site","domain":"renamed-path-contract.example.com","version":"v2","route_group":"gold","enabled":true}`
	updateRecorder := invokeSubsiteHandler(
		t,
		http.MethodPut,
		"/api/subsites/"+site.Code,
		updateBody,
		gin.Params{{Key: "code", Value: site.Code}},
		nil,
		UpdateSubsite,
	)
	require.Equal(t, http.StatusOK, updateRecorder.Code)
	updated, err := model.GetSubsiteByCode("renamed-path-contract-site")
	require.NoError(t, err)
	assert.Equal(t, "renamed-path-contract.example.com", updated.Domain)

	deleteRecorder := invokeSubsiteHandler(
		t,
		http.MethodDelete,
		"/api/subsites/"+updated.Code,
		"",
		gin.Params{{Key: "code", Value: updated.Code}},
		nil,
		DeleteSubsite,
	)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	_, err = model.GetSubsiteByCode(updated.Code)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestUpdateUserAllowsOnlyCommonAndSubsiteAdminRoleTransitions(t *testing.T) {
	db := setupSubsiteControllerTest(t)
	user := &model.User{
		Username:    "subsite-role-user",
		Password:    "test-password-hash",
		DisplayName: "Role Update User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		AffCode:     "subsite-role-user-aff",
	}
	require.NoError(t, db.Create(user).Error)
	t.Cleanup(func() { db.Delete(user) })

	updateRole := func(role int, callerRole int) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"id":%d,"username":%q,"display_name":%q,"role":%d}`, user.Id, user.Username, user.DisplayName, role)
		return invokeSubsiteHandler(
			t,
			http.MethodPut,
			"/api/user",
			body,
			nil,
			func(c *gin.Context) {
				c.Set("id", 9001)
				c.Set("username", "role-manager")
				c.Set("role", callerRole)
			},
			UpdateUser,
		)
	}

	promote := updateRole(common.RoleSubsiteAdminUser, common.RoleAdminUser)
	require.Equal(t, http.StatusOK, promote.Code)
	var promoteResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(promote.Body.Bytes(), &promoteResponse))
	require.True(t, promoteResponse.Success, promote.Body.String())
	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, common.RoleSubsiteAdminUser, user.Role)

	demote := updateRole(common.RoleCommonUser, common.RoleAdminUser)
	require.Equal(t, http.StatusOK, demote.Code)
	require.NoError(t, common.Unmarshal(demote.Body.Bytes(), &promoteResponse))
	require.True(t, promoteResponse.Success, demote.Body.String())
	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, user.Role)

	invalidPromotion := updateRole(common.RoleAdminUser, common.RoleAdminUser)
	require.Equal(t, http.StatusOK, invalidPromotion.Code)
	require.NoError(t, common.Unmarshal(invalidPromotion.Body.Bytes(), &promoteResponse))
	assert.False(t, promoteResponse.Success)
	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, user.Role)

	adminTarget := &model.User{
		Username:    "role-admin-target",
		Password:    "test-password-hash",
		DisplayName: "Admin Target",
		Role:        common.RoleAdminUser,
		Status:      common.UserStatusEnabled,
		AffCode:     "role-admin-target-aff",
	}
	require.NoError(t, db.Create(adminTarget).Error)
	t.Cleanup(func() { db.Delete(adminTarget) })
	adminBody := fmt.Sprintf(`{"id":%d,"username":%q,"display_name":%q,"role":%d}`, adminTarget.Id, adminTarget.Username, adminTarget.DisplayName, common.RoleSubsiteAdminUser)
	adminDemotion := invokeSubsiteHandler(
		t,
		http.MethodPut,
		"/api/user",
		adminBody,
		nil,
		func(c *gin.Context) {
			c.Set("id", 9002)
			c.Set("username", "root-role-manager")
			c.Set("role", common.RoleRootUser)
		},
		UpdateUser,
	)
	require.Equal(t, http.StatusOK, adminDemotion.Code)
	require.NoError(t, common.Unmarshal(adminDemotion.Body.Bytes(), &promoteResponse))
	assert.False(t, promoteResponse.Success)
	require.NoError(t, db.First(adminTarget, adminTarget.Id).Error)
	assert.Equal(t, common.RoleAdminUser, adminTarget.Role)
}
