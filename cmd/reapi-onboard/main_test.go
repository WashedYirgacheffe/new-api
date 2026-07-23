package main

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCheckedInAsyncPricingCatalog(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	models, err := loadCatalog(filepath.Join(repositoryRoot, defaultCatalogPath))
	require.NoError(t, err)
	pricing, err := loadAsyncPricingCatalog(filepath.Join(repositoryRoot, defaultPricingCatalogPath), models)
	require.NoError(t, err)
	assert.Equal(t, 96, pricing.Integrity.AsyncModelCount)
	assert.Equal(t, 95, pricing.Integrity.PublishableModelCount)
	assert.Equal(t, 1, pricing.Integrity.ComingSoonModelCount)
	assert.Equal(t, 8, pricing.Integrity.ExcludedChatCount)
	publishReadyCount := 0
	for _, item := range pricing.Models {
		assert.NotEqual(t, "chat-completions", item.ModelType)
		assert.Positive(t, item.BasePriceUSD, item.ModelName)
		if asyncModelPublishReady(item) {
			publishReadyCount++
		}
	}
	assert.Equal(t, 88, publishReadyCount)
	assert.Len(t, deferredBillingModels, 7)
}

func TestCheckedInAsyncContractCatalog(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	models, err := loadCatalog(filepath.Join(repositoryRoot, defaultCatalogPath))
	require.NoError(t, err)
	pricing, err := loadAsyncPricingCatalog(filepath.Join(repositoryRoot, defaultPricingCatalogPath), models)
	require.NoError(t, err)
	contracts, err := loadAsyncContractCatalog(filepath.Join(repositoryRoot, defaultContractCatalogPath), pricing)
	require.NoError(t, err)

	assert.Len(t, contracts.Models, asyncContractModelCount)
	assert.Len(t, contracts.Exclusions, asyncContractExclusionCount)
	typeCounts := map[string]int{}
	for _, item := range contracts.Models {
		typeCounts[item.ModelType]++
		assert.Equal(t, model.ModelOperationSchemaModeReplace, item.SchemaMode, item.ModelName)
		assert.NotEmpty(t, item.normalizedOverrides, item.ModelName)
		assert.Equal(t, false, item.InputSchema["additionalProperties"], item.ModelName)
		assert.Empty(t, findAsyncContractMaterialURLField(item.InputSchema, ""), item.ModelName)
		properties, ok := item.InputSchema["properties"].(map[string]interface{})
		require.True(t, ok, item.ModelName)
		widgets, _ := item.UISchema["widgets"].(map[string]interface{})
		for field, rawSchema := range properties {
			fieldSchema, ok := rawSchema.(map[string]interface{})
			require.True(t, ok, "%s/%s", item.ModelName, field)
			assert.False(t, asyncContractMaterialURLField(field, fieldSchema), "%s/%s", item.ModelName, field)
			if !asyncContractSelectorField(field) {
				continue
			}
			enumValues, ok := fieldSchema["enum"].([]interface{})
			require.True(t, ok, "%s/%s", item.ModelName, field)
			assert.NotEmpty(t, enumValues, "%s/%s", item.ModelName, field)
			assert.Contains(t, []string{"select", "segmented", "menu", "hidden"}, asyncContractWidgetType(widgets[field]), "%s/%s", item.ModelName, field)
		}
		for materialType, rawRule := range item.MaterialSchema {
			rule, ok := rawRule.(map[string]interface{})
			require.True(t, ok, "%s/%s", item.ModelName, materialType)
			requestField, _ := rule["request_field"].(string)
			assert.NotContains(t, properties, requestField, "%s/%s", item.ModelName, requestField)
			requestFields, _ := rule["request_fields"].([]interface{})
			for _, rawRequestField := range requestFields {
				requestFieldRule, ok := rawRequestField.(map[string]interface{})
				require.True(t, ok, "%s/%s", item.ModelName, materialType)
				nestedRequestField, _ := requestFieldRule["request_field"].(string)
				assert.NotContains(t, properties, nestedRequestField, "%s/%s", item.ModelName, nestedRequestField)
			}
		}
	}
	assert.Equal(t, map[string]int{"image": 32, "video": 34, "audio": 19, "text": 3}, typeCounts)
}

