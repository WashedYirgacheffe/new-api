package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelOperationProfileStatusDraft     = "draft"
	ModelOperationProfileStatusPublished = "published"
	ModelOperationProfileStatusArchived  = "archived"
)

type ModelOperationProfile struct {
	Id          int                            `json:"id"`
	ProfileKey  string                         `json:"profile_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	DisplayName string                         `json:"display_name" gorm:"type:varchar(128);not null"`
	Description string                         `json:"description,omitempty" gorm:"type:text"`
	CreatedTime int64                          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64                          `json:"updated_time" gorm:"bigint"`
	Versions    []ModelOperationProfileVersion `json:"versions,omitempty" gorm:"-"`
}

type ModelOperationProfileVersion struct {
	Id               int    `json:"id"`
	ProfileId        int    `json:"profile_id" gorm:"not null;uniqueIndex:uk_profile_version,priority:1;index"`
	Version          int    `json:"version" gorm:"not null;uniqueIndex:uk_profile_version,priority:2"`
	Operation        string `json:"operation" gorm:"type:varchar(64);not null;index"`
	EndpointType     string `json:"endpoint_type" gorm:"type:varchar(64);not null"`
	ExecutionMode    string `json:"execution_mode" gorm:"type:varchar(32);not null"`
	InputSchema      string `json:"input_schema" gorm:"type:text;not null"`
	UISchema         string `json:"ui_schema" gorm:"type:text;not null"`
	MaterialSchema   string `json:"material_schema" gorm:"type:text;not null"`
	ResponseContract string `json:"response_contract" gorm:"type:varchar(128);not null"`
	SmokeTest        string `json:"smoke_test" gorm:"type:text;not null"`
	Status           string `json:"status" gorm:"type:varchar(16);not null;index"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint"`
}

type ModelOperationBinding struct {
	Id              int    `json:"id"`
	ModelName       string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_operation,priority:1;index"`
	Operation       string `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex:uk_model_operation,priority:2;index"`
	ProfileKey      string `json:"profile_key" gorm:"type:varchar(128);not null;index"`
	ProfileVersion  int    `json:"profile_version" gorm:"not null"`
	ContractVersion int    `json:"contract_version"`
	ContractHash    string `json:"contract_hash" gorm:"type:varchar(64);index"`
	Overrides       string `json:"overrides" gorm:"type:text;not null"`
	Enabled         bool   `json:"enabled" gorm:"not null"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint"`
}

func normalizeContractIdentifier(value string, maxLength int) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("identifier is required")
	}
	if len(value) > maxLength {
		return "", fmt.Errorf("identifier must be %d characters or fewer", maxLength)
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return "", errors.New("identifier may only contain lowercase letters, numbers, dots, underscores, and hyphens")
	}
	return value, nil
}

func normalizeContractJSONObject(field string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "{}"
	}
	var object map[string]interface{}
	if err := common.UnmarshalJsonStr(value, &object); err != nil {
		return "", fmt.Errorf("%s must be a JSON object: %w", field, err)
	}
	if object == nil {
		return "", fmt.Errorf("%s must be a JSON object", field)
	}
	normalized, err := common.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("normalize %s: %w", field, err)
	}
	return string(normalized), nil
}

func normalizeModelOperationProfile(profile *ModelOperationProfile, version *ModelOperationProfileVersion) error {
	profileKey, err := normalizeContractIdentifier(profile.ProfileKey, 128)
	if err != nil {
		return fmt.Errorf("profile_key: %w", err)
	}
	operation, err := normalizeContractIdentifier(version.Operation, 64)
	if err != nil {
		return fmt.Errorf("operation: %w", err)
	}
	endpointType, err := normalizeContractIdentifier(version.EndpointType, 64)
	if err != nil {
		return fmt.Errorf("endpoint_type: %w", err)
	}
	executionMode, err := normalizeContractIdentifier(version.ExecutionMode, 32)
	if err != nil {
		return fmt.Errorf("execution_mode: %w", err)
	}
	status := strings.ToLower(strings.TrimSpace(version.Status))
	switch status {
	case ModelOperationProfileStatusDraft, ModelOperationProfileStatusPublished, ModelOperationProfileStatusArchived:
	default:
		return fmt.Errorf("unsupported profile status %q", version.Status)
	}
	if version.Version <= 0 {
		return errors.New("version must be greater than zero")
	}
	if strings.TrimSpace(profile.DisplayName) == "" {
		return errors.New("display_name is required")
	}
	if len(profile.DisplayName) > 128 {
		return errors.New("display_name must be 128 characters or fewer")
	}
	if strings.TrimSpace(version.ResponseContract) == "" {
		return errors.New("response_contract is required")
	}
	if len(version.ResponseContract) > 128 {
		return errors.New("response_contract must be 128 characters or fewer")
	}
	inputSchema, err := normalizeContractJSONObject("input_schema", version.InputSchema)
	if err != nil {
		return err
	}
	uiSchema, err := normalizeContractJSONObject("ui_schema", version.UISchema)
	if err != nil {
		return err
	}
	materialSchema, err := normalizeContractJSONObject("material_schema", version.MaterialSchema)
	if err != nil {
		return err
	}
	smokeTest, err := normalizeContractJSONObject("smoke_test", version.SmokeTest)
	if err != nil {
		return err
	}

	profile.ProfileKey = profileKey
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.Description = strings.TrimSpace(profile.Description)
	version.Operation = operation
	version.EndpointType = endpointType
	version.ExecutionMode = executionMode
	version.ResponseContract = strings.TrimSpace(version.ResponseContract)
	version.Status = status
	version.InputSchema = inputSchema
	version.UISchema = uiSchema
	version.MaterialSchema = materialSchema
	version.SmokeTest = smokeTest
	return nil
}

