package controller

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const carLabPricingVersion = "3323acfc6c6c609ea47531e95cda90bc"

type tokenCatalogProfileBinding struct {
	Operation         string                                 `json:"operation"`
	ProfileKey        string                                 `json:"profile_key"`
	ProfileVersion    int                                    `json:"profile_version"`
	ContractVersion   int                                    `json:"contract_version"`
	ContractHash      string                                 `json:"contract_hash"`
	EndpointType      string                                 `json:"endpoint_type"`
	ExecutionMode     string                                 `json:"execution_mode"`
	ResponseContract  string                                 `json:"response_contract"`
	Overrides         map[string]interface{}                 `json:"overrides"`
	EffectiveContract *model.ModelOperationEffectiveContract `json:"effective_contract"`
	DispatchReady     bool                                   `json:"dispatch_ready"`
}

type tokenCatalogModel struct {
	ModelId                string                       `json:"model_id"`
	DisplayName            string                       `json:"display_name"`
	Description            string                       `json:"description,omitempty"`
	BrandIcon              string                       `json:"brand_icon,omitempty"`
	ModelType              string                       `json:"model_type"`
	ModelProvider          string                       `json:"model_provider"`
	ChannelProviders       []string                     `json:"channel_providers"`
	SupportedEndpointTypes []constant.EndpointType      `json:"supported_endpoint_types"`
	BillingMode            string                       `json:"billing_mode"`
	BasePrice              float64                      `json:"base_price,omitempty"`
	ModelRatio             float64                      `json:"model_ratio,omitempty"`
	CompletionRatio        float64                      `json:"completion_ratio,omitempty"`
	PricingVersion         string                       `json:"pricing_version"`
	ProfileBindings        []tokenCatalogProfileBinding `json:"profile_bindings"`
	ProfileReady           bool                         `json:"profile_ready"`
	RoutingGroups          []string                     `json:"routing_groups"`
	Routable               bool                         `json:"routable"`
	PriceReady             bool                         `json:"price_ready"`
}

type modelQuoteRequest struct {
	Model        string                 `json:"model"`
	Operation    string                 `json:"operation"`
	InputTokens  int                    `json:"input_tokens"`
	OutputTokens int                    `json:"output_tokens"`
	Parameters   map[string]interface{} `json:"parameters"`
	Usage        *dto.Usage             `json:"usage,omitempty"`
}

func normalizeModelQuoteUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.PromptTokens == 0 && usage.InputTokens > 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 && usage.OutputTokens > 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil && usage.PromptTokensDetails == (dto.InputTokenDetails{}) {
		usage.PromptTokensDetails = *usage.InputTokensDetails
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
}

func validateModelQuoteUsage(usage *dto.Usage) error {
	if usage == nil {
		return nil
	}
	counts := []struct {
		name  string
		value int
	}{
		{name: "prompt_tokens", value: usage.PromptTokens},
		{name: "completion_tokens", value: usage.CompletionTokens},
		{name: "cached_tokens", value: usage.PromptTokensDetails.CachedTokens},
		{name: "cached_creation_tokens", value: usage.PromptTokensDetails.CachedCreationTokens},
		{name: "text_tokens", value: usage.PromptTokensDetails.TextTokens},
		{name: "audio_tokens", value: usage.PromptTokensDetails.AudioTokens},
		{name: "image_tokens", value: usage.PromptTokensDetails.ImageTokens},
		{name: "claude_cache_creation_5_m_tokens", value: usage.ClaudeCacheCreation5mTokens},
		{name: "claude_cache_creation_1_h_tokens", value: usage.ClaudeCacheCreation1hTokens},
	}
	for _, count := range counts {
		if count.value < 0 || count.value > relayhelper.MaxTokensLimit {
			return fmt.Errorf("usage.%s must be between 0 and %d", count.name, relayhelper.MaxTokensLimit)
		}
	}
	return nil
}

func effectiveBillingMode(pricing model.Pricing) string {
	if strings.TrimSpace(pricing.BillingMode) != "" {
		return pricing.BillingMode
	}
	if pricing.QuotaType == 1 {
		return "fixed"
	}
	return "token"
}

