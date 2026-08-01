package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"mime"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const (
	ModelOperationPricingModeParameterMultipliers = "newapi-base-with-parameter-multipliers"
	ModelOperationSchemaModeMerge                 = "merge"
	ModelOperationSchemaModeReplace               = "replace"
	maxModelOperationMaterialSizeMB               = 1024
	maxModelOperationMaterialDurationSeconds      = 3600
)

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

type ModelOperationModeCondition struct {
	ParameterEquals       map[string]interface{} `json:"parameter_equals,omitempty"`
	MaterialSlots         []string               `json:"material_slots,omitempty"`
	ExcludedMaterialSlots []string               `json:"excluded_material_slots,omitempty"`
}

type ModelOperationContractMode struct {
	Id                 string                         `json:"id"`
	Default            bool                           `json:"default,omitempty"`
	When               *ModelOperationModeCondition   `json:"when,omitempty"`
	DispatchModel      string                         `json:"dispatch_model,omitempty"`
	InputSchema        map[string]interface{}         `json:"input_schema,omitempty"`
	UISchema           map[string]interface{}         `json:"ui_schema,omitempty"`
	MaterialSchema     map[string]interface{}         `json:"material_schema,omitempty"`
	RequestContract    *ModelOperationRequestContract `json:"request_contract,omitempty"`
	PricingRule        *ModelOperationPricingRule     `json:"pricing_rule,omitempty"`
	ParameterDefaults  map[string]interface{}         `json:"parameter_defaults,omitempty"`
	ParameterOverrides map[string]interface{}         `json:"parameter_overrides,omitempty"`
	DispatchPath       string                         `json:"dispatch_path,omitempty"`
	PollPath           string                         `json:"poll_path,omitempty"`
	ResponseContract   string                         `json:"response_contract,omitempty"`
}

type ModelOperationBindingOverrides struct {
	SchemaMode         string                         `json:"schema_mode,omitempty"`
	Branding           *ModelOperationBranding        `json:"branding,omitempty"`
	InputSchema        map[string]interface{}         `json:"input_schema,omitempty"`
	UISchema           map[string]interface{}         `json:"ui_schema,omitempty"`
	MaterialSchema     map[string]interface{}         `json:"material_schema,omitempty"`
	RequestContract    *ModelOperationRequestContract `json:"request_contract,omitempty"`
	PricingRule        *ModelOperationPricingRule     `json:"pricing_rule,omitempty"`
	ParameterDefaults  map[string]interface{}         `json:"parameter_defaults,omitempty"`
	ParameterOverrides map[string]interface{}         `json:"parameter_overrides,omitempty"`
	DispatchPath       string                         `json:"dispatch_path,omitempty"`
	PollPath           string                         `json:"poll_path,omitempty"`
	Modes              []ModelOperationContractMode   `json:"modes,omitempty"`
}

type ModelOperationEffectiveContract struct {
	ProfileKey         string                        `json:"profile_key"`
	ProfileVersion     int                           `json:"profile_version"`
	SchemaMode         string                        `json:"schema_mode"`
	Operation          string                        `json:"operation"`
	EndpointType       string                        `json:"endpoint_type"`
	ExecutionMode      string                        `json:"execution_mode"`
	Branding           ModelOperationBranding        `json:"branding"`
	InputSchema        map[string]interface{}        `json:"input_schema"`
	UISchema           map[string]interface{}        `json:"ui_schema"`
	MaterialSchema     map[string]interface{}        `json:"material_schema"`
	RequestContract    ModelOperationRequestContract `json:"request_contract"`
	PricingRule        ModelOperationPricingRule     `json:"pricing_rule"`
	ParameterDefaults  map[string]interface{}        `json:"parameter_defaults"`
	ParameterOverrides map[string]interface{}        `json:"parameter_overrides"`
	DispatchPath       string                        `json:"dispatch_path,omitempty"`
	PollPath           string                        `json:"poll_path,omitempty"`
	ResponseContract   string                        `json:"response_contract"`
	ContractVersion    int                           `json:"contract_version"`
	ContractHash       string                        `json:"contract_hash"`
	Modes              []ModelOperationContractMode  `json:"modes,omitempty"`
	SelectedMode       string                        `json:"selected_mode,omitempty"`
}

func validateContractObjectKeys(field string, object map[string]interface{}, allowed ...string) error {
	allowedKeys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedKeys[key]; !ok {
			if field == "overrides" {
				return fmt.Errorf("overrides contains unsupported override field %q", key)
			}
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
	schemaType, ok := schema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("input_schema.type must be object")
	}
	if additionalProperties, exists := schema["additionalProperties"]; exists {
		if _, ok := additionalProperties.(bool); !ok {
			return errors.New("input_schema.additionalProperties must be a boolean")
		}
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
		if err := validateModelOperationParameterSchema(field, fieldSchema); err != nil {
			return err
		}
	}
	return validateModelOperationRequiredFields("input_schema", schema, properties)
}

func validateModelOperationRequiredFields(path string, schema map[string]interface{}, properties map[string]interface{}) error {
	if rawRequired, exists := schema["required"]; exists {
		required, ok := rawRequired.([]interface{})
		if !ok {
			return fmt.Errorf("%s.required must be an array", path)
		}
		seen := make(map[string]struct{}, len(required))
		for _, value := range required {
			field, ok := value.(string)
			if !ok || strings.TrimSpace(field) == "" {
				return fmt.Errorf("%s.required must contain field names", path)
			}
			field = strings.TrimSpace(field)
			if _, exists := properties[field]; !exists {
				return fmt.Errorf("%s.required references unknown field %s", path, field)
			}
			if _, exists := seen[field]; exists {
				return fmt.Errorf("%s.required contains duplicate field %s", path, field)
			}
			seen[field] = struct{}{}
		}
	}
	return nil
}

