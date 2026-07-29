package reapi

import (
	_ "embed"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

//go:embed pricing/reapi-async-pricing.json
var embeddedPricingCatalog []byte

type pricingCatalog struct {
	Currency string         `json:"currency"`
	Models   []pricingModel `json:"models"`
}

type pricingModel struct {
	ModelName     string       `json:"model_name"`
	UpstreamModel string       `json:"upstream_model_id"`
	Publishable   bool         `json:"publishable"`
	Unit          string       `json:"unit"`
	SourceURL     string       `json:"source_url"`
	BasePriceUSD  float64      `json:"base_price_usd"`
	SKUs          []pricingSKU `json:"skus"`
}

type pricingSKU struct {
	Key      string  `json:"key"`
	PriceUSD float64 `json:"price_usd"`
}

type PricingSKUQuote struct {
	ModelName       string  `json:"model_name"`
	MatchedSKU      string  `json:"matched_sku"`
	Unit            string  `json:"unit"`
	UnitPriceUSD    float64 `json:"unit_price_usd"`
	Quantity        float64 `json:"quantity"`
	QuantityKey     string  `json:"quantity_key,omitempty"`
	SourceURL       string  `json:"source_url,omitempty"`
	PricingBasis    string  `json:"pricing_basis"`
	CatalogCurrency string  `json:"catalog_currency"`
}

var embeddedPricingByModel = loadEmbeddedPricingCatalog()

func loadEmbeddedPricingCatalog() map[string]pricingModel {
	var catalog pricingCatalog
	if err := common.Unmarshal(embeddedPricingCatalog, &catalog); err != nil {
		common.SysError("decode embedded RE pricing catalog failed: " + err.Error())
		return nil
	}
	models := make(map[string]pricingModel, len(catalog.Models))
	for _, item := range catalog.Models {
		if item.ModelName == "" || !item.Publishable || len(item.SKUs) == 0 {
			continue
		}
		models[canonicalREModelName(item.ModelName)] = item
	}
	return models
}

func canonicalREModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}
	if strings.HasPrefix(modelName, "re/") {
		return modelName
	}
	return "re/" + modelName
}

func MergePricingParameters(parameterSets ...map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	for _, parameters := range parameterSets {
		for key, value := range parameters {
			if strings.TrimSpace(key) == "" || value == nil {
				continue
			}
			merged[key] = value
		}
	}
	return merged
}

func MatchPricingSKU(modelName string, parameters map[string]interface{}) (*PricingSKUQuote, error) {
	item, ok := embeddedPricingByModel[canonicalREModelName(modelName)]
	if !ok {
		return nil, nil
	}
	sku, ok, err := item.matchSKU(parameters)
	if err != nil || !ok {
		return nil, err
	}
	quantityKey, quantity, err := pricingQuantity(item.Unit, parameters)
	if err != nil {
		return nil, err
	}
	return &PricingSKUQuote{
		ModelName:       item.ModelName,
		MatchedSKU:      sku.Key,
		Unit:            item.Unit,
		UnitPriceUSD:    sku.PriceUSD,
		Quantity:        quantity,
		QuantityKey:     quantityKey,
		SourceURL:       item.SourceURL,
		PricingBasis:    "re_sku",
		CatalogCurrency: "USD",
	}, nil
}

func ApplyPricingSKUToPriceData(priceData *types.PriceData, modelName string, parameters map[string]interface{}) (*PricingSKUQuote, *common.QuotaClamp, error) {
	if priceData == nil {
		return nil, nil, fmt.Errorf("price data is required")
	}
	quote, err := MatchPricingSKU(modelName, parameters)
	if err != nil || quote == nil {
		return quote, nil, err
	}
	priceData.ModelPrice = quote.UnitPriceUSD
	quota, clamp := common.QuotaFromFloatChecked(quote.UnitPriceUSD * common.QuotaPerUnit * priceData.GroupRatioInfo.GroupRatio)
	priceData.Quota = quota
	priceData.QuotaToPreConsume = quota
	if quote.QuantityKey != "" && quote.Quantity != 1 {
		priceData.AddOtherRatio(quote.QuantityKey, quote.Quantity)
	}
	return quote, clamp, nil
}

