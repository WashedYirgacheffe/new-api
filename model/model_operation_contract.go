package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const ModelOperationPricingModeParameterMultipliers = "newapi-base-with-parameter-multipliers"

var ErrModelOperationBindingNotFound = errors.New("model operation binding not found")

type ModelOperationBranding struct {
	IconKey     string `json:"icon_key,omitempty"`
	Description string `json:"description,omitempty"`
}

type ModelOperationRequestContract struct {
	Adapter   string            `json:"adapter,omitempty"`
	FieldMap  map[string]string `json:"field_map,omitempty"`
	Coercions map[string]string `json:"coercions,omitempty"`
}

type ModelOperationPricingMultiplier struct {
	Field  string             `json:"field"`
	Values map[string]float64 `json:"values"`
}

type ModelOperationPricingRule struct {
	Mode          string                            `json:"mode,omitempty"`
	Multipliers   []ModelOperationPricingMultiplier `json:"multipliers,omitempty"`
	QuantityField string                            `json:"quantity_field,omitempty"`
}

type ModelOperationBindingOverrides struct {
	Branding           *ModelOperationBranding        `json:"branding,omitempty"`
	InputSchema        map[string]interface{}          `json:"input_schema,omitempty"`
	UISchema           map[string]interface{}          `json:"ui_schema,omitempty"`
	MaterialSchema     map[string]interface{}          `json:"material_schema,omitempty"`
	RequestContract    *ModelOperationRequestContract  `json:"request_contract,omitempty"`
	PricingRule        *ModelOperationPricingRule      `json:"pricing_rule,omitempty"`
	ParameterDefaults  map[string]interface{}          `json:"parameter_defaults,omitempty"`
	ParameterOverrides map[string]interface{}          `json:"parameter_overrides,omitempty"`
	DispatchPath       string                          `json:"dispatch_path,omitempty"`
	PollPath           string                          `json:"poll_path,omitempty"`
}

type ModelOperationEffectiveContract struct {
	ProfileKey         string                         `json:"profile_key"`
	ProfileVersion     int                            `json:"profile_version"`
	Operation          string                         `json:"operation"`
	EndpointType       string                         `json:"endpoint_type"`
	ExecutionMode      string                         `json:"execution_mode"`
	Branding           ModelOperationBranding         `json:"branding"`
	InputSchema        map[string]interface{}          `json:"input_schema"`
	UISchema           map[string]interface{}          `json:"ui_schema"`
	MaterialSchema     map[string]interface{}          `json:"material_schema"`
	RequestContract    ModelOperationRequestContract  `json:"request_contract"`
	PricingRule        ModelOperationPricingRule      `json:"pricing_rule"`
	ParameterDefaults  map[string]interface{}          `json:"parameter_defaults"`
	ParameterOverrides map[string]interface{}          `json:"parameter_overrides"`
	DispatchPath       string                         `json:"dispatch_path,omitempty"`
	PollPath           string                         `json:"poll_path,omitempty"`
	ResponseContract   string                         `json:"response_contract"`
	ContractVersion    int                            `json:"contract_version"`
	ContractHash       string                         `json:"contract_hash"`
}

func validateContractObjectKeys(field string, object map[string]interface{}, allowed ...string) error {
	allowedKeys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedKeys[key]; !ok {
			return fmt.Errorf("%s contains unsupported field %q", field, key)
		}
	}
	return nil
}

func contractObject(value interface{}) (map[string]interface{}, bool) {
	object, ok := value.(map[string]interface{})
	return object, ok && object != nil
}

func mergeModelOperationContractObject(base map[string]interface{}, overrides ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base))
	for key, value := range base {
		result[key] = value
	}
	for _, override := range overrides {
		for key, value := range override {
			baseObject, baseIsObject := contractObject(result[key])
			overrideObject, overrideIsObject := contractObject(value)
			if baseIsObject && overrideIsObject {
				result[key] = mergeModelOperationContractObject(baseObject, overrideObject)
				continue
			}
			result[key] = value
		}
	}
	return result
}

func unmarshalModelOperationContractObject(field string, value string) (map[string]interface{}, error) {
	object := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(value, &object); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", field, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return object, nil
}

