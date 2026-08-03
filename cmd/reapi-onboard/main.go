package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/reapi"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultCatalogPath          = "docs/catalog/reapi-model-catalog.json"
	defaultPricingCatalogPath   = "docs/catalog/reapi-async-pricing.json"
	defaultContractCatalogPath  = "docs/catalog/reapi-async-contracts.json"
	asyncContractModelCount     = 88
	asyncContractExclusionCount = 8
	channelProvider             = "re"
	chatChannelName             = "RE Chat"
	taskChannelName             = "RE Async"
	chatBaseURL                 = "https://api.reapi.ai"
	taskBaseURL                 = "https://reapi.ai/api/v1"
)

type catalog struct {
	Integrity catalogIntegrity `json:"integrity"`
	Models    []catalogModel   `json:"models"`
}

type catalogIntegrity struct {
	ModelCount            int    `json:"model_count"`
	ChatModelCount        int    `json:"chat_model_count"`
	AsyncModelCount       int    `json:"async_model_count"`
	SortedModelNameSHA256 string `json:"sorted_model_name_sha256"`
}

type catalogModel struct {
	ModelName      string         `json:"model_name"`
	UpstreamModel  string         `json:"upstream_model_id"`
	DisplayName    string         `json:"display_name"`
	ModelType      string         `json:"model_type"`
	Vendor         string         `json:"vendor"`
	Protocol       string         `json:"protocol"`
	Endpoint       string         `json:"endpoint"`
	SourceURL      string         `json:"source_url"`
	SourceMetadata map[string]any `json:"source_metadata"`
	Status         string         `json:"status"`
}

type asyncPricingCatalog struct {
	SchemaVersion int                          `json:"schema_version"`
	Currency      string                       `json:"currency"`
	Integrity     asyncPricingCatalogIntegrity `json:"integrity"`
	Models        []asyncPricingModel          `json:"models"`
}

type asyncPricingCatalogIntegrity struct {
	AsyncModelCount       int `json:"async_model_count"`
	PricedModelCount      int `json:"priced_model_count"`
	PublishableModelCount int `json:"publishable_model_count"`
	ComingSoonModelCount  int `json:"coming_soon_model_count"`
	ExcludedChatCount     int `json:"excluded_chat_model_count"`
}

type asyncPricingModel struct {
	ModelName     string  `json:"model_name"`
	UpstreamModel string  `json:"upstream_model_id"`
	ModelType     string  `json:"model_type"`
	Publishable   bool    `json:"publishable"`
	ComingSoon    bool    `json:"coming_soon"`
	PricingBasis  string  `json:"pricing_basis"`
	BasePriceUSD  float64 `json:"base_price_usd"`
	MinPriceUSD   float64 `json:"min_price_usd"`
	MaxPriceUSD   float64 `json:"max_price_usd"`
	Unit          string  `json:"unit"`
	SourceURL     string  `json:"source_url"`
}

type asyncContractCatalog struct {
	SchemaVersion int                             `json:"schema_version"`
	GeneratedAt   string                          `json:"generated_at"`
	Sources       map[string]interface{}          `json:"sources"`
	Integrity     map[string]interface{}          `json:"integrity"`
	Exclusions    []asyncContractCatalogExclusion `json:"exclusions"`
	Models        []asyncContractModel            `json:"models"`
}

type asyncContractCatalogExclusion struct {
	ModelName string `json:"model_name"`
	Reason    string `json:"reason"`
}

type asyncContractModel struct {
	ModelName           string                              `json:"model_name"`
	UpstreamModel       string                              `json:"upstream_model_id"`
	ModelType           string                              `json:"model_type"`
	Operation           string                              `json:"operation"`
	EndpointType        string                              `json:"endpoint_type"`
	ExecutionMode       string                              `json:"execution_mode"`
	ResponseContract    string                              `json:"response_contract"`
	SchemaMode          string                              `json:"schema_mode"`
	InputSchema         map[string]interface{}              `json:"input_schema"`
	UISchema            map[string]interface{}              `json:"ui_schema"`
	MaterialSchema      map[string]interface{}              `json:"material_schema"`
	RequestContract     model.ModelOperationRequestContract `json:"request_contract"`
	ParameterDefaults   map[string]interface{}              `json:"parameter_defaults"`
	Modes               []model.ModelOperationContractMode  `json:"modes,omitempty"`
	DispatchPath        string                              `json:"dispatch_path"`
	PollPath            string                              `json:"poll_path"`
	Evidence            interface{}                         `json:"evidence"`
	normalizedOverrides string
}

type reAsyncProfileDefinition struct {
	ModelType        string
	ProfileKey       string
	DisplayName      string
	Operation        string
	ResponseContract string
}

var reAsyncProfileDefinitions = []reAsyncProfileDefinition{
	{ModelType: "image", ProfileKey: "re.image.generate", DisplayName: "RE image generation", Operation: "image.generate", ResponseContract: "re-image-task-v1"},
	{ModelType: "video", ProfileKey: "re.video.generate", DisplayName: "RE video generation", Operation: "video.generate", ResponseContract: "re-video-task-v1"},
	{ModelType: "audio", ProfileKey: "re.audio.generate", DisplayName: "RE audio generation", Operation: "audio.generate", ResponseContract: "re-audio-task-v1"},
	{ModelType: "text", ProfileKey: "re.text.generate", DisplayName: "RE text generation", Operation: "text.generate", ResponseContract: "re-text-task-v1"},
}

