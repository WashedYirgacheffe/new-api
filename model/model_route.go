package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelRoutePolicyLowestEffectiveCostFailover = "lowest_effective_cost_failover"
	ModelRouteCompatibilityCompatible           = "compatible"
	ModelRouteCompatibilityIncompatible         = "incompatible"
)

var (
	ErrModelRouteConflict            = errors.New("model route optimistic lock conflict")
	ErrModelRouteGroupNotFound       = errors.New("model route group not found")
	ErrModelRouteContractUnavailable = errors.New("model route contract unavailable")
)

// ModelRouteOperationLock serializes route graph writes for one operation.
// A route group row cannot provide this lock when the graph is empty, so the
// operation row is created before every save/delete transaction.
type ModelRouteOperationLock struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	Operation string `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex"`
}

type ModelRouteGroup struct {
	Id             int                `json:"id"`
	CanonicalModel string             `json:"canonical_model" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_route_group,priority:1;index"`
	Operation      string             `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex:uk_model_route_group,priority:2;index"`
	Policy         string             `json:"policy" gorm:"type:varchar(64);not null"`
	Enabled        bool               `json:"enabled" gorm:"not null"`
	Version        int                `json:"version" gorm:"not null"`
	RouteHash      string             `json:"route_hash" gorm:"type:varchar(64);not null;index"`
	CreatedTime    int64              `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64              `json:"updated_time" gorm:"bigint"`
	Targets        []ModelRouteTarget `json:"targets" gorm:"-"`
}

type ModelRouteTarget struct {
	Id                      int      `json:"id"`
	GroupId                 int      `json:"group_id" gorm:"not null;uniqueIndex:uk_model_route_target,priority:1;index"`
	TargetModel             string   `json:"target_model" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_route_target,priority:2;index"`
	Priority                int      `json:"priority" gorm:"not null;index"`
	TieBreaker              int      `json:"tie_breaker" gorm:"not null"`
	Enabled                 bool     `json:"enabled" gorm:"not null"`
	RetryableErrorCodesJSON string   `json:"-" gorm:"column:retryable_error_codes;type:text;not null"`
	RetryableErrorCodes     []string `json:"retryable_error_codes" gorm:"-"`
	CompatibilityStatus     string   `json:"compatibility_status" gorm:"type:varchar(32);not null;index"`
	CompatibilityReason     string   `json:"compatibility_reason,omitempty" gorm:"-"`
	CreatedTime             int64    `json:"created_time" gorm:"bigint"`
	UpdatedTime             int64    `json:"updated_time" gorm:"bigint"`
}

type ModelRouteGroupPayload struct {
	CanonicalModel string                    `json:"canonical_model"`
	Operation      string                    `json:"operation"`
	Policy         string                    `json:"policy"`
	Enabled        *bool                     `json:"enabled"`
	Targets        []ModelRouteTargetPayload `json:"targets"`
}

type ModelRouteTargetPayload struct {
	TargetModel         string   `json:"target_model"`
	Priority            int      `json:"priority"`
	TieBreaker          int      `json:"tie_breaker"`
	Enabled             *bool    `json:"enabled"`
	RetryableErrorCodes []string `json:"retryable_error_codes"`
	CompatibilityStatus string   `json:"compatibility_status,omitempty"`
}

type ModelRouteRelations struct {
	ModelName string            `json:"model_name"`
	Operation string            `json:"operation,omitempty"`
	Incoming  []ModelRouteGroup `json:"incoming"`
	Outgoing  []ModelRouteGroup `json:"outgoing"`
}

type modelRouteHashTarget struct {
	TargetModel         string   `json:"target_model"`
	Priority            int      `json:"priority"`
	TieBreaker          int      `json:"tie_breaker"`
	Enabled             bool     `json:"enabled"`
	RetryableErrorCodes []string `json:"retryable_error_codes"`
	CompatibilityStatus string   `json:"compatibility_status"`
}

type modelRouteHashPayload struct {
	CanonicalModel string                 `json:"canonical_model"`
	Operation      string                 `json:"operation"`
	Policy         string                 `json:"policy"`
	Enabled        bool                   `json:"enabled"`
	Targets        []modelRouteHashTarget `json:"targets"`
}

func normalizeModelRouteName(field string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if len(value) > 255 {
		return "", fmt.Errorf("%s must be 255 characters or fewer", field)
	}
	return value, nil
}

func normalizeRetryableErrorCodes(values []string) ([]string, error) {
	if len(values) > 64 {
		return nil, errors.New("retryable_error_codes must contain 64 entries or fewer")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		code, err := normalizeContractIdentifier(value, 64)
		if err != nil {
			return nil, fmt.Errorf("retryable_error_codes: %w", err)
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	sort.Strings(result)
	return result, nil
}

func sortModelRouteTargets(targets []ModelRouteTarget) {
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Priority != targets[j].Priority {
			return targets[i].Priority < targets[j].Priority
		}
		if targets[i].TieBreaker != targets[j].TieBreaker {
			return targets[i].TieBreaker < targets[j].TieBreaker
		}
		return targets[i].TargetModel < targets[j].TargetModel
	})
}

