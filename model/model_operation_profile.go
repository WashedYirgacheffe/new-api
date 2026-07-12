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
	Id             int    `json:"id"`
	ModelName      string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_operation,priority:1;index"`
	Operation      string `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex:uk_model_operation,priority:2;index"`
	ProfileKey     string `json:"profile_key" gorm:"type:varchar(128);not null;index"`
	ProfileVersion int    `json:"profile_version" gorm:"not null"`
	Overrides      string `json:"overrides" gorm:"type:text;not null"`
	Enabled        bool   `json:"enabled" gorm:"not null"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`
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
	overrides, err := normalizeContractJSONObject("overrides", binding.Overrides)
	if err != nil {
		return err
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

	now := common.GetTimestamp()
	binding.Operation = operation
	binding.ProfileKey = profile.ProfileKey
	binding.Overrides = overrides
	binding.UpdatedTime = now
	var stored ModelOperationBinding
	err = DB.Where("model_name = ? AND operation = ?", binding.ModelName, operation).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		binding.CreatedTime = now
		return DB.Create(binding).Error
	}
	if err != nil {
		return err
	}
	binding.Id = stored.Id
	binding.CreatedTime = stored.CreatedTime
	return DB.Model(&stored).Select("profile_key", "profile_version", "overrides", "enabled", "updated_time").Updates(binding).Error
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
	ModelType string
	Profile   ModelOperationProfile
	Version   ModelOperationProfileVersion
}

func defaultModelOperationProfiles() []defaultModelOperationProfile {
	return []defaultModelOperationProfile{
		{
			ModelType: "text",
			Profile:   ModelOperationProfile{ProfileKey: "text.chat.basic", DisplayName: "通用文本对话", Description: "OpenAI 兼容文本对话的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 3, Operation: "text.chat", EndpointType: "openai", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"temperature":{"type":"number","minimum":0,"maximum":2},"top_p":{"type":"number","minimum":0,"maximum":1},"max_tokens":{"type":"integer","minimum":1,"maximum":131072,"default":1024}},"required":["prompt"],"additionalProperties":true}`, UISchema: `{"order":["prompt","temperature","top_p","max_tokens"],"widgets":{"prompt":"textarea","temperature":"stepper","top_p":"stepper","max_tokens":"stepper"}}`, MaterialSchema: `{}`, ResponseContract: "openai-chat-completion-v1", SmokeTest: `{"prompt":"请只回复 OK","max_tokens":128}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "image",
			Profile:   ModelOperationProfile{ProfileKey: "image.generate.basic", DisplayName: "通用图片生成", Description: "通过 OpenAI 兼容接口调用图片模型的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 2, Operation: "image.generate", EndpointType: "image-generation", ExecutionMode: "sync", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"n":{"type":"integer","minimum":1,"maximum":4},"size":{"type":"string","minLength":1,"maxLength":32},"quality":{"type":"string","minLength":1,"maxLength":32},"response_format":{"type":"string","enum":["url","b64_json"]}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","size","quality","n","response_format"],"widgets":{"prompt":"textarea","size":"text","quality":"text","n":"stepper","response_format":"select"}}`, MaterialSchema: `{"image":{"max_items":4}}`, ResponseContract: "openai-image-generation-v1", SmokeTest: `{"prompt":"生成一个白色背景上的红色圆形","size":"1024x1024","n":1}`, Status: ModelOperationProfileStatusPublished},
		},
		{
			ModelType: "video",
			Profile:   ModelOperationProfile{ProfileKey: "video.generate.basic", DisplayName: "通用视频生成", Description: "通过 OpenAI 兼容接口调用视频模型的最低公共能力。"},
			Version:   ModelOperationProfileVersion{Version: 2, Operation: "video.generate", EndpointType: "openai-video", ExecutionMode: "async", InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"integer","minimum":1,"maximum":60},"size":{"type":"string","minLength":1,"maxLength":32},"image_url":{"type":"string","format":"uri","maxLength":4096}},"required":["prompt"],"additionalProperties":false}`, UISchema: `{"order":["prompt","seconds","size","image_url"],"widgets":{"prompt":"textarea","seconds":"stepper","size":"text","image_url":"text"}}`, MaterialSchema: `{"image":{"max_items":1},"video":{"max_items":1}}`, ResponseContract: "openai-video-task-v1", SmokeTest: `{"prompt":"生成一个四秒钟的简单镜头运动","seconds":4}`, Status: ModelOperationProfileStatusPublished},
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

	var models []Model
	if err := DB.Select("model_name", "model_type").Where("status = ?", 1).Find(&models).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
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
		if len(modelNames) == 0 {
			continue
		}
		if err := DB.Model(&ModelOperationBinding{}).
			Where("model_name IN ? AND operation = ? AND profile_key = ?", modelNames, profile.Version.Operation, profile.Profile.ProfileKey).
			Updates(map[string]interface{}{"profile_version": profile.Version.Version, "updated_time": now}).Error; err != nil {
			return err
		}
	}
	return nil
}
