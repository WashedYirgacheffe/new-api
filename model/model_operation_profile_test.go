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