func TestUpsertREAsyncContractsIsIdempotent(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	models, err := loadCatalog(filepath.Join(repositoryRoot, defaultCatalogPath))
	require.NoError(t, err)
	pricing, err := loadAsyncPricingCatalog(filepath.Join(repositoryRoot, defaultPricingCatalogPath), models)
	require.NoError(t, err)
	contracts, err := loadAsyncContractCatalog(filepath.Join(repositoryRoot, defaultContractCatalogPath), pricing)
	require.NoError(t, err)

	originalDB := model.DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open("file:reapi-onboard-idempotence?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
	})
	require.NoError(t, db.AutoMigrate(
		&model.Model{},
		&model.ModelOperationProfile{},
		&model.ModelOperationProfileVersion{},
		&model.ModelOperationBinding{},
		&model.ModelOperationBindingRevision{},
	))

	taskModels := make([]string, 0, len(contracts.Models))
	for _, item := range contracts.Models {
		taskModels = append(taskModels, item.ModelName)
		require.NoError(t, db.Create(&model.Model{
			ModelName:   item.ModelName,
			DisplayName: item.ModelName,
			ModelType:   item.ModelType,
			Status:      1,
		}).Error)
		if item.Operation == "text.generate" {
			require.NoError(t, db.Create(&model.ModelOperationBinding{
				ModelName:       item.ModelName,
				Operation:       "text.chat",
				ProfileKey:      "text.chat.basic",
				ProfileVersion:  1,
				ContractVersion: 1,
				ContractHash:    "legacy-" + item.UpstreamModel,
				Overrides:       `{}`,
				Enabled:         true,
			}).Error)
		}
	}

	require.NoError(t, upsertREAsyncContracts(contracts, taskModels))
	var firstBindings []model.ModelOperationBinding
	require.NoError(t, db.Order("model_name ASC").Find(&firstBindings).Error)
	require.Len(t, firstBindings, asyncContractModelCount)
	firstVersions := make(map[string]int, len(firstBindings))
	firstHashes := make(map[string]string, len(firstBindings))
	for _, binding := range firstBindings {
		firstVersions[binding.ModelName] = binding.ContractVersion
		firstHashes[binding.ModelName] = binding.ContractHash
		assert.Equal(t, 1, binding.ContractVersion, binding.ModelName)
	}
	var profiles []model.ModelOperationProfileVersion
	require.NoError(t, db.Order("operation ASC").Find(&profiles).Error)
	require.Len(t, profiles, len(reAsyncProfileDefinitions))
	for _, definition := range reAsyncProfileDefinitions {
		profile, version, err := model.GetModelOperationProfileVersion(definition.ProfileKey, 1, true)
		require.NoError(t, err)
		assert.Equal(t, definition.ProfileKey, profile.ProfileKey)
		assert.Equal(t, definition.Operation, version.Operation)
		assert.Equal(t, string(constant.EndpointTypeReTask), version.EndpointType)
		assert.Equal(t, "async", version.ExecutionMode)
		assert.Equal(t, definition.ResponseContract, version.ResponseContract)
		assert.Equal(t, model.ModelOperationProfileStatusPublished, version.Status)
	}
	var legacyTextBindingCount int64
	require.NoError(t, db.Model(&model.ModelOperationBinding{}).Where("operation = ?", "text.chat").Count(&legacyTextBindingCount).Error)
	assert.Zero(t, legacyTextBindingCount)

	require.NoError(t, upsertREAsyncContracts(contracts, taskModels))
	var secondBindings []model.ModelOperationBinding
	require.NoError(t, db.Order("model_name ASC").Find(&secondBindings).Error)
	require.Len(t, secondBindings, asyncContractModelCount)
	for _, binding := range secondBindings {
		assert.Equal(t, firstVersions[binding.ModelName], binding.ContractVersion, binding.ModelName)
		assert.Equal(t, firstHashes[binding.ModelName], binding.ContractHash, binding.ModelName)
	}
	var revisionCount int64
	require.NoError(t, db.Model(&model.ModelOperationBindingRevision{}).Count(&revisionCount).Error)
	assert.EqualValues(t, asyncContractModelCount, revisionCount)
}

func TestConfiguredKeysAllowsIndependentChannelOnboarding(t *testing.T) {
	t.Setenv("REAPI_CHAT_API_KEY", "")
	t.Setenv("REAPI_TASK_API_KEY", "")
	_, _, err := configuredKeys()
	require.Error(t, err)

	t.Setenv("REAPI_TASK_API_KEY", "task-key")
	chatKey, taskKey, err := configuredKeys()
	require.NoError(t, err)
	assert.Empty(t, chatKey)
	assert.Equal(t, "task-key", taskKey)

	t.Setenv("REAPI_TASK_API_KEY", "")
	t.Setenv("REAPI_CHAT_API_KEY", "chat-key")
	chatKey, taskKey, err = configuredKeys()
	require.NoError(t, err)
	assert.Equal(t, "chat-key", chatKey)
	assert.Empty(t, taskKey)
}

func TestREAsyncChannelSupportsSuperseedRouteGroup(t *testing.T) {
	assert.Equal(t, "default,gold", reChannelGroups(constant.ChannelTypeReAPI))
	assert.Equal(t, "default", reChannelGroups(1))
}
