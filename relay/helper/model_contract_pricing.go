package helper

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type PreparedModelOperationContractRequest struct {
	Contract            *model.ModelOperationEffectiveContract
	EffectiveParameters map[string]interface{}
	RequestParameters   map[string]interface{}
}

func ModelOperationParameters(request interface{}) (map[string]interface{}, error) {
	if request == nil {
		return map[string]interface{}{}, nil
	}
	payload, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	parameters := map[string]interface{}{}
	if err := common.Unmarshal(payload, &parameters); err != nil {
		return nil, err
	}
	return parameters, nil
}

func modelOperationMappedParameter(value interface{}, mappedName string) (interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if matched, ok := typed[mappedName]; ok {
			return matched, true
		}
		for _, nested := range typed {
			if matched, ok := modelOperationMappedParameter(nested, mappedName); ok {
				return matched, true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if matched, ok := modelOperationMappedParameter(nested, mappedName); ok {
				return matched, true
			}
		}
	}
	return nil, false
}

func modelOperationContractParameters(contract *model.ModelOperationEffectiveContract, parameters map[string]interface{}) map[string]interface{} {
	if contract == nil {
		return parameters
	}
	properties, ok := contract.InputSchema["properties"].(map[string]interface{})
	if !ok {
		return parameters
	}
	resolved := make(map[string]interface{}, len(properties))
	for field, rawSchema := range properties {
		value, exists := parameters[field]
		if !exists && field == "max_tokens" {
			// OpenAI reasoning models expose the same logical limit under
			// max_completion_tokens. Keep it in the contract's max_tokens slot
			// for validation and pricing without changing the outbound field.
			value, exists = parameters["max_completion_tokens"]
		}
		if !exists {
			mappedName := contract.RequestContract.FieldMap[field]
			if mappedName != "" {
				value, exists = modelOperationMappedParameter(parameters, mappedName)
			}
		}
		if !exists {
			continue
		}
		if schema, ok := rawSchema.(map[string]interface{}); ok {
			value = modelOperationCanonicalParameterValue(schema, contract.RequestContract.Coercions[field], value)
		}
		resolved[field] = value
	}
	if _, exists := resolved["prompt"]; !exists {
		if prompt := modelOperationLogicalPrompt(parameters); prompt != "" {
			if _, declared := properties["prompt"]; declared {
				resolved["prompt"] = prompt
			}
		}
	}
	return resolved
}

func modelOperationLogicalPrompt(parameters map[string]interface{}) string {
	if prompt, ok := parameters["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
		return prompt
	}
	texts := make([]string, 0)
	var collect func(interface{}, bool)
	collect = func(value interface{}, textField bool) {
		switch typed := value.(type) {
		case string:
			if textField && strings.TrimSpace(typed) != "" {
				texts = append(texts, typed)
			}
		case []interface{}:
			for _, item := range typed {
				collect(item, textField)
			}
		case map[string]interface{}:
			for key, item := range typed {
				collect(item, key == "content" || key == "text")
			}
		}
	}
	for _, field := range []string{"messages", "contents"} {
		if value, exists := parameters[field]; exists {
			collect(value, false)
		}
	}
	return strings.Join(texts, "\n")
}

func modelOperationCanonicalParameterValue(schema map[string]interface{}, coercion string, value interface{}) interface{} {
	if coercion != "string" {
		return value
	}
	text, ok := value.(string)
	if !ok {
		return value
	}
	switch schema["type"] {
	case "integer":
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(text, 64); err == nil {
			return parsed
		}
	case "boolean":
		if parsed, err := strconv.ParseBool(text); err == nil {
			return parsed
		}
	}
	return value
}

func effectiveModelOperationContractParameters(contract *model.ModelOperationEffectiveContract, parameters map[string]interface{}) (map[string]interface{}, error) {
	return model.NormalizeAndValidateModelOperationParameters(
		contract,
		modelOperationContractParameters(contract, parameters),
	)
}

