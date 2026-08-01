package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	auditDocumented           = "documented"
	auditMissingDocumentation = "missing_documentation"
	auditDocumentConflict     = "document_conflict"
	auditDisabled             = "disabled"
)

type reContractsFile struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedAt   string                 `json:"generated_at"`
	Models        []reContract           `json:"models"`
	Sources       map[string]interface{} `json:"sources"`
}

type reContract struct {
	ModelName         string                 `json:"model_name"`
	UpstreamModelID   string                 `json:"upstream_model_id"`
	ModelType         string                 `json:"model_type"`
	Operation         string                 `json:"operation"`
	EndpointType      string                 `json:"endpoint_type"`
	ExecutionMode     string                 `json:"execution_mode"`
	ResponseContract  string                 `json:"response_contract"`
	InputSchema       map[string]interface{} `json:"input_schema"`
	UISchema          map[string]interface{} `json:"ui_schema"`
	MaterialSchema    map[string]interface{} `json:"material_schema"`
	RequestContract   map[string]interface{} `json:"request_contract"`
	ParameterDefaults map[string]interface{} `json:"parameter_defaults"`
	DispatchPath      string                 `json:"dispatch_path"`
	PollPath          string                 `json:"poll_path"`
	Evidence          map[string]interface{} `json:"evidence"`
}

type rePricingFile struct {
	SchemaVersion int         `json:"schema_version"`
	RetrievedAt   string      `json:"retrieved_at"`
	Models        []rePricing `json:"models"`
	Source        interface{} `json:"source"`
	Currency      string      `json:"currency"`
}

type rePricing struct {
	ModelName           string                   `json:"model_name"`
	Publishable         bool                     `json:"publishable"`
	ComingSoon          bool                     `json:"coming_soon"`
	PricingBasis        string                   `json:"pricing_basis"`
	BasePriceUSD        float64                  `json:"base_price_usd"`
	MinPriceUSD         float64                  `json:"min_price_usd"`
	MaxPriceUSD         float64                  `json:"max_price_usd"`
	Unit                string                   `json:"unit"`
	SKUPrefix           string                   `json:"sku_prefix"`
	SKUs                []map[string]interface{} `json:"skus"`
	PublishedPriceRange map[string]interface{}   `json:"published_price_range"`
	SourceURL           string                   `json:"source_url"`
	SnapshotVersion     int64                    `json:"source_snapshot_version"`
}

type fullCatalogFile struct {
	Namespaces []struct {
		ChannelProvider       string `json:"channel_provider"`
		CatalogModels         int    `json:"catalog_models"`
		EnabledMetadataModels int    `json:"enabled_metadata_models"`
		BusinessVisibleModels int    `json:"business_token_visible_models"`
	} `json:"namespaces"`
}

type deepWLResponse struct {
	Data []map[string]interface{} `json:"data"`
}

type auditModel struct {
	ModelName        string                 `json:"model_name"`
	GatewayModelID   string                 `json:"gateway_model_id,omitempty"`
	Operation        string                 `json:"operation,omitempty"`
	EndpointType     string                 `json:"endpoint_type,omitempty"`
	ProfileKey       string                 `json:"profile_key,omitempty"`
	ProfileVersion   int                    `json:"profile_version,omitempty"`
	ContractVersion  int                    `json:"contract_version,omitempty"`
	ContractHash     string                 `json:"contract_hash,omitempty"`
	InputSchema      map[string]interface{} `json:"input_schema,omitempty"`
	UISchema         map[string]interface{} `json:"ui_schema,omitempty"`
	MaterialSchema   map[string]interface{} `json:"material_schema,omitempty"`
	RequestContract  map[string]interface{} `json:"request_contract,omitempty"`
	DispatchPath     string                 `json:"dispatch_path,omitempty"`
	PollPath         string                 `json:"poll_path,omitempty"`
	ResponseContract string                 `json:"response_contract,omitempty"`
	PricingRule      map[string]interface{} `json:"pricing_rule,omitempty"`
	Status           string                 `json:"status"`
	AuditStatus      string                 `json:"audit_status"`
	EvidenceURLs     []string               `json:"evidence_urls"`
	EvidenceSHA256   string                 `json:"evidence_sha256"`
	DocumentDate     string                 `json:"document_date,omitempty"`
	Differences      []string               `json:"differences,omitempty"`
	Notes            []string               `json:"notes,omitempty"`
}