func validateModelOperationInputSchema(schema map[string]interface{}) error {
	if schemaType, ok := schema["type"].(string); ok && schemaType != "object" {
		return errors.New("input_schema.type must be object")
	}
	properties, ok := contractObject(schema["properties"])
	if !ok {
		return errors.New("input_schema.properties must be an object")
	}
	for field, rawSchema := range properties {
		fieldSchema, ok := contractObject(rawSchema)
		if !ok {
			return fmt.Errorf("input_schema property %s must be an object", field)
		}
		if fieldType, ok := fieldSchema["type"].(string); ok {
			switch fieldType {
			case "string", "number", "integer", "boolean", "array", "object":
			default:
				return fmt.Errorf("input_schema property %s has unsupported type %s", field, fieldType)
			}
		}
		if minimum, minimumOK := numericContractValue(fieldSchema["minimum"]); minimumOK {
			if maximum, maximumOK := numericContractValue(fieldSchema["maximum"]); maximumOK && maximum < minimum {
				return fmt.Errorf("input_schema property %s maximum is below minimum", field)
			}
		}
		if enumValues, ok := fieldSchema["enum"].([]interface{}); ok && len(enumValues) == 0 {
			return fmt.Errorf("input_schema property %s enum must not be empty", field)
		}
	}
	if required, ok := schema["required"].([]interface{}); ok {
		for _, value := range required {
			field, ok := value.(string)
			if !ok || strings.TrimSpace(field) == "" {
				return errors.New("input_schema.required must contain field names")
			}
			if _, exists := properties[field]; !exists {
				return fmt.Errorf("input_schema.required references unknown field %s", field)
			}
		}
	}
	return nil
}

func validateModelOperationUISchema(schema map[string]interface{}, inputSchema map[string]interface{}) error {
	properties, _ := contractObject(inputSchema["properties"])
	if placements, ok := contractObject(schema["placements"]); ok {
		for field, rawPlacement := range placements {
			if _, exists := properties[field]; !exists {
				return fmt.Errorf("ui_schema placement references unknown field %s", field)
			}
			placement, ok := rawPlacement.(string)
			if !ok {
				return fmt.Errorf("ui_schema placement for %s must be a string", field)
			}
			switch placement {
			case "prompt", "footer", "batch", "advanced", "material", "hidden":
			default:
				return fmt.Errorf("ui_schema placement %s is not registered", placement)
			}
		}
	}
	if widgets, ok := contractObject(schema["widgets"]); ok {
		for field, rawWidget := range widgets {
			if _, exists := properties[field]; !exists {
				return fmt.Errorf("ui_schema widget references unknown field %s", field)
			}
			widget := ""
			switch value := rawWidget.(type) {
			case string:
				widget = value
			case map[string]interface{}:
				widget, _ = value["type"].(string)
			}
			switch widget {
			case "text", "textarea", "stepper", "select", "string-list", "menu", "segmented", "toggle", "slider", "hidden", "material":
			default:
				return fmt.Errorf("ui_schema widget %s for %s is not registered", widget, field)
			}
		}
	}
	return nil
}

func validateModelOperationMaterialSchema(schema map[string]interface{}, requireRequestMapping bool) error {
	if err := validateContractObjectKeys("material_schema", schema, "image", "video", "audio"); err != nil {
		return err
	}
	for materialType, rawRule := range schema {
		rule, ok := contractObject(rawRule)
		if !ok {
			return fmt.Errorf("material_schema.%s must be an object", materialType)
		}
		if err := validateContractObjectKeys("material_schema."+materialType, rule,
			"min_items", "max_items", "roles", "mime_types", "max_size_mb", "max_total_duration", "request_field", "transport"); err != nil {
			return err
		}
		minItems, hasMin := numericContractValue(rule["min_items"])
		maxItems, hasMax := numericContractValue(rule["max_items"])
		if hasMin && (minItems < 0 || math.Trunc(minItems) != minItems || minItems > 20) {
			return fmt.Errorf("material_schema.%s.min_items must be an integer between 0 and 20", materialType)
		}
		if hasMax && (maxItems < 0 || math.Trunc(maxItems) != maxItems || maxItems > 20) {
			return fmt.Errorf("material_schema.%s.max_items must be an integer between 0 and 20", materialType)
		}
		if hasMin && hasMax && minItems > maxItems {
			return fmt.Errorf("material_schema.%s.min_items exceeds max_items", materialType)
		}
		requestField, _ := rule["request_field"].(string)
		requestField = strings.TrimSpace(requestField)
		if requestField != "" {
			if _, err := normalizeContractIdentifier(requestField, 128); err != nil {
				return fmt.Errorf("material_schema.%s.request_field: %w", materialType, err)
			}
		}
		transport, _ := rule["transport"].(string)
		transport = strings.TrimSpace(transport)
		if transport != "" && transport != "url" {
			return fmt.Errorf("material_schema.%s.transport %q is not registered", materialType, transport)
		}
		if requireRequestMapping && hasMax && maxItems > 0 && (requestField == "" || transport == "") {
			return fmt.Errorf("material_schema.%s requires request_field and transport when materials are enabled", materialType)
		}
	}
	return nil
}

