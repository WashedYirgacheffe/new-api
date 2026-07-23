package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