var deferredBillingModels = map[string]struct{}{
	"re/audio-multistem":       {},
	"re/audio-music-extractor": {},
	"re/audio-stem-separator":  {},
	"re/audio-voice-change":    {},
	"re/audio-voice-clean":     {},
	"re/enhance-video-1.0":     {},
	"re/topaz-video-upscaler":  {},
}

func main() {
	catalogPath := flag.String("catalog", defaultCatalogPath, "RE catalog JSON path")
	pricingCatalogPath := flag.String("pricing-catalog", defaultPricingCatalogPath, "RE async pricing catalog JSON path")
	contractCatalogPath := flag.String("contract-catalog", defaultContractCatalogPath, "RE async operation contract catalog JSON path")
	dryRun := flag.Bool("dry-run", false, "validate the catalog and required secrets without writing to the database")
	publishAsync := flag.Bool("publish-async", false, "price and enable all currently publishable RE async models; chat remains disabled")
	flag.Parse()

	catalog, err := loadCatalog(*catalogPath)
	if err != nil {
		fatal(err)
	}
	pricing, err := loadAsyncPricingCatalog(*pricingCatalogPath, catalog)
	if err != nil {
		fatal(err)
	}
	contracts, err := loadAsyncContractCatalog(*contractCatalogPath, pricing)
	if err != nil {
		fatal(err)
	}
	if *dryRun {
		fmt.Printf("RE catalogs are valid: %d models (%d chat, %d async), %d upstream-publishable, %d CarLab-ready with exact contracts, %d deferred for billing, %d coming soon; no database changes made\n", len(catalog.Models), catalog.Integrity.ChatModelCount, catalog.Integrity.AsyncModelCount, pricing.Integrity.PublishableModelCount, len(contracts.Models), len(deferredBillingModels), pricing.Integrity.ComingSoonModelCount)
		return
	}
	chatKey, taskKey, err := configuredKeys()
	if err != nil {
		fatal(err)
	}
	if *publishAsync && taskKey == "" {
		fatal(errors.New("--publish-async requires REAPI_TASK_API_KEY; no database changes were made"))
	}

	common.InitEnv()
	ratio_setting.InitRatioSettings()
	if err := model.InitDB(); err != nil {
		fatal(fmt.Errorf("initialize database: %w", err))
	}
	if err := model.InitLogDB(); err != nil {
		fatal(fmt.Errorf("initialize log database: %w", err))
	}
	defer func() { _ = model.CloseDB() }()
	model.InitOptionMap()

	vendorIDs, err := ensureVendors(catalog.Models)
	if err != nil {
		fatal(err)
	}
	pricingByModel := make(map[string]asyncPricingModel, len(pricing.Models))
	for _, item := range pricing.Models {
		pricingByModel[item.ModelName] = item
	}
	if *publishAsync {
		if err := disableExistingREChannels(); err != nil {
			fatal(err)
		}
		if err := publishAsyncPrices(pricingByModel); err != nil {
			fatal(err)
		}
	}
	chatModels, taskModels, err := upsertModels(catalog.Models, vendorIDs, pricingByModel, *publishAsync)
	if err != nil {
		fatal(err)
	}
	if *publishAsync {
		if err := upsertREAsyncContracts(contracts, taskModels); err != nil {
			fatal(err)
		}
	}
	initializedChannels := make([]string, 0, 2)
	if chatKey != "" && !*publishAsync {
		if err := upsertChannel(chatChannelName, constant.ChannelTypeOpenAI, chatBaseURL, chatKey, chatModels, common.ChannelStatusManuallyDisabled, true); err != nil {
			fatal(err)
		}
		initializedChannels = append(initializedChannels, chatChannelName)
	}
	if taskKey != "" {
		status := common.ChannelStatusManuallyDisabled
		preserveExistingStatus := true
		if *publishAsync {
			status = common.ChannelStatusEnabled
			preserveExistingStatus = false
		}
		if err := upsertChannel(taskChannelName, constant.ChannelTypeReAPI, taskBaseURL, taskKey, taskModels, status, preserveExistingStatus); err != nil {
			fatal(err)
		}
		initializedChannels = append(initializedChannels, taskChannelName)
	}
	model.RefreshPricing()

	if *publishAsync {
		fmt.Printf("RE async publishing complete: %d models priced and enabled, %d billing-deferred and %d upstream-unavailable models kept disabled; chat disabled\n", len(taskModels), len(deferredBillingModels), pricing.Integrity.ComingSoonModelCount)
		return
	}
	fmt.Printf("RE onboarding complete: %d catalog models, %d chat mappings, %d publishable async mappings; initialized or refreshed %s without enabling new channels\n", len(catalog.Models), len(chatModels), len(taskModels), strings.Join(initializedChannels, ", "))
}

func loadCatalog(path string) (catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return catalog{}, fmt.Errorf("read RE catalog: %w", err)
	}
	var loaded catalog
	if err := common.Unmarshal(data, &loaded); err != nil {
		return catalog{}, fmt.Errorf("decode RE catalog: %w", err)
	}
	if err := validateCatalog(loaded); err != nil {
		return catalog{}, err
	}
	return loaded, nil
}