func validateRawModelOperationContractParameters(contract *model.ModelOperationEffectiveContract, parameters map[string]interface{}) error {
	if contract == nil {
		return nil
	}
	additionalProperties, exists := contract.InputSchema["additionalProperties"].(bool)
	if !exists || additionalProperties {
		return nil
	}
	properties, _ := contract.InputSchema["properties"].(map[string]interface{})
	allowed := map[string]struct{}{
		"model": {},
		// Protocol envelopes are not model parameters. Their mapped leaves are
		// still projected through request_contract before validation.
		"messages": {}, "contents": {}, "requests": {},
		"generationConfig": {}, "generation_config": {},
		"safetySettings": {}, "safety_settings": {},
		"tools": {}, "toolConfig": {}, "tool_config": {},
		"systemInstruction": {}, "system_instruction": {},
		"cachedContent": {}, "cached_content": {},
		"metadata": {}, "group": {}, "mode": {},
	}
	for field := range properties {
		allowed[field] = struct{}{}
	}
	if _, declaresMaxTokens := properties["max_tokens"]; declaresMaxTokens {
		allowed["max_completion_tokens"] = struct{}{}
	}
	for _, mapped := range contract.RequestContract.FieldMap {
		mapped = strings.TrimSpace(mapped)
		if mapped != "" {
			allowed[mapped] = struct{}{}
		}
	}
	for _, materialType := range []string{"image", "video", "audio"} {
		rule, ok := contract.MaterialSchema[materialType].(map[string]interface{})
		if !ok {
			continue
		}
		requestField, _ := rule["request_field"].(string)
		requestField = strings.TrimSpace(requestField)
		if requestField == "" {
			requestField = materialType
		}
		allowed[requestField] = struct{}{}
	}
	for field := range parameters {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("input contains unknown parameter %s", field)
		}
	}
	return nil
}

func modelOperationOutboundParameterValue(contract *model.ModelOperationEffectiveContract, field string, value interface{}) interface{} {
	if contract == nil || contract.RequestContract.Coercions[field] != "string" {
		return value
	}
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		return fmt.Sprint(value)
	}
}

func setExistingModelOperationMappedParameter(value interface{}, mappedName string, replacement interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		if _, exists := typed[mappedName]; exists {
			typed[mappedName] = replacement
			return true
		}
		for _, nested := range typed {
			if setExistingModelOperationMappedParameter(nested, mappedName, replacement) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if setExistingModelOperationMappedParameter(nested, mappedName, replacement) {
				return true
			}
		}
	}
	return false
}

func modelOperationEnsureObject(parent map[string]interface{}, field string) map[string]interface{} {
	if existing, ok := parent[field].(map[string]interface{}); ok {
		return existing
	}
	created := map[string]interface{}{}
	parent[field] = created
	return created
}

func applyEffectiveModelOperationParameterMap(
	contract *model.ModelOperationEffectiveContract,
	parameters map[string]interface{},
	effective map[string]interface{},
) {
	for field, value := range effective {
		mappedName := strings.TrimSpace(contract.RequestContract.FieldMap[field])
		if mappedName == "" {
			mappedName = field
		}
		outboundValue := modelOperationOutboundParameterValue(contract, field, value)
		if field == "prompt" {
			if _, exists := parameters["prompt"]; !exists {
				// prompt may be the logical text extracted from messages/contents.
				continue
			}
		}
		if field == "max_tokens" {
			_, hasMaxCompletionTokens := parameters["max_completion_tokens"]
			_, hasMaxTokens := parameters["max_tokens"]
			if hasMaxCompletionTokens && !hasMaxTokens {
				parameters["max_completion_tokens"] = outboundValue
				continue
			}
		}
		if setExistingModelOperationMappedParameter(parameters, mappedName, outboundValue) {
			continue
		}
		if contract.RequestContract.Adapter == "gemini-image" && (field == "aspect_ratio" || field == "resolution") {
			generationConfig := modelOperationEnsureObject(parameters, "generationConfig")
			imageConfig := modelOperationEnsureObject(generationConfig, "imageConfig")
			imageConfig[mappedName] = outboundValue
			continue
		}
		parameters[mappedName] = outboundValue
	}
}