func SaveModelOperationProfileVersion(profile *ModelOperationProfile, version *ModelOperationProfileVersion) error {
	if profile == nil || version == nil {
		return errors.New("profile and version are required")
	}
	if err := normalizeModelOperationProfile(profile, version); err != nil {
		return err
	}

	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var storedProfile ModelOperationProfile
		err := tx.Where("profile_key = ?", profile.ProfileKey).First(&storedProfile).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			profile.CreatedTime = now
			profile.UpdatedTime = now
			if err := tx.Create(profile).Error; err != nil {
				return err
			}
			storedProfile = *profile
		case err != nil:
			return err
		default:
			if err := tx.Model(&storedProfile).Updates(map[string]interface{}{
				"display_name": profile.DisplayName,
				"description":  profile.Description,
				"updated_time": now,
			}).Error; err != nil {
				return err
			}
			storedProfile.DisplayName = profile.DisplayName
			storedProfile.Description = profile.Description
			storedProfile.UpdatedTime = now
			*profile = storedProfile
		}

		var storedVersion ModelOperationProfileVersion
		err = tx.Where("profile_id = ? AND version = ?", storedProfile.Id, version.Version).First(&storedVersion).Error
		version.ProfileId = storedProfile.Id
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			version.CreatedTime = now
			version.UpdatedTime = now
			return tx.Create(version).Error
		case err != nil:
			return err
		case storedVersion.Status != ModelOperationProfileStatusDraft:
			return fmt.Errorf("profile %s version %d is immutable after leaving draft status", profile.ProfileKey, version.Version)
		default:
			version.Id = storedVersion.Id
			version.CreatedTime = storedVersion.CreatedTime
			version.UpdatedTime = now
			return tx.Model(&storedVersion).Select(
				"operation", "endpoint_type", "execution_mode", "input_schema", "ui_schema",
				"material_schema", "response_contract", "smoke_test", "status", "updated_time",
			).Updates(version).Error
		}
	})
}

func GetModelOperationProfiles(offset int, limit int) ([]ModelOperationProfile, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&ModelOperationProfile{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var profiles []ModelOperationProfile
	if err := DB.Order("profile_key ASC").Offset(offset).Limit(limit).Find(&profiles).Error; err != nil {
		return nil, 0, err
	}
	if len(profiles) == 0 {
		return profiles, total, nil
	}
	profileIds := make([]int, 0, len(profiles))
	profileIndex := make(map[int]int, len(profiles))
	for index := range profiles {
		profileIds = append(profileIds, profiles[index].Id)
		profileIndex[profiles[index].Id] = index
	}
	var versions []ModelOperationProfileVersion
	if err := DB.Where("profile_id IN ?", profileIds).Order("version DESC").Find(&versions).Error; err != nil {
		return nil, 0, err
	}
	for _, version := range versions {
		index, ok := profileIndex[version.ProfileId]
		if !ok {
			continue
		}
		profiles[index].Versions = append(profiles[index].Versions, version)
	}
	return profiles, total, nil
}

func GetModelOperationProfileVersion(profileKey string, version int, publishedOnly bool) (*ModelOperationProfile, *ModelOperationProfileVersion, error) {
	profileKey, err := normalizeContractIdentifier(profileKey, 128)
	if err != nil {
		return nil, nil, err
	}
	var profile ModelOperationProfile
	if err := DB.Where("profile_key = ?", profileKey).First(&profile).Error; err != nil {
		return nil, nil, err
	}
	query := DB.Where("profile_id = ?", profile.Id)
	if version > 0 {
		query = query.Where("version = ?", version)
	} else {
		query = query.Order("version DESC")
	}
	if publishedOnly {
		query = query.Where("status = ?", ModelOperationProfileStatusPublished)
	}
	var profileVersion ModelOperationProfileVersion
	if err := query.First(&profileVersion).Error; err != nil {
		return nil, nil, err
	}
	return &profile, &profileVersion, nil
}

func SaveModelOperationBinding(binding *ModelOperationBinding) error {
	if binding == nil {
		return errors.New("binding is required")
	}
	binding.ModelName = strings.TrimSpace(binding.ModelName)
	if binding.ModelName == "" || len(binding.ModelName) > 255 {
		return errors.New("model_name is required and must be 255 characters or fewer")
	}
	operation, err := normalizeContractIdentifier(binding.Operation, 64)
	if err != nil {
		return fmt.Errorf("operation: %w", err)
	}
	profileKey, err := normalizeContractIdentifier(binding.ProfileKey, 128)
	if err != nil {
		return fmt.Errorf("profile_key: %w", err)
	}
	if binding.ProfileVersion <= 0 {
		return errors.New("profile_version must be greater than zero")
	}
	var modelCount int64
	if err := DB.Model(&Model{}).Where("model_name = ? AND status = ?", binding.ModelName, 1).Count(&modelCount).Error; err != nil {
		return err
	}
	if modelCount == 0 {
		return fmt.Errorf("enabled model %s does not exist", binding.ModelName)
	}
	profile, version, err := GetModelOperationProfileVersion(profileKey, binding.ProfileVersion, true)
	if err != nil {
		return fmt.Errorf("published profile version does not exist: %w", err)
	}
	if version.Operation != operation {
		return fmt.Errorf("binding operation %s does not match profile operation %s", operation, version.Operation)
	}
	overrides, _, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, version)
	if err != nil {
		return err
	}

	now := common.GetTimestamp()
	binding.Operation = operation
	binding.ProfileKey = profile.ProfileKey
	binding.Overrides = overrides
	binding.UpdatedTime = now
	contractHash, err := computeModelOperationContractHash(*binding, profile, version)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var stored ModelOperationBinding
		err := lockForUpdate(tx).Where("model_name = ? AND operation = ?", binding.ModelName, operation).First(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			binding.ContractVersion = 1
			binding.ContractHash = contractHash
			binding.CreatedTime = now
			return tx.Create(binding).Error
		}
		if err != nil {
			return err
		}
		contractVersion := stored.ContractVersion
		if contractVersion <= 0 {
			contractVersion = 1
		} else if stored.ContractHash != contractHash {
			contractVersion++
		}
		binding.Id = stored.Id
		binding.ContractVersion = contractVersion
		binding.ContractHash = contractHash
		binding.CreatedTime = stored.CreatedTime
		return tx.Model(&stored).Select(
			"profile_key", "profile_version", "contract_version", "contract_hash", "overrides", "enabled", "updated_time",
		).Updates(binding).Error
	})
}

