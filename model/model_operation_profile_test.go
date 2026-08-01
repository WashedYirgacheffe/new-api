package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func defaultModelOperationContractForModel(t *testing.T, modelName string) *ModelOperationEffectiveContract {
	t.Helper()
	for _, item := range defaultModelOperationProfiles() {
		for _, candidate := range item.ModelNames {
			if candidate != modelName {
				continue
			}
			normalizedOverrides, _, err := normalizeModelOperationBindingOverrides(
				item.BindingOverrides,
				&item.Profile,
				&item.Version,
			)
			require.NoError(t, err)
			contract, err := BuildModelOperationEffectiveContract(ModelOperationBinding{
				ModelName:      modelName,
				Operation:      item.Version.Operation,
				ProfileKey:     item.Profile.ProfileKey,
				ProfileVersion: item.Version.Version,
				Overrides:      normalizedOverrides,
				Enabled:        true,
			}, &item.Profile, &item.Version)
			require.NoError(t, err)
			return contract
		}
	}
	t.Fatalf("default model operation contract for %s was not found", modelName)
	return nil
}

func TestCoreDefaultModelOperationContractMatrix(t *testing.T) {
	gptImage2Sizes := []interface{}{
		"1024x1024", "1536x1024", "1024x1536", "1920x1920", "2560x1440", "1440x2560",
		"2560x1920", "1920x2560", "2880x2880", "3840x2160", "2160x3840", "2880x2160", "2160x2880",
	}
	gptImage2AllSizes := []interface{}{"1024x1024", "1536x1024", "1024x1536"}
	for _, test := range []struct {
		modelName string
		sizes     []interface{}
	}{
		{modelName: "deepwl/gpt-image-2", sizes: gptImage2Sizes},
		{modelName: gptImage2CModelName, sizes: gptImage2Sizes},
		{modelName: gptImage2AllModelName, sizes: gptImage2AllSizes},
	} {
		t.Run(test.modelName, func(t *testing.T) {
			contract := defaultModelOperationContractForModel(t, test.modelName)
			assert.Equal(t, "image.generate.gpt-image-2", contract.ProfileKey)
			assert.Equal(t, "image-generation", contract.EndpointType)
			assert.Equal(t, "openai-image", contract.RequestContract.Adapter)

			properties, ok := contractObject(contract.InputSchema["properties"])
			require.True(t, ok)
			size, ok := contractObject(properties["size"])
			require.True(t, ok)
			assert.Equal(t, test.sizes, size["enum"])
			assert.Equal(t, "1024x1024", size["default"])
			quality, ok := contractObject(properties["quality"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{"low", "medium", "high"}, quality["enum"])
			assert.Equal(t, "high", quality["default"])
			n, ok := contractObject(properties["n"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{float64(1)}, n["enum"])
			assert.Equal(t, float64(1), n["default"])
			assert.Equal(t, float64(1), n["maximum"])
			responseFormat, ok := contractObject(properties["response_format"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{"url", "b64_json"}, responseFormat["enum"])
			assert.Equal(t, "url", responseFormat["default"])

			image, ok := contractObject(contract.MaterialSchema["image"])
			require.True(t, ok)
			assert.Equal(t, float64(0), image["max_items"])
		})
	}

	for _, test := range []struct {
		modelName     string
		profileKey    string
		seconds       []interface{}
		defaultSecond float64
		videoEnabled  bool
	}{
		{modelName: omniFastModelName, profileKey: "video.generate.omni", seconds: []interface{}{float64(10)}, defaultSecond: 10},
		{modelName: omniFastV2VModelName, profileKey: "video.generate.omni-v2v", seconds: []interface{}{float64(4), float64(6), float64(8), float64(10)}, defaultSecond: 4, videoEnabled: true},
	} {
		t.Run(test.modelName, func(t *testing.T) {
			contract := defaultModelOperationContractForModel(t, test.modelName)
			assert.Equal(t, test.profileKey, contract.ProfileKey)
			assert.Equal(t, "openai-video", contract.EndpointType)
			assert.Equal(t, "async", contract.ExecutionMode)
			assert.Equal(t, "openai-video", contract.RequestContract.Adapter)
			assert.Equal(t, "/v1/videos", contract.DispatchPath)
			assert.Equal(t, "/v1/videos/{task_id}", contract.PollPath)

			properties, ok := contractObject(contract.InputSchema["properties"])
			require.True(t, ok)
			seconds, ok := contractObject(properties["seconds"])
			require.True(t, ok)
			assert.Equal(t, test.seconds, seconds["enum"])
			assert.Equal(t, test.defaultSecond, seconds["default"])
			if test.modelName == omniFastModelName {
				assert.Equal(t, float64(10), contract.ParameterOverrides["seconds"])
			}
			resolution, ok := contractObject(properties["resolution"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{"720p"}, resolution["enum"])
			assert.Equal(t, "720p", resolution["default"])
			aspectRatio, ok := contractObject(properties["aspect_ratio"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{"16:9", "9:16", "1:1", "4:3", "3:4"}, aspectRatio["enum"])
			assert.Equal(t, "16:9", aspectRatio["default"])

			image, ok := contractObject(contract.MaterialSchema["image"])
			require.True(t, ok)
			assert.Equal(t, float64(5), image["max_items"])
			assert.Equal(t, "images", image["request_field"])
			assert.Equal(t, "url", image["transport"])

			video, ok := contractObject(contract.MaterialSchema["video"])
			require.True(t, ok)
			if test.videoEnabled {
				assert.Equal(t, float64(1), video["min_items"])
				assert.Equal(t, float64(1), video["max_items"])
				assert.Equal(t, float64(15), video["max_size_mb"])
				assert.Equal(t, []interface{}{"video/mp4"}, video["mime_types"])
				assert.Equal(t, "video", video["request_field"])
				assert.Equal(t, "url", video["transport"])
			} else {
				assert.Equal(t, float64(0), video["max_items"])
				assert.NotContains(t, video, "max_size_mb")
			}
		})
	}
}

func TestSeedDefaultModelOperationProfilesRejectsConflictingOmniVersion3(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelOperationBindingRevision{},
	))
	cleanup := func() {
		for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
			DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	var conflicting defaultModelOperationProfile
	for _, item := range defaultModelOperationProfiles() {
		if item.Profile.ProfileKey == "video.generate.omni" {
			conflicting = item
			break
		}
	}
	require.Equal(t, "video.generate.omni", conflicting.Profile.ProfileKey)
	conflicting.Version.SmokeTest = `{"prompt":"administrator-owned v3","seconds":10,"aspect_ratio":"16:9","resolution":"720p"}`
	require.NoError(t, SaveModelOperationProfileVersion(&conflicting.Profile, &conflicting.Version))

	err := SeedDefaultModelOperationProfiles()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts with the fixed-duration contract")
}

func TestSeedDefaultModelOperationProfilesMigratesLegacyGeminiImageBinding(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelOperationBindingRevision{},
	))
	for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
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
		&ModelOperationBindingRevision{},
	))
	for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
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
	_, currentV2VVersion, err := GetModelOperationProfileVersion("video.generate.omni-v2v", 3, false)
	require.NoError(t, err)
	_, currentOmniVersion, err := GetModelOperationProfileVersion("video.generate.omni", 3, false)
	require.NoError(t, err)
	version2Omni := *currentOmniVersion
	version2Omni.Id = 0
	version2Omni.Version = 2
	version2Omni.InputSchema = `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"integer","enum":[4,6,8,10],"default":4},"aspect_ratio":{"type":"string","enum":["16:9","9:16","1:1","4:3","3:4"],"default":"16:9"},"resolution":{"type":"string","enum":["720p"],"default":"720p"}},"required":["prompt"],"additionalProperties":false}`
	version2Omni.SmokeTest = `{"prompt":"生成一个简洁的海浪镜头","seconds":4,"aspect_ratio":"16:9","resolution":"720p"}`
	require.NoError(t, DB.Create(&version2Omni).Error)
	version2V2V := *currentV2VVersion
	version2V2V.Id = 0
	version2V2V.Version = 2
	version2V2V.MaterialSchema = `{"image":{"max_items":5,"request_field":"images","transport":"url"},"video":{"min_items":1,"max_items":1,"max_size_mb":15,"request_field":"video","transport":"url"},"audio":{"max_items":0}}`
	require.NoError(t, DB.Create(&version2V2V).Error)
	legacyV2VVersion := version2V2V
	legacyV2VVersion.Id = 0
	legacyV2VVersion.Version = 1
	require.NoError(t, DB.Create(&legacyV2VVersion).Error)
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
		Where("model_name = ? AND operation = ?", omniFastModelName, "video.generate").
		Updates(map[string]interface{}{
			"profile_key":     "video.generate.omni",
			"profile_version": 2,
			"overrides":       omniFastVersion2Overrides,
		}).Error)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").
		Updates(map[string]interface{}{
			"profile_key":     "video.generate.omni-v2v",
			"profile_version": 1,
			"overrides":       omniFastV2VLegacyOverrides,
		}).Error)

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
		assert.Equal(t, 3, binding.ProfileVersion)
		assert.Equal(t, "/v1/videos", contract.DispatchPath)
		assert.Equal(t, "/v1/videos/{task_id}", contract.PollPath)
		if modelName == omniFastModelName {
			properties, ok := contractObject(contract.InputSchema["properties"])
			require.True(t, ok)
			seconds, ok := contractObject(properties["seconds"])
			require.True(t, ok)
			assert.Equal(t, []interface{}{float64(10)}, seconds["enum"])
			assert.Equal(t, float64(10), contract.ParameterOverrides["seconds"])
		}
		var item Model
		require.NoError(t, DB.Where("model_name = ?", modelName).First(&item).Error)
		assert.Contains(t, parseConfiguredEndpointTypes(item.Endpoints), "openai-video")
	}
}