func (item pricingModel) matchSKU(parameters map[string]interface{}) (pricingSKU, bool, error) {
	if len(item.SKUs) == 0 {
		return pricingSKU{}, false, nil
	}
	if len(item.SKUs) == 1 {
		return item.SKUs[0], true, nil
	}
	suffixes := item.skuSuffixes()
	if hasSuffixContaining(suffixes, "@") {
		return item.matchRatioResolutionQuality(parameters)
	}
	if hasSuffixContaining(suffixes, "_refs") || hasSuffixContaining(suffixes, "_nsfw") {
		return item.matchResolutionNSFWRefs(parameters)
	}
	if hasSuffixContaining(suffixes, ":text") || hasSuffixContaining(suffixes, ":ref") {
		return item.matchResolutionMode(parameters, "ref", "text", hasAnyMaterialParameter(parameters))
	}
	if hasSuffixContaining(suffixes, ":withVideo") || hasSuffixContaining(suffixes, ":noVideo") {
		return item.matchResolutionMode(parameters, "withVideo", "noVideo", hasVideoMaterialParameter(parameters))
	}
	if hasSuffixPrefix(suffixes, "imagine:") {
		return item.matchMidjourneyMode(parameters)
	}
	if item.UpstreamModel == "midjourney-video" {
		return item.matchMidjourneyVideoType(parameters)
	}
	if hasSuffixContaining(suffixes, "720p_1080p") {
		return item.matchVeoResolutionAudio(parameters)
	}
	if hasSuffixContaining(suffixes, ":audio") || hasSuffixContaining(suffixes, ":noaudio") || hasSuffixContaining(suffixes, ":no-audio") {
		if sku, ok := item.matchAudioMode(parameters); ok {
			return sku, true, nil
		}
	}
	if hasSuffixPrefix(suffixes, "gen:") {
		if sku, ok := item.matchGenerationMode(parameters); ok {
			return sku, true, nil
		}
	}
	if sku, ok := item.matchSingleParameter(parameters); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, nil
}

func (item pricingModel) skuSuffixes() []string {
	suffixes := make([]string, 0, len(item.SKUs))
	for _, sku := range item.SKUs {
		suffixes = append(suffixes, skuSuffix(sku.Key))
	}
	return suffixes
}

func skuSuffix(key string) string {
	key = strings.TrimSpace(key)
	if index := strings.Index(key, ":"); index > -1 {
		return key[index+1:]
	}
	return key
}

func hasSuffixContaining(suffixes []string, needle string) bool {
	for _, suffix := range suffixes {
		if strings.Contains(suffix, needle) {
			return true
		}
	}
	return false
}

func hasSuffixPrefix(suffixes []string, prefix string) bool {
	for _, suffix := range suffixes {
		if strings.HasPrefix(suffix, prefix) {
			return true
		}
	}
	return false
}

func (item pricingModel) skuBySuffix(suffix string) (pricingSKU, bool) {
	expected := strings.ToLower(strings.TrimSpace(suffix))
	for _, sku := range item.SKUs {
		if strings.ToLower(skuSuffix(sku.Key)) == expected {
			return sku, true
		}
	}
	return pricingSKU{}, false
}

func (item pricingModel) matchRatioResolutionQuality(parameters map[string]interface{}) (pricingSKU, bool, error) {
	ratio, ok := ratioParameter(parameters)
	if !ok {
		return pricingSKU{}, false, nil
	}
	resolution, ok := stringParameter(parameters, "resolution", "image_size", "output_resolution")
	if !ok {
		return pricingSKU{}, false, nil
	}
	quality, ok := stringParameter(parameters, "quality", "output_quality")
	if !ok {
		return pricingSKU{}, false, nil
	}
	suffix := fmt.Sprintf("%s@%s_%s", normalizeRatio(ratio), normalizeToken(resolution), normalizeToken(quality))
	if sku, ok := item.skuBySuffix(suffix); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with %s", item.ModelName, suffix)
}