func validateModelOperationParameterSchema(field string, schema map[string]interface{}) error {
	fieldTypes, err := modelOperationParameterSchemaTypes(schema)
	if err != nil {
		return fmt.Errorf("input_schema property %s: %w", field, err)
	}
	hasType := func(expected string) bool {
		for _, fieldType := range fieldTypes {
			if fieldType == expected {
				return true
			}
		}
		return false
	}
	if err := validateModelOperationParameterBounds(field, schema); err != nil {
		return err
	}
	if rawProperties, exists := schema["properties"]; exists {
		if !hasType("object") {
			return fmt.Errorf("input_schema property %s properties require type object", field)
		}
		properties, ok := contractObject(rawProperties)
		if !ok {
			return fmt.Errorf("input_schema property %s properties must be an object", field)
		}
		for child, rawChildSchema := range properties {
			childSchema, ok := contractObject(rawChildSchema)
			if !ok {
				return fmt.Errorf("input_schema property %s.%s must be an object", field, child)
			}
			if err := validateModelOperationParameterSchema(field+"."+child, childSchema); err != nil {
				return err
			}
		}
		if err := validateModelOperationRequiredFields("input_schema property "+field, schema, properties); err != nil {
			return err
		}
	} else if _, exists := schema["required"]; exists {
		if !hasType("object") {
			return fmt.Errorf("input_schema property %s required requires type object", field)
		}
		if err := validateModelOperationRequiredFields("input_schema property "+field, schema, map[string]interface{}{}); err != nil {
			return err
		}
	}
	if additionalProperties, exists := schema["additionalProperties"]; exists {
		if !hasType("object") {
			return fmt.Errorf("input_schema property %s additionalProperties requires type object", field)
		}
		if _, ok := additionalProperties.(bool); !ok {
			return fmt.Errorf("input_schema property %s additionalProperties must be a boolean", field)
		}
	}
	if rawItems, exists := schema["items"]; exists {
		if !hasType("array") {
			return fmt.Errorf("input_schema property %s items require type array", field)
		}
		items, ok := contractObject(rawItems)
		if !ok {
			return fmt.Errorf("input_schema property %s items must be an object", field)
		}
		if err := validateModelOperationParameterSchema(field+"[]", items); err != nil {
			return err
		}
	}
	if rawEnum, exists := schema["enum"]; exists {
		enumValues, ok := rawEnum.([]interface{})
		if !ok || len(enumValues) == 0 {
			return fmt.Errorf("input_schema property %s enum must be a non-empty array", field)
		}
		for _, enumValue := range enumValues {
			if err := validateModelOperationParameterValue(field, schema, enumValue, false); err != nil {
				return fmt.Errorf("input_schema property %s enum: %w", field, err)
			}
		}
	}
	if defaultValue, exists := schema["default"]; exists {
		if err := validateModelOperationParameterValue(field, schema, defaultValue, true); err != nil {
			return fmt.Errorf("input_schema property %s default: %w", field, err)
		}
	}
	return nil
}

func validateModelOperationParameterBounds(field string, schema map[string]interface{}) error {
	fieldTypes, err := modelOperationParameterSchemaTypes(schema)
	if err != nil {
		return fmt.Errorf("input_schema property %s: %w", field, err)
	}
	hasType := func(expected string) bool {
		for _, fieldType := range fieldTypes {
			if fieldType == expected {
				return true
			}
		}
		return false
	}
	minimum, hasMinimum := schema["minimum"]
	maximum, hasMaximum := schema["maximum"]
	if hasMinimum || hasMaximum {
		if !hasType("number") && !hasType("integer") {
			return fmt.Errorf("input_schema property %s minimum/maximum require a numeric type", field)
		}
		minimumValue, minimumOK := modelOperationNumericParameterValue(minimum)
		maximumValue, maximumOK := modelOperationNumericParameterValue(maximum)
		if hasMinimum && !minimumOK {
			return fmt.Errorf("input_schema property %s minimum must be a number", field)
		}
		if hasMaximum && !maximumOK {
			return fmt.Errorf("input_schema property %s maximum must be a number", field)
		}
		if hasMinimum && hasMaximum && maximumValue < minimumValue {
			return fmt.Errorf("input_schema property %s maximum is below minimum", field)
		}
	}
	for _, bounds := range []struct {
		minimum string
		maximum string
		kind    string
	}{
		{minimum: "minLength", maximum: "maxLength", kind: "string"},
		{minimum: "minItems", maximum: "maxItems", kind: "array"},
	} {
		minimum, hasMinimum := schema[bounds.minimum]
		maximum, hasMaximum := schema[bounds.maximum]
		if !hasMinimum && !hasMaximum {
			continue
		}
		if !hasType(bounds.kind) {
			return fmt.Errorf("input_schema property %s %s/%s require type %s", field, bounds.minimum, bounds.maximum, bounds.kind)
		}
		minimumValue, minimumOK := modelOperationNumericParameterValue(minimum)
		maximumValue, maximumOK := modelOperationNumericParameterValue(maximum)
		if hasMinimum && (!minimumOK || minimumValue < 0 || math.Trunc(minimumValue) != minimumValue) {
			return fmt.Errorf("input_schema property %s %s must be a non-negative integer", field, bounds.minimum)
		}
		if hasMaximum && (!maximumOK || maximumValue < 0 || math.Trunc(maximumValue) != maximumValue) {
			return fmt.Errorf("input_schema property %s %s must be a non-negative integer", field, bounds.maximum)
		}
		if hasMinimum && hasMaximum && maximumValue < minimumValue {
			return fmt.Errorf("input_schema property %s %s is below %s", field, bounds.maximum, bounds.minimum)
		}
	}
	return nil
}

func modelOperationParameterSchemaTypes(schema map[string]interface{}) ([]string, error) {
	rawType, exists := schema["type"]
	if !exists {
		return nil, errors.New("must define a supported type")
	}
	rawTypes := make([]interface{}, 0, 1)
	switch typed := rawType.(type) {
	case string:
		rawTypes = append(rawTypes, typed)
	case []interface{}:
		rawTypes = typed
	default:
		return nil, errors.New("must define a supported type")
	}
	if len(rawTypes) == 0 {
		return nil, errors.New("must define a supported type")
	}
	types := make([]string, 0, len(rawTypes))
	seen := make(map[string]struct{}, len(rawTypes))
	for _, raw := range rawTypes {
		fieldType, ok := raw.(string)
		if !ok {
			return nil, errors.New("type choices must be strings")
		}
		switch fieldType {
		case "string", "number", "integer", "boolean", "array", "object":
		default:
			return nil, fmt.Errorf("has unsupported type %s", fieldType)
		}
		if _, exists := seen[fieldType]; exists {
			return nil, fmt.Errorf("contains duplicate type %s", fieldType)
		}
		seen[fieldType] = struct{}{}
		types = append(types, fieldType)
	}
	return types, nil
}

func modelOperationParameterValueMatchesType(fieldType string, value interface{}) bool {
	switch fieldType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := modelOperationNumericParameterValue(value)
		return ok
	case "integer":
		number, ok := modelOperationNumericParameterValue(value)
		return ok && math.Trunc(number) == number
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "object":
		_, ok := contractObject(value)
		return ok
	default:
		return false
	}
}

func modelOperationNumericParameterValue(value interface{}) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func modelOperationParameterValuesEqual(fieldType string, left interface{}, right interface{}) bool {
	if fieldType == "number" || fieldType == "integer" {
		leftNumber, leftOK := modelOperationNumericParameterValue(left)
		rightNumber, rightOK := modelOperationNumericParameterValue(right)
		return leftOK && rightOK && leftNumber == rightNumber
	}
	return reflect.DeepEqual(left, right)
}