type auditSnapshot struct {
	SchemaVersion       int                    `json:"schema_version"`
	GeneratedAt         string                 `json:"generated_at"`
	Channel             string                 `json:"channel"`
	Scope               map[string]interface{} `json:"scope"`
	PublishedModelCount int                    `json:"published_model_count"`
	Models              []auditModel           `json:"models"`
	ProtocolContract    map[string]interface{} `json:"protocol_contract,omitempty"`
	Sources             []string               `json:"sources"`
	Integrity           map[string]interface{} `json:"integrity"`
}

func readJSON(path string, target interface{}) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := common.Unmarshal(data, target); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return data, nil
}

func writeJSON(path string, value interface{}) error {
	data, err := common.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest[:])
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				values = append(values, text)
			}
		}
		return strings.Join(values, "|")
	case []string:
		return strings.Join(typed, "|")
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mapValue(value interface{}) map[string]interface{} {
	result, _ := value.(map[string]interface{})
	return result
}

func mapString(value map[string]interface{}, key string) string {
	return stringValue(value[key])
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func modelEvidenceHash(contract reContract, pricing *rePricing) string {
	payload := map[string]interface{}{"contract": contract}
	if pricing != nil {
		payload["pricing"] = pricing
	}
	data, _ := common.Marshal(payload)
	return sha256Hex(data)
}

func buildRESnapshot(contracts reContractsFile, contractBytes []byte, pricing rePricingFile, pricingBytes []byte, generatedAt string) auditSnapshot {
	prices := make(map[string]rePricing, len(pricing.Models))
	for _, item := range pricing.Models {
		prices[item.ModelName] = item
	}
	models := make([]auditModel, 0, len(contracts.Models))
	for _, contract := range contracts.Models {
		price, hasPrice := prices[contract.ModelName]
		var priceRule map[string]interface{}
		var sourceURLs []string
		if hasPrice {
			priceRule = map[string]interface{}{
				"basis":                 price.PricingBasis,
				"currency":              pricing.Currency,
				"base_price_usd":        price.BasePriceUSD,
				"min_price_usd":         price.MinPriceUSD,
				"max_price_usd":         price.MaxPriceUSD,
				"unit":                  price.Unit,
				"skus":                  price.SKUs,
				"published_price_range": price.PublishedPriceRange,
				"snapshot_version":      price.SnapshotVersion,
			}
			if price.SourceURL != "" {
				sourceURLs = append(sourceURLs, price.SourceURL)
			}
		}
		for _, key := range []string{"manifest", "cfg"} {
			if evidence := mapValue(contract.Evidence[key]); evidence != nil {
				for _, evidenceKey := range []string{"chunk_url", "page_url"} {
					if value := mapString(evidence, evidenceKey); value != "" {
						sourceURLs = append(sourceURLs, value)
					}
				}
			}
		}
		differences := make([]string, 0)
		if !hasPrice {
			differences = append(differences, "没有匹配到 RE 价格 SKU，禁止报价和发布")
		}
		if mismatches := mapValue(contract.Evidence["enum_evidence_mismatches"]); mismatches != nil {
			differences = append(differences, "上游枚举证据存在差异，需按合同快照收窄")
		}
		status := "disabled"
		auditStatus := auditMissingDocumentation
		if hasPrice && price.Publishable && !price.ComingSoon {
			status = "published"
		}
		if hasPrice && len(sourceURLs) > 0 {
			auditStatus = auditDocumented
		}
		models = append(models, auditModel{
			ModelName:        contract.ModelName,
			GatewayModelID:   contract.UpstreamModelID,
			Operation:        contract.Operation,
			EndpointType:     contract.EndpointType,
			InputSchema:      contract.InputSchema,
			UISchema:         contract.UISchema,
			MaterialSchema:   contract.MaterialSchema,
			RequestContract:  contract.RequestContract,
			DispatchPath:     contract.DispatchPath,
			PollPath:         contract.PollPath,
			ResponseContract: contract.ResponseContract,
			PricingRule:      priceRule,
			Status:           status,
			AuditStatus:      auditStatus,
			EvidenceURLs:     uniqueStrings(sourceURLs),
			EvidenceSHA256: modelEvidenceHash(contract, func() *rePricing {
				if hasPrice {
					return &price
				}
				return nil
			}()),
			DocumentDate: firstNonEmpty(contracts.GeneratedAt, pricing.RetrievedAt),
			Differences:  uniqueStrings(differences),
			Notes:        []string{"CarLab profile/binding revision需从生产目录导出后填入；本快照保留上游合同与 SKU 证据。"},
		})
	}
	scope := map[string]interface{}{
		"source_contract_model_count": len(contracts.Models),
		"source_pricing_model_count":  len(pricing.Models),
		"contract_source_sha256":      sha256Hex(contractBytes),
		"pricing_source_sha256":       sha256Hex(pricingBytes),
		"authority_order":             []string{"具体模型 API 合同", "RE 不可变合同快照", "RE 模型价格 SKU"},
	}
	return makeSnapshot("re", len(models), models, scope, []string{
		"https://reapi.ai/zh/models",
		"docs/catalog/reapi-async-contracts.json",
		"docs/catalog/reapi-async-pricing.json",
	}, nil, generatedAt)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildDeepWLSnapshot(response deepWLResponse, sourceBytes []byte, generatedAt string) auditSnapshot {
	models := make([]auditModel, 0, len(response.Data))
	for _, item := range response.Data {
		upstreamID := mapString(item, "model_name")
		if upstreamID == "" {
			continue
		}
		modelName := upstreamID
		if !strings.Contains(modelName, "/") {
			modelName = "deepwl/" + modelName
		}
		modelType := mapString(item, "model_type")
		supportedEndpoints := item["supported_endpoint_types"]
		pricingRule := map[string]interface{}{
			"quota_type":       item["quota_type"],
			"model_ratio":      item["model_ratio"],
			"completion_ratio": item["completion_ratio"],
			"cache_ratio":      item["cache_ratio"],
			"model_price":      item["model_price"],
			"pricing_version":  item["pricing_version"],
		}
		differences := []string{
			"当前输入/输出参数合同未随公开价格接口提供",
			"当前 CarLab 已发布 DeepWL 命名空间仅记录为 67 个，匿名价格目录无法逐项映射发布身份",
			"没有具体模型 API 详情页证据时，不在 TapLater 展示或自动发布参数",
		}
		models = append(models, auditModel{
			ModelName:      modelName,
			GatewayModelID: upstreamID,
			Operation:      modelTypeToOperation(modelType),
			EndpointType:   stringValue(supportedEndpoints),
			PricingRule:    pricingRule,
			Status:         "published_catalog_candidate",
			AuditStatus:    auditMissingDocumentation,
			EvidenceURLs: []string{
				"https://doc.deepwl.cn/llms.txt",
				"https://zx1.deepwl.net/pricing",
				"https://zx1.deepwl.net/api/pricing",
			},
			EvidenceSHA256: sha256Hex(mustMarshal(item)),
			DocumentDate:   generatedAt,
			Differences:    differences,
		})
	}
	scope := map[string]interface{}{
		"carlab_published_namespaced_model_count": 67,
		"upstream_pricing_model_count":            len(response.Data),
		"pricing_source_sha256":                   sha256Hex(sourceBytes),
		"authority_order":                         []string{"具体模型 API 文档", "DeepWL llms.txt", "DeepWL /api/pricing", "CarLab 现有合同"},
		"published_identity_export":               "not_available_without_token_scoped_catalog",
	}
	return makeSnapshot("deepwl", 67, models, scope, []string{
		"https://doc.deepwl.cn/",
		"https://doc.deepwl.cn/llms.txt",
		"https://zx1.deepwl.net/pricing",
		"https://zx1.deepwl.net/api/pricing",
	}, nil, generatedAt)
}

func modelTypeToOperation(modelType string) string {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "text":
		return "text.chat"
	case "image":
		return "image.generate"
	case "video":
		return "video.generate"
	case "audio":
		return "audio.generate"
	default:
		return ""
	}
}

func buildNodyHubSnapshot(fullCatalog fullCatalogFile, docBytes []byte, docPath string, generatedAt string) auditSnapshot {
	count := 0
	enabledCount := 0
	visibleCount := 0
	for _, namespace := range fullCatalog.Namespaces {
		if strings.EqualFold(namespace.ChannelProvider, "nodyhub") {
			count = namespace.CatalogModels
			enabledCount = namespace.EnabledMetadataModels
			visibleCount = namespace.BusinessVisibleModels
			break
		}
	}
	if count == 0 {
		count = visibleCount
	}
	protocol := map[string]interface{}{
		"base_url":      "https://nodyhub.com",
		"authorization": "Bearer <NODYHUB_API_KEY>",
		"image_generation": map[string]interface{}{
			"submit_path": "/v1/images/generations?async=true",
			"poll_path":   "/v1/images/tasks/{taskId}",
			"async":       true,
		},
		"video_generation": map[string]interface{}{
			"submit_path": "/v2/videos/generations",
			"poll_path":   "/v2/videos/generations/{taskId}",
			"async":       true,
		},
		"seedance_assets": map[string]interface{}{
			"submit_path": "/v1/seedance/assets2",
			"transports":  []string{"url", "base64", "multipart", "asset://"},
		},
		"material_rule":   "首尾帧和多模态参考素材互斥；具体模型槽位以模型章节为准",
		"source_document": filepath.Base(docPath),
	}
	models := []auditModel{{
		ModelName:      "__nodyhub_published_catalog__",
		GatewayModelID: "*",
		Operation:      "*",
		EndpointType:   "protocol-only",
		Status:         "published",
		AuditStatus:    auditMissingDocumentation,
		EvidenceURLs: []string{
			"https://nodyhub.com/v1/models",
			"https://nodyhub.com/pricing",
		},
		EvidenceSHA256: sha256Hex(docBytes),
		DocumentDate:   generatedAt,
		Differences: []string{
			"Nodyhub API 文档已证实提交、轮询、素材传输和互斥规则，但当前文档未提供可供 CarLab 匿名导出的 726 个逐模型身份清单",
			"Nodyhub 价格页已登记为价格权威入口，但当前未提供可供本地快照逐模型读取的价格数据，不能据页面名称推导 SKU",
			"未提供具体模型的输入枚举、默认值、素材数量/MIME/大小和响应字段时，不生成模型级合同",
		},
		Notes: []string{fmt.Sprintf("catalog_models=%d; enabled_metadata_models=%d; business_token_visible_models=%d", count, enabledCount, visibleCount)},
	}}
	scope := map[string]interface{}{
		"catalog_models":                count,
		"enabled_metadata_models":       enabledCount,
		"business_token_visible_models": visibleCount,
		"protocol_document_sha256":      sha256Hex(docBytes),
		"model_level_identity_export":   "not_available_without_authenticated_catalog",
		"pricing_evidence":              "https://nodyhub.com/pricing",
		"protocol_evidence_status":      auditDocumented,
	}
	return makeSnapshot("nodyhub", count, models, scope, []string{
		"https://nodyhub.com/v1/models",
		"https://nodyhub.com/pricing",
	}, protocol, generatedAt)
}

func mustMarshal(value interface{}) []byte {
	data, _ := common.Marshal(value)
	return data
}

func makeSnapshot(channel string, publishedCount int, models []auditModel, scope map[string]interface{}, sources []string, protocol map[string]interface{}, generatedAt string) auditSnapshot {
	sort.Slice(models, func(i, j int) bool { return models[i].ModelName < models[j].ModelName })
	modelsBytes := mustMarshal(models)
	integrity := map[string]interface{}{
		"digest_algorithm": "sha256",
		"models_sha256":    sha256Hex(modelsBytes),
		"model_rows":       len(models),
	}
	return auditSnapshot{
		SchemaVersion:       1,
		GeneratedAt:         generatedAt,
		Channel:             channel,
		Scope:               scope,
		PublishedModelCount: publishedCount,
		Models:              models,
		ProtocolContract:    protocol,
		Sources:             uniqueStrings(sources),
		Integrity:           integrity,
	}
}

func csvValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return string(mustMarshal(value))
}

