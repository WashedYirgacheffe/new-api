package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentedSeedancePricingOverrides(t *testing.T) {
	tests := []struct {
		model    string
		expected map[string]float64
	}{
		{
			model: "doubao-seedance-2.0-face",
			expected: map[string]float64{
				"doubao-seedance-2.0-face:480p:withVideo":  0.052,
				"doubao-seedance-2.0-face:480p:noVideo":    0.086,
				"doubao-seedance-2.0-face:720p:withVideo":  0.113,
				"doubao-seedance-2.0-face:720p:noVideo":    0.185,
				"doubao-seedance-2.0-face:1080p:withVideo": 0.279,
				"doubao-seedance-2.0-face:1080p:noVideo":   0.459,
				"doubao-seedance-2.0-face:4k:withVideo":    0.576,
				"doubao-seedance-2.0-face:4k:noVideo":      0.936,
			},
		},
		{
			model: "doubao-seedance-2.0-fast-face",
			expected: map[string]float64{
				"doubao-seedance-2.0-fast-face:480p:withVideo": 0.041,
				"doubao-seedance-2.0-fast-face:480p:noVideo":   0.070,
				"doubao-seedance-2.0-fast-face:720p:withVideo": 0.090,
				"doubao-seedance-2.0-fast-face:720p:noVideo":   0.149,
			},
		},
		{
			model: "seedance-2.0-mini",
			expected: map[string]float64{
				"seedance-2.0-mini:480p:withVideo": 0.029,
				"seedance-2.0-mini:480p:noVideo":   0.046,
				"seedance-2.0-mini:720p:withVideo": 0.060,
				"seedance-2.0-mini:720p:noVideo":   0.098,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			override, ok := documentedSeedancePricingOverrides[tt.model]
			require.True(t, ok)
			actual := make(map[string]float64, len(override.SKUs))
			for _, sku := range override.SKUs {
				actual[sku.Key] = sku.PriceUSD
			}
			assert.Equal(t, tt.expected, actual)
			assert.Contains(t, override.SourceURL, "reapi.ai/zh/models/")
		})
	}
}

func TestBuildPricingCatalogAppliesDocumentedSeedanceOverrides(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	catalog, err := loadModelCatalog(filepath.Join(repositoryRoot, defaultCatalogPath))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repositoryRoot, defaultOutputPath))
	require.NoError(t, err)
	var checkedIn pricingCatalog
	require.NoError(t, common.Unmarshal(data, &checkedIn))

	checkedInByModel := make(map[string]pricingModel, len(checkedIn.Models))
	for _, item := range checkedIn.Models {
		checkedInByModel[item.ModelName] = item
	}
	snapshot := upstreamSnapshot{Version: 1, SKUPrices: map[string]float64{}}
	cards := make(map[string]modelCard, catalog.Integrity.AsyncModelCount)
	for _, item := range catalog.Models {
		if item.Protocol != "re-task" {
			continue
		}
		price, ok := checkedInByModel[item.ModelName]
		require.True(t, ok, item.ModelName)
		cards[pricingCardSlug(item)] = modelCard{
			Slug:       pricingCardSlug(item),
			MinPrice:   price.MinPriceUSD,
			MaxPrice:   price.MaxPriceUSD,
			Unit:       price.Unit,
			ComingSoon: price.ComingSoon,
		}
		for _, sku := range price.SKUs {
			snapshot.SKUPrices[sku.Key] = sku.PriceUSD
		}
	}

	generated, err := buildPricingCatalog(catalog, snapshot, []byte(`{"version":1}`), cards, "https://reapi.ai/models", "2026-08-03T00:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, 3, generated.Integrity.DocumentedOverrideCount)

	byModel := make(map[string]pricingModel, len(generated.Models))
	for _, item := range generated.Models {
		byModel[item.ModelName] = item
	}
	for _, modelName := range []string{
		"re/doubao-seedance-2.0-face",
		"re/doubao-seedance-2.0-fast-face",
		"re/seedance-2.0-mini",
	} {
		assert.Equal(t, "documented_model_page_override", byModel[modelName].PricingBasis, modelName)
	}
	assert.Equal(t, 0.185, priceForSKU(t, byModel["re/doubao-seedance-2.0-face"], "doubao-seedance-2.0-face:720p:noVideo"))
	assert.Equal(t, 0.090, priceForSKU(t, byModel["re/doubao-seedance-2.0-fast-face"], "doubao-seedance-2.0-fast-face:720p:withVideo"))
	assert.Equal(t, 0.098, priceForSKU(t, byModel["re/seedance-2.0-mini"], "seedance-2.0-mini:720p:noVideo"))
}

func priceForSKU(t *testing.T, model pricingModel, key string) float64 {
	t.Helper()
	for _, sku := range model.SKUs {
		if sku.Key == key {
			return sku.PriceUSD
		}
	}
	require.Failf(t, "missing SKU", "%s is not configured for %s", key, model.ModelName)
	return 0
}