func TestSeedDefaultModelOperationProfilesRecoversMissingReservedProfileVersion(t *testing.T) {
	for _, test := range []struct {
		name            string
		customize       bool
		expectedEnabled bool
		expectedSeconds float64
	}{
		{name: "built-in binding", expectedEnabled: true, expectedSeconds: 4},
		{name: "administrator overrides", customize: true, expectedEnabled: false, expectedSeconds: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, DB.AutoMigrate(
				&Model{},
				&ModelOperationProfile{},
				&ModelOperationProfileVersion{},
				&ModelOperationBinding{},
				&ModelOperationBindingRevision{},
			))
			for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
				require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
			}
			t.Cleanup(func() {
				for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
					DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
				}
			})

			require.NoError(t, DB.Create(&Model{
				ModelName: omniFastV2VModelName, DisplayName: "Omni Fast V2V", ModelType: "video", Status: 1,
			}).Error)
			require.NoError(t, SeedDefaultModelOperationProfiles())

			profile, currentVersion, err := GetModelOperationProfileVersion("video.generate.omni-v2v", 3, false)
			require.NoError(t, err)
			version2 := *currentVersion
			version2.Id = 0
			version2.Version = 2
			version2.MaterialSchema = `{"image":{"max_items":5,"request_field":"images","transport":"url"},"video":{"min_items":1,"max_items":1,"max_size_mb":15,"request_field":"video","transport":"url"},"audio":{"max_items":0}}`
			require.NoError(t, DB.Create(&version2).Error)
			version1 := version2
			version1.Id = 0
			version1.Version = 1
			require.NoError(t, DB.Create(&version1).Error)

			legacyOverrides := omniFastV2VVersion2Overrides
			if test.customize {
				legacyOverrides = strings.Replace(omniFastV2VOverrides, `"parameter_defaults":{"seconds":4`, `"parameter_defaults":{"seconds":6`, 1)
			}
			normalizedLegacy, _, err := normalizeModelOperationBindingOverrides(legacyOverrides, profile, &version2)
			require.NoError(t, err)
			var legacyBinding ModelOperationBinding
			require.NoError(t, DB.Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").First(&legacyBinding).Error)
			legacyBinding.ProfileKey = profile.ProfileKey
			legacyBinding.ProfileVersion = version2.Version
			legacyBinding.Overrides = normalizedLegacy
			legacyBinding.Enabled = test.expectedEnabled
			require.NoError(t, SaveModelOperationBindingWithExpectedHash(&legacyBinding, legacyBinding.ContractHash))
			require.NoError(t, DB.Delete(&version2).Error)

			require.NoError(t, SeedDefaultModelOperationProfiles())

			var binding ModelOperationBinding
			require.NoError(t, DB.Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").First(&binding).Error)
			assert.Equal(t, profile.ProfileKey, binding.ProfileKey)
			assert.Equal(t, currentVersion.Version, binding.ProfileVersion)
			assert.Equal(t, test.expectedEnabled, binding.Enabled)
			_, overrides, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, currentVersion)
			require.NoError(t, err)
			assert.Equal(t, test.expectedSeconds, overrides.ParameterDefaults["seconds"])
			expectedHash, err := computeModelOperationContractHash(binding, profile, currentVersion)
			require.NoError(t, err)
			assert.Equal(t, expectedHash, binding.ContractHash)
			var latestRevision ModelOperationBindingRevision
			require.NoError(t, DB.Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").
				Order("revision DESC").First(&latestRevision).Error)
			assert.Equal(t, binding.ProfileVersion, latestRevision.ProfileVersion)
			assert.Equal(t, binding.ContractVersion, latestRevision.ContractVersion)
			assert.Equal(t, binding.ContractHash, latestRevision.ContractHash)
			assert.Equal(t, binding.Enabled, latestRevision.Enabled)
		})
	}
}

