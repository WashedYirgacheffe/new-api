package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCatalogChannelProviderFilterDoesNotCreateRoutingAbility(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	catalogOnly := &model.Model{
		ModelName:        "fox-catalog-only-model",
		DisplayName:      "FOX Catalog Only",
		ModelType:        "video",
		Status:           0,
		SyncOfficial:     0,
		ChannelProviders: []string{"DeepWL", "deepwl"},
	}
	require.NoError(t, catalogOnly.Insert())

	runtimeModel := &model.Model{
		ModelName:        "runtime-dmx-model",
		DisplayName:      "Runtime DMX",
		ModelType:        "text",
		Status:           1,
		SyncOfficial:     0,
		ChannelProviders: []string{"dmxapi"},
	}
	require.NoError(t, runtimeModel.Insert())
	driftedModel := &model.Model{
		ModelName:    "disabled-channel-drift-model",
		DisplayName:  "Disabled Channel Drift",
		ModelType:    "text",
		Status:       0,
		SyncOfficial: 0,
	}
	require.NoError(t, driftedModel.Insert())
	require.NoError(t, db.Create(&model.Channel{
		Id:              9001,
		Type:            constant.ChannelTypeOpenAI,
		Key:             "test-key",
		Status:          common.ChannelStatusEnabled,
		Name:            "DMX Runtime",
		ChannelProvider: "dmxapi",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     runtimeModel.ModelName,
		ChannelId: 9001,
		Enabled:   true,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:              9002,
		Type:            constant.ChannelTypeOpenAI,
		Key:             "test-key",
		Status:          common.ChannelStatusManuallyDisabled,
		Name:            "Stale Disabled Runtime",
		ChannelProvider: "stale",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     driftedModel.ModelName,
		ChannelId: 9002,
		Enabled:   true,
	}).Error)

	counts, err := model.GetChannelProviderModelCounts()
	require.NoError(t, err)
	require.Equal(t, int64(1), counts["deepwl"])
	require.Equal(t, int64(1), counts["dmxapi"])
	require.NotContains(t, counts, "stale")

	models, total, err := model.SearchModels("", "", "deepwl", "", "", "", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, models, 1)
	require.Equal(t, catalogOnly.ModelName, models[0].ModelName)

	providers, err := model.GetChannelProvidersByModelsMap([]int{catalogOnly.Id, runtimeModel.Id, driftedModel.Id})
	require.NoError(t, err)
	require.Equal(t, []string{"deepwl"}, providers[catalogOnly.Id])
	require.Equal(t, []string{"dmxapi"}, providers[runtimeModel.Id])
	require.Empty(t, providers[driftedModel.Id])

	missingModel := &model.Model{
		Id:               9999,
		ModelName:        "missing-model",
		ChannelProviders: []string{"deepwl"},
	}
	require.ErrorIs(t, missingModel.Update(), gorm.ErrRecordNotFound)
	var orphanProviderCount int64
	require.NoError(t, db.Model(&model.ModelChannelProvider{}).Where("model_id = ?", missingModel.Id).Count(&orphanProviderCount).Error)
	require.Zero(t, orphanProviderCount)

	boundChannels, err := model.GetBoundChannelsByModelsMap([]string{catalogOnly.ModelName, driftedModel.ModelName})
	require.NoError(t, err)
	require.Empty(t, boundChannels[catalogOnly.ModelName])
	require.Empty(t, boundChannels[driftedModel.ModelName])

	models, total, err = model.SearchModels("", "", "stale", "", "", "", 0, 20)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, models)

	models, total, err = model.SearchModels("", "", "", "video", "", "", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, models, 1)
	require.Equal(t, catalogOnly.ModelName, models[0].ModelName)
}