func (item pricingModel) matchResolutionNSFWRefs(parameters map[string]interface{}) (pricingSKU, bool, error) {
	resolution, ok := resolutionParameter(parameters)
	if !ok {
		if quality, qualityOK := stringParameter(parameters, "quality"); qualityOK {
			switch normalizeToken(quality) {
			case "basic", "standard", "low", "1k":
				resolution, ok = "1k", true
			case "high", "hd", "2k":
				resolution, ok = "2k", true
			}
		}
	}
	if !ok {
		return pricingSKU{}, false, nil
	}
	parts := []string{normalizeToken(resolution)}
	if boolParameter(parameters, "nsfw_checker", "nsfw") {
		parts = append(parts, "nsfw")
	}
	if refs := imageMaterialCount(parameters); refs >= 2 {
		parts = append(parts, fmt.Sprintf("refs%d", refs))
	}
	suffix := strings.Join(parts, "_")
	if sku, ok := item.skuBySuffix(suffix); ok {
		return sku, true, nil
	}
	if imageMaterialCount(parameters) == 1 {
		withoutSingleRef := strings.Replace(suffix, "_refs1", "", 1)
		if sku, ok := item.skuBySuffix(withoutSingleRef); ok {
			return sku, true, nil
		}
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with %s", item.ModelName, suffix)
}

func (item pricingModel) matchResolutionMode(parameters map[string]interface{}, positiveMode, negativeMode string, positive bool) (pricingSKU, bool, error) {
	resolution, ok := resolutionParameter(parameters)
	if !ok {
		return pricingSKU{}, false, nil
	}
	mode := negativeMode
	if positive {
		mode = positiveMode
	}
	suffix := normalizeToken(resolution) + ":" + mode
	if sku, ok := item.skuBySuffix(suffix); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with %s", item.ModelName, suffix)
}

func (item pricingModel) matchAudioMode(parameters map[string]interface{}) (pricingSKU, bool) {
	resolution, ok := resolutionParameter(parameters)
	if !ok {
		resolution, _ = stringParameter(parameters, "quality", "mode")
	}
	if resolution == "" {
		return pricingSKU{}, false
	}
	withAudio := boolParameter(parameters, "generate_audio", "audio", "with_audio", "sound") || hasAudioMaterialParameter(parameters)
	for _, mode := range []string{"audio", "noaudio", "no-audio"} {
		if !withAudio && mode == "audio" {
			continue
		}
		if withAudio && mode != "audio" {
			continue
		}
		if sku, ok := item.skuBySuffix(normalizeToken(resolution) + ":" + mode); ok {
			return sku, true
		}
	}
	return pricingSKU{}, false
}

func (item pricingModel) matchGenerationMode(parameters map[string]interface{}) (pricingSKU, bool) {
	resolution, ok := resolutionParameter(parameters)
	if !ok {
		return pricingSKU{}, false
	}
	if hasAnyMaterialParameter(parameters) {
		if sku, ok := item.skuBySuffix(fmt.Sprintf("ref:%s", normalizeToken(resolution))); ok {
			return sku, true
		}
		return pricingSKU{}, false
	}
	duration, ok := integerParameter(parameters, "duration", "seconds")
	if !ok || duration <= 0 {
		return pricingSKU{}, false
	}
	if sku, ok := item.skuBySuffix(fmt.Sprintf("gen:%s:%d", normalizeToken(resolution), duration)); ok {
		return sku, true
	}
	return pricingSKU{}, false
}

func (item pricingModel) matchMidjourneyMode(parameters map[string]interface{}) (pricingSKU, bool, error) {
	action, _ := stringParameter(parameters, "action")
	action = normalizeToken(action)
	if action == "" || action == "imagine" {
		action = "imagine"
	} else {
		action = "edit"
	}
	speed, _ := stringParameter(parameters, "speed")
	if speed == "" {
		speed = "relax"
	}
	suffix := action + ":" + normalizeToken(speed)
	if sku, ok := item.skuBySuffix(suffix); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with %s", item.ModelName, suffix)
}

func (item pricingModel) matchMidjourneyVideoType(parameters map[string]interface{}) (pricingSKU, bool, error) {
	videoType, ok := stringParameter(parameters, "video_type")
	if !ok {
		return pricingSKU{}, false, nil
	}
	resolution := ""
	switch {
	case strings.HasSuffix(normalizeToken(videoType), "_480"):
		resolution = "480"
	case strings.HasSuffix(normalizeToken(videoType), "_720"):
		resolution = "720"
	}
	if sku, ok := item.skuBySuffix(resolution); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with video_type %s", item.ModelName, videoType)
}

func (item pricingModel) matchVeoResolutionAudio(parameters map[string]interface{}) (pricingSKU, bool, error) {
	resolution, ok := resolutionParameter(parameters)
	if !ok {
		return pricingSKU{}, false, nil
	}
	bucket := normalizeToken(resolution)
	if bucket == "720p" || bucket == "1080p" {
		bucket = "720p_1080p"
	}
	suffixes := item.skuSuffixes()
	if hasSuffixContaining(suffixes, "_audio") {
		if boolParameter(parameters, "generate_audio", "audio", "with_audio", "sound") {
			bucket += "_audio"
		} else {
			bucket += "_no_audio"
		}
	}
	if sku, ok := item.skuBySuffix(bucket); ok {
		return sku, true, nil
	}
	return pricingSKU{}, false, fmt.Errorf("RE pricing SKU not configured for %s with %s", item.ModelName, bucket)
}

func (item pricingModel) matchSingleParameter(parameters map[string]interface{}) (pricingSKU, bool) {
	candidates := []string{"resolution", "image_size", "output_resolution", "quality", "mode", "service_mode", "speed", "size", "action", "type"}
	for _, key := range candidates {
		value, ok := stringParameter(parameters, key)
		if !ok {
			continue
		}
		if sku, ok := item.skuBySuffix(normalizeToken(value)); ok {
			return sku, true
		}
		if sku, ok := item.skuBySuffix(value); ok {
			return sku, true
		}
	}
	return pricingSKU{}, false
}

func pricingQuantity(unit string, parameters map[string]interface{}) (string, float64, error) {
	unit = strings.ToLower(strings.TrimSpace(unit))
	switch {
	case strings.Contains(unit, "per second"):
		duration, ok := integerParameter(parameters, "duration", "seconds")
		if !ok || duration <= 0 {
			return "", 1, nil
		}
		return "seconds", float64(duration), nil
	case strings.Contains(unit, "per image"):
		n, ok := integerParameter(parameters, "n")
		if !ok {
			return "", 1, nil
		}
		if n < 1 || n > dto.MaxImageN {
			return "", 0, fmt.Errorf("n must be between 1 and %d", dto.MaxImageN)
		}
		return "images", float64(n), nil
	case strings.Contains(unit, "per 1,000 words"):
		words := wordCount(parameters)
		if words < 50 {
			words = 50
		}
		return "thousand_words", float64(words) / 1000, nil
	case strings.Contains(unit, "per generation"):
		n, ok := integerParameter(parameters, "sample_count", "n")
		if !ok {
			return "", 1, nil
		}
		if n < 1 || n > dto.MaxImageN {
			return "", 0, fmt.Errorf("sample_count must be between 1 and %d", dto.MaxImageN)
		}
		return "generations", float64(n), nil
	default:
		return "", 1, nil
	}
}

func stringParameter(parameters map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := lookupParameter(parameters, key)
		if !ok {
			continue
		}
		text := parameterText(value)
		if text != "" {
			return text, true
		}
	}
	return "", false
}