func schemaFields(schema map[string]interface{}) string {
	properties := mapValue(schema["properties"])
	fields := make([]string, 0, len(properties))
	for name := range properties {
		fields = append(fields, name)
	}
	return strings.Join(uniqueStrings(fields), ",")
}

func materialSlots(schema map[string]interface{}) string {
	slots := make([]string, 0, len(schema))
	for name := range schema {
		slots = append(slots, name)
	}
	return strings.Join(uniqueStrings(slots), ",")
}

func writeCSV(path string, snapshot auditSnapshot) error {
	var builder strings.Builder
	columns := []string{"channel", "model_name", "gateway_model_id", "operation", "endpoint_type", "profile_key", "profile_version", "contract_version", "contract_hash", "audit_status", "input_fields", "material_slots", "request_adapter", "dispatch_path", "poll_path", "pricing_basis", "source_urls", "difference_summary"}
	builder.WriteString(strings.Join(columns, ","))
	builder.WriteByte('\n')
	for _, model := range snapshot.Models {
		requestAdapter := ""
		if model.RequestContract != nil {
			requestAdapter = mapString(model.RequestContract, "adapter")
		}
		pricingBasis := ""
		if model.PricingRule != nil {
			pricingBasis = mapString(model.PricingRule, "basis")
			if pricingBasis == "" {
				pricingBasis = mapString(model.PricingRule, "pricing_basis")
			}
		}
		values := []string{
			snapshot.Channel,
			model.ModelName,
			model.GatewayModelID,
			model.Operation,
			model.EndpointType,
			model.ProfileKey,
			fmt.Sprint(model.ProfileVersion),
			fmt.Sprint(model.ContractVersion),
			model.ContractHash,
			model.AuditStatus,
			schemaFields(model.InputSchema),
			materialSlots(model.MaterialSchema),
			requestAdapter,
			model.DispatchPath,
			model.PollPath,
			pricingBasis,
			strings.Join(model.EvidenceURLs, " | "),
			strings.Join(model.Differences, "；"),
		}
		quoted := make([]string, len(values))
		for index, value := range values {
			quoted[index] = `"` + strings.ReplaceAll(strings.ReplaceAll(value, `"`, `""`), "\n", " ") + `"`
		}
		builder.WriteString(strings.Join(quoted, ","))
		builder.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func fetchDeepWLPricing(url string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("DeepWL pricing returned HTTP %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func main() {
	root := flag.String("root", ".", "CarLab repository root")
	outputDir := flag.String("output", "docs/catalog", "output directory relative to root")
	nodyDoc := flag.String("nodyhub-doc", "/Users/washed/Downloads/NODYHUB_API_DOC (1).md", "Nodyhub authoritative Markdown document")
	deepwlPricingFile := flag.String("deepwl-pricing-file", "", "use a cached DeepWL pricing JSON instead of fetching")
	deepwlPricingURL := flag.String("deepwl-pricing-url", "https://zx1.deepwl.net/api/pricing", "DeepWL public pricing endpoint")
	flag.Parse()

	contractsPath := filepath.Join(*root, "docs/catalog/reapi-async-contracts.json")
	pricingPath := filepath.Join(*root, "docs/catalog/reapi-async-pricing.json")
	fullCatalogPath := filepath.Join(*root, "docs/catalog/full-channel-catalog-evidence.json")
	contractsBytes, err := os.ReadFile(contractsPath)
	if err != nil {
		panic(err)
	}
	pricingBytes, err := os.ReadFile(pricingPath)
	if err != nil {
		panic(err)
	}
	var contracts reContractsFile
	var pricing rePricingFile
	var fullCatalog fullCatalogFile
	if err := common.Unmarshal(contractsBytes, &contracts); err != nil {
		panic(err)
	}
	if err := common.Unmarshal(pricingBytes, &pricing); err != nil {
		panic(err)
	}
	if _, err := readJSON(fullCatalogPath, &fullCatalog); err != nil {
		panic(err)
	}
	deepwlBytes := []byte(nil)
	if *deepwlPricingFile != "" {
		deepwlBytes, err = os.ReadFile(*deepwlPricingFile)
	} else {
		deepwlBytes, err = fetchDeepWLPricing(*deepwlPricingURL)
	}
	if err != nil {
		panic(err)
	}
	var deepwl deepWLResponse
	if err := common.Unmarshal(deepwlBytes, &deepwl); err != nil {
		panic(err)
	}
	nodyBytes, err := os.ReadFile(*nodyDoc)
	if err != nil {
		panic(err)
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	reSnapshot := buildRESnapshot(contracts, contractsBytes, pricing, pricingBytes, generatedAt)
	deepwlSnapshot := buildDeepWLSnapshot(deepwl, deepwlBytes, generatedAt)
	nodySnapshot := buildNodyHubSnapshot(fullCatalog, nodyBytes, *nodyDoc, generatedAt)
	outDir := filepath.Join(*root, *outputDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	outputs := []struct {
		name     string
		snapshot auditSnapshot
	}{
		{"reapi-published-contracts", reSnapshot},
		{"deepwl-published-contracts", deepwlSnapshot},
		{"nodyhub-published-contracts", nodySnapshot},
	}
	for _, output := range outputs {
		jsonPath := filepath.Join(outDir, output.name+".json")
		csvPath := filepath.Join(outDir, output.name+".csv")
		if err := writeJSON(jsonPath, output.snapshot); err != nil {
			panic(err)
		}
		if err := writeCSV(csvPath, output.snapshot); err != nil {
			panic(err)
		}
		fmt.Printf("%s: %d published, %d audit rows\n", output.snapshot.Channel, output.snapshot.PublishedModelCount, len(output.snapshot.Models))
	}
}
