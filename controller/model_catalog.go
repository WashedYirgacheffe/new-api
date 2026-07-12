package controller

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const carLabPricingVersion = "a42d372ccf0b5dd13ecf71203521f9d2"

type tokenCatalogProfileBinding struct {
	Operation        string                 `json:"operation"`
	ProfileKey       string                 `json:"profile_key"`
	ProfileVersion   int                    `json:"profile_version"`
	EndpointType     string                 `json:"endpoint_type"`
	ExecutionMode    string                 `json:"execution_mode"`
	ResponseContract string                 `json:"response_contract"`
	Overrides        map[string]interface{} `json:"overrides"`
}

type tokenCatalogModel struct {
	ModelId                string                       `json:"model_id"`
	DisplayName            string                       `json:"display_name"`
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

func loadCatalogProfileBindings(bindingsByModel map[string][]model.ModelOperationBinding) map[string][]tokenCatalogProfileBinding {
	result := make(map[string][]tokenCatalogProfileBinding, len(bindingsByModel))
	profileVersions := make(map[string]*model.ModelOperationProfileVersion)
	for modelName, bindings := range bindingsByModel {
		for _, binding := range bindings {
			cacheKey := fmt.Sprintf("%s:%d", binding.ProfileKey, binding.ProfileVersion)
			profileVersion := profileVersions[cacheKey]
			if profileVersion == nil {
				_, loadedVersion, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, true)
				if err != nil {
					continue
				}
				profileVersion = loadedVersion
				profileVersions[cacheKey] = loadedVersion
			}
			result[modelName] = append(result[modelName], tokenCatalogProfileBinding{
				Operation:        binding.Operation,
				ProfileKey:       binding.ProfileKey,
				ProfileVersion:   binding.ProfileVersion,
				EndpointType:     profileVersion.EndpointType,
				ExecutionMode:    profileVersion.ExecutionMode,
				ResponseContract: profileVersion.ResponseContract,
				Overrides:        unmarshalContractObject(binding.Overrides),
			})
		}
	}
	return result
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
	profileBindings := loadCatalogProfileBindings(bindingsByModel)
	items := make([]tokenCatalogModel, 0, len(pricing))
	for _, item := range pricing {
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = item.ModelName
		}
		items = append(items, tokenCatalogModel{
			ModelId:                item.ModelName,
			DisplayName:            displayName,
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
			RoutingGroups:          intersectRoutingGroups(item.EnableGroup, groups.ownerGroups),
			Routable:               true,
			PriceReady:             relayhelper.HasModelBillingConfig(item.ModelName),
		})
	}
	common.ApiSuccess(c, gin.H{
		"items":                 items,
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

func findModelOperationBinding(modelName string, operation string) (*model.ModelOperationBinding, *model.ModelOperationProfile, *model.ModelOperationProfileVersion, error) {
	bindingsByModel, err := model.GetModelOperationBindings([]string{modelName}, true)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, binding := range bindingsByModel[modelName] {
		if binding.Operation != operation {
			continue
		}
		profile, profileVersion, err := model.GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, true)
		if err != nil {
			return nil, nil, nil, err
		}
		return &binding, profile, profileVersion, nil
	}
	return nil, nil, nil, fmt.Errorf("operation %s is not enabled for model %s", operation, modelName)
}

func GetTokenModelProfile(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	operation := strings.TrimSpace(c.Query("operation"))
	if modelName == "" || operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	if _, _, err := findTokenScopedPricing(c, modelName); err != nil {
		common.ApiError(c, err)
		return
	}
	binding, profile, profileVersion, err := findModelOperationBinding(modelName, operation)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"model_id": modelName,
		"profile":  buildModelOperationProfileContract(profile, profileVersion),
		"binding":  buildModelOperationBindingContract(*binding),
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
	pricing, groups, err := findTokenScopedPricing(c, request.Model)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_, _, profileVersion, err := findModelOperationBinding(request.Model, request.Operation)
	if err != nil {
		common.ApiError(c, err)
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
	parameterAdjustmentsApplied := false
	if pricing.QuotaType == 1 && billingMode != "tiered_expr" {
		priceData, err := relayhelper.ModelPriceHelperPerCall(c, relayInfo)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		relayInfo.PriceData = priceData
		estimatedQuota = priceData.Quota
	} else {
		priceData, err := relayhelper.ModelPriceHelper(c, relayInfo, request.InputTokens, &types.TokenCountMeta{MaxTokens: request.OutputTokens})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		relayInfo.PriceData = priceData
		estimatedQuota = priceData.QuotaToPreConsume
		estimateKind = "preconsume_estimate"
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
		"base_price":                    pricing.ModelPrice,
		"model_ratio":                   pricing.ModelRatio,
		"completion_ratio":              pricing.CompletionRatio,
		"group_ratio":                   relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		"matched_tier":                  matchedTier,
		"parameter_multipliers":         relayInfo.PriceData.OtherRatios(),
		"parameter_adjustments_applied": parameterAdjustmentsApplied,
		"estimated_quota":               estimatedQuota,
		"estimated_amount":              float64(estimatedQuota) / common.QuotaPerUnit,
		"estimate_kind":                 estimateKind,
		"assumptions": []string{
			"Estimate uses the gateway's current billing helper and effective group ratio.",
			"Provider-side or post-submit parameter adjustments are excluded until the operation uses a verified task adapter.",
		},
	})
}