func normalizeModelOperationRelayPath(field string, value string, requireTaskPlaceholder bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "/v1/") || strings.ContainsAny(value, "?#") || strings.Contains(value, "..") || len(value) > 255 {
		return "", fmt.Errorf("%s must be a /v1/ path without query or traversal segments", field)
	}
	if requireTaskPlaceholder && !strings.Contains(value, "{task_id}") {
		return "", fmt.Errorf("%s must contain {task_id}", field)
	}
	return value, nil
}

func canonicalContractValue(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "", false
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

func numericContractValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func validateModelOperationPricingRule(rule *ModelOperationPricingRule, inputSchema map[string]interface{}) error {
	if rule == nil {
		return nil
	}
	rule.Mode = strings.TrimSpace(rule.Mode)
	if rule.Mode != ModelOperationPricingModeParameterMultipliers {
		return fmt.Errorf("unsupported pricing_rule mode %q", rule.Mode)
	}
	properties, _ := contractObject(inputSchema["properties"])
	seenFields := make(map[string]struct{}, len(rule.Multipliers))
	for index := range rule.Multipliers {
		multiplier := &rule.Multipliers[index]
		multiplier.Field = strings.TrimSpace(multiplier.Field)
		fieldSchema, ok := contractObject(properties[multiplier.Field])
		if !ok {
			return fmt.Errorf("pricing_rule references unknown field %s", multiplier.Field)
		}
		if _, exists := seenFields[multiplier.Field]; exists {
			return fmt.Errorf("pricing_rule contains duplicate field %s", multiplier.Field)
		}
		seenFields[multiplier.Field] = struct{}{}
		enumValues, ok := fieldSchema["enum"].([]interface{})
		if !ok || len(enumValues) == 0 {
			return fmt.Errorf("pricing_rule field %s must define input_schema enum values", multiplier.Field)
		}
		if len(multiplier.Values) != len(enumValues) {
			return fmt.Errorf("pricing_rule field %s must price every enum value", multiplier.Field)
		}
		for _, enumValue := range enumValues {
			key, ok := canonicalContractValue(enumValue)
			if !ok {
				return fmt.Errorf("pricing_rule field %s contains unsupported enum value", multiplier.Field)
			}
			ratio, exists := multiplier.Values[key]
			if !exists {
				return fmt.Errorf("pricing_rule field %s is missing value %s", multiplier.Field, key)
			}
			if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio > 1000 {
				return fmt.Errorf("pricing_rule field %s has invalid multiplier for %s", multiplier.Field, key)
			}
		}
	}
	rule.QuantityField = strings.TrimSpace(rule.QuantityField)
	if rule.QuantityField != "" {
		if rule.QuantityField != "n" {
			return errors.New("pricing_rule quantity_field currently supports only n")
		}
		fieldSchema, ok := contractObject(properties[rule.QuantityField])
		if !ok || fieldSchema["type"] != "integer" {
			return errors.New("pricing_rule quantity_field n must be an integer input field")
		}
		if maximum, ok := numericContractValue(fieldSchema["maximum"]); ok && maximum > dto.MaxImageN {
			return fmt.Errorf("pricing_rule quantity_field n maximum must not exceed %d", dto.MaxImageN)
		}
	}
	return nil
}

func normalizeModelOperationBindingOverrides(value string, profile *ModelOperationProfile, version *ModelOperationProfileVersion) (string, ModelOperationBindingOverrides, error) {
	normalizedObject, err := unmarshalModelOperationContractObject("overrides", strings.TrimSpace(value))
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	if err := validateContractObjectKeys("overrides", normalizedObject,
		"branding", "input_schema", "ui_schema", "material_schema", "request_contract", "pricing_rule",
		"parameter_defaults", "parameter_overrides", "dispatch_path", "poll_path"); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	for field, allowed := range map[string][]string{
		"branding":        {"icon_key", "description"},
		"request_contract": {"adapter", "field_map", "coercions"},
		"pricing_rule":     {"mode", "multipliers", "quantity_field"},
	} {
		if object, ok := contractObject(normalizedObject[field]); ok {
			if err := validateContractObjectKeys(field, object, allowed...); err != nil {
				return "", ModelOperationBindingOverrides{}, err
			}
		}
	}
	if pricingObject, ok := contractObject(normalizedObject["pricing_rule"]); ok {
		if multipliers, ok := pricingObject["multipliers"].([]interface{}); ok {
			for _, rawMultiplier := range multipliers {
				multiplier, ok := contractObject(rawMultiplier)
				if !ok {
					return "", ModelOperationBindingOverrides{}, errors.New("pricing_rule multipliers must be objects")
				}
				if err := validateContractObjectKeys("pricing_rule multiplier", multiplier, "field", "values"); err != nil {
					return "", ModelOperationBindingOverrides{}, err
				}
			}
		}
	}
	raw, err := common.Marshal(normalizedObject)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	var overrides ModelOperationBindingOverrides
	if err := common.Unmarshal(raw, &overrides); err != nil {
		return "", ModelOperationBindingOverrides{}, fmt.Errorf("invalid overrides contract: %w", err)
	}
	if overrides.Branding != nil {
		overrides.Branding.Description = strings.TrimSpace(overrides.Branding.Description)
		if len(overrides.Branding.Description) > 500 {
			return "", ModelOperationBindingOverrides{}, errors.New("branding.description must be 500 characters or fewer")
		}
		if overrides.Branding.IconKey != "" {
			iconKey, err := normalizeContractIdentifier(overrides.Branding.IconKey, 64)
			if err != nil {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("branding.icon_key: %w", err)
			}
			overrides.Branding.IconKey = iconKey
		}
	}
	if overrides.RequestContract != nil {
		overrides.RequestContract.Adapter = strings.TrimSpace(overrides.RequestContract.Adapter)
		switch overrides.RequestContract.Adapter {
		case "openai-chat":
			if version.EndpointType != "openai" {
				return "", ModelOperationBindingOverrides{}, errors.New("openai-chat request adapter requires openai endpoint_type")
			}
		case "openai-image":
			if version.EndpointType != "image-generation" {
				return "", ModelOperationBindingOverrides{}, errors.New("openai-image request adapter requires image-generation endpoint_type")
			}
		case "openai-video":
			if version.EndpointType != "openai-video" {
				return "", ModelOperationBindingOverrides{}, errors.New("openai-video request adapter requires openai-video endpoint_type")
			}
		default:
			return "", ModelOperationBindingOverrides{}, fmt.Errorf("request_contract adapter %q is not registered", overrides.RequestContract.Adapter)
		}
	}
	overrides.DispatchPath, err = normalizeModelOperationRelayPath("dispatch_path", overrides.DispatchPath, false)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	overrides.PollPath, err = normalizeModelOperationRelayPath("poll_path", overrides.PollPath, true)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	if overrides.PollPath != "" && version.ExecutionMode != "async" {
		return "", ModelOperationBindingOverrides{}, errors.New("poll_path is only valid for async profiles")
	}
	inputSchema, err := unmarshalModelOperationContractObject("input_schema", version.InputSchema)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	uiSchema, err := unmarshalModelOperationContractObject("ui_schema", version.UISchema)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	materialSchema, err := unmarshalModelOperationContractObject("material_schema", version.MaterialSchema)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	inputSchema = mergeModelOperationContractObject(inputSchema, overrides.InputSchema)
	uiSchema = mergeModelOperationContractObject(uiSchema, overrides.UISchema)
	materialSchema = mergeModelOperationContractObject(materialSchema, overrides.MaterialSchema)
	if err := validateModelOperationInputSchema(inputSchema); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	if err := validateModelOperationUISchema(uiSchema, inputSchema); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	if err := validateModelOperationMaterialSchema(materialSchema, overrides.RequestContract != nil); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	if err := validateModelOperationPricingRule(overrides.PricingRule, inputSchema); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	properties, _ := contractObject(inputSchema["properties"])
	if overrides.RequestContract != nil {
		for field, target := range overrides.RequestContract.FieldMap {
			if _, ok := properties[field]; !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("request_contract field_map references unknown field %s", field)
			}
			if _, err := normalizeContractIdentifier(strings.TrimSpace(target), 128); err != nil {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("request_contract field_map target for %s: %w", field, err)
			}
		}
		for field, coercion := range overrides.RequestContract.Coercions {
			if _, ok := properties[field]; !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("request_contract coercion references unknown field %s", field)
			}
			switch coercion {
			case "string", "integer", "number", "boolean", "json":
			default:
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("request_contract coercion %s for %s is not registered", coercion, field)
			}
		}
	}
	for _, defaults := range []map[string]interface{}{overrides.ParameterDefaults, overrides.ParameterOverrides} {
		for field := range defaults {
			if _, ok := properties[field]; !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("parameter configuration references unknown field %s", field)
			}
		}
	}
	normalized, err := common.Marshal(overrides)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, fmt.Errorf("normalize overrides: %w", err)
	}
	return string(normalized), overrides, nil
}

