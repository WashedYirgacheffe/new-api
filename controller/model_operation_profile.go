package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type modelOperationProfilePayload struct {
	ProfileKey       string                 `json:"profile_key"`
	DisplayName      string                 `json:"display_name"`
	Description      string                 `json:"description"`
	Version          int                    `json:"version"`
	Operation        string                 `json:"operation"`
	EndpointType     string                 `json:"endpoint_type"`
	ExecutionMode    string                 `json:"execution_mode"`
	InputSchema      map[string]interface{} `json:"input_schema"`
	UISchema         map[string]interface{} `json:"ui_schema"`
	MaterialSchema   map[string]interface{} `json:"material_schema"`
	ResponseContract string                 `json:"response_contract"`
	SmokeTest        map[string]interface{} `json:"smoke_test"`
	Status           string                 `json:"status"`
}

type modelOperationProfileContract struct {
	ProfileKey       string                 `json:"profile_key"`
	DisplayName      string                 `json:"display_name"`
	Description      string                 `json:"description,omitempty"`
	Version          int                    `json:"version"`
	Operation        string                 `json:"operation"`
	EndpointType     string                 `json:"endpoint_type"`
	ExecutionMode    string                 `json:"execution_mode"`
	InputSchema      map[string]interface{} `json:"input_schema"`
	UISchema         map[string]interface{} `json:"ui_schema"`
	MaterialSchema   map[string]interface{} `json:"material_schema"`
	ResponseContract string                 `json:"response_contract"`
	SmokeTest        map[string]interface{} `json:"smoke_test"`
	Status           string                 `json:"status"`
	CreatedTime      int64                  `json:"created_time"`
	UpdatedTime      int64                  `json:"updated_time"`
}

type modelOperationBindingPayload struct {
	ModelName            string                 `json:"model_name"`
	Operation            string                 `json:"operation"`
	ProfileKey           string                 `json:"profile_key"`
	ProfileVersion       int                    `json:"profile_version"`
	Overrides            map[string]interface{} `json:"overrides"`
	Enabled              *bool                  `json:"enabled"`
	ExpectedContractHash string                 `json:"expected_contract_hash"`
}

type modelOperationBindingContract struct {
	ModelName         string                                 `json:"model_name"`
	Operation         string                                 `json:"operation"`
	ProfileKey        string                                 `json:"profile_key"`
	ProfileVersion    int                                    `json:"profile_version"`
	ContractVersion   int                                    `json:"contract_version"`
	ContractHash      string                                 `json:"contract_hash"`
	Overrides         map[string]interface{}                 `json:"overrides"`
	EffectiveContract *model.ModelOperationEffectiveContract `json:"effective_contract"`
	Enabled           bool                                   `json:"enabled"`
}

type modelOperationBindingRevisionContract struct {
	Id              int                    `json:"id"`
	BindingId       int                    `json:"binding_id"`
	ModelName       string                 `json:"model_name"`
	Operation       string                 `json:"operation"`
	Revision        int                    `json:"revision"`
	ProfileKey      string                 `json:"profile_key"`
	ProfileVersion  int                    `json:"profile_version"`
	ContractVersion int                    `json:"contract_version"`
	ContractHash    string                 `json:"contract_hash"`
	Overrides       map[string]interface{} `json:"overrides"`
	Enabled         bool                   `json:"enabled"`
	CreatedTime     int64                  `json:"created_time"`
}

type modelOperationBindingRollbackPayload struct {
	ModelName            string `json:"model_name"`
	Operation            string `json:"operation"`
	Revision             int    `json:"revision"`
	ExpectedContractHash string `json:"expected_contract_hash"`
}

type modelOperationParameterEvidencePayload struct {
	Id                 int    `json:"id"`
	ModelName          string `json:"model_name"`
	Operation          string `json:"operation"`
	Field              string `json:"field"`
	SourceType         string `json:"source_type"`
	SourceURL          string `json:"source_url"`
	SourceLocator      string `json:"source_locator"`
	VerificationStatus string `json:"verification_status"`
	VerifiedAt         int64  `json:"verified_at"`
	Notes              string `json:"notes"`
}

func writeModelOperationBindingError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrModelOperationBindingConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.ApiError(c, err)
}

func writeModelOperationEvidenceError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrModelOperationParameterEvidenceConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.ApiError(c, err)
}