func normalizeModelRouteGroupPayload(payload ModelRouteGroupPayload) (ModelRouteGroup, error) {
	canonicalModel, err := normalizeModelRouteName("canonical_model", payload.CanonicalModel)
	if err != nil {
		return ModelRouteGroup{}, err
	}
	operation, err := normalizeContractIdentifier(payload.Operation, 64)
	if err != nil {
		return ModelRouteGroup{}, fmt.Errorf("operation: %w", err)
	}
	policy := strings.ToLower(strings.TrimSpace(payload.Policy))
	if policy == "" {
		policy = ModelRoutePolicyLowestEffectiveCostFailover
	}
	if policy != ModelRoutePolicyLowestEffectiveCostFailover {
		return ModelRouteGroup{}, fmt.Errorf("unsupported model route policy %q", payload.Policy)
	}
	if len(payload.Targets) == 0 {
		return ModelRouteGroup{}, errors.New("at least one route target is required")
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	now := common.GetTimestamp()
	targets := make([]ModelRouteTarget, 0, len(payload.Targets))
	seenTargets := make(map[string]struct{}, len(payload.Targets))
	for index, input := range payload.Targets {
		targetModel, err := normalizeModelRouteName(fmt.Sprintf("targets[%d].target_model", index), input.TargetModel)
		if err != nil {
			return ModelRouteGroup{}, err
		}
		if targetModel == canonicalModel {
			return ModelRouteGroup{}, fmt.Errorf("route target %s must not reference its canonical model", targetModel)
		}
		if _, exists := seenTargets[targetModel]; exists {
			return ModelRouteGroup{}, fmt.Errorf("duplicate route target %s", targetModel)
		}
		seenTargets[targetModel] = struct{}{}
		if input.Priority < 0 || input.Priority > 1_000_000 {
			return ModelRouteGroup{}, fmt.Errorf("targets[%d].priority must be between 0 and 1000000", index)
		}
		if input.TieBreaker < 0 || input.TieBreaker > 1_000_000 {
			return ModelRouteGroup{}, fmt.Errorf("targets[%d].tie_breaker must be between 0 and 1000000", index)
		}
		targetEnabled := true
		if input.Enabled != nil {
			targetEnabled = *input.Enabled
		}
		compatibilityStatus := strings.ToLower(strings.TrimSpace(input.CompatibilityStatus))
		if compatibilityStatus == "" {
			compatibilityStatus = ModelRouteCompatibilityCompatible
		}
		switch compatibilityStatus {
		case ModelRouteCompatibilityCompatible, ModelRouteCompatibilityIncompatible:
			// The status is recomputed from the current contracts below. Accepting
			// an incompatible value here lets the UI submit a stale read payload.
		default:
			return ModelRouteGroup{}, fmt.Errorf("targets[%d].compatibility_status must be %s or %s", index, ModelRouteCompatibilityCompatible, ModelRouteCompatibilityIncompatible)
		}
		retryableErrorCodes, err := normalizeRetryableErrorCodes(input.RetryableErrorCodes)
		if err != nil {
			return ModelRouteGroup{}, fmt.Errorf("targets[%d]: %w", index, err)
		}
		retryableErrorCodesJSON, err := common.Marshal(retryableErrorCodes)
		if err != nil {
			return ModelRouteGroup{}, err
		}
		targets = append(targets, ModelRouteTarget{
			TargetModel:             targetModel,
			Priority:                input.Priority,
			TieBreaker:              input.TieBreaker,
			Enabled:                 targetEnabled,
			RetryableErrorCodesJSON: string(retryableErrorCodesJSON),
			RetryableErrorCodes:     retryableErrorCodes,
			// Persisted route rows always carry the last known compatible state;
			// the read path recomputes it when contracts change.
			CompatibilityStatus: ModelRouteCompatibilityCompatible,
			CreatedTime:         now,
			UpdatedTime:         now,
		})
	}
	sortModelRouteTargets(targets)
	return ModelRouteGroup{
		CanonicalModel: canonicalModel,
		Operation:      operation,
		Policy:         policy,
		Enabled:        enabled,
		Targets:        targets,
	}, nil
}

func computeModelRouteHash(group ModelRouteGroup) (string, error) {
	targets := make([]modelRouteHashTarget, 0, len(group.Targets))
	for _, target := range group.Targets {
		targets = append(targets, modelRouteHashTarget{
			TargetModel:         target.TargetModel,
			Priority:            target.Priority,
			TieBreaker:          target.TieBreaker,
			Enabled:             target.Enabled,
			RetryableErrorCodes: append([]string(nil), target.RetryableErrorCodes...),
			CompatibilityStatus: target.CompatibilityStatus,
		})
	}
	payload, err := common.Marshal(modelRouteHashPayload{
		CanonicalModel: group.CanonicalModel,
		Operation:      group.Operation,
		Policy:         group.Policy,
		Enabled:        group.Enabled,
		Targets:        targets,
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(common.Sha256Raw(payload)), nil
}

func loadEnabledModelRouteContract(tx *gorm.DB, modelName string, operation string) (*ModelOperationEffectiveContract, error) {
	var modelCount int64
	if err := tx.Model(&Model{}).Where("model_name = ? AND status = ?", modelName, 1).Count(&modelCount).Error; err != nil {
		return nil, err
	}
	if modelCount == 0 {
		return nil, fmt.Errorf("%w: enabled model %s does not exist", ErrModelRouteContractUnavailable, modelName)
	}

	var binding ModelOperationBinding
	if err := tx.Where("model_name = ? AND operation = ? AND enabled = ?", modelName, operation, true).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: enabled operation binding %s:%s does not exist", ErrModelRouteContractUnavailable, modelName, operation)
		}
		return nil, err
	}
	var profile ModelOperationProfile
	if err := tx.Where("profile_key = ?", binding.ProfileKey).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: profile %s does not exist", ErrModelRouteContractUnavailable, binding.ProfileKey)
		}
		return nil, err
	}
	var version ModelOperationProfileVersion
	if err := tx.Where(
		"profile_id = ? AND version = ? AND status = ?",
		profile.Id,
		binding.ProfileVersion,
		ModelOperationProfileStatusPublished,
	).First(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: published operation contract %s:%s does not exist", ErrModelRouteContractUnavailable, modelName, operation)
		}
		return nil, err
	}
	if version.Operation != operation {
		return nil, fmt.Errorf("%w: operation binding %s:%s references contract operation %s", ErrModelRouteContractUnavailable, modelName, operation, version.Operation)
	}
	contract, err := BuildModelOperationEffectiveContract(binding, &profile, &version)
	if err != nil {
		return nil, fmt.Errorf("%w: build effective contract for %s:%s: %v", ErrModelRouteContractUnavailable, modelName, operation, err)
	}
	return contract, nil
}

