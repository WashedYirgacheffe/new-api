package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageContractFixture(t *testing.T, overrides string) (*ModelOperationProfile, *ModelOperationProfileVersion, ModelOperationBinding) {
	t.Helper()
	profile := &ModelOperationProfile{
		ProfileKey:  "image.generate.basic",
		DisplayName: "Image generation",
	}
	version := &ModelOperationProfileVersion{
		Version:          2,
		Operation:        "image.generate",
		EndpointType:     "image-generation",
		ExecutionMode:    "sync",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"resolution":{"type":"string","enum":["1K","2K","4K"],"default":"1K"},"n":{"type":"integer","enum":[1,2,4],"default":1,"maximum":4}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"placements":{"prompt":"prompt","resolution":"footer","n":"batch"},"widgets":{"prompt":"textarea","resolution":"menu","n":"segmented"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: "openai-image-generation-v1",
		SmokeTest:        `{"prompt":"test"}`,
		Status:           ModelOperationProfileStatusPublished,
	}
	binding := ModelOperationBinding{
		ModelName:      "deepwl/test-image",
		Operation:      version.Operation,
		ProfileKey:     profile.ProfileKey,
		ProfileVersion: version.Version,
		Overrides:      overrides,
		Enabled:        true,
	}
	return profile, version, binding
}

func TestNormalizeModelOperationBindingOverridesRejectsUnknownField(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(`{"script":"return 0"}`, profile, version)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported override field")
}

func TestNormalizeModelOperationBindingOverridesValidatesRequestMapping(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(
		`{"request_contract":{"adapter":"openai-video","field_map":{},"coercions":{}}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires openai-video endpoint_type")

	_, _, err = normalizeModelOperationBindingOverrides(
		`{"request_contract":{"adapter":"openai-image","field_map":{"unknown":"size"},"coercions":{}}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestNormalizeModelOperationBindingOverridesValidatesMaterialMapping(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(
		`{"material_schema":{"image":{"max_items":1}},"request_contract":{"adapter":"openai-image","field_map":{},"coercions":{}}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires request_field and transport")
}

func TestNormalizeModelOperationBindingOverridesRejectsInvalidMaterialLimits(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)
	tests := []struct {
		name      string
		material  string
		errorText string
	}{
		{name: "items type", material: `{"image":{"max_items":"5"}}`, errorText: "max_items must be a number"},
		{name: "items range", material: `{"image":{"max_items":21}}`, errorText: "between 0 and 20"},
		{name: "mime type", material: `{"image":{"max_items":0,"mime_types":["not-a-mime"]}}`, errorText: "invalid MIME type"},
		{name: "size type", material: `{"image":{"max_items":0,"max_size_mb":"15"}}`, errorText: "max_size_mb must be a number"},
		{name: "size range", material: `{"image":{"max_items":0,"max_size_mb":0}}`, errorText: "greater than 0"},
		{name: "duration range", material: `{"video":{"max_items":0,"max_total_duration":3601}}`, errorText: "at most 3600"},
		{name: "request field type", material: `{"image":{"max_items":0,"request_field":1,"transport":"url"}}`, errorText: "request_field must be a non-empty string"},
		{name: "transport type", material: `{"image":{"max_items":0,"request_field":"images","transport":1}}`, errorText: "transport must be a non-empty string"},
		{name: "mapping pair", material: `{"image":{"max_items":0,"request_field":"images"}}`, errorText: "configured together"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := normalizeModelOperationBindingOverrides(
				`{"material_schema":`+test.material+`}`,
				profile,
				version,
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.errorText)
		})
	}
}

func TestModelOperationContractModesResolveMaterialVariant(t *testing.T) {
	profile, version, binding := imageContractFixture(t, `{}`)
	binding.Overrides = `{
		"request_contract":{"adapter":"openai-image","field_map":{},"coercions":{}},
		"modes":[
			{"id":"text-to-image","default":true},
			{
				"id":"reference-image",
				"when":{"material_slots":["reference_image"]},
				"dispatch_model":"deepwl/reference-image-v2",
				"material_schema":{"image":{"max_items":1,"request_fields":[{"slot":"reference_image","request_field":"image","transport":"base64","value_type":"string","min_items":1,"max_items":1}]}},
				"parameter_overrides":{"n":1}
			}
		]
	}`

	normalized, _, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, version)
	require.NoError(t, err)
	binding.Overrides = normalized
	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	require.NoError(t, err)

	defaultContract, err := ResolveModelOperationContractMode(contract, map[string]interface{}{}, map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, "text-to-image", defaultContract.SelectedMode)

	parameters := map[string]interface{}{"image": "encoded-reference"}
	materialContract, err := ResolveModelOperationContractMode(
		contract,
		parameters,
		DetectModelOperationMaterialSlots(contract, parameters),
	)
	require.NoError(t, err)
	assert.Equal(t, "reference-image", materialContract.SelectedMode)
	assert.Equal(t, "deepwl/reference-image-v2", materialContract.Modes[1].DispatchModel)
	assert.Equal(t, float64(1), materialContract.ParameterOverrides["n"])
	imageRule, ok := materialContract.MaterialSchema["image"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1), imageRule["max_items"])
}

func TestNormalizeModelOperationBindingOverridesRejectsOverlappingModes(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(`{
		"modes":[
			{"id":"default","default":true},
			{"id":"two-k","when":{"parameter_equals":{"resolution":"2K"}}},
			{"id":"two-k-copy","when":{"parameter_equals":{"resolution":"2K"}}}
		]
	}`, profile, version)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlapping conditions")
}

func TestNormalizeModelOperationBindingOverridesAcceptsDocumentedMaterialTransportsAndGroups(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(`{
		"material_schema":{"image":{"max_items":2,"ordered":true,"request_fields":[
			{"slot":"first_frame","request_field":"first_image","transport":"multipart","value_type":"string","max_items":1,"exclusive_group":"image_mode","position":0},
			{"slot":"reference_image","request_field":"image_assets","transport":"asset_id","value_type":"array","max_items":2,"exclusive_group":"image_mode","one_of_group":"image_input","position":1}
		]}},
		"request_contract":{"adapter":"openai-image","field_map":{},"coercions":{}}
	}`, profile, version)

	require.NoError(t, err)
}

func TestModelOperationContractIncludesValidatedPollPath(t *testing.T) {
	profile := &ModelOperationProfile{ProfileKey: "video.generate.basic", DisplayName: "Video generation"}
	version := &ModelOperationProfileVersion{
		Version:          3,
		Operation:        "video.generate",
		EndpointType:     "openai-video",
		ExecutionMode:    "async",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"placements":{"prompt":"prompt"},"widgets":{"prompt":"textarea"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: "openai-video-task-v1",
	}
	binding := ModelOperationBinding{
		ModelName:      "deepwl/test-video",
		Operation:      version.Operation,
		ProfileKey:     profile.ProfileKey,
		ProfileVersion: version.Version,
		Overrides:      `{"request_contract":{"adapter":"openai-video","field_map":{},"coercions":{}},"poll_path":"/v1/video/generations/{task_id}"}`,
		Enabled:        true,
	}

	normalized, _, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, version)
	require.NoError(t, err)
	binding.Overrides = normalized
	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	require.NoError(t, err)
	assert.Equal(t, "/v1/video/generations/{task_id}", contract.PollPath)
}

func TestModelOperationContractAcceptsGeminiImageAdapterAndPath(t *testing.T) {
	profile := &ModelOperationProfile{ProfileKey: "image.generate.gemini-native", DisplayName: "Gemini image"}
	version := &ModelOperationProfileVersion{
		Version:          1,
		Operation:        "image.generate",
		EndpointType:     "gemini",
		ExecutionMode:    "sync",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"order":["prompt"],"widgets":{"prompt":"textarea"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: "gemini-image-generation-v1",
	}

	normalized, _, err := normalizeModelOperationBindingOverrides(
		`{"request_contract":{"adapter":"gemini-image","field_map":{},"coercions":{}},"dispatch_path":"/v1beta/models/{model}:generateContent"}`,
		profile,
		version,
	)

	require.NoError(t, err)
	assert.Contains(t, normalized, `"adapter":"gemini-image"`)
	assert.Contains(t, normalized, `/v1beta/models/{model}:generateContent`)
}

func TestModelOperationContractAcceptsRETaskAdapter(t *testing.T) {
	profile := &ModelOperationProfile{ProfileKey: "re.video.generate", DisplayName: "RE video generation"}
	version := &ModelOperationProfileVersion{
		Version:          1,
		Operation:        "video.generate",
		EndpointType:     "re-task",
		ExecutionMode:    "async",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"order":["prompt"],"widgets":{"prompt":"textarea"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: "re-video-task-v1",
	}

	normalized, _, err := normalizeModelOperationBindingOverrides(
		`{"request_contract":{"adapter":"re-task","field_map":{},"coercions":{}},"dispatch_path":"/v1/re/generations","poll_path":"/v1/re/tasks/{task_id}"}`,
		profile,
		version,
	)

	require.NoError(t, err)
	assert.Contains(t, normalized, `"adapter":"re-task"`)
	assert.Contains(t, normalized, `/v1/re/tasks/{task_id}`)
}

func TestModelOperationContractHashIsCanonical(t *testing.T) {
	first := `{"branding":{"description":"Image model","icon_key":"openai"},"pricing_rule":{"mode":"newapi-base-with-parameter-multipliers","quantity_field":"n","multipliers":[{"field":"resolution","values":{"1K":1,"2K":1.5,"4K":2}}]}}`
	second := `{"pricing_rule":{"multipliers":[{"values":{"4K":2,"2K":1.5,"1K":1},"field":"resolution"}],"quantity_field":"n","mode":"newapi-base-with-parameter-multipliers"},"branding":{"icon_key":"openai","description":"Image model"}}`
	profile, version, firstBinding := imageContractFixture(t, first)
	_, _, secondBinding := imageContractFixture(t, second)

	firstNormalized, _, err := normalizeModelOperationBindingOverrides(firstBinding.Overrides, profile, version)
	require.NoError(t, err)
	secondNormalized, _, err := normalizeModelOperationBindingOverrides(secondBinding.Overrides, profile, version)
	require.NoError(t, err)
	firstBinding.Overrides = firstNormalized
	secondBinding.Overrides = secondNormalized

	firstHash, err := computeModelOperationContractHash(firstBinding, profile, version)
	require.NoError(t, err)
	secondHash, err := computeModelOperationContractHash(secondBinding, profile, version)
	require.NoError(t, err)
	assert.Equal(t, firstHash, secondHash)
}

func TestCalculateModelOperationParameterRatios(t *testing.T) {
	overrides := `{"pricing_rule":{"mode":"newapi-base-with-parameter-multipliers","quantity_field":"n","multipliers":[{"field":"resolution","values":{"1K":1,"2K":1.5,"4K":2}}]}}`
	profile, version, binding := imageContractFixture(t, overrides)
	normalized, _, err := normalizeModelOperationBindingOverrides(binding.Overrides, profile, version)
	require.NoError(t, err)
	binding.Overrides = normalized
	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	require.NoError(t, err)

	ratios, err := CalculateModelOperationParameterRatios(contract, map[string]interface{}{
		"resolution": "2K",
		"n":          float64(2),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"resolution": 1.5, "n": 2}, ratios)

	_, err = CalculateModelOperationParameterRatios(contract, map[string]interface{}{
		"resolution": "8K",
		"n":          float64(1),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolution")
}

func TestValidateModelOperationInputSchemaRejectsInvalidFieldContracts(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]interface{}
		match  string
	}{
		{
			name:   "missing type",
			schema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"size": map[string]interface{}{}}},
			match:  "must define a supported type",
		},
		{
			name: "enum type mismatch",
			schema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
				"seconds": map[string]interface{}{"type": "integer", "enum": []interface{}{float64(4), "8"}},
			}},
			match: "must be of type integer",
		},
		{
			name: "invalid default",
			schema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
				"resolution": map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K"}, "default": "4K"},
			}},
			match: "not an allowed enum value",
		},
		{
			name: "required must be an array",
			schema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
				"required":   "prompt",
			},
			match: "required must be an array",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateModelOperationInputSchema(test.schema)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}

func TestNormalizeAndValidateModelOperationParameters(t *testing.T) {
	contract := &ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"resolution": map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K", "4K"}, "default": "1K"},
				"steps":      map[string]interface{}{"type": "integer", "minimum": float64(1), "maximum": float64(8), "default": float64(2)},
			},
		},
		ParameterDefaults:  map[string]interface{}{"resolution": "2K"},
		ParameterOverrides: map[string]interface{}{"steps": float64(4)},
	}

	normalized, err := NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{
		"resolution": "4K",
		"steps":      float64(6),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"resolution": "4K", "steps": float64(4)}, normalized)

	normalized, err = NormalizeAndValidateModelOperationParameters(contract, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"resolution": "2K", "steps": float64(4)}, normalized)

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"unknown": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"resolution": "8K"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enum")

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"steps": float64(9)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 8")

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"steps": "4"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type integer")

	contract.InputSchema["additionalProperties"] = true
	normalized, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"unknown": true})
	require.NoError(t, err)
	assert.NotContains(t, normalized, "unknown")
}

func TestNormalizeAndValidateModelOperationParametersRequiresDeclaredFields(t *testing.T) {
	contract := &ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string", "minLength": float64(1)},
				"size":   map[string]interface{}{"type": "string", "default": "1024x1024"},
			},
			"required": []interface{}{"prompt"},
		},
		ParameterDefaults:  map[string]interface{}{},
		ParameterOverrides: map[string]interface{}{},
	}

	_, err := NormalizeAndValidateModelOperationParameters(contract, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required parameter prompt")

	normalized, err := NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{"prompt": "draw"})
	require.NoError(t, err)
	assert.Equal(t, "draw", normalized["prompt"])
	assert.Equal(t, "1024x1024", normalized["size"])
}

func TestNormalizeAndValidateModelOperationParametersRecursesIntoObjectsAndArrays(t *testing.T) {
	contract := &ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"config": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"size": map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K"}},
					},
					"required": []interface{}{"size"},
				},
				"frames": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "integer", "minimum": float64(1), "maximum": float64(3)},
				},
			},
		},
		ParameterDefaults:  map[string]interface{}{},
		ParameterOverrides: map[string]interface{}{},
	}

	normalized, err := NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{
		"config": map[string]interface{}{"size": "2K"},
		"frames": []interface{}{float64(1), float64(3)},
	})
	require.NoError(t, err)
	assert.Equal(t, "2K", normalized["config"].(map[string]interface{})["size"])

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{
		"config": map[string]interface{}{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required parameter config.size")

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{
		"config": map[string]interface{}{"size": "1K", "unknown": true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter unknown")

	_, err = NormalizeAndValidateModelOperationParameters(contract, map[string]interface{}{
		"config": map[string]interface{}{"size": "1K"},
		"frames": []interface{}{float64(4)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frames[0]")
}

func TestValidateModelOperationInputSchemaRejectsInvalidNestedSchema(t *testing.T) {
	err := validateModelOperationInputSchema(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"config": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"size": map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K"}, "default": "4K"},
				},
			},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.size")
	assert.Contains(t, err.Error(), "not an allowed enum value")
}

func TestNormalizeModelOperationBindingOverridesValidatesParameterConfiguration(t *testing.T) {
	profile, version, _ := imageContractFixture(t, `{}`)

	_, _, err := normalizeModelOperationBindingOverrides(
		`{"parameter_defaults":{"resolution":"8K"}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter_defaults")

	_, _, err = normalizeModelOperationBindingOverrides(
		`{"parameter_overrides":{"n":5}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter_overrides")
}

func TestModelOperationSchemaModeReplaceRemovesInheritedParameters(t *testing.T) {
	profile, version, binding := imageContractFixture(t, `{}`)
	replacement := `{
		"schema_mode":"replace",
		"input_schema":{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"quality":{"type":"string","enum":["standard","high"],"default":"standard"}},"required":["prompt"],"additionalProperties":false},
		"ui_schema":{"placements":{"prompt":"prompt","quality":"footer"},"widgets":{"prompt":"textarea","quality":"select"}},
		"material_schema":{}
	}`
	normalized, overrides, err := normalizeModelOperationBindingOverrides(replacement, profile, version)
	require.NoError(t, err)
	assert.Equal(t, ModelOperationSchemaModeReplace, overrides.SchemaMode)
	binding.Overrides = normalized

	contract, err := BuildModelOperationEffectiveContract(binding, profile, version)
	require.NoError(t, err)
	assert.Equal(t, ModelOperationSchemaModeReplace, contract.SchemaMode)
	properties, ok := contractObject(contract.InputSchema["properties"])
	require.True(t, ok)
	assert.Contains(t, properties, "prompt")
	assert.Contains(t, properties, "quality")
	assert.NotContains(t, properties, "resolution")
	assert.NotContains(t, properties, "n")

	mergeBinding := binding
	mergeBinding.Overrides = `{}`
	mergeHash, err := computeModelOperationContractHash(mergeBinding, profile, version)
	require.NoError(t, err)
	sameSchemasReplace := `{"schema_mode":"replace","input_schema":` + version.InputSchema + `,"ui_schema":` + version.UISchema + `,"material_schema":` + version.MaterialSchema + `}`
	sameSchemasNormalized, _, err := normalizeModelOperationBindingOverrides(sameSchemasReplace, profile, version)
	require.NoError(t, err)
	replaceBinding := mergeBinding
	replaceBinding.Overrides = sameSchemasNormalized
	replaceHash, err := computeModelOperationContractHash(replaceBinding, profile, version)
	require.NoError(t, err)
	assert.NotEqual(t, mergeHash, replaceHash)

	_, _, err = normalizeModelOperationBindingOverrides(
		`{"schema_mode":"replace","input_schema":{"type":"object","properties":{}},"ui_schema":{}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "material_schema")

	_, _, err = normalizeModelOperationBindingOverrides(
		`{"schema_mode":"replace","input_schema":{"type":"object","properties":{"quality":{"enum":["high"]}},"additionalProperties":false},"ui_schema":{},"material_schema":{}}`,
		profile,
		version,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "supported type")
}

func TestValidateModelOperationMaterialSchemaAcceptsMultipleCanvasSlots(t *testing.T) {
	schema := map[string]interface{}{
		"image": map[string]interface{}{
			"min_items": float64(0),
			"max_items": float64(3),
			"request_fields": []interface{}{
				map[string]interface{}{
					"slot": "reference_image", "request_field": "media", "transport": "url",
					"min_items": float64(0), "max_items": float64(2), "value_type": "array",
					"item_template": map[string]interface{}{"type": "reference_image"}, "url_field": "url",
				},
				map[string]interface{}{
					"slot": "last_frame", "request_field": "media", "transport": "url",
					"min_items": float64(0), "max_items": float64(1), "value_type": "array",
					"item_template": map[string]interface{}{"type": "last_frame"}, "url_field": "url",
				},
			},
		},
	}

	require.NoError(t, validateModelOperationMaterialSchema(schema, true))

	fields := schema["image"].(map[string]interface{})["request_fields"].([]interface{})
	fields[1].(map[string]interface{})["slot"] = "reference_image"
	err := validateModelOperationMaterialSchema(schema, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate slot")
}
