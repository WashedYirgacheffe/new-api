package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	defaultCatalogPath = "docs/catalog/reapi-model-catalog.json"
	defaultOutputPath  = "docs/catalog/reapi-async-pricing.json"
	defaultModelsURL   = "https://reapi.ai/models"
)

var (
	snapshotPattern  = regexp.MustCompile(`(?s)window\.__pricing_snapshot__=(\{.*?\});</script>`)
	modelCardPattern = regexp.MustCompile(`(?s)\\"slug\\":\\"([^"\\]+)\\",\\"displayName\\":\\"([^"\\]+)\\".*?\\"priceMin\\":([0-9.]+),\\"priceMax\\":([0-9.]+),\\"priceUnitLabel\\":\\"([^"\\]+)\\".*?\\"comingSoon\\":(true|false)`)
)

type modelCatalog struct {
	Integrity catalogIntegrity `json:"integrity"`
	Models    []catalogModel   `json:"models"`
}

type catalogIntegrity struct {
	ModelCount      int `json:"model_count"`
	ChatModelCount  int `json:"chat_model_count"`
	AsyncModelCount int `json:"async_model_count"`
}

type catalogModel struct {
	ModelName      string                `json:"model_name"`
	UpstreamModel  string                `json:"upstream_model_id"`
	ModelType      string                `json:"model_type"`
	Protocol       string                `json:"protocol"`
	SourceMetadata catalogSourceMetadata `json:"source_metadata"`
}

type catalogSourceMetadata struct {
	ModelPageSlug *string `json:"model_page_slug"`
}

type upstreamSnapshot struct {
	Version   int64              `json:"version"`
	SKUPrices map[string]float64 `json:"skuPrices"`
}

type modelCard struct {
	Slug        string
	DisplayName string
	MinPrice    float64
	MaxPrice    float64
	Unit        string
	ComingSoon  bool
}

type pricingCatalog struct {
	SchemaVersion int              `json:"schema_version"`
	RetrievedAt   string           `json:"retrieved_at"`
	Currency      string           `json:"currency"`
	Source        pricingSource    `json:"source"`
	Integrity     pricingIntegrity `json:"integrity"`
	Models        []pricingModel   `json:"models"`
}

type pricingSource struct {
	ModelsURL              string `json:"models_url"`
	PricingSnapshotVersion int64  `json:"pricing_snapshot_version"`
	PricingSnapshotSHA256  string `json:"pricing_snapshot_sha256"`
}

type pricingIntegrity struct {
	AsyncModelCount          int `json:"async_model_count"`
	PricedModelCount         int `json:"priced_model_count"`
	SnapshotPricedModelCount int `json:"snapshot_priced_model_count"`
	PublishableModelCount    int `json:"publishable_model_count"`
	ComingSoonModelCount     int `json:"coming_soon_model_count"`
	ExcludedChatModelCount   int `json:"excluded_chat_model_count"`
	SnapshotSKUCount         int `json:"snapshot_sku_count"`
	MatchedSKUCount          int `json:"matched_sku_count"`
}

type pricingModel struct {
	ModelName             string              `json:"model_name"`
	UpstreamModel         string              `json:"upstream_model_id"`
	ModelType             string              `json:"model_type"`
	Publishable           bool                `json:"publishable"`
	ComingSoon            bool                `json:"coming_soon"`
	PricingBasis          string              `json:"pricing_basis"`
	BasePriceUSD          float64             `json:"base_price_usd"`
	MinPriceUSD           float64             `json:"min_price_usd"`
	MaxPriceUSD           float64             `json:"max_price_usd"`
	Unit                  string              `json:"unit"`
	SKUPrefix             string              `json:"sku_prefix"`
	SKUAliasOf            string              `json:"sku_alias_of,omitempty"`
	SKUs                  []pricingSKU        `json:"skus"`
	PublishedPriceRange   publishedPriceRange `json:"published_price_range"`
	SourceURL             string              `json:"source_url"`
	SourceSnapshotVersion int64               `json:"source_snapshot_version"`
}