func marshalContractObject(value map[string]interface{}) (string, error) {
	if value == nil {
		value = map[string]interface{}{}
	}
	data, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalContractObject(value string) map[string]interface{} {
	object := map[string]interface{}{}
	if strings.TrimSpace(value) == "" {
		return object
	}
	if err := common.UnmarshalJsonStr(value, &object); err != nil {
		return map[string]interface{}{}
	}
	return object
}

func buildModelOperationProfileContract(profile *model.ModelOperationProfile, version *model.ModelOperationProfileVersion) modelOperationProfileContract {
	return modelOperationProfileContract{
		ProfileKey:       profile.ProfileKey,
		DisplayName:      profile.DisplayName,
		Description:      profile.Description,
		Version:          version.Version,
		Operation:        version.Operation,
		EndpointType:     version.EndpointType,
		ExecutionMode:    version.ExecutionMode,
		InputSchema:      unmarshalContractObject(version.InputSchema),
		UISchema:         unmarshalContractObject(version.UISchema),
		MaterialSchema:   unmarshalContractObject(version.MaterialSchema),
		ResponseContract: version.ResponseContract,
		SmokeTest:        unmarshalContractObject(version.SmokeTest),
		Status:           version.Status,
		CreatedTime:      version.CreatedTime,
		UpdatedTime:      version.UpdatedTime,
	}
}

func buildModelOperationBindingContract(binding model.ModelOperationBinding, profile *model.ModelOperationProfile, version *model.ModelOperationProfileVersion) (modelOperationBindingContract, error) {
	effectiveContract, err := model.BuildModelOperationEffectiveContract(binding, profile, version)
	if err != nil {
		return modelOperationBindingContract{}, err
	}
	return modelOperationBindingContract{
		ModelName:         binding.ModelName,
		Operation:         binding.Operation,
		ProfileKey:        binding.ProfileKey,
		ProfileVersion:    binding.ProfileVersion,
		ContractVersion:   binding.ContractVersion,
		ContractHash:      binding.ContractHash,
		Overrides:         unmarshalContractObject(binding.Overrides),
		EffectiveContract: effectiveContract,
		Enabled:           binding.Enabled,
	}, nil
}

func GetModelOperationProfiles(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	profiles, total, err := model.GetModelOperationProfiles(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]modelOperationProfileContract, 0)
	for profileIndex := range profiles {
		profile := &profiles[profileIndex]
		for versionIndex := range profile.Versions {
			version := &profile.Versions[versionIndex]
			items = append(items, buildModelOperationProfileContract(profile, version))
		}
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func GetModelOperationProfile(c *gin.Context) {
	version := 0
	if rawVersion := strings.TrimSpace(c.Query("version")); rawVersion != "" {
		parsedVersion, err := strconv.Atoi(rawVersion)
		if err != nil || parsedVersion <= 0 {
			common.ApiErrorMsg(c, "version must be a positive integer")
			return
		}
		version = parsedVersion
	}
	profile, profileVersion, err := model.GetModelOperationProfileVersion(c.Param("profile_key"), version, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildModelOperationProfileContract(profile, profileVersion))
}

func SaveModelOperationProfile(c *gin.Context) {
	var payload modelOperationProfilePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	inputSchema, err := marshalContractObject(payload.InputSchema)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	uiSchema, err := marshalContractObject(payload.UISchema)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	materialSchema, err := marshalContractObject(payload.MaterialSchema)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	smokeTest, err := marshalContractObject(payload.SmokeTest)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	profile := model.ModelOperationProfile{
		ProfileKey:  payload.ProfileKey,
		DisplayName: payload.DisplayName,
		Description: payload.Description,
	}
	profileVersion := model.ModelOperationProfileVersion{
		Version:          payload.Version,
		Operation:        payload.Operation,
		EndpointType:     payload.EndpointType,
		ExecutionMode:    payload.ExecutionMode,
		InputSchema:      inputSchema,
		UISchema:         uiSchema,
		MaterialSchema:   materialSchema,
		ResponseContract: payload.ResponseContract,
		SmokeTest:        smokeTest,
		Status:           payload.Status,
	}
	if err := model.SaveModelOperationProfileVersion(&profile, &profileVersion); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildModelOperationProfileContract(&profile, &profileVersion))
}

func GetModelOperationBindings(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "model is required")
		return
	}
	bindingsByModel, err := model.GetModelOperationBindings([]string{modelName}, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	bindings := bindingsByModel[modelName]
	items := make([]modelOperationBindingContract, 0, len(bindings))
	for _, binding := range bindings {
		profile, version, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		contract, err := buildModelOperationBindingContract(binding, profile, version)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		items = append(items, contract)
	}
	common.ApiSuccess(c, items)
}