func BuildModelOperationEffectiveContract(binding ModelOperationBinding, profile *ModelOperationProfile, version *ModelOperationProfileVersion) (*ModelOperationEffectiveContract, error) {
	if profile == nil || version == nil {
		return nil, errors.New("profile and version are required")
	}
	var overrides ModelOperationBindingOverrides
	if strings.TrimSpace(binding.Overrides) != "" {
		if err := common.UnmarshalJsonStr(binding.Overrides, &overrides); err != nil {
			return nil, fmt.Errorf("decode binding overrides: %w", err)
		}
	}
	inputSchema, err := unmarshalModelOperationContractObject("input_schema", version.InputSchema)
	if err != nil {
		return nil, err
	}
	uiSchema, err := unmarshalModelOperationContractObject("ui_schema", version.UISchema)
	if err != nil {
		return nil, err
	}
	materialSchema, err := unmarshalModelOperationContractObject("material_schema", version.MaterialSchema)
	if err != nil {
		return nil, err
	}
	contract := &ModelOperationEffectiveContract{
		ProfileKey:         profile.ProfileKey,
		ProfileVersion:     version.Version,
		Operation:          version.Operation,
		EndpointType:       version.EndpointType,
		ExecutionMode:      version.ExecutionMode,
		InputSchema:        mergeModelOperationContractObject(inputSchema, overrides.InputSchema),
		UISchema:           mergeModelOperationContractObject(uiSchema, overrides.UISchema),
		MaterialSchema:     mergeModelOperationContractObject(materialSchema, overrides.MaterialSchema),
		ParameterDefaults:  overrides.ParameterDefaults,
		ParameterOverrides: overrides.ParameterOverrides,
		DispatchPath:       overrides.DispatchPath,
		PollPath:           overrides.PollPath,
		ResponseContract:   version.ResponseContract,
		ContractVersion:    binding.ContractVersion,
		ContractHash:       binding.ContractHash,
	}
	if contract.ParameterDefaults == nil {
		contract.ParameterDefaults = map[string]interface{}{}
	}
	if contract.ParameterOverrides == nil {
		contract.ParameterOverrides = map[string]interface{}{}
	}
	if overrides.Branding != nil {
		contract.Branding = *overrides.Branding
	}
	if overrides.RequestContract != nil {
		contract.RequestContract = *overrides.RequestContract
	}
	if overrides.PricingRule != nil {
		contract.PricingRule = *overrides.PricingRule
	}
	return contract, nil
}