func validateModelOperationParameterValue(field string, schema map[string]interface{}, value interface{}, checkEnum bool) error {
	fieldTypes, err := modelOperationParameterSchemaTypes(schema)
	if err != nil {
		return err
	}
	matchedType := ""
	for _, fieldType := range fieldTypes {
		if modelOperationParameterValueMatchesType(fieldType, value) {
			matchedType = fieldType
			break
		}
	}
	if matchedType == "" {
		return fmt.Errorf("parameter %s must be of type %s", field, strings.Join(fieldTypes, " or "))
	}
	if checkEnum {
		if enumValues, ok := schema["enum"].([]interface{}); ok {
			matched := false
			for _, enumValue := range enumValues {
				if modelOperationParameterValuesEqual(matchedType, value, enumValue) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("parameter %s is not an allowed enum value", field)
			}
		}
	}
	if matchedType == "number" || matchedType == "integer" {
		number, _ := modelOperationNumericParameterValue(value)
		if minimum, ok := modelOperationNumericParameterValue(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("parameter %s must be at least %s", field, canonicalNumber(minimum))
		}
		if maximum, ok := modelOperationNumericParameterValue(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("parameter %s must be at most %s", field, canonicalNumber(maximum))
		}
	}
	if matchedType == "string" {
		length := float64(len([]rune(value.(string))))
		if minimum, ok := modelOperationNumericParameterValue(schema["minLength"]); ok && length < minimum {
			return fmt.Errorf("parameter %s is shorter than minLength", field)
		}
		if maximum, ok := modelOperationNumericParameterValue(schema["maxLength"]); ok && length > maximum {
			return fmt.Errorf("parameter %s is longer than maxLength", field)
		}
	}
	if matchedType == "array" {
		items := value.([]interface{})
		length := float64(len(items))
		if minimum, ok := modelOperationNumericParameterValue(schema["minItems"]); ok && length < minimum {
			return fmt.Errorf("parameter %s has fewer than minItems", field)
		}
		if maximum, ok := modelOperationNumericParameterValue(schema["maxItems"]); ok && length > maximum {
			return fmt.Errorf("parameter %s has more than maxItems", field)
		}
		if itemSchema, ok := contractObject(schema["items"]); ok {
			for index, item := range items {
				if err := validateModelOperationParameterValue(fmt.Sprintf("%s[%d]", field, index), itemSchema, item, true); err != nil {
					return err
				}
			}
		}
	}
	if matchedType == "object" {
		object, _ := contractObject(value)
		properties, hasProperties := contractObject(schema["properties"])
		if hasProperties {
			rejectUnknown := false
			if additionalProperties, exists := schema["additionalProperties"].(bool); exists {
				rejectUnknown = !additionalProperties
			}
			for child, childValue := range object {
				childSchema, known := contractObject(properties[child])
				if !known {
					if rejectUnknown {
						return fmt.Errorf("parameter %s contains unknown parameter %s", field, child)
					}
					continue
				}
				if err := validateModelOperationParameterValue(field+"."+child, childSchema, childValue, true); err != nil {
					return err
				}
			}
			if required, _ := schema["required"].([]interface{}); len(required) > 0 {
				for _, rawChild := range required {
					child, _ := rawChild.(string)
					if _, exists := object[child]; !exists {
						return fmt.Errorf("required parameter %s.%s is missing", field, child)
					}
				}
			}
		}
	}
	return nil
}

func canonicalNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
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
			"min_items", "max_items", "roles", "mime_types", "max_size_mb", "max_total_duration", "request_field", "request_fields", "transport", "ordered"); err != nil {
			return err
		}
		if ordered, exists := rule["ordered"]; exists {
			if _, ok := ordered.(bool); !ok {
				return fmt.Errorf("material_schema.%s.ordered must be a boolean", materialType)
			}
		}
		minItemsRaw, hasMin := rule["min_items"]
		maxItemsRaw, hasMax := rule["max_items"]
		minItems, minItemsValid := modelOperationNumericParameterValue(minItemsRaw)
		maxItems, maxItemsValid := modelOperationNumericParameterValue(maxItemsRaw)
		if hasMin && !minItemsValid {
			return fmt.Errorf("material_schema.%s.min_items must be a number", materialType)
		}
		if hasMax && !maxItemsValid {
			return fmt.Errorf("material_schema.%s.max_items must be a number", materialType)
		}
		if hasMin && (minItems < 0 || math.Trunc(minItems) != minItems || minItems > 20) {
			return fmt.Errorf("material_schema.%s.min_items must be an integer between 0 and 20", materialType)
		}
		if hasMax && (maxItems < 0 || math.Trunc(maxItems) != maxItems || maxItems > 20) {
			return fmt.Errorf("material_schema.%s.max_items must be an integer between 0 and 20", materialType)
		}
		if hasMin && hasMax && minItems > maxItems {
			return fmt.Errorf("material_schema.%s.min_items exceeds max_items", materialType)
		}
		if rawRoles, exists := rule["roles"]; exists {
			roles, ok := rawRoles.([]interface{})
			if !ok || len(roles) == 0 || len(roles) > 20 {
				return fmt.Errorf("material_schema.%s.roles must be a non-empty array with at most 20 entries", materialType)
			}
			seen := make(map[string]struct{}, len(roles))
			for _, rawRole := range roles {
				role, ok := rawRole.(string)
				if !ok {
					return fmt.Errorf("material_schema.%s.roles must contain strings", materialType)
				}
				role, err := normalizeContractIdentifier(role, 64)
				if err != nil {
					return fmt.Errorf("material_schema.%s.roles: %w", materialType, err)
				}
				if _, duplicate := seen[role]; duplicate {
					return fmt.Errorf("material_schema.%s.roles contains duplicate role %s", materialType, role)
				}
				seen[role] = struct{}{}
			}
		}
		if rawMimeTypes, exists := rule["mime_types"]; exists {
			mimeTypes, ok := rawMimeTypes.([]interface{})
			if !ok || len(mimeTypes) == 0 || len(mimeTypes) > 64 {
				return fmt.Errorf("material_schema.%s.mime_types must be a non-empty array with at most 64 entries", materialType)
			}
			seen := make(map[string]struct{}, len(mimeTypes))
			for _, rawMimeType := range mimeTypes {
				mimeType, ok := rawMimeType.(string)
				if !ok {
					return fmt.Errorf("material_schema.%s.mime_types must contain strings", materialType)
				}
				mimeType = strings.ToLower(strings.TrimSpace(mimeType))
				parsed, parameters, err := mime.ParseMediaType(mimeType)
				if err != nil || parsed != mimeType || len(parameters) != 0 || !strings.Contains(mimeType, "/") {
					return fmt.Errorf("material_schema.%s.mime_types contains invalid MIME type %q", materialType, mimeType)
				}
				if _, duplicate := seen[mimeType]; duplicate {
					return fmt.Errorf("material_schema.%s.mime_types contains duplicate MIME type %s", materialType, mimeType)
				}
				seen[mimeType] = struct{}{}
			}
		}
		for field, maximum := range map[string]float64{
			"max_size_mb":        maxModelOperationMaterialSizeMB,
			"max_total_duration": maxModelOperationMaterialDurationSeconds,
		} {
			rawValue, exists := rule[field]
			if !exists {
				continue
			}
			value, ok := modelOperationNumericParameterValue(rawValue)
			if !ok || value <= 0 || value > maximum {
				return fmt.Errorf("material_schema.%s.%s must be a number greater than 0 and at most %s", materialType, field, canonicalNumber(maximum))
			}
		}
		requestFieldRaw, hasRequestField := rule["request_field"]
		requestField, requestFieldValid := requestFieldRaw.(string)
		requestField = strings.TrimSpace(requestField)
		if hasRequestField && (!requestFieldValid || requestField == "") {
			return fmt.Errorf("material_schema.%s.request_field must be a non-empty string", materialType)
		}
		if hasRequestField {
			if _, err := normalizeContractIdentifier(requestField, 128); err != nil {
				return fmt.Errorf("material_schema.%s.request_field: %w", materialType, err)
			}
		}
		transportRaw, hasTransport := rule["transport"]
		transport, transportValid := transportRaw.(string)
		transport = strings.TrimSpace(transport)
		if hasTransport && (!transportValid || transport == "") {
			return fmt.Errorf("material_schema.%s.transport must be a non-empty string", materialType)
		}
		if hasTransport && transport != "url" && transport != "base64" && transport != "multipart" && transport != "asset_id" && transport != "deepwl-asset-url" {
			return fmt.Errorf("material_schema.%s.transport %q is not registered", materialType, transport)
		}
		if hasRequestField != hasTransport {
			return fmt.Errorf("material_schema.%s.request_field and transport must be configured together", materialType)
		}
		rawRequestFields, hasRequestFields := rule["request_fields"]
		if hasRequestFields {
			if hasRequestField || hasTransport {
				return fmt.Errorf("material_schema.%s cannot combine request_field with request_fields", materialType)
			}
			requestFields, ok := rawRequestFields.([]interface{})
			if !ok || len(requestFields) == 0 || len(requestFields) > 20 {
				return fmt.Errorf("material_schema.%s.request_fields must be a non-empty array with at most 20 entries", materialType)
			}
			seenSlots := make(map[string]struct{}, len(requestFields))
			requiredSlots := make([]string, 0)
			for index, rawRequestField := range requestFields {
				requestFieldRule, ok := contractObject(rawRequestField)
				if !ok {
					return fmt.Errorf("material_schema.%s.request_fields[%d] must be an object", materialType, index)
				}
				if err := validateContractObjectKeys(fmt.Sprintf("material_schema.%s.request_fields[%d]", materialType, index), requestFieldRule,
					"slot", "min_items", "max_items", "roles", "mime_types", "max_size_mb", "request_field", "transport", "value_type", "label", "url_field", "item_template", "requires_slot", "exclusive_group", "one_of_group", "position"); err != nil {
					return err
				}
				slot, _ := requestFieldRule["slot"].(string)
				slot = strings.TrimSpace(slot)
				if slot == "" {
					return fmt.Errorf("material_schema.%s.request_fields[%d].slot is required", materialType, index)
				}
				if _, err := normalizeContractIdentifier(slot, 128); err != nil {
					return fmt.Errorf("material_schema.%s.request_fields[%d].slot: %w", materialType, index, err)
				}
				if _, duplicate := seenSlots[slot]; duplicate {
					return fmt.Errorf("material_schema.%s.request_fields contains duplicate slot %s", materialType, slot)
				}
				seenSlots[slot] = struct{}{}
				nestedRequestField, _ := requestFieldRule["request_field"].(string)
				nestedRequestField = strings.TrimSpace(nestedRequestField)
				if nestedRequestField == "" {
					return fmt.Errorf("material_schema.%s.request_fields[%d].request_field is required", materialType, index)
				}
				valueType, _ := requestFieldRule["value_type"].(string)
				if valueType != "string" && valueType != "array" {
					return fmt.Errorf("material_schema.%s.request_fields[%d].value_type must be string or array", materialType, index)
				}
				transport, _ := requestFieldRule["transport"].(string)
				if transport != "url" && transport != "base64" && transport != "multipart" && transport != "asset_id" && transport != "deepwl-asset-url" {
					return fmt.Errorf("material_schema.%s.request_fields[%d].transport %q is not registered", materialType, index, transport)
				}
				for _, groupField := range []string{"exclusive_group", "one_of_group"} {
					if rawGroup, exists := requestFieldRule[groupField]; exists {
						group, ok := rawGroup.(string)
						if !ok || strings.TrimSpace(group) == "" {
							return fmt.Errorf("material_schema.%s.request_fields[%d].%s must be a non-empty string", materialType, index, groupField)
						}
						if _, err := normalizeContractIdentifier(group, 128); err != nil {
							return fmt.Errorf("material_schema.%s.request_fields[%d].%s: %w", materialType, index, groupField, err)
						}
					}
				}
				if rawPosition, exists := requestFieldRule["position"]; exists {
					position, ok := modelOperationNumericParameterValue(rawPosition)
					if !ok || position < 0 || position > 20 || math.Trunc(position) != position {
						return fmt.Errorf("material_schema.%s.request_fields[%d].position must be an integer between 0 and 20", materialType, index)
					}
				}
				urlFieldRaw, hasURLField := requestFieldRule["url_field"]
				urlField, urlFieldValid := urlFieldRaw.(string)
				urlField = strings.TrimSpace(urlField)
				if hasURLField {
					if !urlFieldValid || valueType != "array" || urlField == "" {
						return fmt.Errorf("material_schema.%s.request_fields[%d].url_field requires an array value type", materialType, index)
					}
					if _, err := normalizeContractIdentifier(urlField, 128); err != nil {
						return fmt.Errorf("material_schema.%s.request_fields[%d].url_field: %w", materialType, index, err)
					}
				}
				if rawTemplate, exists := requestFieldRule["item_template"]; exists {
					template, ok := contractObject(rawTemplate)
					if !ok || len(template) == 0 || valueType != "array" {
						return fmt.Errorf("material_schema.%s.request_fields[%d].item_template must be a non-empty object for an array value", materialType, index)
					}
					for field, value := range template {
						if _, err := normalizeContractIdentifier(field, 128); err != nil {
							return fmt.Errorf("material_schema.%s.request_fields[%d].item_template: %w", materialType, index, err)
						}
						switch value.(type) {
						case string, bool, float64:
						default:
							return fmt.Errorf("material_schema.%s.request_fields[%d].item_template.%s must be a scalar", materialType, index, field)
						}
					}
				}
				if requiredSlot, exists := requestFieldRule["requires_slot"]; exists {
					requiredSlotText, ok := requiredSlot.(string)
					requiredSlotText = strings.TrimSpace(requiredSlotText)
					if !ok || requiredSlotText == "" {
						return fmt.Errorf("material_schema.%s.request_fields[%d].requires_slot must be a non-empty string", materialType, index)
					}
					if _, err := normalizeContractIdentifier(requiredSlotText, 128); err != nil {
						return fmt.Errorf("material_schema.%s.request_fields[%d].requires_slot: %w", materialType, index, err)
					}
					requiredSlots = append(requiredSlots, requiredSlotText)
				}
				if label, exists := requestFieldRule["label"]; exists {
					labelText, ok := label.(string)
					if !ok || strings.TrimSpace(labelText) == "" || len([]rune(labelText)) > 128 {
						return fmt.Errorf("material_schema.%s.request_fields[%d].label must be a non-empty string with at most 128 characters", materialType, index)
					}
				}

				legacyRule := make(map[string]interface{}, len(requestFieldRule))
				for field, value := range requestFieldRule {
					if field != "slot" && field != "value_type" && field != "label" && field != "url_field" && field != "item_template" && field != "requires_slot" && field != "exclusive_group" && field != "one_of_group" && field != "position" {
						legacyRule[field] = value
					}
				}
				if err := validateModelOperationMaterialSchema(map[string]interface{}{materialType: legacyRule}, true); err != nil {
					return fmt.Errorf("material_schema.%s.request_fields[%d]: %w", materialType, index, err)
				}
			}
			for _, requiredSlot := range requiredSlots {
				if _, exists := seenSlots[requiredSlot]; !exists {
					return fmt.Errorf("material_schema.%s.request_fields requires unknown slot %s", materialType, requiredSlot)
				}
			}
		}
		if requireRequestMapping && hasMax && maxItems > 0 && !hasRequestFields && (requestField == "" || transport == "") {
			return fmt.Errorf("material_schema.%s requires request_field and transport when materials are enabled", materialType)
		}
	}
	return nil
}