func ratioParameter(parameters map[string]interface{}) (string, bool) {
	for _, key := range []string{"aspect_ratio", "aspectRatio", "ratio", "size"} {
		value, ok := stringParameter(parameters, key)
		if ok && strings.Contains(value, ":") {
			return value, true
		}
	}
	return "", false
}

func resolutionParameter(parameters map[string]interface{}) (string, bool) {
	return stringParameter(parameters, "resolution", "image_size", "output_resolution")
}

func boolParameter(parameters map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		value, ok := lookupParameter(parameters, key)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
			return err == nil && parsed
		case float64:
			return typed != 0
		case int:
			return typed != 0
		}
	}
	return false
}

func integerParameter(parameters map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := lookupParameter(parameters, key)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if math.Trunc(typed) == typed && typed <= float64(math.MaxInt) && typed >= float64(math.MinInt) {
				return int(typed), true
			}
		case int:
			return typed, true
		case int64:
			if typed <= int64(math.MaxInt) && typed >= int64(math.MinInt) {
				return int(typed), true
			}
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func lookupParameter(parameters map[string]interface{}, key string) (interface{}, bool) {
	if parameters == nil {
		return nil, false
	}
	if value, ok := parameters[key]; ok {
		return value, true
	}
	normalizedKey := normalizeFieldName(key)
	for candidate, value := range parameters {
		if normalizeFieldName(candidate) == normalizedKey {
			return value, true
		}
	}
	for _, container := range []string{"metadata", "model_params"} {
		if nested, ok := parameters[container].(map[string]interface{}); ok {
			if value, found := lookupParameter(nested, key); found {
				return value, true
			}
		}
	}
	return nil, false
}

func parameterText(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func normalizeFieldName(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}

func normalizeRatio(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), " ", "")
}

