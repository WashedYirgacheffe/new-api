package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const routeTestOperation = "image.generate"

func modelRouteBool(value bool) *bool {
	return &value
}

func setupModelRouteTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelRouteGroup{},
		&ModelRouteTarget{},
		&ModelRouteOperationLock{},
	))
	tables := []interface{}{
		&ModelRouteTarget{},
		&ModelRouteGroup{},
		&ModelRouteOperationLock{},
		&ModelOperationBinding{},
		&ModelOperationProfileVersion{},
		&ModelOperationProfile{},
		&Model{},
	}
	for _, table := range tables {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	t.Cleanup(func() {
		for _, table := range tables {
			DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
		}
	})
}

func insertModelRouteContract(
	t *testing.T,
	modelName string,
	profileKey string,
	endpointType string,
	executionMode string,
	responseContract string,
) {
	t.Helper()
	require.NoError(t, DB.Create(&Model{
		ModelName:   modelName,
		DisplayName: modelName,
		ModelType:   "image",
		Status:      1,
	}).Error)
	profile := ModelOperationProfile{
		ProfileKey:  profileKey,
		DisplayName: profileKey,
	}
	require.NoError(t, DB.Create(&profile).Error)
	version := ModelOperationProfileVersion{
		ProfileId:        profile.Id,
		Version:          1,
		Operation:        routeTestOperation,
		EndpointType:     endpointType,
		ExecutionMode:    executionMode,
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"order":["prompt"],"widgets":{"prompt":"textarea"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: responseContract,
		SmokeTest:        `{"prompt":"test"}`,
		Status:           ModelOperationProfileStatusPublished,
	}
	require.NoError(t, DB.Create(&version).Error)
	require.NoError(t, DB.Create(&ModelOperationBinding{
		ModelName:       modelName,
		Operation:       routeTestOperation,
		ProfileKey:      profile.ProfileKey,
		ProfileVersion:  version.Version,
		ContractVersion: 1,
		ContractHash:    profileKey,
		Overrides:       `{}`,
		Enabled:         true,
	}).Error)
}

func routeTarget(modelName string, priority int, tieBreaker int, errorCodes ...string) ModelRouteTargetPayload {
	return ModelRouteTargetPayload{
		TargetModel:         modelName,
		Priority:            priority,
		TieBreaker:          tieBreaker,
		RetryableErrorCodes: errorCodes,
	}
}

func TestResolveModelRouteTargetsUsesStablePriorityOrder(t *testing.T) {
	setupModelRouteTest(t)
	insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-c", "route.c", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-d", "route.d", "image-generation", "sync", "openai-image-generation-v1")

	group, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets: []ModelRouteTargetPayload{
			routeTarget("deepwl/route-d", 20, 0, "timeout"),
			routeTarget("deepwl/route-c", 10, 2, "upstream_5xx"),
			routeTarget("deepwl/route-b", 10, 1, "timeout", "rate_limit", "timeout"),
		},
	}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, group.Version)

	targets, err := ResolveModelRouteTargets("deepwl/route-a", routeTestOperation)
	require.NoError(t, err)
	require.Len(t, targets, 3)
	assert.Equal(t, []string{"deepwl/route-b", "deepwl/route-c", "deepwl/route-d"}, []string{
		targets[0].TargetModel,
		targets[1].TargetModel,
		targets[2].TargetModel,
	})
	assert.Equal(t, []string{"rate_limit", "timeout"}, targets[0].RetryableErrorCodes)

	relations, err := GetModelRouteRelations("deepwl/route-b", routeTestOperation)
	require.NoError(t, err)
	require.Len(t, relations.Incoming, 1)
	assert.Equal(t, "deepwl/route-a", relations.Incoming[0].CanonicalModel)
	assert.Empty(t, relations.Outgoing)
}

func TestSaveModelRouteGroupRejectsDuplicateAndSelfTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []ModelRouteTargetPayload
		message string
	}{
		{
			name: "duplicate target",
			targets: []ModelRouteTargetPayload{
				routeTarget("deepwl/route-b", 10, 0),
				routeTarget(" deepwl/route-b ", 20, 0),
			},
			message: "duplicate route target",
		},
		{
			name: "self target",
			targets: []ModelRouteTargetPayload{
				routeTarget("deepwl/route-a", 10, 0),
			},
			message: "must not reference its canonical model",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupModelRouteTest(t)
			insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
			insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")

			_, err := SaveModelRouteGroup(ModelRouteGroupPayload{
				CanonicalModel: "deepwl/route-a",
				Operation:      routeTestOperation,
				Targets:        test.targets,
			}, "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestSaveModelRouteGroupRejectsExistingGraphCycle(t *testing.T) {
	setupModelRouteTest(t)
	insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")

	_, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-b", 10, 0)},
	}, "")
	require.NoError(t, err)

	_, err = SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-b",
		Operation:      routeTestOperation,
		Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-a", 10, 0)},
	}, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "route graph contains cycle")
}

