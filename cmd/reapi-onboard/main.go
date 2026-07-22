package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/reapi"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

const (
	defaultCatalogPath = "docs/catalog/reapi-model-catalog.json"
	channelProvider    = "re"
	chatChannelName    = "RE Chat"
	taskChannelName    = "RE Async"
	chatBaseURL        = "https://api.reapi.ai"
	taskBaseURL        = "https://reapi.ai/api/v1"
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

func main() {
	catalogPath := flag.String("catalog", defaultCatalogPath, "RE catalog JSON path")
	dryRun := flag.Bool("dry-run", false, "validate the catalog and required secrets without writing to the database")
	flag.Parse()

	catalog, err := loadCatalog(*catalogPath)
	if err != nil {
		fatal(err)
	}
	if *dryRun {
		fmt.Printf("RE catalog is valid: %d models (%d chat, %d async); no database changes made\n", len(catalog.Models), catalog.Integrity.ChatModelCount, catalog.Integrity.AsyncModelCount)
		return
	}
	chatKey, taskKey, err := requiredKeys()
	if err != nil {
		fatal(err)
	}

	common.InitEnv()
	ratio_setting.InitRatioSettings()
	if err := model.InitDB(); err != nil {
		fatal(fmt.Errorf("initialize database: %w", err))
	}
	defer func() { _ = model.CloseDB() }()

	vendorIDs, err := ensureVendors(catalog.Models)
	if err != nil {
		fatal(err)
	}
	chatModels, taskModels, err := upsertModels(catalog.Models, vendorIDs)
	if err != nil {
		fatal(err)
	}
	if err := upsertChannel(chatChannelName, constant.ChannelTypeOpenAI, chatBaseURL, chatKey, chatModels); err != nil {
		fatal(err)
	}
	if err := upsertChannel(taskChannelName, constant.ChannelTypeReAPI, taskBaseURL, taskKey, taskModels); err != nil {
		fatal(err)
	}
	model.RefreshPricing()

	fmt.Printf("RE onboarding complete: %d catalog models, %d chat mappings, %d async mappings; both channels are manually disabled\n", len(catalog.Models), len(chatModels), len(taskModels))
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
			if item.Endpoint != "/v1/chat/completions" {
				return fmt.Errorf("RE chat model %q has invalid endpoint %q", item.ModelName, item.Endpoint)
			}
		case "re-task":
			asyncCount++
			endpoint, supported := reapi.EndpointForModel(item.UpstreamModel)
			if !supported || endpoint != item.Endpoint {
				return fmt.Errorf("RE async model %q does not match the runtime adaptor", item.ModelName)
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

func requiredKeys() (string, string, error) {
	chatKey := strings.TrimSpace(os.Getenv("REAPI_CHAT_API_KEY"))
	taskKey := strings.TrimSpace(os.Getenv("REAPI_TASK_API_KEY"))
	if chatKey == "" || taskKey == "" {
		return "", "", errors.New("RE onboarding requires both REAPI_CHAT_API_KEY and REAPI_TASK_API_KEY; no database changes were made")
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

func upsertModels(items []catalogModel, vendorIDs map[string]int) ([]string, []string, error) {
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
		metadata := model.Model{
			ModelName:        item.ModelName,
			DisplayName:      item.DisplayName,
			ModelType:        item.ModelType,
			SourceURL:        item.SourceURL,
			SourceMetadata:   string(sourceMetadata),
			Tags:             "re,disabled_pending_pricing," + item.Protocol,
			VendorID:         vendorIDs[item.Vendor],
			Endpoints:        endpoints,
			Status:           0,
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
			if err := metadata.Update(); err != nil {
				return nil, nil, fmt.Errorf("update model %q: %w", item.ModelName, err)
			}
		}
		if item.Protocol == "chat-completions" {
			chatModels = append(chatModels, item.ModelName)
		} else {
			taskModels = append(taskModels, item.ModelName)
		}
	}
	sort.Strings(chatModels)
	sort.Strings(taskModels)
	return chatModels, taskModels, nil
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

func upsertChannel(name string, channelType int, baseURL string, key string, modelNames []string) error {
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
		Status:          common.ChannelStatusManuallyDisabled,
		Name:            name,
		ChannelProvider: channelProvider,
		BaseURL:         &baseURLCopy,
		Models:          strings.Join(modelNames, ","),
		Group:           "default",
		ModelMapping:    &mappingCopy,
	}
	var existing model.Channel
	err = model.DB.Where("name = ? AND channel_provider = ?", name, channelProvider).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := channel.Insert(); err != nil {
			return fmt.Errorf("create channel %q: %w", name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load channel %q: %w", name, err)
	}
	channel.Id = existing.Id
	if err := channel.Update(); err != nil {
		return fmt.Errorf("update channel %q: %w", name, err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "RE onboarding failed:", err)
	os.Exit(1)
}