func validateCatalog(catalog catalog) error {
	if len(catalog.Models) != catalog.Integrity.ModelCount || catalog.Integrity.ModelCount != 104 {
		return fmt.Errorf("RE catalog must contain 104 models, got %d", len(catalog.Models))
	}
	if catalog.Integrity.ChatModelCount != 8 || catalog.Integrity.AsyncModelCount != 96 {
		return fmt.Errorf("RE catalog must contain 8 chat and 96 async models")
	}
	seen := make(map[string]struct{}, len(catalog.Models))
	chatCount := 0
	asyncCount := 0
	names := make([]string, 0, len(catalog.Models))
	for _, item := range catalog.Models {
		if item.ModelName != "re/"+item.UpstreamModel || strings.TrimSpace(item.UpstreamModel) == "" {
			return fmt.Errorf("invalid RE model namespace for %q", item.ModelName)
		}
		if _, exists := seen[item.ModelName]; exists {
			return fmt.Errorf("duplicate RE model %q", item.ModelName)
		}
		seen[item.ModelName] = struct{}{}
		names = append(names, item.ModelName)
		if item.Status != "disabled_pending_pricing" || item.SourceURL == "" || item.Vendor == "" {
			return fmt.Errorf("RE model %q is missing its disabled status, source URL, or vendor", item.ModelName)
		}
		switch item.Protocol {
		case "chat-completions":
			chatCount++
			if item.Endpoint != "/v1/chat/completions" || item.ModelType != "text" {
				return fmt.Errorf("RE chat model %q has invalid endpoint %q", item.ModelName, item.Endpoint)
			}
		case "re-task":
			asyncCount++
			endpoint, supported := reapi.EndpointForModel(item.UpstreamModel)
			if !supported || endpoint != item.Endpoint {
				return fmt.Errorf("RE async model %q does not match the runtime adaptor", item.ModelName)
			}
			operation, supported := reapi.OperationForModel(item.UpstreamModel)
			expectedOperation := map[string]string{
				"image": "image.generate",
				"video": "video.generate",
				"audio": "audio.generate",
				"text":  "text.generate",
			}[item.ModelType]
			if !supported || operation != expectedOperation {
				return fmt.Errorf("RE async model %q has invalid model type %q for operation %q", item.ModelName, item.ModelType, operation)
			}
		default:
			return fmt.Errorf("RE model %q has unsupported protocol %q", item.ModelName, item.Protocol)
		}
	}
	if chatCount != catalog.Integrity.ChatModelCount || asyncCount != catalog.Integrity.AsyncModelCount {
		return fmt.Errorf("RE catalog protocol counts do not match integrity metadata")
	}
	sort.Strings(names)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(names, "\n")+"\n")))
	if catalog.Integrity.ModelCount != len(names) || digest != catalog.Integrity.SortedModelNameSHA256 {
		return errors.New("RE catalog integrity check failed")
	}
	return nil
}

func loadAsyncPricingCatalog(path string, models catalog) (asyncPricingCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return asyncPricingCatalog{}, fmt.Errorf("read RE async pricing catalog: %w", err)
	}
	var loaded asyncPricingCatalog
	if err := common.Unmarshal(data, &loaded); err != nil {
		return asyncPricingCatalog{}, fmt.Errorf("decode RE async pricing catalog: %w", err)
	}
	if err := validateAsyncPricingCatalog(loaded, models); err != nil {
		return asyncPricingCatalog{}, err
	}
	return loaded, nil
}

func validateAsyncPricingCatalog(pricing asyncPricingCatalog, models catalog) error {
	asyncModels := make(map[string]catalogModel, models.Integrity.AsyncModelCount)
	for _, item := range models.Models {
		if item.Protocol == "re-task" {
			asyncModels[item.ModelName] = item
		}
	}
	if pricing.SchemaVersion <= 0 || pricing.Currency != "USD" {
		return errors.New("RE async pricing catalog must declare a schema version and USD currency")
	}
	if len(pricing.Models) != len(asyncModels) || pricing.Integrity.AsyncModelCount != len(asyncModels) {
		return fmt.Errorf("RE async pricing catalog must contain %d models, got %d", len(asyncModels), len(pricing.Models))
	}
	seen := make(map[string]struct{}, len(pricing.Models))
	publishableCount := 0
	comingSoonCount := 0
	for _, item := range pricing.Models {
		catalogItem, exists := asyncModels[item.ModelName]
		if !exists {
			return fmt.Errorf("RE async pricing contains unknown or chat model %q", item.ModelName)
		}
		if _, exists := seen[item.ModelName]; exists {
			return fmt.Errorf("RE async pricing contains duplicate model %q", item.ModelName)
		}
		seen[item.ModelName] = struct{}{}
		if item.ModelName != "re/"+item.UpstreamModel || item.UpstreamModel != catalogItem.UpstreamModel || item.ModelType != catalogItem.ModelType {
			return fmt.Errorf("RE async pricing identity does not match model catalog for %q", item.ModelName)
		}
		if item.PricingBasis == "" || item.Unit == "" || item.SourceURL == "" {
			return fmt.Errorf("RE async pricing evidence is incomplete for %q", item.ModelName)
		}
		if item.BasePriceUSD <= 0 || item.MinPriceUSD <= 0 || item.MaxPriceUSD <= 0 ||
			math.IsNaN(item.BasePriceUSD) || math.IsInf(item.BasePriceUSD, 0) || item.MinPriceUSD > item.MaxPriceUSD || item.BasePriceUSD != item.MaxPriceUSD {
			return fmt.Errorf("RE async pricing for %q must use its finite, non-zero maximum price as the base price", item.ModelName)
		}
		if item.Publishable == item.ComingSoon {
			return fmt.Errorf("RE async pricing for %q must be either publishable or coming soon", item.ModelName)
		}
		if item.Publishable {
			publishableCount++
		} else {
			comingSoonCount++
		}
	}
	if publishableCount != pricing.Integrity.PublishableModelCount || comingSoonCount != pricing.Integrity.ComingSoonModelCount ||
		pricing.Integrity.PricedModelCount != len(pricing.Models) || pricing.Integrity.ExcludedChatCount != models.Integrity.ChatModelCount {
		return errors.New("RE async pricing integrity counts do not match its model entries")
	}
	return nil
}