func tokenScopedPricing(c *gin.Context) ([]model.Pricing, modelListGroups, error) {
	groups, err := getModelListGroups(c)
	if err != nil {
		return nil, groups, err
	}
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel {
		userSettings, _ := model.GetUserSetting(c.GetInt("id"), false)
		acceptUnsetRatioModel = userSettings.AcceptUnsetRatioModel
	}
	modelNames := filterTokenScopedModelNames(c, getRoutableModelNames(groups), acceptUnsetRatioModel)
	allowed := make(map[string]struct{}, len(modelNames))
	for _, modelName := range modelNames {
		allowed[modelName] = struct{}{}
	}
	pricing := make([]model.Pricing, 0, len(modelNames))
	for _, item := range model.GetPricing() {
		if _, ok := allowed[item.ModelName]; !ok {
			continue
		}
		pricing = append(pricing, item)
	}
	sort.Slice(pricing, func(i, j int) bool {
		return pricing[i].ModelName < pricing[j].ModelName
	})
	return pricing, groups, nil
}

func intersectRoutingGroups(enabledGroups []string, ownerGroups []string) []string {
	ownerSet := make(map[string]struct{}, len(ownerGroups))
	for _, group := range ownerGroups {
		ownerSet[group] = struct{}{}
	}
	groups := make([]string, 0)
	for _, group := range enabledGroups {
		if group == "all" {
			groups = append(groups, ownerGroups...)
			break
		}
		if _, ok := ownerSet[group]; ok {
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	return groups
}

func endpointSupported(supported []constant.EndpointType, endpoint string) bool {
	for _, item := range supported {
		if string(item) == endpoint {
			return true
		}
	}
	return false
}

func profileDispatchReady(operation, endpointType, executionMode, responseContract string) bool {
	switch operation {
	case "text.chat":
		return endpointType == string(constant.EndpointTypeOpenAI) && executionMode == "sync" && responseContract == "openai-chat-completion-v1"
	case "image.generate":
		return (endpointType == string(constant.EndpointTypeImageGeneration) && executionMode == "sync" && responseContract == "openai-image-generation-v1") ||
			(endpointType == string(constant.EndpointTypeOpenAI) && executionMode == "sync" && responseContract == "openai-chat-markdown-images-v1")
	case "video.generate":
		return endpointType == string(constant.EndpointTypeOpenAIVideo) && executionMode == "async" && responseContract == "openai-video-task-v1"
	case "audio.generate":
		return endpointType == string(constant.EndpointTypeOpenAI) && executionMode == "sync"
	case "embedding.create":
		return endpointType == string(constant.EndpointTypeEmbeddings) && executionMode == "sync"
	case "rerank.create":
		return endpointType == string(constant.EndpointTypeJinaRerank) && executionMode == "sync"
	default:
		return false
	}
}

func loadCatalogProfileBindings(bindingsByModel map[string][]model.ModelOperationBinding, supportedEndpointsByModel map[string][]constant.EndpointType) (map[string][]tokenCatalogProfileBinding, []modelOperationProfileContract) {
	result := make(map[string][]tokenCatalogProfileBinding, len(bindingsByModel))
	profileModels := make(map[string]*model.ModelOperationProfile)
	profileVersions := make(map[string]*model.ModelOperationProfileVersion)
	profileContracts := make(map[string]modelOperationProfileContract)
	for modelName, bindings := range bindingsByModel {
		for _, binding := range bindings {
			cacheKey := fmt.Sprintf("%s:%d", binding.ProfileKey, binding.ProfileVersion)
			profileVersion := profileVersions[cacheKey]
			if profileVersion == nil {
				profile, loadedVersion, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, true)
				if err != nil {
					continue
				}
				profileVersion = loadedVersion
				profileModels[cacheKey] = profile
				profileVersions[cacheKey] = loadedVersion
				profileContracts[cacheKey] = buildModelOperationProfileContract(profile, loadedVersion)
			}
			dispatchReady := profileDispatchReady(binding.Operation, profileVersion.EndpointType, profileVersion.ExecutionMode, profileVersion.ResponseContract) && endpointSupported(supportedEndpointsByModel[modelName], profileVersion.EndpointType)
			effectiveContract, err := model.BuildModelOperationEffectiveContract(binding, profileModels[cacheKey], profileVersion)
			if err != nil {
				continue
			}
			result[modelName] = append(result[modelName], tokenCatalogProfileBinding{
				Operation:         binding.Operation,
				ProfileKey:        binding.ProfileKey,
				ProfileVersion:    binding.ProfileVersion,
				ContractVersion:   binding.ContractVersion,
				ContractHash:      binding.ContractHash,
				EndpointType:      profileVersion.EndpointType,
				ExecutionMode:     profileVersion.ExecutionMode,
				ResponseContract:  profileVersion.ResponseContract,
				Overrides:         unmarshalContractObject(binding.Overrides),
				EffectiveContract: effectiveContract,
				DispatchReady:     dispatchReady,
			})
		}
	}
	profiles := make([]modelOperationProfileContract, 0, len(profileContracts))
	for _, profile := range profileContracts {
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].ProfileKey == profiles[j].ProfileKey {
			return profiles[i].Version < profiles[j].Version
		}
		return profiles[i].ProfileKey < profiles[j].ProfileKey
	})
	return result, profiles
}