func TestSeedDefaultModelOperationProfilesPreservesAdministratorBinding(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelOperationBindingRevision{},
	))
	for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []interface{}{&ModelOperationBindingRevision{}, &ModelOperationBinding{}, &ModelOperationProfileVersion{}, &ModelOperationProfile{}, &Model{}} {
			DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
		}
	})

	require.NoError(t, DB.Create(&Model{
		ModelName: omniFastV2VModelName, DisplayName: "Omni Fast V2V", ModelType: "video", Status: 1,
	}).Error)
	require.NoError(t, SeedDefaultModelOperationProfiles())

	adminProfile, adminVersion, err := GetModelOperationProfileVersion("video.generate.basic", 3, false)
	require.NoError(t, err)
	adminOverrides, _, err := normalizeModelOperationBindingOverrides(
		`{"parameter_defaults":{"seconds":"6"}}`,
		adminProfile,
		adminVersion,
	)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ModelOperationBinding{}).
		Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").
		Updates(map[string]interface{}{
			"profile_key":     adminProfile.ProfileKey,
			"profile_version": adminVersion.Version,
			"overrides":       adminOverrides,
			"enabled":         false,
		}).Error)

	require.NoError(t, SeedDefaultModelOperationProfiles())

	var binding ModelOperationBinding
	require.NoError(t, DB.Where("model_name = ? AND operation = ?", omniFastV2VModelName, "video.generate").First(&binding).Error)
	assert.Equal(t, adminProfile.ProfileKey, binding.ProfileKey)
	assert.Equal(t, adminVersion.Version, binding.ProfileVersion)
	assert.Equal(t, adminOverrides, binding.Overrides)
	assert.False(t, binding.Enabled)
}