func SaveModelOperationBinding(c *gin.Context) {
	var payload modelOperationBindingPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	overrides, err := marshalContractObject(payload.Overrides)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	binding := model.ModelOperationBinding{
		ModelName:      payload.ModelName,
		Operation:      payload.Operation,
		ProfileKey:     payload.ProfileKey,
		ProfileVersion: payload.ProfileVersion,
		Overrides:      overrides,
		Enabled:        enabled,
	}
	if err := model.SaveModelOperationBindingWithExpectedHash(&binding, payload.ExpectedContractHash); err != nil {
		writeModelOperationBindingError(c, err)
		return
	}
	profile, version, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, false)
	if err != nil {
		writeModelOperationBindingError(c, err)
		return
	}
	contract, err := buildModelOperationBindingContract(binding, profile, version)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, contract)
}

func GetModelOperationBindingRevisions(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	operation := strings.TrimSpace(c.Query("operation"))
	if modelName == "" || operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	pageInfo := common.GetPageQuery(c)
	revisions, total, err := model.GetModelOperationBindingRevisions(modelName, operation, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]modelOperationBindingRevisionContract, 0, len(revisions))
	for _, revision := range revisions {
		items = append(items, modelOperationBindingRevisionContract{
			Id:              revision.Id,
			BindingId:       revision.BindingId,
			ModelName:       revision.ModelName,
			Operation:       revision.Operation,
			Revision:        revision.Revision,
			ProfileKey:      revision.ProfileKey,
			ProfileVersion:  revision.ProfileVersion,
			ContractVersion: revision.ContractVersion,
			ContractHash:    revision.ContractHash,
			Overrides:       unmarshalContractObject(revision.Overrides),
			Enabled:         revision.Enabled,
			CreatedTime:     revision.CreatedTime,
		})
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func RollbackModelOperationBinding(c *gin.Context) {
	var payload modelOperationBindingRollbackPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	binding, err := model.RollbackModelOperationBinding(
		payload.ModelName,
		payload.Operation,
		payload.Revision,
		payload.ExpectedContractHash,
	)
	if err != nil {
		writeModelOperationBindingError(c, err)
		return
	}
	profile, version, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	contract, err := buildModelOperationBindingContract(*binding, profile, version)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, contract)
}

func GetModelOperationParameterEvidence(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "model is required")
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.GetModelOperationParameterEvidence(
		modelName,
		c.Query("operation"),
		c.Query("field"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func SaveModelOperationParameterEvidence(c *gin.Context) {
	var payload modelOperationParameterEvidencePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	evidence := model.ModelOperationParameterEvidence{
		Id:                 payload.Id,
		ModelName:          payload.ModelName,
		Operation:          payload.Operation,
		Field:              payload.Field,
		SourceType:         payload.SourceType,
		SourceURL:          payload.SourceURL,
		SourceLocator:      payload.SourceLocator,
		VerificationStatus: payload.VerificationStatus,
		VerifiedAt:         payload.VerifiedAt,
		Notes:              payload.Notes,
	}
	if err := model.SaveModelOperationParameterEvidence(&evidence); err != nil {
		writeModelOperationEvidenceError(c, err)
		return
	}
	common.ApiSuccess(c, evidence)
}

func DeleteModelOperationParameterEvidence(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "evidence id must be a positive integer")
		return
	}
	if err := model.DeleteModelOperationParameterEvidence(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DeleteModelOperationBinding(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	operation := strings.TrimSpace(c.Query("operation"))
	if modelName == "" || operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	expectedHash := strings.TrimSpace(c.Query("expected_contract_hash"))
	if expectedHash == "" {
		expectedHash = strings.TrimSpace(c.GetHeader("If-Match"))
	}
	expectedHash = strings.TrimPrefix(expectedHash, "W/")
	expectedHash = strings.Trim(expectedHash, `"`)
	if err := model.DeleteModelOperationBinding(modelName, operation, expectedHash); err != nil {
		writeModelOperationBindingError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