func catalogBrandIcon(item model.Pricing) string {
	candidate := strings.ToLower(strings.TrimSpace(item.Icon + " " + item.ModelProvider + " " + item.ModelName))
	switch {
	case strings.Contains(candidate, "gemini") || strings.Contains(candidate, "google"):
		return "gemini"
	case strings.Contains(candidate, "openai") || strings.Contains(candidate, "gpt"):
		return "openai"
	case strings.Contains(candidate, "grok") || strings.Contains(candidate, "xai"):
		return "grok"
	case strings.Contains(candidate, "qwen") || strings.Contains(candidate, "alibaba") || strings.Contains(candidate, "阿里"):
		return "qwen"
	case strings.Contains(candidate, "wan"):
		return "wan"
	case strings.Contains(candidate, "doubao") || strings.Contains(candidate, "豆包"):
		return "doubao"
	case strings.Contains(candidate, "kling") || strings.Contains(candidate, "可灵"):
		return "kling"
	case strings.Contains(candidate, "flux"):
		return "flux"
	case strings.Contains(candidate, "kimi") || strings.Contains(candidate, "moonshot"):
		return "kimi"
	case strings.Contains(candidate, "zhipu") || strings.Contains(candidate, "chatglm") || strings.Contains(candidate, "智谱"):
		return "zhipu"
	case strings.Contains(candidate, "tencent") || strings.Contains(candidate, "hunyuan") || strings.Contains(candidate, "腾讯"):
		return "tencent"
	default:
		return ""
	}
}