func TestSaveModelRouteGroupRejectsIncompatibleEffectiveContract(t *testing.T) {
	tests := []struct {
		name             string
		endpointType     string
		executionMode    string
		responseContract string
		message          string
	}{
		{"endpoint type", "gemini", "sync", "openai-image-generation-v1", "incompatible endpoint_type"},
		{"execution mode", "image-generation", "async", "openai-image-generation-v1", "incompatible execution_mode"},
		{"response contract", "image-generation", "sync", "gemini-image-generation-v1", "incompatible response_contract"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupModelRouteTest(t)
			insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
			insertModelRouteContract(t, "deepwl/route-b", "route.b", test.endpointType, test.executionMode, test.responseContract)

			_, err := SaveModelRouteGroup(ModelRouteGroupPayload{
				CanonicalModel: "deepwl/route-a",
				Operation:      routeTestOperation,
				Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-b", 10, 0)},
			}, "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestSaveModelRouteGroupRejectsInputMaterialAndRequestContractDifferences(t *testing.T) {
	tests := []struct {
		name   string
		update func(t *testing.T)
		needle string
	}{
		{
			name: "input schema",
			update: func(t *testing.T) {
				require.NoError(t, DB.Model(&ModelOperationProfileVersion{}).
					Where("operation = ?", routeTestOperation).
					Where("profile_id = (SELECT id FROM model_operation_profiles WHERE profile_key = ?)", "route.b").
					Update("input_schema", `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt","size"],"additionalProperties":false}`).Error)
			},
			needle: "incompatible input_schema",
		},
		{
			name: "material schema",
			update: func(t *testing.T) {
				require.NoError(t, DB.Model(&ModelOperationProfileVersion{}).
					Where("operation = ?", routeTestOperation).
					Where("profile_id = (SELECT id FROM model_operation_profiles WHERE profile_key = ?)", "route.b").
					Update("material_schema", `{"image":{"max_items":1}}`).Error)
			},
			needle: "incompatible material_schema",
		},
		{
			name: "request contract",
			update: func(t *testing.T) {
				require.NoError(t, DB.Model(&ModelOperationBinding{}).
					Where("model_name = ?", "deepwl/route-b").
					Update("overrides", `{"request_contract":{"adapter":"openai-image"}}`).Error)
			},
			needle: "incompatible request_contract",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupModelRouteTest(t)
			insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
			insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")
			test.update(t)
			_, err := SaveModelRouteGroup(ModelRouteGroupPayload{
				CanonicalModel: "deepwl/route-a",
				Operation:      routeTestOperation,
				Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-b", 10, 0)},
			}, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.needle)
		})
	}
}

func TestRouteInputSchemaCompatibleRecursesIntoObjectAndArrayItems(t *testing.T) {
	source := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"config": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"size": map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K"}},
				},
			},
			"modes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string", "enum": []interface{}{"fast", "quality"}},
			},
		},
	}
	target := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"config": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"size": map[string]interface{}{"type": "string", "enum": []interface{}{"1K"}},
				},
			},
			"modes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string", "enum": []interface{}{"fast", "quality"}},
			},
		},
	}

	compatible, reason := routeInputSchemaCompatible(source, target, &ModelOperationEffectiveContract{})

	assert.False(t, compatible)
	assert.Contains(t, reason, "input_schema.config.size")

	target["properties"].(map[string]interface{})["config"].(map[string]interface{})["properties"].(map[string]interface{})["size"] = map[string]interface{}{
		"type": "string", "enum": []interface{}{"1K", "2K"},
	}
	target["properties"].(map[string]interface{})["modes"].(map[string]interface{})["items"] = map[string]interface{}{
		"type": "string", "enum": []interface{}{"fast"},
	}

	compatible, reason = routeInputSchemaCompatible(source, target, &ModelOperationEffectiveContract{})

	assert.False(t, compatible)
	assert.Contains(t, reason, "input_schema.modes[]")
}

