package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedDefaultModelOperationProfilesMigratesLegacyGeminiImageBinding(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
	))
	for _, table := range []interface{}{&ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
			DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
		}
	})

	require.NoError(t, DB.Create(&Model{
		ModelName:   gemini25FlashImageModelName,
		DisplayName: "nanoBanana",
		ModelType:   "image",
		Status:      1,
	}).Error)
	require.NoError(t, SeedDefaultModelOperationProfiles())

	legacyProfile, legacyVersion, err := GetModelOperationProfileVersion("image.generate.gemini-openai", 1, false)
	require.NoError(t, err)
	legacyOverrides, _, err := normalizeModelOperationBindingOverrides(geminiOpenAIOverrides, legacyProfile, legacyVersion)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", gemini25FlashImageModelName, "image.generate").
		Updates(map[string]interface{}{
			"profile_key":     legacyProfile.ProfileKey,
			"profile_version": legacyVersion.Version,
			"overrides":       legacyOverrides,
		}).Error)

	require.NoError(t, SeedDefaultModelOperationProfiles())

	var binding ModelOperationBinding
	require.NoError(t, DB.Where("model_name = ? AND operation = ?", gemini25FlashImageModelName, "image.generate").First(&binding).Error)
	assert.Equal(t, "image.generate.gemini-native", binding.ProfileKey)
	assert.Equal(t, 1, binding.ProfileVersion)
	nativeProfile, nativeVersion, err := GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, false)
	require.NoError(t, err)
	_, overrides, err := normalizeModelOperationBindingOverrides(binding.Overrides, nativeProfile, nativeVersion)
	require.NoError(t, err)
	require.NotNil(t, overrides.RequestContract)
	assert.Equal(t, "gemini-image", overrides.RequestContract.Adapter)
	assert.Equal(t, "/v1beta/models/{model}:generateContent", overrides.DispatchPath)
}

func TestSeedDefaultModelOperationProfilesMigratesCoreProductionBindings(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
	))
	for _, table := range []interface{}{&ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
			DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
		}
	})

	require.NoError(t, DB.Create(&[]Model{
		{ModelName: gptImage2AllModelName, DisplayName: "GPT Image 2 All", ModelType: "image", Endpoints: `["image-generation"]`, Status: 1},
		{ModelName: geminiProImageModelName, DisplayName: "nanoBananaPRO", ModelType: "image", Endpoints: `["gemini"]`, Status: 1},
		{ModelName: gemini31FlashImageModelName, DisplayName: "nanoBanana2", ModelType: "image", Endpoints: `["gemini"]`, Status: 1},
		{ModelName: omniFastModelName, DisplayName: "Omni Fast", ModelType: "video", Endpoints: `["openai"]`, Status: 1},
		{ModelName: omniFastV2VModelName, DisplayName: "Omni Fast V2V", ModelType: "video", Endpoints: `["openai"]`, Status: 1},
	}).Error)
	require.NoError(t, SeedDefaultModelOperationProfiles())

	chatProfile, chatVersion, err := GetModelOperationProfileVersion("image.generate.chat", 1, false)
	require.NoError(t, err)
	legacyGPTOverrides, _, err := normalizeModelOperationBindingOverrides(gptImage2AllLegacyOverrides, chatProfile, chatVersion)
	require.NoError(t, err)
	nativeProfile, nativeVersion, err := GetModelOperationProfileVersion("image.generate.gemini-native", 1, false)
	require.NoError(t, err)
	legacyGeminiOverrides, _, err := normalizeModelOperationBindingOverrides(geminiNativeOverrides, nativeProfile, nativeVersion)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", gptImage2AllModelName, "image.generate").
		Updates(map[string]interface{}{"profile_key": "image.generate.chat", "profile_version": 1, "overrides": legacyGPTOverrides}).Error)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name IN ? AND operation = ?", []string{geminiProImageModelName, gemini31FlashImageModelName}, "image.generate").
		Update("overrides", legacyGeminiOverrides).Error)
	require.NoError(t, DB.Model(&Model{}).
		Where("model_name IN ?", []string{omniFastModelName, omniFastV2VModelName}).
		Update("endpoints", `["openai"]`).Error)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").
		Updates(map[string]interface{}{"profile_key": "video.generate.basic", "profile_version": 3, "overrides": "{}"}).Error)

	require.NoError(t, SeedDefaultModelOperationProfiles())

	gptBinding, _, _, gptContract, err := GetEnabledModelOperationContract(gptImage2AllModelName, "image.generate")
	require.NoError(t, err)
	assert.Equal(t, "image.generate.gpt-image-2", gptBinding.ProfileKey)
	assert.Equal(t, "openai-image", gptContract.RequestContract.Adapter)

	for modelName, expected2K := range map[string]float64{
		geminiProImageModelName:     1.25,
		gemini31FlashImageModelName: 1.2,
	} {
		_, _, _, contract, err := GetEnabledModelOperationContract(modelName, "image.generate")
		require.NoError(t, err)
		ratios, err := CalculateModelOperationParameterRatios(contract, map[string]interface{}{"resolution": "2K"})
		require.NoError(t, err)
		assert.Equal(t, expected2K, ratios["resolution"])
	}

	for modelName, expectedProfile := range map[string]string{
		omniFastModelName:    "video.generate.omni",
		omniFastV2VModelName: "video.generate.omni-v2v",
	} {
		binding, _, _, contract, err := GetEnabledModelOperationContract(modelName, "video.generate")
		require.NoError(t, err)
		assert.Equal(t, expectedProfile, binding.ProfileKey)
		assert.Equal(t, "/v1/videos", contract.DispatchPath)
		assert.Equal(t, "/v1/videos/{task_id}", contract.PollPath)
		var item Model
		require.NoError(t, DB.Where("model_name = ?", modelName).First(&item).Error)
		assert.Contains(t, parseConfiguredEndpointTypes(item.Endpoints), "openai-video")
	}
}