func modelOperationMaterialSlotSet(schema map[string]interface{}) map[string]struct{} {
	result := make(map[string]struct{})
	for _, rawRule := range schema {
		rule, ok := contractObject(rawRule)
		if !ok {
			continue
		}
		if requestField, ok := rule["request_field"].(string); ok && strings.TrimSpace(requestField) != "" {
			result[strings.TrimSpace(requestField)] = struct{}{}
		}
		requestFields, _ := rule["request_fields"].([]interface{})
		for _, rawField := range requestFields {
			field, ok := contractObject(rawField)
			if !ok {
				continue
			}
			for _, key := range []string{"slot", "request_field"} {
				if value, ok := field[key].(string); ok && strings.TrimSpace(value) != "" {
					result[strings.TrimSpace(value)] = struct{}{}
				}
			}
		}
	}
	return result
}

func normalizeModelOperationModeSlots(field string, values []string, available map[string]struct{}) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s must contain non-empty slot names", field)
		}
		if _, exists := available[value]; !exists {
			return nil, fmt.Errorf("%s references unknown material slot %s", field, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate material slot %s", field, value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func modelOperationModeConditionEmpty(condition *ModelOperationModeCondition) bool {
	return condition == nil || (len(condition.ParameterEquals) == 0 && len(condition.MaterialSlots) == 0 && len(condition.ExcludedMaterialSlots) == 0)
}

func modelOperationModeConditionsOverlap(left, right *ModelOperationModeCondition) bool {
	for field, leftValue := range left.ParameterEquals {
		if rightValue, exists := right.ParameterEquals[field]; exists && !reflect.DeepEqual(leftValue, rightValue) {
			return false
		}
	}
	leftRequired := make(map[string]struct{}, len(left.MaterialSlots))
	leftExcluded := make(map[string]struct{}, len(left.ExcludedMaterialSlots))
	rightRequired := make(map[string]struct{}, len(right.MaterialSlots))
	rightExcluded := make(map[string]struct{}, len(right.ExcludedMaterialSlots))
	for _, slot := range left.MaterialSlots {
		leftRequired[slot] = struct{}{}
	}
	for _, slot := range left.ExcludedMaterialSlots {
		leftExcluded[slot] = struct{}{}
	}
	for _, slot := range right.MaterialSlots {
		rightRequired[slot] = struct{}{}
	}
	for _, slot := range right.ExcludedMaterialSlots {
		rightExcluded[slot] = struct{}{}
	}
	for slot := range leftRequired {
		if _, conflict := rightExcluded[slot]; conflict {
			return false
		}
	}
	for slot := range rightRequired {
		if _, conflict := leftExcluded[slot]; conflict {
			return false
		}
	}
	return true
}

func validateModelOperationModeRequestContract(contract *ModelOperationRequestContract, endpointType string, properties map[string]interface{}) error {
	if contract == nil {
		return nil
	}
	contract.Adapter = strings.TrimSpace(contract.Adapter)
	validEndpoint := map[string]string{
		"openai-chat": "openai", "openai-image": "image-generation", "gemini-image": "gemini",
		"openai-video": "openai-video", "re-task": "re-task",
	}[contract.Adapter]
	if validEndpoint == "" {
		return fmt.Errorf("request_contract adapter %q is not registered", contract.Adapter)
	}
	if endpointType != validEndpoint {
		return fmt.Errorf("%s request adapter requires %s endpoint_type", contract.Adapter, validEndpoint)
	}
	for field, target := range contract.FieldMap {
		if _, exists := properties[field]; !exists {
			return fmt.Errorf("request_contract field_map references unknown field %s", field)
		}
		if _, err := normalizeContractIdentifier(strings.TrimSpace(target), 128); err != nil {
			return fmt.Errorf("request_contract field_map target for %s: %w", field, err)
		}
	}
	for field, coercion := range contract.Coercions {
		if _, exists := properties[field]; !exists {
			return fmt.Errorf("request_contract coercion references unknown field %s", field)
		}
		switch coercion {
		case "string", "integer", "number", "boolean", "json":
		default:
			return fmt.Errorf("request_contract coercion %s for %s is not registered", coercion, field)
		}
	}
	return nil
}

func validateAndNormalizeModelOperationModes(
	modes []ModelOperationContractMode,
	baseInputSchema, baseUISchema, baseMaterialSchema map[string]interface{},
	version *ModelOperationProfileVersion,
) ([]ModelOperationContractMode, error) {
	if len(modes) == 0 {
		return nil, nil
	}
	if len(modes) > 20 {
		return nil, errors.New("modes must contain at most 20 entries")
	}
	defaultCount := 0
	seenIds := make(map[string]struct{}, len(modes))
	for index := range modes {
		mode := &modes[index]
		id, err := normalizeContractIdentifier(mode.Id, 64)
		if err != nil {
			return nil, fmt.Errorf("modes[%d].id: %w", index, err)
		}
		if _, duplicate := seenIds[id]; duplicate {
			return nil, fmt.Errorf("modes contains duplicate id %s", id)
		}
		seenIds[id] = struct{}{}
		mode.Id = id
		mode.DispatchModel = strings.TrimSpace(mode.DispatchModel)
		if mode.DispatchModel != "" {
			mode.DispatchModel, err = normalizeModelRouteName(fmt.Sprintf("modes[%d].dispatch_model", index), mode.DispatchModel)
			if err != nil {
				return nil, err
			}
		}
		if mode.Default {
			defaultCount++
			if !modelOperationModeConditionEmpty(mode.When) {
				return nil, fmt.Errorf("modes[%d] default mode must not define when", index)
			}
		} else if modelOperationModeConditionEmpty(mode.When) {
			return nil, fmt.Errorf("modes[%d] non-default mode requires when", index)
		}

		inputSchema := mergeModelOperationContractObject(baseInputSchema, mode.InputSchema)
		uiSchema := mergeModelOperationContractObject(baseUISchema, mode.UISchema)
		materialSchema := mergeModelOperationContractObject(baseMaterialSchema, mode.MaterialSchema)
		if err := validateModelOperationInputSchema(inputSchema); err != nil {
			return nil, fmt.Errorf("modes[%d]: %w", index, err)
		}
		if err := validateModelOperationUISchema(uiSchema, inputSchema); err != nil {
			return nil, fmt.Errorf("modes[%d]: %w", index, err)
		}
		if err := validateModelOperationMaterialSchema(materialSchema, mode.RequestContract != nil); err != nil {
			return nil, fmt.Errorf("modes[%d]: %w", index, err)
		}
		properties, _ := contractObject(inputSchema["properties"])
		if err := validateModelOperationModeRequestContract(mode.RequestContract, version.EndpointType, properties); err != nil {
			return nil, fmt.Errorf("modes[%d]: %w", index, err)
		}
		if err := validateModelOperationPricingRule(mode.PricingRule, inputSchema); err != nil {
			return nil, fmt.Errorf("modes[%d]: %w", index, err)
		}
		for configurationName, values := range map[string]map[string]interface{}{
			"parameter_defaults": mode.ParameterDefaults, "parameter_overrides": mode.ParameterOverrides,
		} {
			for field, value := range values {
				fieldSchema, ok := contractObject(properties[field])
				if !ok {
					return nil, fmt.Errorf("modes[%d].%s references unknown field %s", index, configurationName, field)
				}
				if err := validateModelOperationParameterValue(field, fieldSchema, value, true); err != nil {
					return nil, fmt.Errorf("modes[%d].%s: %w", index, configurationName, err)
				}
			}
		}
		mode.DispatchPath, err = normalizeModelOperationRelayPath(fmt.Sprintf("modes[%d].dispatch_path", index), mode.DispatchPath, false)
		if err != nil {
			return nil, err
		}
		mode.PollPath, err = normalizeModelOperationRelayPath(fmt.Sprintf("modes[%d].poll_path", index), mode.PollPath, true)
		if err != nil {
			return nil, err
		}
		if mode.PollPath != "" && version.ExecutionMode != "async" {
			return nil, fmt.Errorf("modes[%d].poll_path is only valid for async profiles", index)
		}
		mode.ResponseContract = strings.TrimSpace(mode.ResponseContract)
		if len(mode.ResponseContract) > 128 {
			return nil, fmt.Errorf("modes[%d].response_contract must be 128 characters or fewer", index)
		}
		if mode.When != nil {
			for field, value := range mode.When.ParameterEquals {
				fieldSchema, ok := contractObject(properties[field])
				if !ok {
					return nil, fmt.Errorf("modes[%d].when.parameter_equals references unknown field %s", index, field)
				}
				if err := validateModelOperationParameterValue(field, fieldSchema, value, true); err != nil {
					return nil, fmt.Errorf("modes[%d].when.parameter_equals: %w", index, err)
				}
			}
			availableSlots := modelOperationMaterialSlotSet(materialSchema)
			mode.When.MaterialSlots, err = normalizeModelOperationModeSlots(fmt.Sprintf("modes[%d].when.material_slots", index), mode.When.MaterialSlots, availableSlots)
			if err != nil {
				return nil, err
			}
			mode.When.ExcludedMaterialSlots, err = normalizeModelOperationModeSlots(fmt.Sprintf("modes[%d].when.excluded_material_slots", index), mode.When.ExcludedMaterialSlots, availableSlots)
			if err != nil {
				return nil, err
			}
			for _, slot := range mode.When.MaterialSlots {
				for _, excluded := range mode.When.ExcludedMaterialSlots {
					if slot == excluded {
						return nil, fmt.Errorf("modes[%d].when cannot require and exclude material slot %s", index, slot)
					}
				}
			}
		}
	}
	if defaultCount != 1 {
		return nil, errors.New("modes must define exactly one default mode")
	}
	for left := 0; left < len(modes); left++ {
		if modes[left].Default {
			continue
		}
		for right := left + 1; right < len(modes); right++ {
			if modes[right].Default {
				continue
			}
			if modelOperationModeConditionsOverlap(modes[left].When, modes[right].When) {
				return nil, fmt.Errorf("modes %s and %s have overlapping conditions", modes[left].Id, modes[right].Id)
			}
		}
	}
	return modes, nil
}

func normalizeModelOperationRelayPath(field string, value string, requireTaskPlaceholder bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	validPrefix := strings.HasPrefix(value, "/v1/") || strings.HasPrefix(value, "/v1beta/")
	if !validPrefix || strings.ContainsAny(value, "?#") || strings.Contains(value, "..") || len(value) > 255 {
		return "", fmt.Errorf("%s must be a /v1/ or /v1beta/ path without query or traversal segments", field)
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
		"schema_mode", "branding", "input_schema", "ui_schema", "material_schema", "request_contract", "pricing_rule",
		"parameter_defaults", "parameter_overrides", "dispatch_path", "poll_path", "modes"); err != nil {
		return "", ModelOperationBindingOverrides{}, err
	}
	for field, allowed := range map[string][]string{
		"branding":         {"icon_key", "description"},
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
	if rawModes, exists := normalizedObject["modes"]; exists {
		modes, ok := rawModes.([]interface{})
		if !ok || len(modes) == 0 {
			return "", ModelOperationBindingOverrides{}, errors.New("modes must be a non-empty array")
		}
		for index, rawMode := range modes {
			mode, ok := contractObject(rawMode)
			if !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("modes[%d] must be an object", index)
			}
			if err := validateContractObjectKeys(fmt.Sprintf("modes[%d]", index), mode,
				"id", "default", "when", "dispatch_model", "input_schema", "ui_schema", "material_schema", "request_contract", "pricing_rule",
				"parameter_defaults", "parameter_overrides", "dispatch_path", "poll_path", "response_contract"); err != nil {
				return "", ModelOperationBindingOverrides{}, err
			}
			if when, ok := contractObject(mode["when"]); ok {
				if err := validateContractObjectKeys(fmt.Sprintf("modes[%d].when", index), when,
					"parameter_equals", "material_slots", "excluded_material_slots"); err != nil {
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
	overrides.SchemaMode = strings.ToLower(strings.TrimSpace(overrides.SchemaMode))
	if overrides.SchemaMode == "" {
		overrides.SchemaMode = ModelOperationSchemaModeMerge
	}
	switch overrides.SchemaMode {
	case ModelOperationSchemaModeMerge:
	case ModelOperationSchemaModeReplace:
		for _, field := range []string{"input_schema", "ui_schema", "material_schema"} {
			if _, ok := contractObject(normalizedObject[field]); !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("schema_mode replace requires %s to be a complete object", field)
			}
		}
	default:
		return "", ModelOperationBindingOverrides{}, fmt.Errorf("unsupported schema_mode %q", overrides.SchemaMode)
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
		case "gemini-image":
			if version.EndpointType != "gemini" {
				return "", ModelOperationBindingOverrides{}, errors.New("gemini-image request adapter requires gemini endpoint_type")
			}
		case "openai-video":
			if version.EndpointType != "openai-video" {
				return "", ModelOperationBindingOverrides{}, errors.New("openai-video request adapter requires openai-video endpoint_type")
			}
		case "re-task":
			if version.EndpointType != "re-task" {
				return "", ModelOperationBindingOverrides{}, errors.New("re-task request adapter requires re-task endpoint_type")
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
	if overrides.SchemaMode == ModelOperationSchemaModeReplace {
		inputSchema = overrides.InputSchema
		uiSchema = overrides.UISchema
		materialSchema = overrides.MaterialSchema
	} else {
		inputSchema = mergeModelOperationContractObject(inputSchema, overrides.InputSchema)
		uiSchema = mergeModelOperationContractObject(uiSchema, overrides.UISchema)
		materialSchema = mergeModelOperationContractObject(materialSchema, overrides.MaterialSchema)
	}
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
	overrides.Modes, err = validateAndNormalizeModelOperationModes(overrides.Modes, inputSchema, uiSchema, materialSchema, version)
	if err != nil {
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
	for configurationName, defaults := range map[string]map[string]interface{}{
		"parameter_defaults":  overrides.ParameterDefaults,
		"parameter_overrides": overrides.ParameterOverrides,
	} {
		for field, value := range defaults {
			fieldSchema, ok := contractObject(properties[field])
			if !ok {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("parameter configuration references unknown field %s", field)
			}
			if err := validateModelOperationParameterValue(field, fieldSchema, value, true); err != nil {
				return "", ModelOperationBindingOverrides{}, fmt.Errorf("%s: %w", configurationName, err)
			}
		}
	}
	normalizedOverrides := overrides
	if _, schemaModeWasExplicit := normalizedObject["schema_mode"]; !schemaModeWasExplicit && overrides.SchemaMode == ModelOperationSchemaModeMerge {
		normalizedOverrides.SchemaMode = ""
	}
	normalized, err := common.Marshal(normalizedOverrides)
	if err != nil {
		return "", ModelOperationBindingOverrides{}, fmt.Errorf("normalize overrides: %w", err)
	}
	if overrides.SchemaMode == ModelOperationSchemaModeReplace {
		var normalizedReplace map[string]interface{}
		if err := common.Unmarshal(normalized, &normalizedReplace); err != nil {
			return "", ModelOperationBindingOverrides{}, fmt.Errorf("normalize replace overrides: %w", err)
		}
		normalizedReplace["input_schema"] = overrides.InputSchema
		normalizedReplace["ui_schema"] = overrides.UISchema
		normalizedReplace["material_schema"] = overrides.MaterialSchema
		normalized, err = common.Marshal(normalizedReplace)
		if err != nil {
			return "", ModelOperationBindingOverrides{}, fmt.Errorf("normalize replace overrides: %w", err)
		}
	}
	return string(normalized), overrides, nil
}

// NormalizeAndValidateModelOperationBindingOverrides exposes the same validation
// used by persisted bindings for trusted initialization tools.
func NormalizeAndValidateModelOperationBindingOverrides(value string, profile *ModelOperationProfile, version *ModelOperationProfileVersion) (string, error) {
	normalized, _, err := normalizeModelOperationBindingOverrides(value, profile, version)
	return normalized, err
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
	schemaMode := overrides.SchemaMode
	if schemaMode == "" {
		schemaMode = ModelOperationSchemaModeMerge
	}
	switch schemaMode {
	case ModelOperationSchemaModeReplace:
		if overrides.InputSchema == nil || overrides.UISchema == nil || overrides.MaterialSchema == nil {
			return nil, errors.New("schema_mode replace requires complete input_schema, ui_schema, and material_schema objects")
		}
		inputSchema = overrides.InputSchema
		uiSchema = overrides.UISchema
		materialSchema = overrides.MaterialSchema
	case ModelOperationSchemaModeMerge:
		inputSchema = mergeModelOperationContractObject(inputSchema, overrides.InputSchema)
		uiSchema = mergeModelOperationContractObject(uiSchema, overrides.UISchema)
		materialSchema = mergeModelOperationContractObject(materialSchema, overrides.MaterialSchema)
	default:
		return nil, fmt.Errorf("unsupported schema_mode %q", schemaMode)
	}
	contract := &ModelOperationEffectiveContract{
		ProfileKey:         profile.ProfileKey,
		ProfileVersion:     version.Version,
		SchemaMode:         schemaMode,
		Operation:          version.Operation,
		EndpointType:       version.EndpointType,
		ExecutionMode:      version.ExecutionMode,
		InputSchema:        inputSchema,
		UISchema:           uiSchema,
		MaterialSchema:     materialSchema,
		ParameterDefaults:  overrides.ParameterDefaults,
		ParameterOverrides: overrides.ParameterOverrides,
		DispatchPath:       overrides.DispatchPath,
		PollPath:           overrides.PollPath,
		ResponseContract:   version.ResponseContract,
		ContractVersion:    binding.ContractVersion,
		ContractHash:       binding.ContractHash,
		Modes:              overrides.Modes,
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

func modelOperationModeMatches(condition *ModelOperationModeCondition, parameters map[string]interface{}, materialSlots map[string]bool) bool {
	if condition == nil {
		return false
	}
	for field, expected := range condition.ParameterEquals {
		if actual, exists := parameters[field]; !exists || !reflect.DeepEqual(actual, expected) {
			return false
		}
	}
	for _, slot := range condition.MaterialSlots {
		if !materialSlots[slot] {
			return false
		}
	}
	for _, slot := range condition.ExcludedMaterialSlots {
		if materialSlots[slot] {
			return false
		}
	}
	return true
}

func modelOperationContractValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
}

// DetectModelOperationMaterialSlots maps request fields and semantic slot names
// to presence flags without interpreting provider payload contents.
func DetectModelOperationMaterialSlots(contract *ModelOperationEffectiveContract, parameters map[string]interface{}) map[string]bool {
	result := make(map[string]bool)
	if contract == nil {
		return result
	}
	schemas := []map[string]interface{}{contract.MaterialSchema}
	for _, mode := range contract.Modes {
		if len(mode.MaterialSchema) > 0 {
			schemas = append(schemas, mergeModelOperationContractObject(contract.MaterialSchema, mode.MaterialSchema))
		}
	}
	for _, schema := range schemas {
		for _, rawRule := range schema {
			rule, ok := contractObject(rawRule)
			if !ok {
				continue
			}
			if requestField, ok := rule["request_field"].(string); ok {
				if modelOperationContractValuePresent(parameters[requestField]) {
					result[requestField] = true
				}
			}
			requestFields, _ := rule["request_fields"].([]interface{})
			for _, rawField := range requestFields {
				field, ok := contractObject(rawField)
				if !ok {
					continue
				}
				slot, _ := field["slot"].(string)
				requestField, _ := field["request_field"].(string)
				present := modelOperationContractValuePresent(parameters[requestField]) || modelOperationContractValuePresent(parameters[slot])
				if present {
					result[slot] = true
					result[requestField] = true
				}
			}
		}
	}
	return result
}

// ResolveModelOperationContractMode selects one documented mode and returns a
// detached effective contract. Ambiguous matches fail closed.
func ResolveModelOperationContractMode(contract *ModelOperationEffectiveContract, parameters map[string]interface{}, materialSlots map[string]bool) (*ModelOperationEffectiveContract, error) {
	if contract == nil || len(contract.Modes) == 0 {
		return contract, nil
	}
	var selected *ModelOperationContractMode
	var fallback *ModelOperationContractMode
	for index := range contract.Modes {
		mode := &contract.Modes[index]
		if mode.Default {
			fallback = mode
			continue
		}
		if !modelOperationModeMatches(mode.When, parameters, materialSlots) {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("contract modes %s and %s both match the request", selected.Id, mode.Id)
		}
		selected = mode
	}
	if selected == nil {
		selected = fallback
	}
	if selected == nil {
		return nil, errors.New("contract modes do not define a default mode")
	}
	resolved := *contract
	resolved.SelectedMode = selected.Id
	resolved.InputSchema = mergeModelOperationContractObject(contract.InputSchema, selected.InputSchema)
	resolved.UISchema = mergeModelOperationContractObject(contract.UISchema, selected.UISchema)
	resolved.MaterialSchema = mergeModelOperationContractObject(contract.MaterialSchema, selected.MaterialSchema)
	resolved.ParameterDefaults = mergeModelOperationContractObject(contract.ParameterDefaults, selected.ParameterDefaults)
	resolved.ParameterOverrides = mergeModelOperationContractObject(contract.ParameterOverrides, selected.ParameterOverrides)
	if selected.RequestContract != nil {
		resolved.RequestContract = *selected.RequestContract
	}
	if selected.PricingRule != nil {
		resolved.PricingRule = *selected.PricingRule
	}
	if selected.DispatchPath != "" {
		resolved.DispatchPath = selected.DispatchPath
	}
	if selected.PollPath != "" {
		resolved.PollPath = selected.PollPath
	}
	if selected.ResponseContract != "" {
		resolved.ResponseContract = selected.ResponseContract
	}
	return &resolved, nil
}

// NormalizeAndValidateModelOperationParameters applies the effective contract's
// defaults and forced overrides while returning only declared schema fields.
func NormalizeAndValidateModelOperationParameters(contract *ModelOperationEffectiveContract, input map[string]interface{}) (map[string]interface{}, error) {
	if contract == nil {
		return nil, errors.New("model operation contract is required")
	}
	if err := validateModelOperationInputSchema(contract.InputSchema); err != nil {
		return nil, err
	}
	properties, _ := contractObject(contract.InputSchema["properties"])
	rejectUnknownInput := false
	if additionalProperties, exists := contract.InputSchema["additionalProperties"].(bool); exists {
		rejectUnknownInput = !additionalProperties
	}
	result := make(map[string]interface{}, len(properties))

	for field, rawSchema := range properties {
		fieldSchema, _ := contractObject(rawSchema)
		if value, exists := fieldSchema["default"]; exists {
			result[field] = value
		}
	}
	for _, values := range []struct {
		name   string
		values map[string]interface{}
		input  bool
	}{
		{name: "parameter_defaults", values: contract.ParameterDefaults},
		{name: "input", values: input, input: true},
		{name: "parameter_overrides", values: contract.ParameterOverrides},
	} {
		for field, value := range values.values {
			fieldSchema, known := contractObject(properties[field])
			if !known {
				if values.input && !rejectUnknownInput {
					continue
				}
				return nil, fmt.Errorf("%s contains unknown parameter %s", values.name, field)
			}
			if err := validateModelOperationParameterValue(field, fieldSchema, value, true); err != nil {
				return nil, fmt.Errorf("%s: %w", values.name, err)
			}
			result[field] = value
		}
	}
	if required, _ := contract.InputSchema["required"].([]interface{}); len(required) > 0 {
		for _, rawField := range required {
			field, _ := rawField.(string)
			if _, exists := result[field]; !exists {
				return nil, fmt.Errorf("required parameter %s is missing", field)
			}
		}
	}
	return result, nil
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
		now := common.GetTimestamp()
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if binding.ContractHash != contractHash || binding.ContractVersion != contractVersion || storedOverrides != normalizedOverrides {
				if err := tx.Model(binding).Updates(map[string]interface{}{
					"overrides":        normalizedOverrides,
					"contract_version": contractVersion,
					"contract_hash":    contractHash,
					"updated_time":     now,
				}).Error; err != nil {
					return err
				}
				binding.Overrides = normalizedOverrides
				binding.ContractVersion = contractVersion
				binding.ContractHash = contractHash
				binding.UpdatedTime = now
			}
			return appendModelOperationBindingRevisionIfChanged(tx, binding, now)
		}); err != nil {
			return err
		}
	}
	return nil
}