func GetModelOperationBindings(modelNames []string, enabledOnly bool) (map[string][]ModelOperationBinding, error) {
	result := make(map[string][]ModelOperationBinding)
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return result, nil
	}
	query := DB.Where("model_name IN ?", modelNames)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var bindings []ModelOperationBinding
	if err := query.Order("operation ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		result[binding.ModelName] = append(result[binding.ModelName], binding)
	}
	return result, nil
}

func DeleteModelOperationBinding(modelName string, operation string) error {
	modelName = strings.TrimSpace(modelName)
	operation, err := normalizeContractIdentifier(operation, 64)
	if err != nil {
		return err
	}
	return DB.Where("model_name = ? AND operation = ?", modelName, operation).Delete(&ModelOperationBinding{}).Error
}

type defaultModelOperationProfile struct {
	ModelType        string
	ModelNames       []string
	BindingOverrides string
	Profile          ModelOperationProfile
	Version          ModelOperationProfileVersion
}

const (
	gemini25FlashImageModelName = "deepwl/gemini-2.5-flash-image"
	geminiProImageModelName     = "deepwl/gemini-3-pro-image"
	gemini31FlashImageModelName = "deepwl/gemini-3.1-flash-image-preview"
	gptImage2AllModelName       = "deepwl/gpt-image-2-all"
	omniFastModelName           = "deepwl/omni-fast"
	omniFastV2VModelName        = "deepwl/omni-fast-v2v"
	geminiNativeOverrides       = `{"branding":{"icon_key":"gemini","description":"Gemini 原生图像生成，支持 8 种比例与 1K/2K/4K 输出。"},"ui_schema":{"placements":{"prompt":"prompt","aspect_ratio":"footer","resolution":"footer"},"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}},"request_contract":{"adapter":"gemini-image","field_map":{"aspect_ratio":"aspectRatio","resolution":"imageSize"},"coercions":{}},"dispatch_path":"/v1beta/models/{model}:generateContent","parameter_defaults":{"aspect_ratio":"1:1","resolution":"1K"}}`
	geminiProImageOverrides     = `{"branding":{"icon_key":"gemini","description":"Gemini 原生图像生成，支持 8 种比例与 1K/2K/4K 输出。"},"ui_schema":{"placements":{"prompt":"prompt","aspect_ratio":"footer","resolution":"footer"},"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}},"request_contract":{"adapter":"gemini-image","field_map":{"aspect_ratio":"aspectRatio","resolution":"imageSize"},"coercions":{}},"pricing_rule":{"mode":"newapi-base-with-parameter-multipliers","multipliers":[{"field":"resolution","values":{"1K":1,"2K":1.25,"4K":1.5}}]},"dispatch_path":"/v1beta/models/{model}:generateContent","parameter_defaults":{"aspect_ratio":"1:1","resolution":"1K"}}`
	gemini31FlashImageOverrides = `{"branding":{"icon_key":"gemini","description":"Gemini 原生图像生成，支持 8 种比例与 1K/2K/4K 输出。"},"ui_schema":{"placements":{"prompt":"prompt","aspect_ratio":"footer","resolution":"footer"},"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}},"request_contract":{"adapter":"gemini-image","field_map":{"aspect_ratio":"aspectRatio","resolution":"imageSize"},"coercions":{}},"pricing_rule":{"mode":"newapi-base-with-parameter-multipliers","multipliers":[{"field":"resolution","values":{"1K":1,"2K":1.2,"4K":1.5}}]},"dispatch_path":"/v1beta/models/{model}:generateContent","parameter_defaults":{"aspect_ratio":"1:1","resolution":"1K"}}`
	geminiOpenAIOverrides       = `{"branding":{"icon_key":"gemini","description":"Gemini 2.5 Flash Image 快速图片生成，使用 OpenAI Images 兼容入口。"},"ui_schema":{"placements":{"prompt":"prompt","size":"footer","n":"batch"},"widgets":{"prompt":"textarea","size":"select","n":"segmented"}},"request_contract":{"adapter":"openai-image","field_map":{"size":"size","n":"n"},"coercions":{}},"parameter_defaults":{"size":"1024x1024","n":1}}`
	gptImage2AllLegacyOverrides = `{"branding":{"icon_key":"openai","description":"GPT Image 2 图像生成模型，当前发布合同仅开放已验证的文本生图能力。"},"ui_schema":{"placements":{"prompt":"prompt"},"widgets":{"prompt":"textarea"}},"request_contract":{"adapter":"openai-chat","field_map":{},"coercions":{}}}`
	gptImage2Overrides          = `{"branding":{"icon_key":"openai","description":"GPT Image 2 图像生成模型，支持六种已记录尺寸和 URL/Base64 返回。"},"ui_schema":{"placements":{"prompt":"prompt","size":"footer","n":"batch","response_format":"hidden"},"widgets":{"prompt":"textarea","size":"select","n":"segmented","response_format":"hidden"}},"request_contract":{"adapter":"openai-image","field_map":{"size":"size","n":"n","response_format":"response_format"},"coercions":{}},"parameter_defaults":{"size":"1024x1024","n":1,"response_format":"url"}}`
	omniFastOverrides           = `{"branding":{"icon_key":"openai","description":"Omni Video 支持文生视频和最多 5 张参考图，当前按独立模型固定采购价结算。"},"ui_schema":{"placements":{"prompt":"prompt","seconds":"footer","aspect_ratio":"footer","resolution":"footer"},"widgets":{"prompt":"textarea","seconds":"stepper","aspect_ratio":"select","resolution":"segmented"}},"material_schema":{"image":{"max_items":5,"request_field":"images","transport":"url"},"video":{"max_items":0},"audio":{"max_items":0}},"request_contract":{"adapter":"openai-video","field_map":{"seconds":"seconds","aspect_ratio":"aspect_ratio","resolution":"resolution"},"coercions":{"seconds":"string"}},"dispatch_path":"/v1/videos","poll_path":"/v1/videos/{task_id}","parameter_defaults":{"seconds":8,"aspect_ratio":"16:9","resolution":"720p"}}`
	omniFastV2VOverrides        = `{"branding":{"icon_key":"openai","description":"Omni Video V2V 支持单个公网 MP4 参考视频的编辑、延长或重新生成。"},"ui_schema":{"placements":{"prompt":"prompt","seconds":"footer","aspect_ratio":"footer","resolution":"footer"},"widgets":{"prompt":"textarea","seconds":"stepper","aspect_ratio":"select","resolution":"segmented"}},"material_schema":{"image":{"max_items":0},"video":{"min_items":1,"max_items":1,"request_field":"video","transport":"url"},"audio":{"max_items":0}},"request_contract":{"adapter":"openai-video","field_map":{"seconds":"seconds","aspect_ratio":"aspect_ratio","resolution":"resolution"},"coercions":{"seconds":"string"}},"dispatch_path":"/v1/videos","poll_path":"/v1/videos/{task_id}","parameter_defaults":{"seconds":8,"aspect_ratio":"16:9","resolution":"720p"}}`
)

