package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfiguredEndpointTypesSupportsArrayAndObject(t *testing.T) {
	assert.Equal(t, []string{"image-generation", "gemini"}, parseConfiguredEndpointTypes(`["image-generation","gemini","image-generation"]`))

	objectEndpoints := parseConfiguredEndpointTypes(`{"openai-video":{"path":"/v1/videos","method":"POST"},"gemini":"/v1beta/models/{model}:generateContent","ignored":false}`)
	assert.ElementsMatch(t, []string{"openai-video", "gemini"}, objectEndpoints)
	assert.Nil(t, parseConfiguredEndpointTypes(`not-json`))
}
