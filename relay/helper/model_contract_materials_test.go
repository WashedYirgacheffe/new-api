package helper

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func videoMaterialContract() *model.ModelOperationEffectiveContract {
	return &model.ModelOperationEffectiveContract{
		MaterialSchema: map[string]interface{}{
			"video": map[string]interface{}{
				"min_items":     float64(1),
				"max_items":     float64(1),
				"max_size_mb":   float64(1),
				"mime_types":    []interface{}{"video/mp4"},
				"request_field": "video",
				"transport":     "url",
			},
		},
	}
}

func TestValidateModelOperationContractMaterialsChecksKnownInputs(t *testing.T) {
	contract := videoMaterialContract()

	err := ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1")

	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": []interface{}{"https://example.com/a.mp4", "https://example.com/b.mp4"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1")

	delete(contract.MaterialSchema["video"].(map[string]interface{}), "request_field")
	delete(contract.MaterialSchema["video"].(map[string]interface{}), "transport")
	mp4Header := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm'}
	validData := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(mp4Header)
	require.NoError(t, ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{"video": validData}))

	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	wrongMime := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngHeader)
	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{"video": wrongMime})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not allow MIME type")

	contract.MaterialSchema["video"].(map[string]interface{})["max_size_mb"] = float64(0.000001)
	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{"video": validData})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestValidateModelOperationContractMaterialsAcceptsMIMEWildcard(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		MaterialSchema: map[string]interface{}{
			"image": map[string]interface{}{
				"min_items":     float64(1),
				"max_items":     float64(1),
				"mime_types":    []interface{}{"image/*"},
				"request_field": "image",
				"transport":     "inline",
			},
		},
	}
	parameters := map[string]interface{}{
		"image": "data:image/png;base64,iVBORw0KGgo=",
	}

	require.NoError(t, ValidateModelOperationContractMaterials(nil, contract, parameters))
}

func TestValidateModelOperationContractMaterialsRejectsPrivateRemoteURLs(t *testing.T) {
	contract := videoMaterialContract()
	contract.MaterialSchema["video"].(map[string]interface{})["max_size_mb"] = float64(0.000001)

	err := ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": "https://127.0.0.1:1/must-not-be-fetched.mp4",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "IP literal")
}

func TestValidateModelOperationContractMaterialsProbesRemoteURL(t *testing.T) {
	contract := videoMaterialContract()
	originalProbe := probeModelOperationPublicMaterialURL
	probeModelOperationPublicMaterialURL = func(_ context.Context, rawURL string) (service.PublicMaterialMetadata, error) {
		assert.Equal(t, "https://media.example/source.mp4", rawURL)
		return service.PublicMaterialMetadata{MIMEType: "video/mp4", Size: 512 * 1024}, nil
	}
	t.Cleanup(func() { probeModelOperationPublicMaterialURL = originalProbe })

	require.NoError(t, ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": "https://media.example/source.mp4",
	}))

	contract.MaterialSchema["video"].(map[string]interface{})["max_size_mb"] = float64(0.25)
	err := ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": "https://media.example/source.mp4",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestValidateModelOperationContractMaterialsExecutesTransportRolesAndDuration(t *testing.T) {
	originalProbe := probeModelOperationPublicMaterialURL
	probeModelOperationPublicMaterialURL = func(_ context.Context, _ string) (service.PublicMaterialMetadata, error) {
		return service.PublicMaterialMetadata{MIMEType: "video/mp4", Size: 1024}, nil
	}
	t.Cleanup(func() { probeModelOperationPublicMaterialURL = originalProbe })

	contract := videoMaterialContract()
	mp4Header := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	dataURI := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(mp4Header)
	err := ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{"video": dataURI})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport url")

	rule := contract.MaterialSchema["video"].(map[string]interface{})
	rule["roles"] = []interface{}{"source"}
	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": "https://media.example/source.mp4",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declare a role")

	require.NoError(t, ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": map[string]interface{}{"url": "https://media.example/source.mp4", "role": "source"},
	}))

	rule["max_total_duration"] = float64(60)
	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"video": map[string]interface{}{"url": "https://media.example/source.mp4", "role": "source"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be verified")
}