func routeComparableJSON(value interface{}) (string, error) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if typed == nil {
			typed = map[string]interface{}{}
		}
		payload, err := common.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	case map[string]string:
		if typed == nil {
			typed = map[string]string{}
		}
		payload, err := common.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	default:
		payload, err := common.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}
}

func routeComparableRequestContract(contract ModelOperationRequestContract) map[string]interface{} {
	fieldMap := contract.FieldMap
	if fieldMap == nil {
		fieldMap = map[string]string{}
	}
	coercions := contract.Coercions
	if coercions == nil {
		coercions = map[string]string{}
	}
	return map[string]interface{}{
		"adapter":   contract.Adapter,
		"field_map": fieldMap,
		"coercions": coercions,
	}
}

func routeSchemaTypes(schema map[string]interface{}) (map[string]struct{}, error) {
	types, err := modelOperationParameterSchemaTypes(schema)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(types))
	for _, item := range types {
		result[item] = struct{}{}
	}
	return result, nil
}

func routeSchemaBoundCompatible(source, target map[string]interface{}, field string, lower bool) (bool, string) {
	sourceValue, sourceSet := modelOperationNumericParameterValue(source[field])
	targetValue, targetSet := modelOperationNumericParameterValue(target[field])
	if !targetSet {
		if _, exists := target[field]; exists {
			return false, fmt.Sprintf("incompatible %s: target bound is invalid", field)
		}
		if !sourceSet {
			return true, ""
		}
		if lower {
			return true, ""
		}
		// An omitted upper bound is unbounded and therefore accepts the source.
		return true, ""
	}
	if !sourceSet {
		return false, fmt.Sprintf("incompatible %s: target adds a narrower bound", field)
	}
	if lower && targetValue > sourceValue {
		return false, fmt.Sprintf("incompatible %s: target is narrower", field)
	}
	if !lower && targetValue < sourceValue {
		return false, fmt.Sprintf("incompatible %s: target is narrower", field)
	}
	return true, ""
}

func routeInputSchemaCompatible(source, target map[string]interface{}, targetContract *ModelOperationEffectiveContract) (bool, string) {
	var parameterDefaults, parameterOverrides map[string]interface{}
	if targetContract != nil {
		parameterDefaults = targetContract.ParameterDefaults
		parameterOverrides = targetContract.ParameterOverrides
	}
	return routeParameterSchemaCompatible(source, target, "input_schema", parameterDefaults, parameterOverrides, true)
}