func GetTokenModelCatalog(c *gin.Context) {
	pricing, groups, err := tokenScopedPricing(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	modelNames := make([]string, 0, len(pricing))
	for _, item := range pricing {
		modelNames = append(modelNames, item.ModelName)
	}
	bindingsByModel, err := model.GetModelOperationBindings(modelNames, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	supportedEndpointsByModel := make(map[string][]constant.EndpointType, len(pricing))
	for _, item := range pricing {
		supportedEndpointsByModel[item.ModelName] = item.SupportedEndpointTypes
	}
	profileBindings, profiles := loadCatalogProfileBindings(bindingsByModel, supportedEndpointsByModel)
	items := make([]tokenCatalogModel, 0, len(pricing))
	for _, item := range pricing {
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = item.ModelName
		}
		ready := false
		description := strings.TrimSpace(item.Description)
		brandIcon := catalogBrandIcon(item)
		for _, binding := range profileBindings[item.ModelName] {
			if binding.DispatchReady {
				ready = true
			}
			if binding.EffectiveContract != nil {
				if binding.EffectiveContract.Branding.Description != "" {
					description = binding.EffectiveContract.Branding.Description
				}
				if binding.EffectiveContract.Branding.IconKey != "" {
					brandIcon = binding.EffectiveContract.Branding.IconKey
				}
			}
		}
		items = append(items, tokenCatalogModel{
			ModelId:                item.ModelName,
			DisplayName:            displayName,
			Description:            description,
			BrandIcon:              brandIcon,
			ModelType:              item.ModelType,
			ModelProvider:          item.ModelProvider,
			ChannelProviders:       item.ChannelProviders,
			SupportedEndpointTypes: item.SupportedEndpointTypes,
			BillingMode:            effectiveBillingMode(item),
			BasePrice:              item.ModelPrice,
			ModelRatio:             item.ModelRatio,
			CompletionRatio:        item.CompletionRatio,
			PricingVersion:         carLabPricingVersion,
			ProfileBindings:        profileBindings[item.ModelName],
			ProfileReady:           ready,
			RoutingGroups:          intersectRoutingGroups(item.EnableGroup, groups.ownerGroups),
			Routable:               true,
			PriceReady:             relayhelper.HasModelBillingConfig(item.ModelName),
		})
	}
	common.ApiSuccess(c, gin.H{
		"items":                 items,
		"profiles":              profiles,
		"total":                 len(items),
		"token_group":           groups.tokenGroup,
		"routing_groups":        groups.ownerGroups,
		"pricing_version":       carLabPricingVersion,
		"supported_model_types": model.GetSupportedTokenModelTypes(),
	})
}

func findTokenScopedPricing(c *gin.Context, modelName string) (*model.Pricing, modelListGroups, error) {
	pricing, groups, err := tokenScopedPricing(c)
	if err != nil {
		return nil, groups, err
	}
	for index := range pricing {
		if pricing[index].ModelName == modelName {
			return &pricing[index], groups, nil
		}
	}
	return nil, groups, fmt.Errorf("model %s is not available to this token", modelName)
}

func findModelOperationBinding(modelName string, operation string) (*model.ModelOperationBinding, *model.ModelOperationProfile, *model.ModelOperationProfileVersion, *model.ModelOperationEffectiveContract, error) {
	binding, profile, profileVersion, contract, err := model.GetEnabledModelOperationContract(modelName, operation)
	if errors.Is(err, model.ErrModelOperationBindingNotFound) {
		return nil, nil, nil, nil, fmt.Errorf("operation %s is not enabled for model %s", operation, modelName)
	}
	return binding, profile, profileVersion, contract, err
}

func GetTokenModelProfile(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	operation := strings.TrimSpace(c.Query("operation"))
	if modelName == "" || operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	pricing, _, err := findTokenScopedPricing(c, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	binding, profile, profileVersion, effectiveContract, err := findModelOperationBinding(modelName, operation)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	bindingContract, err := buildModelOperationBindingContract(*binding, profile, profileVersion)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"model_id":          modelName,
		"profile":           buildModelOperationProfileContract(profile, profileVersion),
		"binding":           bindingContract,
		"effective_contract": effectiveContract,
		"dispatch_ready": profileDispatchReady(operation, profileVersion.EndpointType, profileVersion.ExecutionMode, profileVersion.ResponseContract) && endpointSupported(pricing.SupportedEndpointTypes, profileVersion.EndpointType),
	})
}

func resolveQuoteGroup(c *gin.Context, groups modelListGroups, modelName string, endpointType string) (string, error) {
	requestPath := "/v1/chat/completions"
	if endpoint, ok := model.GetSupportedEndpointMap()[endpointType]; ok && strings.TrimSpace(endpoint.Path) != "" {
		requestPath = endpoint.Path
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	channel, selectedGroup, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  usingGroup,
		ModelName:   modelName,
		RequestPath: requestPath,
		Retry:       common.GetPointer(0),
	})
	if err != nil {
		return "", err
	}
	if channel == nil {
		return "", fmt.Errorf("model %s has no route for operation in token groups %s", modelName, strings.Join(groups.ownerGroups, ","))
	}
	return selectedGroup, nil
}