func TestValidateModelOperationContractMaterialsRoutesMixedRequestFieldSlots(t *testing.T) {
	originalProbe := probeModelOperationPublicMaterialURL
	probeModelOperationPublicMaterialURL = func(_ context.Context, _ string) (service.PublicMaterialMetadata, error) {
		return service.PublicMaterialMetadata{MIMEType: "image/png", Size: 1024}, nil
	}
	t.Cleanup(func() { probeModelOperationPublicMaterialURL = originalProbe })

	contract := &model.ModelOperationEffectiveContract{
		MaterialSchema: map[string]interface{}{
			"image": map[string]interface{}{
				"min_items":  float64(1),
				"max_items":  float64(3),
				"mime_types": []interface{}{"image/png"},
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
			"video": map[string]interface{}{
				"min_items": float64(0),
				"max_items": float64(1),
				"request_fields": []interface{}{
					map[string]interface{}{
						"slot": "video", "request_field": "media", "transport": "url",
						"min_items": float64(0), "max_items": float64(1), "value_type": "array",
						"item_template": map[string]interface{}{"type": "video"}, "url_field": "url",
					},
				},
			},
		},
	}
	parameters := map[string]interface{}{
		"media": []interface{}{
			map[string]interface{}{"type": "reference_image", "url": "https://media.example/reference.png"},
			map[string]interface{}{"type": "last_frame", "url": "https://media.example/last.png"},
			map[string]interface{}{"type": "video", "url": "https://media.example/video.mp4"},
		},
	}

	require.NoError(t, ValidateModelOperationContractMaterials(nil, contract, parameters))

	parameters["media"] = []interface{}{
		map[string]interface{}{"type": "last_frame", "url": "https://media.example/last-1.png"},
		map[string]interface{}{"type": "last_frame", "url": "https://media.example/last-2.png"},
	}
	err := ValidateModelOperationContractMaterials(nil, contract, parameters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1")
}

func TestValidateModelOperationContractMaterialsRejectsMalformedMixedSlots(t *testing.T) {
	originalProbe := probeModelOperationPublicMaterialURL
	probeModelOperationPublicMaterialURL = func(_ context.Context, _ string) (service.PublicMaterialMetadata, error) {
		return service.PublicMaterialMetadata{MIMEType: "audio/wav", Size: 1024}, nil
	}
	t.Cleanup(func() { probeModelOperationPublicMaterialURL = originalProbe })

	contract := &model.ModelOperationEffectiveContract{
		MaterialSchema: map[string]interface{}{
			"audio": map[string]interface{}{
				"min_items": float64(0), "max_items": float64(2),
				"request_fields": []interface{}{
					map[string]interface{}{
						"slot": "driving_audio", "request_field": "media", "transport": "url",
						"min_items": float64(0), "max_items": float64(1), "value_type": "array",
						"item_template": map[string]interface{}{"type": "driving_audio"}, "url_field": "url",
					},
					map[string]interface{}{
						"slot": "reference_voice", "request_field": "media", "transport": "url",
						"min_items": float64(0), "max_items": float64(1), "value_type": "array",
						"item_template": map[string]interface{}{"type": "driving_audio"}, "url_field": "reference_voice",
						"requires_slot": "driving_audio",
					},
				},
			},
		},
	}

	err := ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"media": []interface{}{map[string]interface{}{
			"type": "driving_audio", "reference_voice": "https://media.example/voice.wav",
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a matching driving_audio")

	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"media": []interface{}{map[string]interface{}{"type": "unknown", "url": "https://media.example/audio.wav"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match a configured slot")

	err = ValidateModelOperationContractMaterials(nil, contract, map[string]interface{}{
		"media": []interface{}{map[string]interface{}{"type": "driving_audio"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty URL")
}

func TestModelOperationRequestParametersPreservesRawContractFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		"POST",
		"/pg/videos",
		strings.NewReader(`{"model":"deepwl/omni-fast-v2v","video":"https://example.com/source.mp4","aspect_ratio":"9:16"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	parameters, err := ModelOperationRequestParameters(context, map[string]interface{}{
		"model": "deepwl/omni-fast-v2v",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/source.mp4", parameters["video"])
	assert.Equal(t, "9:16", parameters["aspect_ratio"])
}
