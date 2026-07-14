package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestModelOperationContractParametersResolvesNestedMappedFields(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		RequestContract: model.ModelOperationRequestContract{
			FieldMap: map[string]string{
				"aspect_ratio": "aspectRatio",
				"resolution":   "imageSize",
			},
		},
	}
	parameters := map[string]interface{}{
		"generationConfig": map[string]interface{}{
			"imageConfig": map[string]interface{}{
				"aspectRatio": "16:9",
				"imageSize":   "4K",
			},
		},
	}

	resolved := modelOperationContractParameters(contract, parameters)

	assert.Equal(t, "16:9", resolved["aspect_ratio"])
	assert.Equal(t, "4K", resolved["resolution"])
	assert.NotContains(t, parameters, "resolution")
}
