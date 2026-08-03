package reapi

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skuPriceData() types.PriceData {
	return types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		Quota:          common.QuotaFromFloat(1.0 * common.QuotaPerUnit),
		ModelPrice:     1.0,
		UsePrice:       true,
	}
}

func finalAmount(priceData types.PriceData) float64 {
	quota, _ := common.QuotaFromFloatChecked(priceData.ApplyOtherRatiosToFloat(float64(priceData.Quota)))
	return float64(quota) / common.QuotaPerUnit
}

func TestApplyPricingSKUToPriceDataMatchesGeminiResolutionAndImageCount(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/gemini-3.1-flash-image-preview", map[string]interface{}{
		"resolution": "2k",
		"n":          float64(3),
	})

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "gemini-3.1-flash-image-preview:2k", quote.MatchedSKU)
	assert.Equal(t, 0.0477, quote.UnitPriceUSD)
	assert.Equal(t, 3.0, quote.Quantity)
	assert.Equal(t, "images", quote.QuantityKey)
	assert.InDelta(t, 0.0477*3, finalAmount(priceData), 0.00001)
}

func TestApplyPricingSKUToPriceDataMatchesGPTImageRatioResolutionQuality(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/gpt-image-2-official", map[string]interface{}{
		"size":       "1:1",
		"resolution": "1k",
		"quality":    "low",
		"n":          float64(2),
	})

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "gpt-image-2-official:1:1@1k_low", quote.MatchedSKU)
	assert.Equal(t, 0.0048, quote.UnitPriceUSD)
	assert.Equal(t, 2.0, quote.Quantity)
	assert.InDelta(t, 0.0048*2, finalAmount(priceData), 0.00001)
}

func TestApplyPricingSKUToPriceDataMatchesSeedanceTextAndReferenceModes(t *testing.T) {
	textPrice := skuPriceData()
	textQuote, _, err := ApplyPricingSKUToPriceData(&textPrice, "re/doubao-seedance-2.0", map[string]interface{}{
		"resolution": "720p",
		"duration":   float64(5),
	})
	require.NoError(t, err)
	require.NotNil(t, textQuote)
	assert.Equal(t, "doubao-seedance-2.0:720p:text", textQuote.MatchedSKU)
	assert.Equal(t, 5.0, textQuote.Quantity)
	assert.InDelta(t, textQuote.UnitPriceUSD*5, finalAmount(textPrice), 0.00001)

	refPrice := skuPriceData()
	refQuote, _, err := ApplyPricingSKUToPriceData(&refPrice, "re/doubao-seedance-2.0", map[string]interface{}{
		"resolution": "720p",
		"duration":   float64(5),
		"image_urls": []interface{}{"https://example.com/frame.png"},
	})
	require.NoError(t, err)
	require.NotNil(t, refQuote)
	assert.Equal(t, "doubao-seedance-2.0:720p:ref", refQuote.MatchedSKU)
	assert.Less(t, refQuote.UnitPriceUSD, textQuote.UnitPriceUSD)
	assert.InDelta(t, refQuote.UnitPriceUSD*5, finalAmount(refPrice), 0.00001)
}

func TestApplyPricingSKUToPriceDataMatchesDocumentedSeedanceFaceAndMiniPrices(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		parameters map[string]interface{}
		expected   string
		unitPrice  float64
	}{
		{
			name:  "Face image input keeps no-video price",
			model: "re/doubao-seedance-2.0-face",
			parameters: map[string]interface{}{
				"resolution": "720p",
				"duration":   float64(5),
				"image_urls": []interface{}{"https://example.com/reference.png"},
			},
			expected:  "doubao-seedance-2.0-face:720p:noVideo",
			unitPrice: 0.185,
		},
		{
			name:  "Face video input gets video price",
			model: "re/doubao-seedance-2.0-face",
			parameters: map[string]interface{}{
				"resolution": "1080p",
				"duration":   float64(5),
				"video_urls": []interface{}{"https://example.com/reference.mp4"},
			},
			expected:  "doubao-seedance-2.0-face:1080p:withVideo",
			unitPrice: 0.279,
		},
		{
			name:  "Fast Face video price",
			model: "re/doubao-seedance-2.0-fast-face",
			parameters: map[string]interface{}{
				"resolution": "480p",
				"duration":   float64(4),
				"video_urls": []interface{}{"https://example.com/reference.mp4"},
			},
			expected:  "doubao-seedance-2.0-fast-face:480p:withVideo",
			unitPrice: 0.041,
		},
		{
			name:  "Mini nsfw checker remains a valid quoted parameter",
			model: "re/seedance-2.0-mini",
			parameters: map[string]interface{}{
				"resolution":   "720p",
				"duration":     float64(5),
				"nsfw_checker": false,
			},
			expected:  "seedance-2.0-mini:720p:noVideo",
			unitPrice: 0.098,
		},
		{
			name:  "Mini reference video price",
			model: "re/seedance-2.0-mini",
			parameters: map[string]interface{}{
				"resolution":           "720p",
				"duration":             float64(5),
				"reference_video_urls": []interface{}{"https://example.com/reference.mp4"},
			},
			expected:  "seedance-2.0-mini:720p:withVideo",
			unitPrice: 0.060,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priceData := skuPriceData()
			quote, _, err := ApplyPricingSKUToPriceData(&priceData, tt.model, tt.parameters)

			require.NoError(t, err)
			require.NotNil(t, quote)
			assert.Equal(t, tt.expected, quote.MatchedSKU)
			assert.Equal(t, tt.unitPrice, quote.UnitPriceUSD)
			duration, ok := tt.parameters["duration"].(float64)
			require.True(t, ok)
			assert.InDelta(t, tt.unitPrice*duration, finalAmount(priceData), 0.00001)
		})
	}
}