func loadAsyncContractCatalog(path string, pricing asyncPricingCatalog) (asyncContractCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return asyncContractCatalog{}, fmt.Errorf("read RE async contract catalog: %w", err)
	}
	var loaded asyncContractCatalog
	if err := common.Unmarshal(data, &loaded); err != nil {
		return asyncContractCatalog{}, fmt.Errorf("decode RE async contract catalog: %w", err)
	}
	if err := validateAsyncContractCatalog(&loaded, pricing); err != nil {
		return asyncContractCatalog{}, err
	}
	return loaded, nil
}

func validateAsyncContractCatalog(contracts *asyncContractCatalog, pricing asyncPricingCatalog) error {
	if contracts == nil {
		return errors.New("RE async contract catalog is required")
	}
	if contracts.SchemaVersion <= 0 || strings.TrimSpace(contracts.GeneratedAt) == "" || len(contracts.Sources) == 0 || len(contracts.Integrity) == 0 {
		return errors.New("RE async contract catalog must declare schema version, generation time, sources, and integrity")
	}
	if len(contracts.Models) != asyncContractModelCount {
		return fmt.Errorf("RE async contract catalog must contain %d models, got %d", asyncContractModelCount, len(contracts.Models))
	}
	if len(contracts.Exclusions) != asyncContractExclusionCount {
		return fmt.Errorf("RE async contract catalog must contain %d exclusions, got %d", asyncContractExclusionCount, len(contracts.Exclusions))
	}
	if err := validateAsyncContractIntegrity(*contracts); err != nil {
		return err
	}

	publishReady := make(map[string]asyncPricingModel, asyncContractModelCount)
	expectedExclusions := make(map[string]struct{}, asyncContractExclusionCount)
	for _, item := range pricing.Models {
		if asyncModelPublishReady(item) {
			publishReady[item.ModelName] = item
			continue
		}
		expectedExclusions[item.ModelName] = struct{}{}
	}
	if len(publishReady) != asyncContractModelCount || len(expectedExclusions) != asyncContractExclusionCount {
		return fmt.Errorf("RE pricing publish-ready split must be %d contracts and %d exclusions, got %d and %d", asyncContractModelCount, asyncContractExclusionCount, len(publishReady), len(expectedExclusions))
	}

	excluded := make(map[string]struct{}, len(contracts.Exclusions))
	for _, item := range contracts.Exclusions {
		item.ModelName = strings.TrimSpace(item.ModelName)
		if _, exists := expectedExclusions[item.ModelName]; !exists {
			return fmt.Errorf("RE async contract catalog excludes unknown or publish-ready model %q", item.ModelName)
		}
		if strings.TrimSpace(item.Reason) == "" {
			return fmt.Errorf("RE async contract exclusion %q is missing a reason", item.ModelName)
		}
		if _, exists := excluded[item.ModelName]; exists {
			return fmt.Errorf("RE async contract catalog contains duplicate exclusion %q", item.ModelName)
		}
		excluded[item.ModelName] = struct{}{}
	}
	for modelName := range expectedExclusions {
		if _, exists := excluded[modelName]; !exists {
			return fmt.Errorf("RE async contract catalog is missing exclusion %q", modelName)
		}
	}

	seen := make(map[string]struct{}, len(contracts.Models))
	for index := range contracts.Models {
		item := &contracts.Models[index]
		price, exists := publishReady[item.ModelName]
		if !exists {
			return fmt.Errorf("RE async contract contains non-publish-ready or unknown model %q", item.ModelName)
		}
		if _, exists := seen[item.ModelName]; exists {
			return fmt.Errorf("RE async contract contains duplicate model %q", item.ModelName)
		}
		seen[item.ModelName] = struct{}{}
		if item.ModelName != "re/"+item.UpstreamModel || item.UpstreamModel != price.UpstreamModel || item.ModelType != price.ModelType {
			return fmt.Errorf("RE async contract identity does not match pricing for %q", item.ModelName)
		}
		definition, exists := reAsyncProfileDefinitionForType(item.ModelType)
		if !exists || item.Operation != definition.Operation || item.EndpointType != string(constant.EndpointTypeReTask) || item.ExecutionMode != "async" || item.ResponseContract != definition.ResponseContract {
			return fmt.Errorf("RE async contract routing metadata is invalid for %q", item.ModelName)
		}
		if item.SchemaMode != model.ModelOperationSchemaModeReplace || item.InputSchema == nil || item.UISchema == nil || item.MaterialSchema == nil {
			return fmt.Errorf("RE async contract %q must provide complete replace schemas", item.ModelName)
		}
		if additionalProperties, ok := item.InputSchema["additionalProperties"].(bool); !ok || additionalProperties {
			return fmt.Errorf("RE async contract %q must reject undeclared input parameters", item.ModelName)
		}
		if item.RequestContract.Adapter != "re-task" || item.DispatchPath != "/v1/re/generations" || item.PollPath != "/v1/re/tasks/{task_id}" {
			return fmt.Errorf("RE async contract dispatch metadata is invalid for %q", item.ModelName)
		}
		if item.Evidence == nil {
			return fmt.Errorf("RE async contract %q is missing evidence", item.ModelName)
		}
		if err := validateAsyncContractSelectionUI(*item); err != nil {
			return err
		}

		profile, version := reAsyncProfileContract(definition)
		overrides, err := marshalAsyncContractOverrides(*item)
		if err != nil {
			return fmt.Errorf("encode RE async contract %q: %w", item.ModelName, err)
		}
		normalized, err := model.NormalizeAndValidateModelOperationBindingOverrides(overrides, profile, version)
		if err != nil {
			return fmt.Errorf("validate RE async contract %q: %w", item.ModelName, err)
		}
		item.normalizedOverrides = normalized
	}
	for modelName := range publishReady {
		if _, exists := seen[modelName]; !exists {
			return fmt.Errorf("RE async contract catalog is missing publish-ready model %q", modelName)
		}
	}
	return nil
}