type pricingSKU struct {
	Key      string  `json:"key"`
	PriceUSD float64 `json:"price_usd"`
}

type publishedPriceRange struct {
	MinPriceUSD float64 `json:"min_price_usd"`
	MaxPriceUSD float64 `json:"max_price_usd"`
	Unit        string  `json:"unit"`
}

func main() {
	catalogPath := flag.String("catalog", defaultCatalogPath, "RE model catalog JSON path")
	outputPath := flag.String("output", defaultOutputPath, "generated RE async pricing JSON path")
	modelsURL := flag.String("models-url", defaultModelsURL, "public RE models page containing the pricing snapshot")
	retrievedAt := flag.String("retrieved-at", "", "RFC3339 retrieval time override")
	flag.Parse()

	catalog, err := loadModelCatalog(*catalogPath)
	if err != nil {
		fatal(err)
	}
	page, err := fetchPage(*modelsURL)
	if err != nil {
		fatal(err)
	}
	snapshot, snapshotRaw, err := parseSnapshot(page)
	if err != nil {
		fatal(err)
	}
	cards, err := parseModelCards(page)
	if err != nil {
		fatal(err)
	}

	timestamp := strings.TrimSpace(*retrievedAt)
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		fatal(fmt.Errorf("parse retrieved-at: %w", err))
	}
	pricing, err := buildPricingCatalog(catalog, snapshot, snapshotRaw, cards, *modelsURL, timestamp)
	if err != nil {
		fatal(err)
	}
	if err := writePricingCatalog(*outputPath, pricing); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %d RE async prices (%d publishable, %d coming soon) to %s\n", len(pricing.Models), pricing.Integrity.PublishableModelCount, pricing.Integrity.ComingSoonModelCount, *outputPath)
}

func loadModelCatalog(path string) (modelCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return modelCatalog{}, fmt.Errorf("read RE model catalog: %w", err)
	}
	var catalog modelCatalog
	if err := common.Unmarshal(data, &catalog); err != nil {
		return modelCatalog{}, fmt.Errorf("decode RE model catalog: %w", err)
	}
	if len(catalog.Models) != catalog.Integrity.ModelCount || catalog.Integrity.AsyncModelCount != 96 || catalog.Integrity.ChatModelCount != 8 {
		return modelCatalog{}, errors.New("RE model catalog integrity metadata is invalid")
	}
	return catalog, nil
}

func fetchPage(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create RE pricing request: %w", err)
	}
	req.Header.Set("User-Agent", "CarLabAPI-reapi-pricing-snapshot/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch RE models page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch RE models page: unexpected HTTP %d", resp.StatusCode)
	}
	page, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read RE models page: %w", err)
	}
	return page, nil
}

func parseSnapshot(page []byte) (upstreamSnapshot, []byte, error) {
	match := snapshotPattern.FindSubmatch(page)
	if len(match) != 2 {
		return upstreamSnapshot{}, nil, errors.New("RE pricing snapshot was not found")
	}
	var snapshot upstreamSnapshot
	if err := common.Unmarshal(match[1], &snapshot); err != nil {
		return upstreamSnapshot{}, nil, fmt.Errorf("decode RE pricing snapshot: %w", err)
	}
	if snapshot.Version <= 0 || len(snapshot.SKUPrices) == 0 {
		return upstreamSnapshot{}, nil, errors.New("RE pricing snapshot is empty")
	}
	return snapshot, match[1], nil
}