func TestMatchPricingSKURejectsFastFaceHighResolutions(t *testing.T) {
	for _, resolution := range []string{"1080p", "4k"} {
		t.Run(resolution, func(t *testing.T) {
			quote, err := MatchPricingSKU("re/doubao-seedance-2.0-fast-face", map[string]interface{}{
				"resolution": resolution,
				"duration":   float64(5),
			})

			require.Error(t, err)
			assert.Nil(t, quote)
			assert.Contains(t, err.Error(), "pricing SKU not configured")
		})
	}
}

func TestApplyPricingSKUToPriceDataMatchesSeedreamNSFWReferenceCount(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/doubao-seedream-5-0-pro", map[string]interface{}{
		"quality":      "basic",
		"nsfw_checker": true,
		"image_urls":   []interface{}{"https://example.com/a.png", "https://example.com/b.png", "https://example.com/c.png"},
	})

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "doubao-seedream-5-0-pro:1k_nsfw_refs3", quote.MatchedSKU)
	assert.InDelta(t, quote.UnitPriceUSD, finalAmount(priceData), 0.00001)
}

func TestApplyPricingSKUToPriceDataMatchesGeminiOmniReferenceMode(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/gemini-omni", map[string]interface{}{
		"resolution": "720p",
		"duration":   float64(10),
		"image_urls": []interface{}{"https://example.com/reference.png"},
	})

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "gemini-omni:ref:720p", quote.MatchedSKU)
	assert.Equal(t, 1.32, quote.UnitPriceUSD)
}

func TestApplyPricingSKUToPriceDataMatchesMidjourneyModes(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		parameters map[string]interface{}
		expected   string
	}{
		{
			name:       "default imagine relax",
			model:      "re/midjourney",
			parameters: map[string]interface{}{},
			expected:   "midjourney:imagine:relax",
		},
		{
			name:  "edit turbo",
			model: "re/midjourney",
			parameters: map[string]interface{}{
				"action": "edits",
				"speed":  "turbo",
			},
			expected: "midjourney:edit:turbo",
		},
		{
			name:  "nested model params",
			model: "re/mj-v7",
			parameters: map[string]interface{}{
				"model_params": map[string]interface{}{"speed": "draft"},
			},
			expected: "mj-v7:draft",
		},
		{
			name:  "video type resolution",
			model: "re/midjourney-video",
			parameters: map[string]interface{}{
				"video_type": "vid_1.1_i2v_start_end_720",
			},
			expected: "midjourney-video:720",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priceData := skuPriceData()
			quote, _, err := ApplyPricingSKUToPriceData(&priceData, tt.model, tt.parameters)
			require.NoError(t, err)
			require.NotNil(t, quote)
			assert.Equal(t, tt.expected, quote.MatchedSKU)
		})
	}
}

func TestApplyPricingSKUToPriceDataMatchesAudioAndVeoVariants(t *testing.T) {
	klingPrice := skuPriceData()
	klingQuote, _, err := ApplyPricingSKUToPriceData(&klingPrice, "re/kling-3-0", map[string]interface{}{
		"mode":     "pro",
		"sound":    true,
		"duration": float64(5),
	})
	require.NoError(t, err)
	require.NotNil(t, klingQuote)
	assert.Equal(t, "kling-3-0:pro:audio", klingQuote.MatchedSKU)

	veoPrice := skuPriceData()
	veoQuote, _, err := ApplyPricingSKUToPriceData(&veoPrice, "re/veo3.1-fast-official", map[string]interface{}{
		"resolution":     "1080p",
		"generate_audio": true,
		"sample_count":   float64(3),
	})
	require.NoError(t, err)
	require.NotNil(t, veoQuote)
	assert.Equal(t, "veo3.1-fast-official:720p_1080p_audio", veoQuote.MatchedSKU)
	assert.Equal(t, 3.0, veoQuote.Quantity)
	assert.Equal(t, "generations", veoQuote.QuantityKey)
	assert.InDelta(t, veoQuote.UnitPriceUSD*3, finalAmount(veoPrice), 0.00001)

	unsupported := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&unsupported, "re/veo3.1-fast-official", map[string]interface{}{
		"resolution":     "4k",
		"generate_audio": false,
	})
	require.Error(t, err)
	assert.Nil(t, quote)
}

func TestApplyPricingSKUToPriceDataUsesPerThousandWordQuantity(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/humanize", map[string]interface{}{
		"text": "make this sound natural",
	})

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "humanize:flat", quote.MatchedSKU)
	assert.Equal(t, "thousand_words", quote.QuantityKey)
	assert.Equal(t, 0.05, quote.Quantity)
	assert.InDelta(t, quote.UnitPriceUSD*0.05, finalAmount(priceData), 0.00001)
}

func TestApplyPricingSKUToPriceDataRejectsUnsupportedCombination(t *testing.T) {
	priceData := skuPriceData()
	quote, _, err := ApplyPricingSKUToPriceData(&priceData, "re/gpt-image-2-official", map[string]interface{}{
		"size":       "1:1",
		"resolution": "8k",
		"quality":    "low",
	})

	require.Error(t, err)
	assert.Nil(t, quote)
}