func QuoteTokenModel(c *gin.Context) {
	var request modelQuoteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	request.Model = strings.TrimSpace(request.Model)
	request.Operation = strings.TrimSpace(request.Operation)
	if request.Model == "" || request.Operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	if request.InputTokens < 0 || request.OutputTokens < 0 || request.InputTokens > relayhelper.MaxTokensLimit || request.OutputTokens > relayhelper.MaxTokensLimit {
		common.ApiErrorMsg(c, fmt.Sprintf("input_tokens and output_tokens must be between 0 and %d", relayhelper.MaxTokensLimit))
		return
	}
	normalizeModelQuoteUsage(request.Usage)
	if err := validateModelQuoteUsage(request.Usage); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Usage != nil && request.Operation != "text.chat" {
		common.ApiErrorMsg(c, "usage settlement quote is only supported for text.chat")
		return
	}
	pricing, groups, err := findTokenScopedPricing(c, request.Model)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_, _, profileVersion, effectiveContract, err := findModelOperationBinding(request.Model, request.Operation)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !profileDispatchReady(request.Operation, profileVersion.EndpointType, profileVersion.ExecutionMode, profileVersion.ResponseContract) || !endpointSupported(pricing.SupportedEndpointTypes, profileVersion.EndpointType) {
		common.ApiErrorMsg(c, fmt.Sprintf("operation %s is not dispatch-ready for model %s", request.Operation, request.Model))
		return
	}
	selectedGroup, err := resolveQuoteGroup(c, groups, request.Model, profileVersion.EndpointType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quoteBody := make(map[string]interface{}, len(request.Parameters)+1)
	quoteBody["model"] = request.Model
	for key, value := range request.Parameters {
		quoteBody[key] = value
	}
	bodyBytes, err := common.Marshal(quoteBody)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	relayInfo := &relaycommon.RelayInfo{
		UserId:          c.GetInt("id"),
		UserGroup:       common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		UsingGroup:      selectedGroup,
		OriginModelName: request.Model,
		BillingRequestInput: &billingexpr.RequestInput{
			Body: bodyBytes,
		},
	}
	billingMode := effectiveBillingMode(*pricing)
	estimatedQuota := 0
	estimateKind := "base_fixed_price"
	if pricing.QuotaType == 1 && billingMode != "tiered_expr" {
		priceData, err := relayhelper.ModelPriceHelperPerCall(c, relayInfo)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		parameterRatios, err := model.CalculateModelOperationParameterRatios(effectiveContract, request.Parameters)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for key, ratio := range parameterRatios {
			priceData.AddOtherRatio(key, ratio)
		}
		estimatedQuota, relayInfo.QuotaClamp = common.QuotaFromFloatChecked(priceData.ApplyOtherRatiosToFloat(float64(priceData.Quota)))
		priceData.Quota = estimatedQuota
		relayInfo.PriceData = priceData
	} else {
		priceData, err := relayhelper.ModelPriceHelper(c, relayInfo, request.InputTokens, &types.TokenCountMeta{MaxTokens: request.OutputTokens})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		parameterRatios, err := model.CalculateModelOperationParameterRatios(effectiveContract, request.Parameters)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for key, ratio := range parameterRatios {
			priceData.AddOtherRatio(key, ratio)
		}
		if request.Usage != nil {
			relayInfo.PriceData = priceData
			estimatedQuota = service.CalculateTextQuotaForQuote(c, relayInfo, request.Usage)
			estimateKind = "settlement_quote"
		} else {
			estimatedQuota, relayInfo.QuotaClamp = common.QuotaFromFloatChecked(priceData.ApplyOtherRatiosToFloat(float64(priceData.QuotaToPreConsume)))
		}
		priceData.QuotaToPreConsume = estimatedQuota
		relayInfo.PriceData = priceData
		if request.Usage == nil {
			estimateKind = "preconsume_estimate"
		}
	}
	matchedTier := ""
	if relayInfo.TieredBillingSnapshot != nil {
		matchedTier = relayInfo.TieredBillingSnapshot.EstimatedTier
	}
	common.ApiSuccess(c, gin.H{
		"model_id":                      request.Model,
		"operation":                     request.Operation,
		"effective_group":               selectedGroup,
		"billing_mode":                  billingMode,
		"pricing_version":               carLabPricingVersion,
		"contract_version":              effectiveContract.ContractVersion,
		"contract_hash":                 effectiveContract.ContractHash,
		"base_price":                    pricing.ModelPrice,
		"model_ratio":                   pricing.ModelRatio,
		"completion_ratio":              pricing.CompletionRatio,
		"group_ratio":                   relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		"matched_tier":                  matchedTier,
		"parameter_multipliers":         relayInfo.PriceData.OtherRatios(),
		"parameter_adjustments_applied": len(relayInfo.PriceData.OtherRatios()) > 0,
		"estimated_quota":               estimatedQuota,
		"estimated_amount":              float64(estimatedQuota) / common.QuotaPerUnit,
		"estimate_kind":                 estimateKind,
		"assumptions": []string{
			"Estimate uses the gateway's current billing helper, effective group ratio, and versioned model-contract multipliers.",
			"Provider-side post-submit adjustments remain authoritative only for parameters not priced by the model contract.",
		},
	})
}