func modelOperationIntParameter(value interface{}) (int64, bool) {
	text := fmt.Sprint(value)
	if strings.ContainsAny(text, ".eE") {
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < math.MinInt64 || parsed > math.MaxInt64 {
			return 0, false
		}
		return int64(parsed), true
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	return parsed, err == nil
}

func applyEffectiveModelOperationTypedRequest(
	contract *model.ModelOperationEffectiveContract,
	request interface{},
	effective map[string]interface{},
) error {
	switch typed := request.(type) {
	case *dto.ImageRequest:
		if value, ok := effective["prompt"].(string); ok {
			typed.Prompt = value
		}
		if value, ok := effective["size"].(string); ok {
			typed.Size = value
		}
		if value, ok := effective["quality"].(string); ok {
			typed.Quality = value
		}
		if value, ok := effective["response_format"].(string); ok {
			typed.ResponseFormat = value
		}
		if value, exists := effective["n"]; exists {
			n, ok := modelOperationIntParameter(value)
			if !ok || n < 1 || n > dto.MaxImageN {
				return fmt.Errorf("effective parameter n must be between 1 and %d", dto.MaxImageN)
			}
			converted := uint(n)
			typed.N = &converted
		}
	case *dto.GeminiChatRequest:
		imageConfig := map[string]interface{}{}
		if len(typed.GenerationConfig.ImageConfig) > 0 {
			if err := common.Unmarshal(typed.GenerationConfig.ImageConfig, &imageConfig); err != nil {
				return fmt.Errorf("decode Gemini imageConfig: %w", err)
			}
		}
		for _, field := range []string{"aspect_ratio", "resolution"} {
			value, exists := effective[field]
			if !exists {
				continue
			}
			mappedName := strings.TrimSpace(contract.RequestContract.FieldMap[field])
			if mappedName == "" {
				mappedName = field
			}
			imageConfig[mappedName] = modelOperationOutboundParameterValue(contract, field, value)
		}
		if len(imageConfig) > 0 {
			encoded, err := common.Marshal(imageConfig)
			if err != nil {
				return err
			}
			typed.GenerationConfig.ImageConfig = encoded
		}
	case *dto.GeneralOpenAIRequest:
		if _, usesMessages := effective["prompt"]; !usesMessages || len(typed.Messages) == 0 {
			if value, exists := effective["prompt"]; exists {
				typed.Prompt = value
			}
		}
		if value, exists := effective["temperature"]; exists {
			if parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil {
				typed.Temperature = &parsed
			}
		}
		if value, exists := effective["top_p"]; exists {
			if parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil {
				typed.TopP = &parsed
			}
		}
		if value, exists := effective["max_tokens"]; exists {
			parsed, ok := modelOperationIntParameter(value)
			if !ok || parsed < 1 || parsed > MaxTokensLimit {
				return fmt.Errorf("effective parameter max_tokens must be between 1 and %d", MaxTokensLimit)
			}
			converted := uint(parsed)
			if typed.MaxCompletionTokens != nil && typed.MaxTokens == nil {
				typed.MaxCompletionTokens = &converted
			} else {
				typed.MaxTokens = &converted
			}
		}
	case *relaycommon.TaskSubmitReq:
		if value, ok := effective["prompt"].(string); ok {
			typed.Prompt = value
		}
		if value, ok := effective["size"].(string); ok {
			typed.Size = value
		}
		if value, exists := effective["seconds"]; exists {
			typed.Seconds = fmt.Sprint(modelOperationOutboundParameterValue(contract, "seconds", value))
			seconds, ok := modelOperationIntParameter(value)
			if !ok || seconds < 1 || seconds > relaycommon.MaxTaskDurationSeconds {
				return fmt.Errorf("effective parameter seconds must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
			}
			typed.Duration = int(seconds)
		}
		if value, exists := effective["duration"]; exists {
			duration, ok := modelOperationIntParameter(value)
			if !ok || duration < 1 || duration > relaycommon.MaxTaskDurationSeconds {
				return fmt.Errorf("effective parameter duration must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
			}
			typed.Duration = int(duration)
		}
		if typed.Metadata == nil {
			typed.Metadata = map[string]interface{}{}
		}
		for _, field := range []string{"aspect_ratio", "resolution", "generate_audio", "watermark"} {
			if value, exists := effective[field]; exists {
				typed.Metadata[field] = modelOperationOutboundParameterValue(contract, field, value)
			}
		}
	}
	return nil
}

func replaceModelOperationJSONBody(c *gin.Context, parameters map[string]interface{}) error {
	if c == nil || c.Request == nil || !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return nil
	}
	encoded, err := common.Marshal(parameters)
	if err != nil {
		return err
	}
	oldStorage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	newStorage, err := common.CreateBodyStorage(encoded)
	if err != nil {
		return err
	}
	c.Set(common.KeyBodyStorage, newStorage)
	c.Set(common.KeyRequestBody, encoded)
	c.Request.Body = io.NopCloser(newStorage)
	c.Request.ContentLength = int64(len(encoded))
	if err := oldStorage.Close(); err != nil {
		return err
	}
	return nil
}

func applyEffectiveModelOperationForm(
	c *gin.Context,
	contract *model.ModelOperationEffectiveContract,
	effective map[string]interface{},
) error {
	if c == nil || c.Request == nil || !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		return nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return err
	}
	for field, value := range effective {
		mappedName := strings.TrimSpace(contract.RequestContract.FieldMap[field])
		if mappedName == "" {
			mappedName = field
		}
		form.Value[mappedName] = []string{fmt.Sprint(modelOperationOutboundParameterValue(contract, field, value))}
	}
	c.Request.MultipartForm = form
	c.Request.PostForm = form.Value
	return nil
}

func prepareModelOperationContractRequestWithContract(
	c *gin.Context,
	contract *model.ModelOperationEffectiveContract,
	request interface{},
) (*PreparedModelOperationContractRequest, error) {
	parameters, err := ModelOperationRequestParameters(c, request)
	if err != nil {
		return nil, err
	}
	if err := validateRawModelOperationContractParameters(contract, parameters); err != nil {
		return nil, err
	}
	effective, err := effectiveModelOperationContractParameters(contract, parameters)
	if err != nil {
		return nil, err
	}
	if err := applyEffectiveModelOperationTypedRequest(contract, request, effective); err != nil {
		return nil, err
	}
	applyEffectiveModelOperationParameterMap(contract, parameters, effective)
	if err := applyEffectiveModelOperationForm(c, contract, effective); err != nil {
		return nil, err
	}
	if err := replaceModelOperationJSONBody(c, parameters); err != nil {
		return nil, err
	}
	if taskRequest, ok := request.(*relaycommon.TaskSubmitReq); ok && c != nil {
		c.Set("task_request", *taskRequest)
	}
	return &PreparedModelOperationContractRequest{
		Contract:            contract,
		EffectiveParameters: effective,
		RequestParameters:   parameters,
	}, nil
}

func PrepareModelOperationContractRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	operation string,
	request interface{},
) (*PreparedModelOperationContractRequest, error) {
	if info == nil {
		return nil, errors.New("relay info is required")
	}
	_, _, _, contract, err := model.GetEnabledModelOperationContract(info.OriginModelName, operation)
	if errors.Is(err, model.ErrModelOperationBindingNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return prepareModelOperationContractRequestWithContract(c, contract, request)
}

func ApplyPreparedModelOperationContractPricing(
	info *relaycommon.RelayInfo,
	prepared *PreparedModelOperationContractRequest,
) error {
	if info == nil {
		return errors.New("relay info is required")
	}
	if prepared == nil || prepared.Contract == nil {
		return nil
	}
	ratios, err := model.CalculateModelOperationParameterRatios(prepared.Contract, prepared.EffectiveParameters)
	if err != nil {
		return err
	}
	for key, ratio := range ratios {
		info.PriceData.AddOtherRatio(key, ratio)
	}
	return nil
}

func ApplyModelOperationContractPricing(info *relaycommon.RelayInfo, operation string, parameters map[string]interface{}) (*model.ModelOperationEffectiveContract, error) {
	if info == nil {
		return nil, errors.New("relay info is required")
	}
	_, _, _, contract, err := model.GetEnabledModelOperationContract(info.OriginModelName, operation)
	if errors.Is(err, model.ErrModelOperationBindingNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	effectiveParameters, err := effectiveModelOperationContractParameters(contract, parameters)
	if err != nil {
		return nil, err
	}
	ratios, err := model.CalculateModelOperationParameterRatios(contract, effectiveParameters)
	if err != nil {
		return nil, err
	}
	for key, ratio := range ratios {
		info.PriceData.AddOtherRatio(key, ratio)
	}
	return contract, nil
}

func ApplyModelOperationRatiosToPreConsume(info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("relay info is required")
	}
	if len(info.PriceData.OtherRatios()) == 0 {
		return nil
	}
	adjusted := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.QuotaToPreConsume))
	quota, clamp := common.QuotaFromFloatChecked(adjusted)
	if quota < 0 {
		return fmt.Errorf("model operation contract produced negative pre-consume quota")
	}
	info.PriceData.QuotaToPreConsume = quota
	if clamp != nil {
		info.QuotaClamp = clamp
	}
	return nil
}