func wordCount(parameters map[string]interface{}) int {
	parts := make([]string, 0, 4)
	for _, key := range []string{"text", "input", "content", "prompt"} {
		if value, ok := stringParameter(parameters, key); ok {
			parts = append(parts, value)
		}
	}
	return len(strings.Fields(strings.Join(parts, " ")))
}

func imageMaterialCount(parameters map[string]interface{}) int {
	return materialCount(parameters, "image", "images", "image_url", "image_urls", "input_image", "input_images", "input_image_url", "input_image_urls", "input_reference", "input_references", "reference_image", "reference_images", "reference_image_url", "reference_image_urls", "image_with_roles", "first_frame", "last_frame", "mask", "mask_url")
}

func hasAnyMaterialParameter(parameters map[string]interface{}) bool {
	return imageMaterialCount(parameters) > 0 || hasVideoMaterialParameter(parameters) || hasAudioMaterialParameter(parameters)
}

func hasVideoMaterialParameter(parameters map[string]interface{}) bool {
	return materialCount(parameters, "video", "videos", "video_url", "video_urls", "input_video", "input_videos", "input_video_url", "input_video_urls", "reference_video", "reference_videos", "reference_video_url", "reference_video_urls") > 0
}

func hasAudioMaterialParameter(parameters map[string]interface{}) bool {
	return materialCount(parameters, "audio", "audios", "audio_url", "audio_urls", "input_audio", "input_audios", "input_audio_url", "input_audio_urls", "reference_voice_urls") > 0
}

func materialCount(parameters map[string]interface{}, keys ...string) int {
	count := 0
	for _, key := range keys {
		value, ok := lookupParameter(parameters, key)
		if !ok {
			continue
		}
		count += countMaterialValue(value)
	}
	return count
}

func countMaterialValue(value interface{}) int {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0
		}
		return 1
	case []interface{}:
		count := 0
		for _, item := range typed {
			count += countMaterialValue(item)
		}
		return count
	case []string:
		count := 0
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				count++
			}
		}
		return count
	case map[string]interface{}:
		if url, ok := typed["url"].(string); ok && strings.TrimSpace(url) != "" {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func AvailablePricingSKUs(modelName string) []string {
	item, ok := embeddedPricingByModel[canonicalREModelName(modelName)]
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(item.SKUs))
	for _, sku := range item.SKUs {
		keys = append(keys, sku.Key)
	}
	sort.Strings(keys)
	return keys
}