func TestSeedDefaultModelOperationProfilesCreatesDisabledGPTRouteCandidate(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelOperationBindingRevision{},
		&ModelOperationParameterEvidence{},
		&ModelRouteGroup{},
		&ModelRouteTarget{},
		&ModelRouteOperationLock{},
	))
	cleanup := func() {
		var routeGroups []ModelRouteGroup
		DB.Where("canonical_model = ? AND operation = ?", gptImage2AllModelName, "image.generate").Find(&routeGroups)
		for _, routeGroup := range routeGroups {
			DB.Where("group_id = ?", routeGroup.Id).Delete(&ModelRouteTarget{})
		}
		DB.Where("canonical_model = ? AND operation = ?", gptImage2AllModelName, "image.generate").Delete(&ModelRouteGroup{})
		DB.Where("operation = ?", "image.generate").Delete(&ModelRouteOperationLock{})
		DB.Where("model_name IN ?", []string{gptImage2AllModelName, gptImage2CModelName, "deepwl/gpt-image-2", omniFastModelName, omniFastV2VModelName}).Delete(&ModelOperationParameterEvidence{})
		DB.Where("model_name IN ?", []string{gptImage2AllModelName, gptImage2CModelName, "deepwl/gpt-image-2"}).Delete(&ModelOperationBindingRevision{})
		DB.Where("model_name IN ?", []string{gptImage2AllModelName, gptImage2CModelName, "deepwl/gpt-image-2"}).Delete(&ModelOperationBinding{})
		DB.Where("model_name IN ?", []string{gptImage2AllModelName, gptImage2CModelName, "deepwl/gpt-image-2"}).Delete(&Model{})
		for _, profileKey := range []string{"image.generate.gpt-image-2", "image.generate.chat", "image.generate.basic"} {
			var profile ModelOperationProfile
			if DB.Where("profile_key = ?", profileKey).First(&profile).Error == nil {
				DB.Where("profile_id = ?", profile.Id).Delete(&ModelOperationProfileVersion{})
				DB.Delete(&profile)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	require.NoError(t, DB.Create(&[]Model{
		{ModelName: "deepwl/gpt-image-2", DisplayName: "GPT Image 2", ModelType: "image", Status: 1},
		{ModelName: gptImage2CModelName, DisplayName: "GPT Image 2 C", ModelType: "image", Status: 1},
		{ModelName: gptImage2AllModelName, DisplayName: "GPT Image 2 All", ModelType: "image", Status: 1},
	}).Error)
	require.NoError(t, SeedDefaultModelOperationProfiles())
	require.NoError(t, SeedDefaultModelOperationProfiles())
	var group ModelRouteGroup
	require.NoError(t, DB.Where("canonical_model = ? AND operation = ?", gptImage2AllModelName, "image.generate").First(&group).Error)
	assert.False(t, group.Enabled)
	var targets []ModelRouteTarget
	require.NoError(t, DB.Where("group_id = ?", group.Id).Order("priority ASC").Find(&targets).Error)
	require.Len(t, targets, 2)
	assert.Equal(t, ModelRouteCompatibilityCompatible, targets[0].CompatibilityStatus)
	assert.Equal(t, ModelRouteCompatibilityCompatible, targets[1].CompatibilityStatus)
}
