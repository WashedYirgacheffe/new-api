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
