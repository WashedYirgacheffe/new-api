package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileDispatchReadyAcceptsREAsyncContracts(t *testing.T) {
	tests := []struct {
		operation        string
		responseContract string
	}{
		{operation: "image.generate", responseContract: "re-image-task-v1"},
		{operation: "video.generate", responseContract: "re-video-task-v1"},
		{operation: "audio.generate", responseContract: "re-audio-task-v1"},
		{operation: "text.generate", responseContract: "re-text-task-v1"},
	}

	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			assert.True(t, profileDispatchReady(test.operation, "re-task", "async", test.responseContract))
			assert.False(t, profileDispatchReady(test.operation, "re-task", "sync", test.responseContract))
		})
	}
	assert.False(t, profileDispatchReady("image.generate", "re-task", "async", "re-video-task-v1"))
}

func TestNormalizeModelQuoteParametersRejectsFastFaceUnsupportedResolution(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"resolution": map[string]interface{}{
					"type":    "string",
					"enum":    []interface{}{"480p", "720p"},
					"default": "480p",
				},
			},
		},
		MaterialSchema: map[string]interface{}{},
	}

	normalized, _, err := normalizeModelQuoteParameters(contract, map[string]interface{}{"resolution": "720p"})
	require.NoError(t, err)
	assert.Equal(t, "720p", normalized["resolution"])

	for _, resolution := range []string{"1080p", "4k"} {
		t.Run(resolution, func(t *testing.T) {
			_, _, err := normalizeModelQuoteParameters(contract, map[string]interface{}{"resolution": resolution})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "parameter resolution is not an allowed enum value")
		})
	}
}