func validateAsyncContractIntegrity(contracts asyncContractCatalog) error {
	modelCount, ok := contractCatalogInteger(contracts.Integrity["model_count"])
	if !ok || modelCount != len(contracts.Models) {
		return errors.New("RE async contract integrity model_count does not match models")
	}
	exclusionCount, ok := contractCatalogInteger(contracts.Integrity["exclusion_count"])
	if !ok || exclusionCount != len(contracts.Exclusions) {
		return errors.New("RE async contract integrity exclusion_count does not match exclusions")
	}
	names := make([]string, 0, len(contracts.Models))
	for _, item := range contracts.Models {
		names = append(names, item.ModelName)
	}
	sort.Strings(names)
	expectedDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(names, "\n")+"\n")))
	digest, ok := contracts.Integrity["sorted_model_name_sha256"].(string)
	if !ok || digest != expectedDigest {
		return errors.New("RE async contract catalog integrity hash does not match models")
	}
	return nil
}

func contractCatalogInteger(value interface{}) (int, bool) {
	number, ok := value.(float64)
	if !ok || number < 0 || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

func validateAsyncContractSelectionUI(item asyncContractModel) error {
	properties, ok := item.InputSchema["properties"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("RE async contract %q input_schema.properties must be an object", item.ModelName)
	}
	if field := findAsyncContractMaterialURLField(item.InputSchema, ""); field != "" {
		return fmt.Errorf("RE async contract %q material URL field %s must not be exposed in input_schema", item.ModelName, field)
	}
	widgets, _ := item.UISchema["widgets"].(map[string]interface{})
	for field, rawSchema := range properties {
		fieldSchema, ok := rawSchema.(map[string]interface{})
		if !ok {
			continue
		}
		if asyncContractSelectorField(field) {
			enumValues, hasEnum := fieldSchema["enum"].([]interface{})
			if !hasEnum || len(enumValues) == 0 {
				return fmt.Errorf("RE async contract %q field %s must use finite enum choices", item.ModelName, field)
			}
			widget := asyncContractWidgetType(widgets[field])
			switch widget {
			case "select", "segmented", "menu", "hidden":
			default:
				return fmt.Errorf("RE async contract %q field %s must use a choice widget, got %q", item.ModelName, field, widget)
			}
		}
	}
	for materialType, rawRule := range item.MaterialSchema {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		requestField, _ := rule["request_field"].(string)
		if requestField != "" {
			if _, exists := properties[requestField]; exists {
				return fmt.Errorf("RE async contract %q material field %s for %s must not be exposed in input_schema", item.ModelName, requestField, materialType)
			}
		}
		requestFields, _ := rule["request_fields"].([]interface{})
		for index, rawRequestField := range requestFields {
			requestFieldRule, ok := rawRequestField.(map[string]interface{})
			if !ok {
				continue
			}
			nestedRequestField, _ := requestFieldRule["request_field"].(string)
			if _, exists := properties[nestedRequestField]; exists {
				return fmt.Errorf("RE async contract %q material field %s for %s slot %d must not be exposed in input_schema", item.ModelName, nestedRequestField, materialType, index)
			}
		}
	}
	return nil
}

func findAsyncContractMaterialURLField(schema map[string]interface{}, path string) string {
	properties, _ := schema["properties"].(map[string]interface{})
	for field, rawFieldSchema := range properties {
		fieldSchema, ok := rawFieldSchema.(map[string]interface{})
		if !ok {
			continue
		}
		fieldPath := field
		if path != "" {
			fieldPath = path + "." + field
		}
		if asyncContractMaterialURLField(fieldPath, fieldSchema) {
			return fieldPath
		}
		if nested := findAsyncContractMaterialURLField(fieldSchema, fieldPath); nested != "" {
			return nested
		}
		if items, ok := fieldSchema["items"].(map[string]interface{}); ok {
			if nested := findAsyncContractMaterialURLField(items, fieldPath+"[]"); nested != "" {
				return nested
			}
		}
	}
	for _, branchKey := range []string{"allOf", "anyOf", "oneOf"} {
		branches, _ := schema[branchKey].([]interface{})
		for _, rawBranch := range branches {
			branch, ok := rawBranch.(map[string]interface{})
			if !ok {
				continue
			}
			if nested := findAsyncContractMaterialURLField(branch, path); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func asyncContractMaterialURLField(field string, schema map[string]interface{}) bool {
	name := strings.ToLower(strings.TrimSpace(field))
	if (strings.HasSuffix(name, "url") || strings.HasSuffix(name, "urls")) &&
		(schema["type"] == "string" || schema["type"] == "array") {
		return true
	}
	if schema["format"] == "uri" {
		return true
	}
	items, _ := schema["items"].(map[string]interface{})
	if items["format"] == "uri" {
		return true
	}
	itemProperties, _ := items["properties"].(map[string]interface{})
	urlSchema, _ := itemProperties["url"].(map[string]interface{})
	return urlSchema["format"] == "uri"
}

func asyncContractSelectorField(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "n", "count", "quantity", "duration", "seconds", "aspect_ratio", "aspectratio", "ratio", "size", "resolution", "quality", "image_count", "video_count", "output_count", "num_images", "num_videos", "num_outputs", "batch_size":
		return true
	}
	return strings.HasSuffix(field, "_duration") || strings.HasSuffix(field, "_seconds") ||
		strings.HasSuffix(field, "_aspect_ratio") || strings.HasSuffix(field, "_size") ||
		strings.HasSuffix(field, "_resolution") || strings.Contains(field, "aspectratio") ||
		strings.HasSuffix(field, "_quality") || strings.HasSuffix(field, "_count") ||
		strings.HasPrefix(field, "num_") || strings.HasPrefix(field, "number_of_")
}

func asyncContractWidgetType(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	case map[string]interface{}:
		widget, _ := value["type"].(string)
		return widget
	default:
		return ""
	}
}

func reAsyncProfileDefinitionForType(modelType string) (reAsyncProfileDefinition, bool) {
	for _, definition := range reAsyncProfileDefinitions {
		if definition.ModelType == modelType {
			return definition, true
		}
	}
	return reAsyncProfileDefinition{}, false
}

func reAsyncProfileContract(definition reAsyncProfileDefinition) (*model.ModelOperationProfile, *model.ModelOperationProfileVersion) {
	profile := &model.ModelOperationProfile{
		ProfileKey:  definition.ProfileKey,
		DisplayName: definition.DisplayName,
		Description: "RE async model contracts; model-specific schemas are stored in binding replacements.",
	}
	version := &model.ModelOperationProfileVersion{
		Version:          1,
		Operation:        definition.Operation,
		EndpointType:     string(constant.EndpointTypeReTask),
		ExecutionMode:    "async",
		InputSchema:      `{"additionalProperties":false,"properties":{},"type":"object"}`,
		UISchema:         `{}`,
		MaterialSchema:   `{}`,
		ResponseContract: definition.ResponseContract,
		SmokeTest:        `{}`,
		Status:           model.ModelOperationProfileStatusPublished,
	}
	return profile, version
}

func marshalAsyncContractOverrides(item asyncContractModel) (string, error) {
	payload := map[string]interface{}{
		"schema_mode":      item.SchemaMode,
		"input_schema":     item.InputSchema,
		"ui_schema":        item.UISchema,
		"material_schema":  item.MaterialSchema,
		"request_contract": item.RequestContract,
		"dispatch_path":    item.DispatchPath,
		"poll_path":        item.PollPath,
	}
	if item.ParameterDefaults != nil {
		payload["parameter_defaults"] = item.ParameterDefaults
	}
	if len(item.Modes) > 0 {
		payload["modes"] = item.Modes
	}
	encoded, err := common.Marshal(payload)
	return string(encoded), err
}

func asyncModelPublishReady(item asyncPricingModel) bool {
	if !item.Publishable {
		return false
	}
	_, deferred := deferredBillingModels[item.ModelName]
	return !deferred
}

func configuredKeys() (string, string, error) {
	chatKey := strings.TrimSpace(os.Getenv("REAPI_CHAT_API_KEY"))
	taskKey := strings.TrimSpace(os.Getenv("REAPI_TASK_API_KEY"))
	if chatKey == "" && taskKey == "" {
		return "", "", errors.New("RE onboarding requires REAPI_CHAT_API_KEY or REAPI_TASK_API_KEY; no database changes were made")
	}
	return chatKey, taskKey, nil
}

func ensureVendors(items []catalogModel) (map[string]int, error) {
	vendorIDs := make(map[string]int)
	for _, item := range items {
		if _, exists := vendorIDs[item.Vendor]; exists {
			continue
		}
		var vendor model.Vendor
		err := model.DB.Where("name = ?", item.Vendor).First(&vendor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			vendor = model.Vendor{Name: item.Vendor, Status: 1}
			if err := vendor.Insert(); err != nil {
				return nil, fmt.Errorf("create vendor %q: %w", item.Vendor, err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("load vendor %q: %w", item.Vendor, err)
		}
		vendorIDs[item.Vendor] = vendor.Id
	}
	return vendorIDs, nil
}

func upsertModels(items []catalogModel, vendorIDs map[string]int, pricing map[string]asyncPricingModel, publishAsync bool) ([]string, []string, error) {
	chatModels := make([]string, 0, 8)
	taskModels := make([]string, 0, 96)
	for _, item := range items {
		endpoints, err := endpointsFor(item)
		if err != nil {
			return nil, nil, err
		}
		sourceMetadata, err := common.Marshal(item.SourceMetadata)
		if err != nil {
			return nil, nil, fmt.Errorf("encode source metadata for %q: %w", item.ModelName, err)
		}
		status := 0
		tags := "re,disabled_pending_pricing," + item.Protocol
		if item.Protocol == "re-task" {
			price := pricing[item.ModelName]
			if asyncModelPublishReady(price) {
				taskModels = append(taskModels, item.ModelName)
				if publishAsync {
					status = 1
					tags = "re,re-task"
				}
			} else if price.ComingSoon {
				tags = "re,coming_soon,re-task"
			} else {
				tags = "re,deferred_billing,re-task"
			}
		} else {
			chatModels = append(chatModels, item.ModelName)
			if publishAsync {
				tags = "re,disabled_not_requested,chat-completions"
			}
		}
		metadata := model.Model{
			ModelName:        item.ModelName,
			DisplayName:      item.DisplayName,
			ModelType:        item.ModelType,
			SourceURL:        item.SourceURL,
			SourceMetadata:   string(sourceMetadata),
			Tags:             tags,
			VendorID:         vendorIDs[item.Vendor],
			Endpoints:        endpoints,
			Status:           status,
			SyncOfficial:     0,
			ChannelProviders: []string{channelProvider},
			NameRule:         model.NameRuleExact,
		}
		var existing model.Model
		err = model.DB.Where("model_name = ?", item.ModelName).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := metadata.Insert(); err != nil {
				return nil, nil, fmt.Errorf("create model %q: %w", item.ModelName, err)
			}
		} else if err != nil {
			return nil, nil, fmt.Errorf("load model %q: %w", item.ModelName, err)
		} else {
			metadata.Id = existing.Id
			if !publishAsync {
				metadata.Status = existing.Status
				metadata.Tags = existing.Tags
			}
			if err := metadata.Update(); err != nil {
				return nil, nil, fmt.Errorf("update model %q: %w", item.ModelName, err)
			}
		}
	}
	sort.Strings(chatModels)
	sort.Strings(taskModels)
	return chatModels, taskModels, nil
}

func upsertREAsyncContracts(contracts asyncContractCatalog, taskModels []string) error {
	taskModelSet := make(map[string]struct{}, len(taskModels))
	for _, modelName := range taskModels {
		if _, exists := taskModelSet[modelName]; exists {
			return fmt.Errorf("RE async task model list contains duplicate %q", modelName)
		}
		taskModelSet[modelName] = struct{}{}
	}
	if len(taskModelSet) != len(contracts.Models) {
		return fmt.Errorf("RE async contract bindings require %d task models, got %d", len(contracts.Models), len(taskModelSet))
	}
	for _, item := range contracts.Models {
		if _, exists := taskModelSet[item.ModelName]; !exists {
			return fmt.Errorf("RE async contract binding is not an active task model: %q", item.ModelName)
		}
		if strings.TrimSpace(item.normalizedOverrides) == "" {
			return fmt.Errorf("RE async contract %q was not validated before database initialization", item.ModelName)
		}
	}
	if err := ensureREAsyncProfiles(); err != nil {
		return err
	}
	for _, item := range contracts.Models {
		definition, _ := reAsyncProfileDefinitionForType(item.ModelType)
		binding := model.ModelOperationBinding{
			ModelName:      item.ModelName,
			Operation:      definition.Operation,
			ProfileKey:     definition.ProfileKey,
			ProfileVersion: 1,
			Overrides:      item.normalizedOverrides,
			Enabled:        true,
		}
		if err := model.SaveModelOperationBinding(&binding); err != nil {
			return fmt.Errorf("upsert RE async contract binding %q: %w", item.ModelName, err)
		}
	}
	for _, item := range contracts.Models {
		if item.Operation != "text.generate" {
			continue
		}
		var legacy model.ModelOperationBinding
		err := model.DB.Where("model_name = ? AND operation = ?", item.ModelName, "text.chat").First(&legacy).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load legacy RE text binding %q: %w", item.ModelName, err)
		}
		if err := model.DeleteModelOperationBinding(item.ModelName, "text.chat", legacy.ContractHash); err != nil {
			return fmt.Errorf("delete legacy RE text binding %q: %w", item.ModelName, err)
		}
	}
	return nil
}

func ensureREAsyncProfiles() error {
	for _, definition := range reAsyncProfileDefinitions {
		expectedProfile, expectedVersion := reAsyncProfileContract(definition)
		storedProfile, storedVersion, err := model.GetModelOperationProfileVersion(definition.ProfileKey, expectedVersion.Version, false)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := model.SaveModelOperationProfileVersion(expectedProfile, expectedVersion); err != nil {
				return fmt.Errorf("create RE async profile %q: %w", definition.ProfileKey, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("load RE async profile %q: %w", definition.ProfileKey, err)
		}
		if storedProfile.ProfileKey != expectedProfile.ProfileKey ||
			storedVersion.Version != expectedVersion.Version ||
			storedVersion.Operation != expectedVersion.Operation ||
			storedVersion.EndpointType != expectedVersion.EndpointType ||
			storedVersion.ExecutionMode != expectedVersion.ExecutionMode ||
			storedVersion.InputSchema != expectedVersion.InputSchema ||
			storedVersion.UISchema != expectedVersion.UISchema ||
			storedVersion.MaterialSchema != expectedVersion.MaterialSchema ||
			storedVersion.ResponseContract != expectedVersion.ResponseContract ||
			storedVersion.SmokeTest != expectedVersion.SmokeTest ||
			storedVersion.Status != expectedVersion.Status {
			return fmt.Errorf("published RE async profile %q version %d differs from onboarding definition", definition.ProfileKey, expectedVersion.Version)
		}
	}
	return nil
}

func publishAsyncPrices(pricing map[string]asyncPricingModel) error {
	var encoded string
	initialPrices := ratio_setting.GetModelPriceCopy()
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var option model.Option
		found := true
		priceQuery := tx.Where(&model.Option{Key: "ModelPrice"})
		if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			priceQuery = priceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err := priceQuery.First(&option).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			found = false
			option = model.Option{Key: "ModelPrice"}
		} else if err != nil {
			return fmt.Errorf("load current ModelPrice: %w", err)
		}
		modelPrices := initialPrices
		if strings.TrimSpace(option.Value) != "" {
			modelPrices = make(map[string]float64)
			if err := common.Unmarshal([]byte(option.Value), &modelPrices); err != nil {
				return fmt.Errorf("decode current ModelPrice: %w", err)
			}
		}
		for modelName, item := range pricing {
			if asyncModelPublishReady(item) {
				modelPrices[modelName] = item.BasePriceUSD
			}
		}
		encodedBytes, err := common.Marshal(modelPrices)
		if err != nil {
			return fmt.Errorf("encode ModelPrice with RE async pricing: %w", err)
		}
		encoded = string(encodedBytes)
		if !found {
			option.Value = encoded
			return tx.Create(&option).Error
		}
		option.Value = encoded
		return tx.Save(&option).Error
	})
	if err != nil {
		return fmt.Errorf("persist RE async pricing: %w", err)
	}
	if err := ratio_setting.UpdateModelPriceByJSONString(encoded); err != nil {
		return fmt.Errorf("refresh in-memory RE async pricing: %w", err)
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap["ModelPrice"] = encoded
	common.OptionMapRWMutex.Unlock()
	for modelName, item := range pricing {
		if !asyncModelPublishReady(item) {
			continue
		}
		price, ok := ratio_setting.GetModelPrice(modelName, false)
		if !ok || price != item.BasePriceUSD {
			return fmt.Errorf("verify persisted RE async price for %q", modelName)
		}
	}
	return nil
}

func disableExistingREChannels() error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var channels []model.Channel
		if err := tx.Where("channel_provider = ?", channelProvider).Find(&channels).Error; err != nil {
			return fmt.Errorf("load existing RE channels before publishing: %w", err)
		}
		for _, channel := range channels {
			if err := tx.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
				return fmt.Errorf("disable channel %q before publishing: %w", channel.Name, err)
			}
			if err := tx.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", false).Error; err != nil {
				return fmt.Errorf("disable abilities for channel %q before publishing: %w", channel.Name, err)
			}
		}
		return nil
	})
}