func defaultModelOperationProfiles() []defaultModelOperationProfile {
	return []defaultModelOperationProfile{
		{
			ModelType:        "text",
			ModelNames:       []string{"deepwl/gemini-3.5-flash"},
			BindingOverrides: `{"branding":{"icon_key":"gemini","description":"Gemini 3.5 Flash 已正式发布，适合智能体执行、编码和长任务。"},"ui_schema":{"placements":{"prompt":"prompt","temperature":"advanced","top_p":"advanced","max_tokens":"advanced"},"widgets":{"prompt":"textarea","temperature":"stepper","top_p":"stepper","max_tokens":"stepper"}},"request_contract":{"adapter":"openai-chat","field_map":{},"coercions":{}}}`,
			Profile:          ModelOperationProfile{ProfileKey: "text.chat.basic", DisplayName: "通用文本对话", Description: "OpenAI 兼容文本对话的最低公共能力。"},
			Version:          ModelOperationProfileVersion{Version: 3, Operation: "text.chat", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"temperature":{"type":"number","minimum":0,"maximum":2},"top_p":{"type":"number","minimum":0,"maximum":1},"max_tokens":{"type":"integer","minimum":1,"maximum":131072,"default":1024}},"required":["prompt"],"additionalProperties":true}`, UISchema: `{"order":["prompt","temperature","top_p","max_tokens"],"widgets":{"prompt":"textarea","temperature":"stepper","top_p":"stepper","max_tokens":"stepper"}}`, MaterialSchema: `{}`, ResponseContract: "openai-chat-completion-v1", SmokeTest: `{"prompt":"请只回复 OK","max_tokens":128}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "image",
			Profile:   ModelOperationProfile{ProfileKey: "image.generate.basic", DisplayName: "通用图片生成", Description: "通过 OpenAI 兼容接口调用图片模型的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 2, Operation: "image.generate", EndpointType: "image-generation", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"n":{"type":"integer","minimum":1,"maximum":4},"size":{"type":"string","minLength":1,"maxLength":32},"quality":{"type":"string","minLength":1,"maxLength":32},"response_format":{"type":"string","enum":["url","b64_json"]}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","size","quality","n","response_format"],"widgets":{"prompt":"textarea","size":"text","quality":"text","n":"stepper","response_format":"select"}}`, MaterialSchema: `{"image":{"max_items":4}}`, ResponseContract: "openai-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","size":"1024x1024","n":1}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			Profile: ModelOperationProfile{ProfileKey: "image.generate.chat", DisplayName: "Chat 图片生成", Description: "通过 OpenAI Chat Completions 返回 Markdown 图片链接的同步图片能力。"},
			Version: ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt"],"widgets":{"prompt":"textarea"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "openai-chat-markdown-images-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{"deepwl/gpt-image-2", gptImage2AllModelName},
			BindingOverrides: gptImage2Overrides,
			Profile:          ModelOperationProfile{ProfileKey: "image.generate.gpt-image-2", DisplayName: "GPT Image 2 图片生成", Description: "DeepWL GPT Image 2 的 OpenAI Images 标准能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "image-generation", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"size":{"type":"string","enum":["1024x1024","1536x1152","1536x1024","1024x1536","1920x1080","1080x1920"],"default":"1024x1024"},"n":{"type":"integer","enum":[1],"default":1,"maximum":1},"response_format":{"type":"string","enum":["url","b64_json"],"default":"url"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","size","n","response_format"],"widgets":{"prompt":"textarea","size":"select","n":"segmented","response_format":"select"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "openai-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","size":"1024x1024","n":1,"response_format":"url"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{geminiProImageModelName},
			BindingOverrides: geminiProImageOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "image.generate.gemini-native", DisplayName: "Gemini 原生图片生成", Description: "DeepWL Gemini generateContent 图片生成能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "gemini", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"aspect_ratio":{"type":"string","enum":["1:1","16:9","9:16","4:3","3:4","3:2","2:3","21:9"],"default":"1:1"},"resolution":{"type":"string","enum":["1K","2K","4K"],"default":"1K"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","aspect_ratio","resolution"],"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "gemini-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","aspect_ratio":"1:1","resolution":"1K"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{gemini31FlashImageModelName},
			BindingOverrides: gemini31FlashImageOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "image.generate.gemini-native", DisplayName: "Gemini 原生图片生成", Description: "DeepWL Gemini generateContent 图片生成能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "gemini", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"aspect_ratio":{"type":"string","enum":["1:1","16:9","9:16","4:3","3:4","3:2","2:3","21:9"],"default":"1:1"},"resolution":{"type":"string","enum":["1K","2K","4K"],"default":"1K"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","aspect_ratio","resolution"],"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "gemini-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","aspect_ratio":"1:1","resolution":"1K"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{gemini25FlashImageModelName},
			BindingOverrides: geminiNativeOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "image.generate.gemini-native", DisplayName: "Gemini 原生图片生成", Description: "DeepWL Gemini generateContent 图片生成能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "gemini", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"aspect_ratio":{"type":"string","enum":["1:1","16:9","9:16","4:3","3:4","3:2","2:3","21:9"],"default":"1:1"},"resolution":{"type":"string","enum":["1K","2K","4K"],"default":"1K"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","aspect_ratio","resolution"],"widgets":{"prompt":"textarea","aspect_ratio":"select","resolution":"segmented"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "gemini-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","aspect_ratio":"1:1","resolution":"1K"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			BindingOverrides: geminiOpenAIOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "image.generate.gemini-openai", DisplayName: "Gemini OpenAI 图片生成", Description: "DeepWL Gemini 图片模型的 OpenAI Images 兼容能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "image.generate", EndpointType: "image-generation", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"size":{"type":"string","enum":["1024x1024","1536x1152","1536x1024","1024x1536","1920x1080","1080x1920"],"default":"1024x1024"},"n":{"type":"integer","enum":[1],"default":1,"maximum":1}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","size","n"],"widgets":{"prompt":"textarea","size":"select","n":"segmented"}}`, MaterialSchema: `{"image":{"max_items":0}}`, ResponseContract: "openai-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","size":"1024x1024","n":1}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType:        "video",
			ModelNames:       []string{"deepwl/grok-video-3"},
			BindingOverrides: `{"branding":{"icon_key":"grok","description":"xAI Grok 视频生成模型；当前生产合同固定为已验证的 6 秒基础调用。"},"input_schema":{"properties":{"size":{"type":"string","enum":["720P"],"default":"720P"}}},"ui_schema":{"placements":{"prompt":"prompt","seconds":"footer","size":"footer","image_url":"hidden"},"widgets":{"prompt":"textarea","seconds":"segmented","size":"segmented","image_url":"hidden"}},"material_schema":{"image":{"max_items":0},"video":{"max_items":0}},"request_contract":{"adapter":"openai-video","field_map":{"seconds":"seconds","size":"size"},"coercions":{}},"poll_path":"/v1/video/generations/{task_id}"}`,
			Profile:          ModelOperationProfile{ProfileKey: "video.generate.basic", DisplayName: "通用视频生成", Description: "通过 OpenAI 兼容接口调用视频模型的最低公共能力。"},
			Version:          ModelOperationProfileVersion{Version: 3, Operation: "video.generate", EndpointType: "openai-video", ExecutionMode: "async", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"string","enum":["6"],"default":"6"},"size":{"type":"string","minLength":1,"maxLength":32},"image_url":{"type":"string","format":"uri","maxLength":4096}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","seconds","size","image_url"],"widgets":{"prompt":"textarea","seconds":"select","size":"text","image_url":"text"}}`, MaterialSchema: `{"image":{"max_items":1},"video":{"max_items":1}}`, ResponseContract: "openai-video-task-v1", SmokeTest: `{"prompt":"生成一个六秒钟的简单镜头运动","seconds":"6","size":"720P"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{omniFastModelName},
			BindingOverrides: omniFastOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "video.generate.omni", DisplayName: "Omni 视频生成", Description: "DeepWL Omni Fast 的 JSON 视频生成能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "video.generate", EndpointType: "openai-video", ExecutionMode: "async", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"integer","minimum":4,"maximum":30,"default":8},"aspect_ratio":{"type":"string","enum":["16:9","9:16","1:1","4:3","3:4"],"default":"16:9"},"resolution":{"type":"string","enum":["720p","1080p","2k","4k"],"default":"720p"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","seconds","aspect_ratio","resolution"],"widgets":{"prompt":"textarea","seconds":"stepper","aspect_ratio":"select","resolution":"segmented"}}`, MaterialSchema: `{"image":{"max_items":5,"request_field":"images","transport":"url"},"video":{"max_items":0},"audio":{"max_items":0}}`, ResponseContract: "openai-video-task-v1", SmokeTest: `{"prompt":"生成一个简洁的海浪镜头","seconds":4,"aspect_ratio":"16:9","resolution":"720p"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelNames:       []string{omniFastV2VModelName},
			BindingOverrides: omniFastV2VOverrides,
			Profile:          ModelOperationProfile{ProfileKey: "video.generate.omni-v2v", DisplayName: "Omni 参考视频生成", Description: "DeepWL Omni Fast V2V 的参考视频编辑与重新生成能力。"},
			Version:          ModelOperationProfileVersion{Version: 1, Operation: "video.generate", EndpointType: "openai-video", ExecutionMode: "async", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"integer","minimum":4,"maximum":30,"default":8},"aspect_ratio":{"type":"string","enum":["16:9","9:16","1:1","4:3","3:4"],"default":"16:9"},"resolution":{"type":"string","enum":["720p","1080p","2k","4k"],"default":"720p"}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","seconds","aspect_ratio","resolution"],"widgets":{"prompt":"textarea","seconds":"stepper","aspect_ratio":"select","resolution":"segmented"}}`, MaterialSchema: `{"image":{"max_items":0},"video":{"min_items":1,"max_items":1,"request_field":"video","transport":"url"},"audio":{"max_items":0}}`, ResponseContract: "openai-video-task-v1", SmokeTest: `{"prompt":"将参考视频重新生成成夜景风格","seconds":4,"aspect_ratio":"16:9","resolution":"720p"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			Profile: ModelOperationProfile{ProfileKey: "video.generate.seedance-2", DisplayName: "Seedance 2.0 视频生成", Description: "DeepWL Seedance 2.0 多模态视频能力模板；当前 Key 未授权模型，仅保存文档合同。"},
			Version: ModelOperationProfileVersion{Version: 1, Operation: "video.generate", EndpointType: "openai-video", ExecutionMode: "async", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"duration":{"type":"integer","minimum":4,"maximum":15,"default":5},"resolution":{"type":"string","enum":["480p","720p","1080p","4k"],"default":"720p"},"aspect_ratio":{"type":"string","enum":["16:9","9:16","1:1","4:3","adaptive"],"default":"16:9"},"generate_audio":{"type":"boolean","default":false},"watermark":{"type":"boolean","default":false}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","duration","resolution","aspect_ratio","generate_audio","watermark"],"widgets":{"prompt":"textarea","duration":"stepper","resolution":"segmented","aspect_ratio":"select","generate_audio":"switch","watermark":"switch"}}`, MaterialSchema: `{"image":{"max_items":9},"video":{"max_items":3},"audio":{"max_items":3}}`, ResponseContract: "openai-video-task-v1", SmokeTest: `{"prompt":"生成一个简洁的镜头运动","duration":4,"resolution":"480p","aspect_ratio":"16:9","generate_audio":false,"watermark":false}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "audio",
			Profile:   ModelOperationProfile{ProfileKey: "audio.generate.basic", DisplayName: "通用音频生成", Description: "通过 OpenAI 兼容接口调用音频模型的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 1, Operation: "audio.generate", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"input":{"type":"string","minLength":1}},"required":["input"],"additionalProperties":true}`, UISchema: `{"order":["input"],"widgets":{"input":"textarea"}}`, MaterialSchema: `{}`, ResponseContract: "openai-chat-completion-v1", SmokeTest: `{"input":"你好，这是一次音频测试。"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "embedding",
			Profile:   ModelOperationProfile{ProfileKey: "embedding.create.basic", DisplayName: "通用向量生成", Description: "OpenAI 兼容向量接口的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 1, Operation: "embedding.create", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"input":{"type":["string","array"]}},"required":["input"],"additionalProperties":true}`, UISchema: `{"order":["input"],"widgets":{"input":"textarea"}}`, MaterialSchema: `{}`, ResponseContract: "openai-chat-completion-v1", SmokeTest: `{"input":"向量测试"}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "rerank",
			Profile:   ModelOperationProfile{ProfileKey: "rerank.create.basic", DisplayName: "通用重排序", Description: "OpenAI 兼容调用下重排序模型的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 1, Operation: "rerank.create", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"query":{"type":"string","minLength":1},"documents":{"type":"array","minItems":1}},"required":["query","documents"],"additionalProperties":true}`, UISchema: `{"order":["query","documents"],"widgets":{"query":"textarea","documents":"string-list"}}`, MaterialSchema: `{}`, ResponseContract: "openai-chat-completion-v1", SmokeTest: `{"query":"测试","documents":["测试文档","无关文档"]}`, Status: ModelOperationProfileStatusPublished},
		},
	}
}

func migrateReservedModelOperationBinding(modelName, operation, legacyProfileKey string, legacyProfileVersion int, legacyOverrides, nextProfileKey string, nextProfileVersion int, nextOverrides string) error {
	legacyProfile, legacyVersion, err := GetModelOperationProfileVersion(legacyProfileKey, legacyProfileVersion, false)
	if err != nil {
		return err
	}
	legacyNormalized, _, err := normalizeModelOperationBindingOverrides(legacyOverrides, legacyProfile, legacyVersion)
	if err != nil {
		return err
	}
	nextProfile, nextVersion, err := GetModelOperationProfileVersion(nextProfileKey, nextProfileVersion, false)
	if err != nil {
		return err
	}
	nextNormalized, _, err := normalizeModelOperationBindingOverrides(nextOverrides, nextProfile, nextVersion)
	if err != nil {
		return err
	}
	return DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", modelName, operation).
		Where("profile_key = ? AND profile_version = ?", legacyProfileKey, legacyProfileVersion).
		Where("overrides = ? OR overrides = ?", legacyOverrides, legacyNormalized).
		Updates(map[string]interface{}{
			"profile_key":     nextProfileKey,
			"profile_version": nextProfileVersion,
			"overrides":       nextNormalized,
			"enabled":         true,
			"updated_time":    common.GetTimestamp(),
		}).Error
}

func ensureCoreModelEndpointTypes() error {
	required := map[string][]string{
		gptImage2AllModelName: {"image-generation"},
		omniFastModelName:     {"openai-video"},
		omniFastV2VModelName:  {"openai-video"},
	}
	modelNames := make([]string, 0, len(required))
	for modelName := range required {
		modelNames = append(modelNames, modelName)
	}
	var models []Model
	if err := DB.Select("id", "model_name", "endpoints").Where("model_name IN ?", modelNames).Find(&models).Error; err != nil {
		return err
	}
	for _, item := range models {
		endpoints := parseConfiguredEndpointTypes(item.Endpoints)
		changed := false
		for _, endpoint := range required[item.ModelName] {
			if common.StringsContains(endpoints, endpoint) {
				continue
			}
			endpoints = append(endpoints, endpoint)
			changed = true
		}
		if !changed {
			continue
		}
		sort.Strings(endpoints)
		payload, err := common.Marshal(endpoints)
		if err != nil {
			return err
		}
		if err := DB.Model(&Model{}).Where("id = ?", item.Id).Updates(map[string]interface{}{
			"endpoints":    string(payload),
			"updated_time": common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func SeedDefaultModelOperationProfiles() error {
	profiles := defaultModelOperationProfiles()
	bindingsByType := make(map[string]ModelOperationBinding, len(profiles))
	for index := range profiles {
		item := &profiles[index]
		storedProfile, storedVersion, err := GetModelOperationProfileVersion(item.Profile.ProfileKey, item.Version.Version, false)
		if err == nil {
			if storedVersion.Status != ModelOperationProfileStatusPublished || storedVersion.Operation != item.Version.Operation {
				return fmt.Errorf("reserved profile %s version %d conflicts with the default contract", item.Profile.ProfileKey, item.Version.Version)
			}
			item.Profile = *storedProfile
			item.Version = *storedVersion
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := SaveModelOperationProfileVersion(&item.Profile, &item.Version); err != nil {
				return fmt.Errorf("seed profile %s: %w", item.Profile.ProfileKey, err)
			}
		} else {
			return fmt.Errorf("load profile %s: %w", item.Profile.ProfileKey, err)
		}
		bindingsByType[item.ModelType] = ModelOperationBinding{
			Operation:      item.Version.Operation,
			ProfileKey:     item.Profile.ProfileKey,
			ProfileVersion: item.Version.Version,
			Overrides:      "{}",
			Enabled:        true,
		}
	}
	legacyProfile, legacyVersion, err := GetModelOperationProfileVersion("image.generate.gemini-openai", 1, false)
	if err != nil {
		return fmt.Errorf("load legacy Gemini image profile: %w", err)
	}
	legacyOverrides, _, err := normalizeModelOperationBindingOverrides(geminiOpenAIOverrides, legacyProfile, legacyVersion)
	if err != nil {
		return fmt.Errorf("normalize legacy Gemini image binding: %w", err)
	}
	nativeProfile, nativeVersion, err := GetModelOperationProfileVersion("image.generate.gemini-native", 1, false)
	if err != nil {
		return fmt.Errorf("load native Gemini image profile: %w", err)
	}
	nativeOverrides, _, err := normalizeModelOperationBindingOverrides(geminiNativeOverrides, nativeProfile, nativeVersion)
	if err != nil {
		return fmt.Errorf("normalize native Gemini image binding: %w", err)
	}
	now := common.GetTimestamp()
	if err := DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", gemini25FlashImageModelName, nativeVersion.Operation).
		Where("overrides = ? OR overrides = ?", geminiOpenAIOverrides, legacyOverrides).
		Updates(map[string]interface{}{
			"profile_key":     nativeProfile.ProfileKey,
			"profile_version": nativeVersion.Version,
			"overrides":       nativeOverrides,
			"enabled":         true,
			"updated_time":    now,
		}).Error; err != nil {
		return fmt.Errorf("migrate legacy Gemini image binding: %w", err)
	}
	if err := migrateReservedModelOperationBinding(
		gptImage2AllModelName,
		"image.generate",
		"image.generate.chat",
		1,
		gptImage2AllLegacyOverrides,
		"image.generate.gpt-image-2",
		1,
		gptImage2Overrides,
	); err != nil {
		return fmt.Errorf("migrate GPT Image 2 All binding: %w", err)
	}
	for _, migration := range []struct {
		modelName string
		overrides string
	}{
		{modelName: geminiProImageModelName, overrides: geminiProImageOverrides},
		{modelName: gemini31FlashImageModelName, overrides: gemini31FlashImageOverrides},
	} {
		if err := migrateReservedModelOperationBinding(
			migration.modelName,
			"image.generate",
			"image.generate.gemini-native",
			1,
			geminiNativeOverrides,
			"image.generate.gemini-native",
			1,
			migration.overrides,
		); err != nil {
			return fmt.Errorf("migrate Gemini image pricing binding %s: %w", migration.modelName, err)
		}
	}
	if err := ensureCoreModelEndpointTypes(); err != nil {
		return fmt.Errorf("ensure core model endpoint types: %w", err)
	}

	var models []Model
	if err := DB.Select("model_name", "model_type").Where("status = ?", 1).Find(&models).Error; err != nil {
		return err
	}
	bindings := make([]ModelOperationBinding, 0, len(models))
	for _, modelItem := range models {
		template, ok := bindingsByType[strings.ToLower(strings.TrimSpace(modelItem.ModelType))]
		if !ok {
			continue
		}
		template.ModelName = modelItem.ModelName
		template.CreatedTime = now
		template.UpdatedTime = now
		bindings = append(bindings, template)
	}
	for _, profile := range profiles {
		for _, modelName := range profile.ModelNames {
			bindings = append(bindings, ModelOperationBinding{
				ModelName:      modelName,
				Operation:      profile.Version.Operation,
				ProfileKey:     profile.Profile.ProfileKey,
				ProfileVersion: profile.Version.Version,
				Overrides:      profile.BindingOverrides,
				Enabled:        true,
				CreatedTime:    now,
				UpdatedTime:    now,
			})
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ModelName == bindings[j].ModelName {
			return bindings[i].Operation < bindings[j].Operation
		}
		return bindings[i].ModelName < bindings[j].ModelName
	})
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(bindings, 200).Error; err != nil {
		return err
	}
	for _, profile := range profiles {
		modelNames := make([]string, 0)
		for _, modelItem := range models {
			if strings.EqualFold(strings.TrimSpace(modelItem.ModelType), profile.ModelType) {
				modelNames = append(modelNames, modelItem.ModelName)
			}
		}
		if len(modelNames) > 0 {
			if err := DB.Model(&ModelOperationBinding{}).
				Where("model_name IN ? AND operation = ? AND profile_key = ?", modelNames, profile.Version.Operation, profile.Profile.ProfileKey).
				Updates(map[string]interface{}{"profile_version": profile.Version.Version, "updated_time": now}).Error; err != nil {
				return err
			}
		}
		if len(profile.ModelNames) > 0 {
			if err := DB.Model(&ModelOperationBinding{}).
				Where("model_name IN ? AND operation = ?", profile.ModelNames, profile.Version.Operation).
				Updates(map[string]interface{}{
					"profile_key":     profile.Profile.ProfileKey,
					"profile_version": profile.Version.Version,
					"enabled":         true,
					"updated_time":    now,
				}).Error; err != nil {
				return err
			}
			if profile.BindingOverrides != "" {
				if err := DB.Model(&ModelOperationBinding{}).
					Where("model_name IN ? AND operation = ?", profile.ModelNames, profile.Version.Operation).
					Where("overrides = ? OR overrides = ?", "", "{}").
					Updates(map[string]interface{}{
						"overrides":    profile.BindingOverrides,
						"updated_time": now,
					}).Error; err != nil {
					return err
				}
			}
		}
	}
	return RefreshModelOperationBindingContracts()
}