func routeParameterSchemaCompatible(
	source map[string]interface{},
	target map[string]interface{},
	path string,
	targetDefaults map[string]interface{},
	targetOverrides map[string]interface{},
	root bool,
) (bool, string) {
	sourceTypes, err := routeSchemaTypes(source)
	if err != nil {
		return false, fmt.Sprintf("incompatible %s type: %v", path, err)
	}
	targetTypes, err := routeSchemaTypes(target)
	if err != nil {
		return false, fmt.Sprintf("incompatible %s type: %v", path, err)
	}
	for sourceType := range sourceTypes {
		if _, exists := targetTypes[sourceType]; exists {
			continue
		}
		if sourceType == "integer" {
			if _, numberTarget := targetTypes["number"]; numberTarget {
				continue
			}
		}
		return false, fmt.Sprintf("incompatible %s type: target omits %s", path, sourceType)
	}
	if sourceEnum, exists := source["enum"]; exists {
		sourceValues, ok := sourceEnum.([]interface{})
		if !ok {
			return false, fmt.Sprintf("incompatible %s enum", path)
		}
		if targetEnum, targetHasEnum := target["enum"]; targetHasEnum {
			targetValues, ok := targetEnum.([]interface{})
			if !ok || len(targetValues) == 0 {
				return false, fmt.Sprintf("incompatible %s enum", path)
			}
			for _, sourceValue := range sourceValues {
				found := false
				for _, targetValue := range targetValues {
					for sourceType := range sourceTypes {
						if modelOperationParameterValuesEqual(sourceType, sourceValue, targetValue) {
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					return false, fmt.Sprintf("incompatible %s enum: target omits a source value", path)
				}
			}
		}
	} else if _, targetHasEnum := target["enum"]; targetHasEnum {
		return false, fmt.Sprintf("incompatible %s enum: target narrows an unbounded source", path)
	}
	for _, bound := range []struct {
		name  string
		lower bool
	}{
		{"minimum", true}, {"minLength", true}, {"minItems", true},
		{"maximum", false}, {"maxLength", false}, {"maxItems", false},
	} {
		compatible, reason := routeSchemaBoundCompatible(source, target, bound.name, bound.lower)
		if !compatible {
			return false, fmt.Sprintf("incompatible %s: %s", path, reason)
		}
	}

	if _, sourceObject := sourceTypes["object"]; sourceObject {
		sourceAdditional, sourceAdditionalSet := source["additionalProperties"].(bool)
		targetAdditional, targetAdditionalSet := target["additionalProperties"].(bool)
		if !sourceAdditionalSet {
			sourceAdditional = true
		}
		if !targetAdditionalSet {
			targetAdditional = true
		}
		if sourceAdditional && !targetAdditional {
			return false, fmt.Sprintf("incompatible %s.additionalProperties: target rejects source extras", path)
		}
		sourceProperties, sourcePropertiesSet := contractObject(source["properties"])
		targetProperties, targetPropertiesSet := contractObject(target["properties"])
		if sourcePropertiesSet && !targetPropertiesSet {
			if !targetAdditional {
				return false, fmt.Sprintf("incompatible %s.properties: target rejects source properties", path)
			}
		} else if sourcePropertiesSet {
			for field, rawSourceField := range sourceProperties {
				sourceField, sourceFieldOK := contractObject(rawSourceField)
				targetField, targetFieldOK := contractObject(targetProperties[field])
				if !sourceFieldOK || !targetFieldOK {
					return false, fmt.Sprintf("incompatible %s.%s", path, field)
				}
				if compatible, reason := routeParameterSchemaCompatible(
					sourceField,
					targetField,
					path+"."+field,
					targetDefaults,
					targetOverrides,
					false,
				); !compatible {
					return false, reason
				}
			}
		}

		sourceRequired, _ := source["required"].([]interface{})
		targetRequired, _ := target["required"].([]interface{})
		sourceRequiredSet := make(map[string]struct{}, len(sourceRequired))
		for _, raw := range sourceRequired {
			if field, ok := raw.(string); ok {
				sourceRequiredSet[field] = struct{}{}
			}
		}
		for _, raw := range targetRequired {
			field, ok := raw.(string)
			if !ok {
				continue
			}
			if _, requiredBySource := sourceRequiredSet[field]; requiredBySource {
				continue
			}
			targetField, _ := contractObject(targetProperties[field])
			_, hasDefault := targetField["default"]
			hasForced := false
			if root {
				_, hasForced = targetDefaults[field]
				if !hasForced {
					_, hasForced = targetOverrides[field]
				}
			}
			if !hasDefault && !hasForced {
				return false, fmt.Sprintf("incompatible %s.required: target adds %s without default or forced value", path, field)
			}
		}
	}

	if _, sourceArray := sourceTypes["array"]; sourceArray {
		sourceItems, sourceItemsSet := contractObject(source["items"])
		targetItems, targetItemsSet := contractObject(target["items"])
		if !sourceItemsSet && targetItemsSet {
			return false, fmt.Sprintf("incompatible %s.items: target narrows unbounded source items", path)
		}
		if sourceItemsSet && targetItemsSet {
			if compatible, reason := routeParameterSchemaCompatible(
				sourceItems,
				targetItems,
				path+"[]",
				targetDefaults,
				targetOverrides,
				false,
			); !compatible {
				return false, reason
			}
		}
	}
	return true, ""
}

func routeMaterialNumber(rule map[string]interface{}, field string, fallback float64) (float64, bool) {
	if value, exists := rule[field]; exists {
		return modelOperationNumericParameterValue(value)
	}
	return fallback, false
}

func routeMIMETypeContains(target string, source string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	source = strings.ToLower(strings.TrimSpace(source))
	if target == source || target == "*/*" {
		return true
	}
	targetFamily, targetSubtype, targetOK := strings.Cut(target, "/")
	sourceFamily, _, sourceOK := strings.Cut(source, "/")
	return targetOK && sourceOK && targetSubtype == "*" && targetFamily == sourceFamily
}

func routeMaterialCompatible(source, target map[string]interface{}, materialType string) (bool, string) {
	sourceMin, sourceMinSet := routeMaterialNumber(source, "min_items", 0)
	targetMin, targetMinSet := routeMaterialNumber(target, "min_items", 0)
	if targetMinSet && (!sourceMinSet || targetMin > sourceMin) {
		return false, fmt.Sprintf("incompatible material_schema.%s.min_items", materialType)
	}
	sourceMax, sourceMaxSet := routeMaterialNumber(source, "max_items", -1)
	targetMax, targetMaxSet := routeMaterialNumber(target, "max_items", -1)
	if sourceMaxSet {
		if targetMaxSet && targetMax < sourceMax {
			return false, fmt.Sprintf("incompatible material_schema.%s.max_items", materialType)
		}
	} else if targetMaxSet {
		return false, fmt.Sprintf("incompatible material_schema.%s.max_items: target adds a limit", materialType)
	}
	for _, field := range []string{"max_size_mb", "max_total_duration"} {
		sourceValue, sourceSet := modelOperationNumericParameterValue(source[field])
		targetValue, targetSet := modelOperationNumericParameterValue(target[field])
		if sourceSet {
			if targetSet && targetValue < sourceValue {
				return false, fmt.Sprintf("incompatible material_schema.%s.%s", materialType, field)
			}
		} else if targetSet {
			return false, fmt.Sprintf("incompatible material_schema.%s.%s: target adds a limit", materialType, field)
		}
	}
	for _, field := range []string{"mime_types", "roles"} {
		sourceValues, sourceSet := source[field].([]interface{})
		targetValues, targetSet := target[field].([]interface{})
		if !sourceSet {
			if targetSet {
				return false, fmt.Sprintf("incompatible material_schema.%s.%s: target narrows an unbounded source", materialType, field)
			}
			continue
		}
		if !targetSet {
			continue
		}
		targetTexts := make([]string, 0, len(targetValues))
		seen := make(map[string]struct{}, len(targetValues))
		for _, value := range targetValues {
			if text, ok := value.(string); ok {
				normalized := strings.ToLower(strings.TrimSpace(text))
				targetTexts = append(targetTexts, normalized)
				seen[normalized] = struct{}{}
			}
		}
		for _, value := range sourceValues {
			text, ok := value.(string)
			if !ok {
				return false, fmt.Sprintf("incompatible material_schema.%s.%s", materialType, field)
			}
			normalized := strings.ToLower(strings.TrimSpace(text))
			if field == "mime_types" {
				covered := false
				for _, targetText := range targetTexts {
					if routeMIMETypeContains(targetText, normalized) {
						covered = true
						break
					}
				}
				if covered {
					continue
				}
			}
			if _, exists := seen[normalized]; !exists {
				return false, fmt.Sprintf("incompatible material_schema.%s.%s: target omits a source value", materialType, field)
			}
		}
	}
	for _, field := range []string{"request_field", "transport"} {
		sourceValue, sourceSet := source[field].(string)
		targetValue, targetSet := target[field].(string)
		if sourceSet && strings.TrimSpace(sourceValue) != "" && (!targetSet || strings.TrimSpace(targetValue) != strings.TrimSpace(sourceValue)) {
			return false, fmt.Sprintf("incompatible material_schema.%s.%s", materialType, field)
		}
	}
	return true, ""
}

func routeMaterialSchemaCompatible(source, target map[string]interface{}) (bool, string) {
	for materialType, rawSource := range source {
		sourceRule, sourceOK := contractObject(rawSource)
		if !sourceOK {
			return false, fmt.Sprintf("incompatible material_schema.%s", materialType)
		}
		targetRule, targetOK := contractObject(target[materialType])
		if !targetOK {
			maxItems, hasMax := modelOperationNumericParameterValue(sourceRule["max_items"])
			if hasMax && maxItems == 0 {
				continue
			}
			return false, fmt.Sprintf("incompatible material_schema.%s: target omits material rule", materialType)
		}
		if compatible, reason := routeMaterialCompatible(sourceRule, targetRule, materialType); !compatible {
			return false, reason
		}
	}
	for materialType, rawTarget := range target {
		if _, sourceExists := source[materialType]; sourceExists {
			continue
		}
		targetRule, targetOK := contractObject(rawTarget)
		if !targetOK || len(targetRule) > 0 {
			return false, fmt.Sprintf("incompatible material_schema.%s: target adds a material restriction", materialType)
		}
	}
	return true, ""
}

func routeParameterConfigurationCompatible(source *ModelOperationEffectiveContract, target *ModelOperationEffectiveContract) (bool, string) {
	sourceProperties, _ := contractObject(source.InputSchema["properties"])
	targetProperties, _ := contractObject(target.InputSchema["properties"])
	for field, rawTargetField := range targetProperties {
		targetField, _ := contractObject(rawTargetField)
		defaultValue, hasDefault := targetField["default"]
		if !hasDefault {
			continue
		}
		sourceField, sourceOK := contractObject(sourceProperties[field])
		if !sourceOK {
			continue
		}
		if err := validateModelOperationParameterValue(field, sourceField, defaultValue, true); err != nil {
			return false, fmt.Sprintf("incompatible parameter default %s: %v", field, err)
		}
	}
	for _, values := range []map[string]interface{}{target.ParameterDefaults, target.ParameterOverrides} {
		for field, value := range values {
			sourceField, sourceOK := contractObject(sourceProperties[field])
			targetField, targetOK := contractObject(targetProperties[field])
			if !sourceOK || !targetOK {
				return false, fmt.Sprintf("incompatible parameter configuration %s", field)
			}
			if err := validateModelOperationParameterValue(field, sourceField, value, true); err != nil {
				return false, fmt.Sprintf("incompatible parameter configuration %s: %v", field, err)
			}
			if err := validateModelOperationParameterValue(field, targetField, value, true); err != nil {
				return false, fmt.Sprintf("incompatible parameter configuration %s: %v", field, err)
			}
		}
	}
	return true, ""
}

func modelRouteContractsCompatible(canonical *ModelOperationEffectiveContract, target *ModelOperationEffectiveContract) (bool, string) {
	if canonical == nil || target == nil {
		return false, "effective contract is unavailable"
	}
	for _, comparison := range []struct {
		name  string
		left  interface{}
		right interface{}
	}{
		{"request_contract", routeComparableRequestContract(canonical.RequestContract), routeComparableRequestContract(target.RequestContract)},
		{"dispatch_path", canonical.DispatchPath, target.DispatchPath},
		{"poll_path", canonical.PollPath, target.PollPath},
		{"endpoint_type", canonical.EndpointType, target.EndpointType},
		{"execution_mode", canonical.ExecutionMode, target.ExecutionMode},
		{"response_contract", canonical.ResponseContract, target.ResponseContract},
	} {
		left, err := routeComparableJSON(comparison.left)
		if err != nil {
			return false, fmt.Sprintf("incompatible %s: %v", comparison.name, err)
		}
		right, err := routeComparableJSON(comparison.right)
		if err != nil {
			return false, fmt.Sprintf("incompatible %s: %v", comparison.name, err)
		}
		if left != right {
			return false, fmt.Sprintf("incompatible %s: differs from canonical contract", comparison.name)
		}
	}
	canonicalInput, targetInput := canonical.InputSchema, target.InputSchema
	if compatible, reason := routeInputSchemaCompatible(canonicalInput, targetInput, target); !compatible {
		return false, reason
	}
	if compatible, reason := routeMaterialSchemaCompatible(canonical.MaterialSchema, target.MaterialSchema); !compatible {
		return false, reason
	}
	if compatible, reason := routeParameterConfigurationCompatible(canonical, target); !compatible {
		return false, reason
	}
	return true, ""
}

func validateModelRouteContracts(tx *gorm.DB, group *ModelRouteGroup) error {
	if group == nil {
		return errors.New("model route group is required")
	}
	canonicalContract, err := loadEnabledModelRouteContract(tx, group.CanonicalModel, group.Operation)
	if err != nil {
		return err
	}
	for _, target := range group.Targets {
		targetContract, err := loadEnabledModelRouteContract(tx, target.TargetModel, group.Operation)
		if err != nil {
			return err
		}
		compatible, reason := modelRouteContractsCompatible(canonicalContract, targetContract)
		if !compatible {
			return fmt.Errorf("route target %s is incompatible: %s", target.TargetModel, reason)
		}
	}
	return nil
}

func validateModelRouteGraph(tx *gorm.DB, candidate ModelRouteGroup) error {
	groups := make([]ModelRouteGroup, 0)
	if err := lockForUpdate(tx).Where("operation = ?", candidate.Operation).Find(&groups).Error; err != nil {
		return err
	}
	groupIds := make([]int, 0, len(groups))
	groupsById := make(map[int]ModelRouteGroup, len(groups))
	for _, group := range groups {
		if group.CanonicalModel == candidate.CanonicalModel {
			continue
		}
		groupIds = append(groupIds, group.Id)
		groupsById[group.Id] = group
	}
	var storedTargets []ModelRouteTarget
	if len(groupIds) > 0 {
		if err := tx.Where("group_id IN ?", groupIds).Find(&storedTargets).Error; err != nil {
			return err
		}
	}

	adjacency := make(map[string][]string, len(groups)+1)
	for _, target := range storedTargets {
		group, exists := groupsById[target.GroupId]
		if !exists {
			continue
		}
		adjacency[group.CanonicalModel] = append(adjacency[group.CanonicalModel], target.TargetModel)
	}
	for _, target := range candidate.Targets {
		adjacency[candidate.CanonicalModel] = append(adjacency[candidate.CanonicalModel], target.TargetModel)
	}
	for node := range adjacency {
		sort.Strings(adjacency[node])
	}

	state := make(map[string]uint8, len(adjacency))
	stack := make([]string, 0, len(adjacency))
	stackIndex := make(map[string]int, len(adjacency))
	var visit func(string) error
	visit = func(node string) error {
		state[node] = 1
		stackIndex[node] = len(stack)
		stack = append(stack, node)
		for _, target := range adjacency[node] {
			switch state[target] {
			case 0:
				if err := visit(target); err != nil {
					return err
				}
			case 1:
				cycle := append([]string(nil), stack[stackIndex[target]:]...)
				cycle = append(cycle, target)
				return fmt.Errorf("model route graph contains cycle: %s", strings.Join(cycle, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		delete(stackIndex, node)
		state[node] = 2
		return nil
	}

	nodes := make([]string, 0, len(adjacency))
	for node := range adjacency {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if state[node] == 0 {
			if err := visit(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func hydrateModelRouteTarget(target *ModelRouteTarget) error {
	if target == nil {
		return errors.New("model route target is required")
	}
	target.RetryableErrorCodes = make([]string, 0)
	if strings.TrimSpace(target.RetryableErrorCodesJSON) == "" {
		return nil
	}
	if err := common.UnmarshalJsonStr(target.RetryableErrorCodesJSON, &target.RetryableErrorCodes); err != nil {
		return fmt.Errorf("decode retryable_error_codes for target %s: %w", target.TargetModel, err)
	}
	return nil
}

func loadModelRouteTargets(tx *gorm.DB, groupIds []int) (map[int][]ModelRouteTarget, error) {
	result := make(map[int][]ModelRouteTarget, len(groupIds))
	if len(groupIds) == 0 {
		return result, nil
	}
	var targets []ModelRouteTarget
	if err := tx.Where("group_id IN ?", groupIds).Find(&targets).Error; err != nil {
		return nil, err
	}
	for index := range targets {
		if err := hydrateModelRouteTarget(&targets[index]); err != nil {
			return nil, err
		}
		result[targets[index].GroupId] = append(result[targets[index].GroupId], targets[index])
	}
	for groupId := range result {
		sortModelRouteTargets(result[groupId])
	}
	return result, nil
}

func attachModelRouteTargets(tx *gorm.DB, groups []ModelRouteGroup) error {
	groupIds := make([]int, 0, len(groups))
	for _, group := range groups {
		groupIds = append(groupIds, group.Id)
	}
	targetsByGroup, err := loadModelRouteTargets(tx, groupIds)
	if err != nil {
		return err
	}
	for index := range groups {
		groups[index].Targets = targetsByGroup[groups[index].Id]
		if groups[index].Targets == nil {
			groups[index].Targets = make([]ModelRouteTarget, 0)
		}
	}
	for index := range groups {
		if err := refreshModelRouteCompatibility(tx, &groups[index]); err != nil {
			return err
		}
	}
	return nil
}

func refreshModelRouteCompatibility(tx *gorm.DB, group *ModelRouteGroup) error {
	if group == nil {
		return errors.New("model route group is required")
	}
	canonicalContract, canonicalErr := loadEnabledModelRouteContract(tx, group.CanonicalModel, group.Operation)
	if canonicalErr != nil && !errors.Is(canonicalErr, ErrModelRouteContractUnavailable) {
		return canonicalErr
	}
	for index := range group.Targets {
		target := &group.Targets[index]
		target.CompatibilityStatus = ModelRouteCompatibilityIncompatible
		target.CompatibilityReason = "canonical contract unavailable"
		if canonicalErr != nil {
			continue
		}
		targetContract, targetErr := loadEnabledModelRouteContract(tx, target.TargetModel, group.Operation)
		if targetErr != nil {
			if !errors.Is(targetErr, ErrModelRouteContractUnavailable) {
				return targetErr
			}
			target.CompatibilityReason = targetErr.Error()
			continue
		}
		compatible, reason := modelRouteContractsCompatible(canonicalContract, targetContract)
		if compatible {
			target.CompatibilityStatus = ModelRouteCompatibilityCompatible
			target.CompatibilityReason = ""
		} else {
			target.CompatibilityReason = reason
		}
	}
	return nil
}

func lockModelRouteOperation(tx *gorm.DB, operation string) error {
	lock := ModelRouteOperationLock{Operation: operation}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	return lockForUpdate(tx).Where("operation = ?", operation).First(&lock).Error
}

func SaveModelRouteGroup(payload ModelRouteGroupPayload, expectedHash string) (*ModelRouteGroup, error) {
	group, err := normalizeModelRouteGroupPayload(payload)
	if err != nil {
		return nil, err
	}
	routeHash, err := computeModelRouteHash(group)
	if err != nil {
		return nil, err
	}
	expectedHash = strings.TrimSpace(expectedHash)
	now := common.GetTimestamp()

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockModelRouteOperation(tx, group.Operation); err != nil {
			return err
		}
		var stored ModelRouteGroup
		storedErr := lockForUpdate(tx).
			Where("canonical_model = ? AND operation = ?", group.CanonicalModel, group.Operation).
			First(&stored).Error
		switch {
		case errors.Is(storedErr, gorm.ErrRecordNotFound):
			if expectedHash != "" {
				return fmt.Errorf("%w: route group does not exist", ErrModelRouteConflict)
			}
		case storedErr != nil:
			return storedErr
		case expectedHash == "" || expectedHash != stored.RouteHash:
			return fmt.Errorf("%w: expected %q, current %q", ErrModelRouteConflict, expectedHash, stored.RouteHash)
		}

		if err := validateModelRouteContracts(tx, &group); err != nil {
			return err
		}
		if err := validateModelRouteGraph(tx, group); err != nil {
			return err
		}

		if storedErr == nil && stored.RouteHash == routeHash {
			stored.Targets = nil
			groups := []ModelRouteGroup{stored}
			if err := attachModelRouteTargets(tx, groups); err != nil {
				return err
			}
			group = groups[0]
			return nil
		}

		if errors.Is(storedErr, gorm.ErrRecordNotFound) {
			group.Version = 1
			group.RouteHash = routeHash
			group.CreatedTime = now
			group.UpdatedTime = now
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
		} else {
			version := stored.Version
			if version <= 0 {
				version = 1
			} else {
				version++
			}
			if err := tx.Model(&stored).Updates(map[string]interface{}{
				"policy":       group.Policy,
				"enabled":      group.Enabled,
				"version":      version,
				"route_hash":   routeHash,
				"updated_time": now,
			}).Error; err != nil {
				return err
			}
			group.Id = stored.Id
			group.Version = version
			group.RouteHash = routeHash
			group.CreatedTime = stored.CreatedTime
			group.UpdatedTime = now
			if err := tx.Where("group_id = ?", group.Id).Delete(&ModelRouteTarget{}).Error; err != nil {
				return err
			}
		}

		for index := range group.Targets {
			group.Targets[index].Id = 0
			group.Targets[index].GroupId = group.Id
			group.Targets[index].CreatedTime = now
			group.Targets[index].UpdatedTime = now
		}
		return tx.Create(&group.Targets).Error
	})
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func ListModelRouteGroups(modelName string, operation string, offset int, limit int) ([]ModelRouteGroup, int64, error) {
	modelName = strings.TrimSpace(modelName)
	operation = strings.TrimSpace(operation)
	if operation != "" {
		normalized, err := normalizeContractIdentifier(operation, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("operation: %w", err)
		}
		operation = normalized
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := DB.Model(&ModelRouteGroup{})
	if modelName != "" {
		query = query.Where("canonical_model = ?", modelName)
	}
	if operation != "" {
		query = query.Where("operation = ?", operation)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	groups := make([]ModelRouteGroup, 0)
	if err := query.Order("operation ASC, canonical_model ASC").Offset(offset).Limit(limit).Find(&groups).Error; err != nil {
		return nil, 0, err
	}
	if err := attachModelRouteTargets(DB, groups); err != nil {
		return nil, 0, err
	}
	return groups, total, nil
}

func GetModelRouteRelations(modelName string, operation string) (*ModelRouteRelations, error) {
	var err error
	modelName, err = normalizeModelRouteName("model", modelName)
	if err != nil {
		return nil, err
	}
	operation = strings.TrimSpace(operation)
	if operation != "" {
		operation, err = normalizeContractIdentifier(operation, 64)
		if err != nil {
			return nil, fmt.Errorf("operation: %w", err)
		}
	}

	outgoingQuery := DB.Where("canonical_model = ?", modelName)
	if operation != "" {
		outgoingQuery = outgoingQuery.Where("operation = ?", operation)
	}
	outgoing := make([]ModelRouteGroup, 0)
	if err := outgoingQuery.Order("operation ASC, canonical_model ASC").Find(&outgoing).Error; err != nil {
		return nil, err
	}
	if err := attachModelRouteTargets(DB, outgoing); err != nil {
		return nil, err
	}

	var incomingTargetRows []ModelRouteTarget
	if err := DB.Where("target_model = ?", modelName).Find(&incomingTargetRows).Error; err != nil {
		return nil, err
	}
	incomingGroupIds := make([]int, 0, len(incomingTargetRows))
	seenGroupIds := make(map[int]struct{}, len(incomingTargetRows))
	for _, target := range incomingTargetRows {
		if _, exists := seenGroupIds[target.GroupId]; exists {
			continue
		}
		seenGroupIds[target.GroupId] = struct{}{}
		incomingGroupIds = append(incomingGroupIds, target.GroupId)
	}
	incoming := make([]ModelRouteGroup, 0)
	if len(incomingGroupIds) > 0 {
		incomingQuery := DB.Where("id IN ?", incomingGroupIds)
		if operation != "" {
			incomingQuery = incomingQuery.Where("operation = ?", operation)
		}
		if err := incomingQuery.Order("operation ASC, canonical_model ASC").Find(&incoming).Error; err != nil {
			return nil, err
		}
		if err := attachModelRouteTargets(DB, incoming); err != nil {
			return nil, err
		}
	}
	return &ModelRouteRelations{
		ModelName: modelName,
		Operation: operation,
		Incoming:  incoming,
		Outgoing:  outgoing,
	}, nil
}

func ResolveModelRouteTargets(canonicalModel string, operation string) ([]ModelRouteTarget, error) {
	var err error
	canonicalModel, err = normalizeModelRouteName("canonical_model", canonicalModel)
	if err != nil {
		return nil, err
	}
	operation, err = normalizeContractIdentifier(operation, 64)
	if err != nil {
		return nil, fmt.Errorf("operation: %w", err)
	}
	var group ModelRouteGroup
	if err := DB.Where(
		"canonical_model = ? AND operation = ? AND enabled = ?",
		canonicalModel,
		operation,
		true,
	).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelRouteGroupNotFound
		}
		return nil, err
	}
	groups := []ModelRouteGroup{group}
	if err := attachModelRouteTargets(DB, groups); err != nil {
		return nil, err
	}
	group = groups[0]
	compatibleTargets := make([]ModelRouteTarget, 0, len(group.Targets))
	for _, target := range group.Targets {
		if target.Enabled && target.CompatibilityStatus == ModelRouteCompatibilityCompatible {
			compatibleTargets = append(compatibleTargets, target)
		}
	}
	sortModelRouteTargets(compatibleTargets)
	return compatibleTargets, nil
}

func DeleteModelRouteGroup(canonicalModel string, operation string, expectedHash string) error {
	var err error
	canonicalModel, err = normalizeModelRouteName("canonical_model", canonicalModel)
	if err != nil {
		return err
	}
	operation, err = normalizeContractIdentifier(operation, 64)
	if err != nil {
		return fmt.Errorf("operation: %w", err)
	}
	expectedHash = strings.TrimSpace(expectedHash)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockModelRouteOperation(tx, operation); err != nil {
			return err
		}
		var group ModelRouteGroup
		if err := lockForUpdate(tx).
			Where("canonical_model = ? AND operation = ?", canonicalModel, operation).
			First(&group).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrModelRouteGroupNotFound
			}
			return err
		}
		if expectedHash == "" || expectedHash != group.RouteHash {
			return fmt.Errorf("%w: expected %q, current %q", ErrModelRouteConflict, expectedHash, group.RouteHash)
		}
		if err := tx.Where("group_id = ?", group.Id).Delete(&ModelRouteTarget{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	})
}

// SeedDefaultModelRouteCandidates creates the disabled GPT Image alias map
// for operator visibility. It uses the same directional compatibility gate as
// administrator writes and never enables the group for Relay execution.
func SeedDefaultModelRouteCandidates() error {
	if DB == nil || !DB.Migrator().HasTable(&ModelRouteGroup{}) || !DB.Migrator().HasTable(&ModelRouteTarget{}) {
		return nil
	}
	const operation = "image.generate"
	var existing ModelRouteGroup
	if err := DB.Where("canonical_model = ? AND operation = ?", "deepwl/gpt-image-2-all", operation).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	canonicalContract, err := loadEnabledModelRouteContract(DB, "deepwl/gpt-image-2-all", operation)
	if err != nil {
		if errors.Is(err, ErrModelRouteContractUnavailable) {
			return nil
		}
		return err
	}
	for _, modelName := range []string{"deepwl/gpt-image-2-c", "deepwl/gpt-image-2"} {
		targetContract, err := loadEnabledModelRouteContract(DB, modelName, operation)
		if err != nil {
			if errors.Is(err, ErrModelRouteContractUnavailable) {
				return nil
			}
			return err
		}
		if compatible, _ := modelRouteContractsCompatible(canonicalContract, targetContract); !compatible {
			return nil
		}
	}
	groupEnabled := false
	targetEnabled := true
	_, err = SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/gpt-image-2-all",
		Operation:      operation,
		Policy:         ModelRoutePolicyLowestEffectiveCostFailover,
		Enabled:        &groupEnabled,
		Targets: []ModelRouteTargetPayload{
			{TargetModel: "deepwl/gpt-image-2-c", Priority: 10, TieBreaker: 10, Enabled: &targetEnabled},
			{TargetModel: "deepwl/gpt-image-2", Priority: 20, TieBreaker: 20, Enabled: &targetEnabled},
		},
	}, "")
	if errors.Is(err, ErrModelRouteConflict) {
		// An administrator may have created the same group between the read
		// above and this seed call. Preserve that group unchanged.
		var concurrent ModelRouteGroup
		if lookupErr := DB.Where("canonical_model = ? AND operation = ?", "deepwl/gpt-image-2-all", operation).First(&concurrent).Error; lookupErr == nil {
			return nil
		}
	}
	return err
}