func TestModelRouteRelationsRecomputeCompatibilityAfterContractChange(t *testing.T) {
	setupModelRouteTest(t)
	insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")
	group, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-b", 10, 0)},
	}, "")
	require.NoError(t, err)

	var profile ModelOperationProfile
	require.NoError(t, DB.Where("profile_key = ?", "route.b").First(&profile).Error)
	require.NoError(t, DB.Model(&ModelOperationProfileVersion{}).
		Where("profile_id = ? AND version = ?", profile.Id, 1).
		Update("input_schema", `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt","size"],"additionalProperties":false}`).Error)

	relations, err := GetModelRouteRelations("deepwl/route-a", routeTestOperation)
	require.NoError(t, err)
	require.Len(t, relations.Outgoing, 1)
	require.Len(t, relations.Outgoing[0].Targets, 1)
	assert.Equal(t, ModelRouteCompatibilityIncompatible, relations.Outgoing[0].Targets[0].CompatibilityStatus)
	assert.Contains(t, relations.Outgoing[0].Targets[0].CompatibilityReason, "input_schema")

	resolved, err := ResolveModelRouteTargets("deepwl/route-a", routeTestOperation)
	require.NoError(t, err)
	assert.Empty(t, resolved)
	_ = group
}

func TestSeedDefaultModelRouteCandidatesIsDisabledAndIdempotent(t *testing.T) {
	setupModelRouteTest(t)
	for _, modelName := range []string{"deepwl/gpt-image-2-all", "deepwl/gpt-image-2-c", "deepwl/gpt-image-2"} {
		insertModelRouteContract(t, modelName, strings.ReplaceAll(modelName, "/", "."), "image-generation", "sync", "openai-image-generation-v1")
	}
	for _, profileKey := range []string{"deepwl.gpt-image-2-all", "deepwl.gpt-image-2-c", "deepwl.gpt-image-2"} {
		enum := `["1024x1024","1536x1024"]`
		if profileKey != "deepwl.gpt-image-2-all" {
			enum = `["1024x1024","1536x1024","1024x1536"]`
		}
		require.NoError(t, DB.Model(&ModelOperationProfileVersion{}).
			Where("profile_id = (SELECT id FROM model_operation_profiles WHERE profile_key = ?)", profileKey).
			Update("input_schema", `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string","enum":`+enum+`}},"required":["prompt"],"additionalProperties":false}`).Error)
	}
	require.NoError(t, SeedDefaultModelRouteCandidates())
	require.NoError(t, SeedDefaultModelRouteCandidates())
	var groups []ModelRouteGroup
	require.NoError(t, DB.Where("canonical_model = ? AND operation = ?", "deepwl/gpt-image-2-all", routeTestOperation).Find(&groups).Error)
	require.Len(t, groups, 1)
	assert.False(t, groups[0].Enabled)
	var targets []ModelRouteTarget
	require.NoError(t, DB.Where("group_id = ?", groups[0].Id).Order("priority ASC").Find(&targets).Error)
	require.Len(t, targets, 2)
	assert.Equal(t, "deepwl/gpt-image-2-c", targets[0].TargetModel)
	assert.Equal(t, "deepwl/gpt-image-2", targets[1].TargetModel)
	relations, err := GetModelRouteRelations("deepwl/gpt-image-2-all", routeTestOperation)
	require.NoError(t, err)
	require.Len(t, relations.Outgoing[0].Targets, 2)
	assert.Equal(t, ModelRouteCompatibilityCompatible, relations.Outgoing[0].Targets[0].CompatibilityStatus)
	assert.Equal(t, ModelRouteCompatibilityCompatible, relations.Outgoing[0].Targets[1].CompatibilityStatus)

	_, err = SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/gpt-image-2-c",
		Operation:      routeTestOperation,
		Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/gpt-image-2-all", 10, 0)},
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible input_schema")
}

func TestSaveModelRouteGroupUsesExpectedHashAndStableVersion(t *testing.T) {
	setupModelRouteTest(t)
	insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-c", "route.c", "image-generation", "sync", "openai-image-generation-v1")

	initial, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets: []ModelRouteTargetPayload{
			routeTarget("deepwl/route-c", 20, 0, "timeout", "rate_limit"),
			routeTarget("deepwl/route-b", 10, 0),
		},
	}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, initial.Version)

	idempotent, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: " deepwl/route-a ",
		Operation:      routeTestOperation,
		Policy:         ModelRoutePolicyLowestEffectiveCostFailover,
		Enabled:        modelRouteBool(true),
		Targets: []ModelRouteTargetPayload{
			{
				TargetModel:         "deepwl/route-b",
				Priority:            10,
				Enabled:             modelRouteBool(true),
				RetryableErrorCodes: []string{},
			},
			{
				TargetModel:         "deepwl/route-c",
				Priority:            20,
				Enabled:             modelRouteBool(true),
				RetryableErrorCodes: []string{"rate_limit", "timeout"},
			},
		},
	}, initial.RouteHash)
	require.NoError(t, err)
	assert.Equal(t, initial.RouteHash, idempotent.RouteHash)
	assert.Equal(t, 1, idempotent.Version)

	_, err = SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets: []ModelRouteTargetPayload{
			routeTarget("deepwl/route-b", 20, 0),
			routeTarget("deepwl/route-c", 10, 0),
		},
	}, "stale-hash")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelRouteConflict))

	updated, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets: []ModelRouteTargetPayload{
			routeTarget("deepwl/route-b", 20, 0),
			routeTarget("deepwl/route-c", 10, 0),
		},
	}, idempotent.RouteHash)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	assert.NotEqual(t, idempotent.RouteHash, updated.RouteHash)

	err = DeleteModelRouteGroup("deepwl/route-a", routeTestOperation, idempotent.RouteHash)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelRouteConflict))
	require.NoError(t, DeleteModelRouteGroup("deepwl/route-a", routeTestOperation, updated.RouteHash))
	_, err = ResolveModelRouteTargets("deepwl/route-a", routeTestOperation)
	assert.ErrorIs(t, err, ErrModelRouteGroupNotFound)
}