func computeModelOperationContractHash(binding ModelOperationBinding, profile *ModelOperationProfile, version *ModelOperationProfileVersion) (string, error) {
	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	if err != nil {
		return "", err
	}
	contract.ContractVersion = 0
	contract.ContractHash = ""
	payload, err := common.Marshal(contract)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(common.Sha256Raw(payload)), nil
}

func CalculateModelOperationParameterRatios(contract *ModelOperationEffectiveContract, parameters map[string]interface{}) (map[string]float64, error) {
	if contract == nil || contract.PricingRule.Mode == "" {
		return nil, nil
	}
	if contract.PricingRule.Mode != ModelOperationPricingModeParameterMultipliers {
		return nil, fmt.Errorf("unsupported pricing mode %s", contract.PricingRule.Mode)
	}
	properties, _ := contractObject(contract.InputSchema["properties"])
	ratios := make(map[string]float64, len(contract.PricingRule.Multipliers)+1)
	for _, multiplier := range contract.PricingRule.Multipliers {
		value, exists := parameters[multiplier.Field]
		if !exists {
			if fieldSchema, ok := contractObject(properties[multiplier.Field]); ok {
				value, exists = fieldSchema["default"]
			}
		}
		if !exists {
			continue
		}
		key, ok := canonicalContractValue(value)
		if !ok {
			return nil, fmt.Errorf("pricing field %s has unsupported value", multiplier.Field)
		}
		ratio, ok := multiplier.Values[key]
		if !ok {
			return nil, fmt.Errorf("pricing field %s value %s is not configured", multiplier.Field, key)
		}
		ratios[multiplier.Field] = ratio
	}
	if quantityField := contract.PricingRule.QuantityField; quantityField != "" {
		value, exists := parameters[quantityField]
		if !exists {
			if fieldSchema, ok := contractObject(properties[quantityField]); ok {
				value, exists = fieldSchema["default"]
			}
		}
		if exists {
			quantity, ok := numericContractValue(value)
			if !ok || quantity < 1 || quantity > dto.MaxImageN || math.Trunc(quantity) != quantity {
				return nil, fmt.Errorf("pricing quantity %s must be an integer between 1 and %d", quantityField, dto.MaxImageN)
			}
			ratios[quantityField] = quantity
		}
	}
	if len(ratios) == 0 {
		return nil, nil
	}
	return ratios, nil
}