func endpointsFor(item catalogModel) (string, error) {
	if item.Protocol == "chat-completions" {
		endpoints, err := common.Marshal([]string{string(constant.EndpointTypeOpenAI)})
		return string(endpoints), err
	}
	endpoints, err := common.Marshal(map[string]common.EndpointInfo{
		string(constant.EndpointTypeReTask): {Path: "/v1/re/generations", Method: "POST"},
	})
	return string(endpoints), err
}

func upsertChannel(name string, channelType int, baseURL string, key string, modelNames []string, status int, preserveExistingStatus bool) error {
	mapping := make(map[string]string, len(modelNames))
	for _, modelName := range modelNames {
		mapping[modelName] = strings.TrimPrefix(modelName, "re/")
	}
	mappingJSON, err := common.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("encode model mapping for %s: %w", name, err)
	}
	baseURLCopy := baseURL
	mappingCopy := string(mappingJSON)
	channel := model.Channel{
		Type:            channelType,
		Key:             key,
		Status:          status,
		Name:            name,
		ChannelProvider: channelProvider,
		BaseURL:         &baseURLCopy,
		Models:          strings.Join(modelNames, ","),
		Group:           reChannelGroups(channelType),
		ModelMapping:    &mappingCopy,
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var existing model.Channel
		err := tx.Where("name = ? AND channel_provider = ?", name, channelProvider).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&channel).Error; err != nil {
				return fmt.Errorf("create channel %q: %w", name, err)
			}
			if err := channel.AddAbilities(tx); err != nil {
				return fmt.Errorf("create abilities for channel %q: %w", name, err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("load channel %q: %w", name, err)
		}
		channel.Id = existing.Id
		if preserveExistingStatus {
			channel.Status = existing.Status
			channel.Models = existing.Models
			channel.ModelMapping = existing.ModelMapping
		}
		if err := tx.Model(&channel).Updates(&channel).Error; err != nil {
			return fmt.Errorf("update channel %q: %w", name, err)
		}
		if err := tx.First(&channel, "id = ?", channel.Id).Error; err != nil {
			return fmt.Errorf("reload channel %q: %w", name, err)
		}
		if err := channel.UpdateAbilities(tx); err != nil {
			return fmt.Errorf("update abilities for channel %q: %w", name, err)
		}
		return nil
	})
}

func reChannelGroups(channelType int) string {
	if channelType == constant.ChannelTypeReAPI {
		return "default,gold"
	}
	return "default"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "RE onboarding failed:", err)
	os.Exit(1)
}