func TestSaveModelRouteGroupCreatesOperationLock(t *testing.T) {
	setupModelRouteTest(t)
	insertModelRouteContract(t, "deepwl/route-a", "route.a", "image-generation", "sync", "openai-image-generation-v1")
	insertModelRouteContract(t, "deepwl/route-b", "route.b", "image-generation", "sync", "openai-image-generation-v1")
	_, err := SaveModelRouteGroup(ModelRouteGroupPayload{
		CanonicalModel: "deepwl/route-a",
		Operation:      routeTestOperation,
		Targets:        []ModelRouteTargetPayload{routeTarget("deepwl/route-b", 10, 0)},
	}, "")
	require.NoError(t, err)
	var operationLock ModelRouteOperationLock
	require.NoError(t, DB.Where("operation = ?", routeTestOperation).First(&operationLock).Error)
	assert.Equal(t, routeTestOperation, operationLock.Operation)
}

func TestModelRouteDirectionalContractCompatibility(t *testing.T) {
	canonical := &ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string"},
				"size":   map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K"}},
			},
			"required": []interface{}{"prompt"},
		},
		MaterialSchema: map[string]interface{}{
			"image": map[string]interface{}{"max_items": float64(5), "mime_types": []interface{}{"image/png"}},
		},
		RequestContract:   ModelOperationRequestContract{Adapter: "openai-image"},
		ParameterDefaults: map[string]interface{}{}, ParameterOverrides: map[string]interface{}{},
		EndpointType: "image-generation", ExecutionMode: "sync", ResponseContract: "image-v1",
	}
	target := *canonical
	target.InputSchema = map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{"type": "string"},
			"size":   map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K", "4K"}},
		},
		"required": []interface{}{"prompt"},
	}
	target.MaterialSchema = map[string]interface{}{
		"image": map[string]interface{}{"max_items": float64(6), "mime_types": []interface{}{"image/png", "image/jpeg"}},
	}
	compatible, reason := modelRouteContractsCompatible(canonical, &target)
	assert.True(t, compatible, reason)

	reverseCompatible, reason := modelRouteContractsCompatible(&target, canonical)
	assert.False(t, reverseCompatible)
	assert.Contains(t, reason, "incompatible input_schema")

	narrowMaterial := target
	narrowMaterial.MaterialSchema = map[string]interface{}{"image": map[string]interface{}{"max_items": float64(3)}}
	compatible, reason = modelRouteContractsCompatible(canonical, &narrowMaterial)
	assert.False(t, compatible)
	assert.Contains(t, reason, "material_schema.image.max_items")

	extraRequired := target
	extraRequired.InputSchema = map[string]interface{}{
		"type": "object", "additionalProperties": false,
		"properties": map[string]interface{}{
			"prompt":  map[string]interface{}{"type": "string"},
			"size":    map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K", "4K"}},
			"quality": map[string]interface{}{"type": "string", "enum": []interface{}{"low", "high"}},
		},
		"required": []interface{}{"prompt", "quality"},
	}
	compatible, reason = modelRouteContractsCompatible(canonical, &extraRequired)
	assert.False(t, compatible)
	assert.Contains(t, reason, "target adds quality")
}

func TestRouteMaterialMIMECompatibilityUsesDirectionalWildcards(t *testing.T) {
	source := map[string]interface{}{
		"image": map[string]interface{}{"mime_types": []interface{}{"image/png"}},
	}
	target := map[string]interface{}{
		"image": map[string]interface{}{"mime_types": []interface{}{"image/*"}},
	}

	compatible, reason := routeMaterialSchemaCompatible(source, target)
	require.True(t, compatible, reason)

	compatible, _ = routeMaterialSchemaCompatible(target, source)
	require.False(t, compatible)
}