func GetEnabledModelOperationContract(modelName string, operation string) (*ModelOperationBinding, *ModelOperationProfile, *ModelOperationProfileVersion, *ModelOperationEffectiveContract, error) {
	modelName = strings.TrimSpace(modelName)
	operation = strings.TrimSpace(operation)
	query := DB.Where("model_name = ? AND enabled = ?", modelName, true)
	if operation != "" {
		var exact ModelOperationBinding
		err := query.Where("operation = ?", operation).First(&exact).Error
		if err == nil {
			profile, version, err := GetModelOperationProfileVersion(exact.ProfileKey, exact.ProfileVersion, true)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			contract, err := BuildModelOperationEffectiveContract(exact, profile, version)
			return &exact, profile, version, contract, err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, nil, err
		}
	}
	var bindings []ModelOperationBinding
	if err := DB.Where("model_name = ? AND enabled = ?", modelName, true).Order("operation ASC").Find(&bindings).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	if len(bindings) != 1 {
		return nil, nil, nil, nil, ErrModelOperationBindingNotFound
	}
	binding := bindings[0]
	profile, version, err := GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	return &binding, profile, version, contract, err
}

func RefreshModelOperationBindingContracts() error {
	var bindings []ModelOperationBinding
	if err := DB.Find(&bindings).Error; err != nil {
		return err
	}
	profiles := make(map[string]*ModelOperationProfile)
	versions := make(map[string]*ModelOperationProfileVersion)
	for index := range bindings {
		binding := &bindings[index]
		storedOverrides := binding.Overrides
		cacheKey := fmt.Sprintf("%s:%d", binding.ProfileKey, binding.ProfileVersion)
		profile := profiles[cacheKey]
		version := versions[cacheKey]
		if profile == nil || version == nil {
			loadedProfile, loadedVersion, err := GetModelOperationProfileVersion(binding.ProfileKey, binding.ProfileVersion, false)
			if err != nil {
				return err
			}
			profile = loadedProfile
			version = loadedVersion
			profiles[cacheKey] = profile
			versions[cacheKey] = version
		}
		normalizedOverrides, _, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, version)
		if err != nil {
			return fmt.Errorf("binding %s/%s: %w", binding.ModelName, binding.Operation, err)
		}
		binding.Overrides = normalizedOverrides
		contractHash, err := computeModelOperationContractHash(*binding, profile, version)
		if err != nil {
			return err
		}
		contractVersion := binding.ContractVersion
		if contractVersion <= 0 {
			contractVersion = 1
		} else if binding.ContractHash != contractHash {
			contractVersion++
		}
		if binding.ContractHash == contractHash && binding.ContractVersion == contractVersion && storedOverrides == normalizedOverrides {
			continue
		}
		if err := DB.Model(binding).Updates(map[string]interface{}{
			"overrides":        normalizedOverrides,
			"contract_version": contractVersion,
			"contract_hash":    contractHash,
			"updated_time":     common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