func parseModelCards(page []byte) (map[string]modelCard, error) {
	matches := modelCardPattern.FindAllSubmatch(page, -1)
	if len(matches) == 0 {
		return nil, errors.New("RE model price cards were not found")
	}
	cards := make(map[string]modelCard, len(matches))
	for _, match := range matches {
		minPrice, err := strconv.ParseFloat(string(match[3]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse minimum price for %q: %w", match[1], err)
		}
		maxPrice, err := strconv.ParseFloat(string(match[4]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse maximum price for %q: %w", match[1], err)
		}
		comingSoon, err := strconv.ParseBool(string(match[6]))
		if err != nil {
			return nil, fmt.Errorf("parse availability for %q: %w", match[1], err)
		}
		slug := strings.ToLower(string(match[1]))
		cards[slug] = modelCard{
			Slug:        slug,
			DisplayName: string(match[2]),
			MinPrice:    minPrice,
			MaxPrice:    maxPrice,
			Unit:        string(match[5]),
			ComingSoon:  comingSoon,
		}
	}
	return cards, nil
}

func buildPricingCatalog(catalog modelCatalog, snapshot upstreamSnapshot, snapshotRaw []byte, cards map[string]modelCard, modelsURL string, retrievedAt string) (pricingCatalog, error) {
	models := make([]pricingModel, 0, catalog.Integrity.AsyncModelCount)
	matchedSKUKeys := make(map[string]struct{})
	snapshotPricedCount := 0
	publishableCount := 0
	comingSoonCount := 0
	for _, item := range catalog.Models {
		if item.Protocol != "re-task" {
			continue
		}
		cardSlug := pricingCardSlug(item)
		card, ok := cards[cardSlug]
		if !ok {
			return pricingCatalog{}, fmt.Errorf("RE model %q has no public price card for slug %q", item.ModelName, cardSlug)
		}
		skuPrefix := item.UpstreamModel
		skuAliasOf := ""
		if item.UpstreamModel == "kling-3-0-turbo-beta" {
			skuPrefix = "kling-3-0-turbo"
			skuAliasOf = skuPrefix
		}
		skus := collectSKUs(snapshot.SKUPrices, skuPrefix)
		entry := pricingModel{
			ModelName:             item.ModelName,
			UpstreamModel:         item.UpstreamModel,
			ModelType:             item.ModelType,
			Publishable:           !card.ComingSoon,
			ComingSoon:            card.ComingSoon,
			Unit:                  card.Unit,
			SKUPrefix:             skuPrefix,
			SKUAliasOf:            skuAliasOf,
			SKUs:                  skus,
			PublishedPriceRange:   publishedPriceRange{MinPriceUSD: card.MinPrice, MaxPriceUSD: card.MaxPrice, Unit: card.Unit},
			SourceURL:             modelsURL + "/" + card.Slug,
			SourceSnapshotVersion: snapshot.Version,
		}
		if len(skus) == 0 {
			if !card.ComingSoon || card.MinPrice <= 0 || card.MaxPrice <= 0 {
				return pricingCatalog{}, fmt.Errorf("RE model %q has no non-zero snapshot SKU", item.ModelName)
			}
			entry.PricingBasis = "models_index_range"
			entry.BasePriceUSD = card.MaxPrice
			entry.MinPriceUSD = card.MinPrice
			entry.MaxPriceUSD = card.MaxPrice
		} else {
			entry.PricingBasis = "snapshot_sku_max"
			entry.MinPriceUSD = skus[0].PriceUSD
			entry.MaxPriceUSD = skus[0].PriceUSD
			for _, sku := range skus[1:] {
				entry.MinPriceUSD = min(entry.MinPriceUSD, sku.PriceUSD)
				entry.MaxPriceUSD = max(entry.MaxPriceUSD, sku.PriceUSD)
			}
			entry.BasePriceUSD = entry.MaxPriceUSD
			for _, sku := range skus {
				matchedSKUKeys[sku.Key] = struct{}{}
			}
			snapshotPricedCount++
		}
		if entry.BasePriceUSD <= 0 {
			return pricingCatalog{}, fmt.Errorf("RE model %q has a non-positive base price", item.ModelName)
		}
		if entry.Publishable {
			publishableCount++
		}
		if entry.ComingSoon {
			comingSoonCount++
		}
		models = append(models, entry)
	}
	if len(models) != catalog.Integrity.AsyncModelCount || publishableCount != 95 || comingSoonCount != 1 || snapshotPricedCount != 95 {
		return pricingCatalog{}, fmt.Errorf("unexpected RE async pricing counts: models=%d publishable=%d coming_soon=%d snapshot_priced=%d", len(models), publishableCount, comingSoonCount, snapshotPricedCount)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelName < models[j].ModelName })
	snapshotDigest := fmt.Sprintf("%x", sha256.Sum256(snapshotRaw))
	return pricingCatalog{
		SchemaVersion: 1,
		RetrievedAt:   retrievedAt,
		Currency:      "USD",
		Source: pricingSource{
			ModelsURL:              modelsURL,
			PricingSnapshotVersion: snapshot.Version,
			PricingSnapshotSHA256:  snapshotDigest,
		},
		Integrity: pricingIntegrity{
			AsyncModelCount:          len(models),
			PricedModelCount:         len(models),
			SnapshotPricedModelCount: snapshotPricedCount,
			PublishableModelCount:    publishableCount,
			ComingSoonModelCount:     comingSoonCount,
			ExcludedChatModelCount:   catalog.Integrity.ChatModelCount,
			SnapshotSKUCount:         len(snapshot.SKUPrices),
			MatchedSKUCount:          len(matchedSKUKeys),
		},
		Models: models,
	}, nil
}

func collectSKUs(prices map[string]float64, prefix string) []pricingSKU {
	skus := make([]pricingSKU, 0)
	for key, price := range prices {
		if price > 0 && (key == prefix || strings.HasPrefix(key, prefix+":")) {
			skus = append(skus, pricingSKU{Key: key, PriceUSD: price})
		}
	}
	sort.Slice(skus, func(i, j int) bool { return skus[i].Key < skus[j].Key })
	return skus
}

func pricingCardSlug(item catalogModel) string {
	if item.SourceMetadata.ModelPageSlug != nil && strings.TrimSpace(*item.SourceMetadata.ModelPageSlug) != "" {
		return strings.ToLower(strings.TrimSpace(*item.SourceMetadata.ModelPageSlug))
	}
	id := item.UpstreamModel
	switch {
	case strings.HasPrefix(id, "doubao-seedance-2.0"), strings.HasPrefix(id, "seedance-2.0"):
		return "seedance-2-0"
	case id == "seedance-2.5":
		return "seedance-2-5"
	case strings.HasPrefix(id, "flux-2"):
		return "flux-2"
	case strings.HasPrefix(id, "gemini-2.5-flash-image-preview"):
		return "gemini-2-5-flash-image-preview"
	case strings.HasPrefix(id, "gemini-3-pro-image-preview"):
		return "gemini-3-pro-image-preview"
	case strings.HasPrefix(id, "gemini-3.1-flash-image-preview"):
		return "gemini-3-1-flash-image-preview"
	case strings.HasPrefix(id, "gemini-omni"):
		return "gemini-omni"
	case strings.HasPrefix(id, "gpt-image-2"):
		return "gpt-image-2"
	case strings.HasPrefix(id, "grok-imagine-video-1.5"):
		return "grok-imagine-video-1-5"
	case strings.HasPrefix(id, "happyhorse-1.0"):
		return "happyhorse-1-0"
	case strings.HasPrefix(id, "kling-3-0-turbo"):
		return "kling-3-0-turbo"
	case id == "kling-v2-6-motion-control" || id == "kling-v3-motion-control":
		return "kling-motion-control"
	case strings.HasPrefix(id, "mj-v7"):
		return "midjourney-v7"
	case id == "midjourney" || id == "midjourney-video":
		return "midjourney-v8"
	case strings.HasPrefix(id, "suno-"):
		return "suno"
	case strings.HasPrefix(id, "veo3.1"):
		return "veo3-1"
	case strings.HasPrefix(id, "viduq3"):
		return "viduq3"
	case strings.HasPrefix(id, "wan2.7-image"):
		return "wan-2-7-image"
	case strings.HasPrefix(id, "wan2.7-video"):
		return "wan-2-7-video"
	default:
		return strings.ReplaceAll(id, ".", "-")
	}
}

func writePricingCatalog(path string, catalog pricingCatalog) error {
	data, err := common.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("encode RE async pricing catalog: %w", err)
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return fmt.Errorf("format RE async pricing catalog: %w", err)
	}
	formatted.WriteByte('\n')
	if err := os.WriteFile(path, formatted.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write RE async pricing catalog: %w", err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
