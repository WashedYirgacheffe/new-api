package helper

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func modelOperationJSONContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/test", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(context) })
	return context
}

func modelOperationStoredJSONBody(t *testing.T, context *gin.Context) map[string]interface{} {
	t.Helper()
	storage, err := common.GetBodyStorage(context)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	decoded := map[string]interface{}{}
	require.NoError(t, common.Unmarshal(body, &decoded))
	return decoded
}

func TestModelOperationContractParametersResolvesNestedMappedFields(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"aspect_ratio": map[string]interface{}{"type": "string"},
				"resolution":   map[string]interface{}{"type": "string"},
			},
		},
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
	assert.Len(t, resolved, 2)
	assert.NotContains(t, parameters, "resolution")
}

func TestEffectiveModelOperationContractParametersMatchesQuotePrecedence(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"resolution": map[string]interface{}{
					"type": "string", "enum": []interface{}{"1K", "2K", "4K"}, "default": "1K",
				},
				"seconds": map[string]interface{}{
					"type": "integer", "enum": []interface{}{float64(4), float64(8)}, "default": float64(4),
				},
			},
		},
		RequestContract: model.ModelOperationRequestContract{
			Coercions: map[string]string{"seconds": "string"},
		},
		ParameterDefaults:  map[string]interface{}{"resolution": "2K"},
		ParameterOverrides: map[string]interface{}{"resolution": "4K"},
	}

	effective, err := effectiveModelOperationContractParameters(contract, map[string]interface{}{
		"model":      "deepwl/example",
		"resolution": "1K",
		"seconds":    "8",
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"resolution": "4K",
		"seconds":    int64(8),
	}, effective)
}

func TestPrepareModelOperationContractRequestRejectsRawUnknownParameter(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string", "minLength": float64(1)},
				"size":   map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"prompt"},
		},
		RequestContract:    model.ModelOperationRequestContract{Adapter: "openai-image"},
		ParameterDefaults:  map[string]interface{}{},
		ParameterOverrides: map[string]interface{}{},
	}
	context := modelOperationJSONContext(t, `{"model":"deepwl/image","prompt":"draw","size":"1024x1024","bogus":true}`)
	request := &dto.ImageRequest{Model: "deepwl/image", Prompt: "draw", Size: "1024x1024"}

	_, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter bogus")
}

func TestRawRETaskContractParametersRejectMetadataEnvelope(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string"},
			},
		},
		RequestContract: model.ModelOperationRequestContract{Adapter: "re-task"},
	}

	err := validateRawModelOperationContractParameters(contract, map[string]interface{}{
		"prompt": "draw", "metadata": map[string]interface{}{"duration": float64(999)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter metadata")
}

func TestRawContractParametersAllowEveryMaterialRequestSlot(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{"type": "string"},
			},
		},
		MaterialSchema: map[string]interface{}{
			"image": map[string]interface{}{
				"request_fields": []interface{}{
					map[string]interface{}{"slot": "source", "request_field": "image_urls"},
					map[string]interface{}{"slot": "mask", "request_field": "mask_url"},
				},
			},
		},
	}

	require.NoError(t, validateRawModelOperationContractParameters(contract, map[string]interface{}{
		"prompt": "draw", "image_urls": []interface{}{"https://media.example/source.png"}, "mask_url": "https://media.example/mask.png",
	}))
	err := validateRawModelOperationContractParameters(contract, map[string]interface{}{"rogue_url": "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter rogue_url")
}

func TestPrepareOpenAIImageRequestWritesEffectiveParametersToDTOAndBody(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt":          map[string]interface{}{"type": "string", "minLength": float64(1)},
				"size":            map[string]interface{}{"type": "string", "enum": []interface{}{"1024x1024", "1536x1024"}},
				"quality":         map[string]interface{}{"type": "string", "enum": []interface{}{"low", "high"}},
				"n":               map[string]interface{}{"type": "integer", "enum": []interface{}{float64(1)}},
				"response_format": map[string]interface{}{"type": "string", "enum": []interface{}{"url", "b64_json"}},
			},
			"required": []interface{}{"prompt"},
		},
		RequestContract: model.ModelOperationRequestContract{
			Adapter: "openai-image",
			FieldMap: map[string]string{
				"size": "size", "quality": "quality", "n": "n", "response_format": "response_format",
			},
		},
		ParameterDefaults: map[string]interface{}{
			"size": "1024x1024", "quality": "low", "n": float64(1), "response_format": "url",
		},
		ParameterOverrides: map[string]interface{}{"size": "1536x1024", "quality": "high"},
	}
	context := modelOperationJSONContext(t, `{"model":"deepwl/image","prompt":"draw","size":"1024x1024","quality":"low","n":1,"response_format":"url"}`)
	n := uint(1)
	request := &dto.ImageRequest{
		Model: "deepwl/image", Prompt: "draw", Size: "1024x1024", Quality: "low", N: &n, ResponseFormat: "url",
	}

	prepared, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.NoError(t, err)
	assert.Equal(t, "1536x1024", prepared.EffectiveParameters["size"])
	assert.Equal(t, "high", prepared.EffectiveParameters["quality"])
	assert.Equal(t, "1536x1024", request.Size)
	assert.Equal(t, "high", request.Quality)
	body := modelOperationStoredJSONBody(t, context)
	assert.Equal(t, "1536x1024", body["size"])
	assert.Equal(t, "high", body["quality"])
}

func TestPrepareTextChatRequestWritesEffectiveParametersWithoutSyntheticPrompt(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
			"properties": map[string]interface{}{
				"prompt":      map[string]interface{}{"type": "string", "minLength": float64(1)},
				"temperature": map[string]interface{}{"type": "number", "minimum": float64(0), "maximum": float64(2)},
				"top_p":       map[string]interface{}{"type": "number", "minimum": float64(0), "maximum": float64(1)},
				"max_tokens":  map[string]interface{}{"type": "integer", "minimum": float64(1), "maximum": float64(131072)},
			},
			"required": []interface{}{"prompt"},
		},
		ParameterDefaults: map[string]interface{}{"max_tokens": float64(1024)},
		ParameterOverrides: map[string]interface{}{
			"temperature": float64(0.25), "top_p": float64(0.8), "max_tokens": float64(2048),
		},
	}
	bodyText := `{"model":"deepwl/chat","messages":[{"role":"user","content":"hello"}],"temperature":1,"top_p":1}`
	context := modelOperationJSONContext(t, bodyText)
	request := &dto.GeneralOpenAIRequest{}
	require.NoError(t, common.Unmarshal([]byte(bodyText), request))

	prepared, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.NoError(t, err)
	assert.Equal(t, "hello", prepared.EffectiveParameters["prompt"])
	require.NotNil(t, request.Temperature)
	assert.Equal(t, 0.25, *request.Temperature)
	require.NotNil(t, request.TopP)
	assert.Equal(t, 0.8, *request.TopP)
	require.NotNil(t, request.MaxTokens)
	assert.Equal(t, uint(2048), *request.MaxTokens)
	body := modelOperationStoredJSONBody(t, context)
	assert.Equal(t, float64(0.25), body["temperature"])
	assert.Equal(t, float64(0.8), body["top_p"])
	assert.Equal(t, float64(2048), body["max_tokens"])
	assert.NotContains(t, body, "prompt")
}

func TestPrepareTextChatRequestPreservesExplicitMaxCompletionTokens(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt":     map[string]interface{}{"type": "string", "minLength": float64(1)},
				"max_tokens": map[string]interface{}{"type": "integer", "minimum": float64(1), "maximum": float64(131072)},
			},
			"required": []interface{}{"prompt"},
		},
		ParameterDefaults: map[string]interface{}{"max_tokens": float64(1024)},
	}
	bodyText := `{"model":"deepwl/chat","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":256}`
	context := modelOperationJSONContext(t, bodyText)
	request := &dto.GeneralOpenAIRequest{}
	require.NoError(t, common.Unmarshal([]byte(bodyText), request))

	prepared, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.NoError(t, err)
	assert.Equal(t, float64(256), prepared.EffectiveParameters["max_tokens"])
	assert.Nil(t, request.MaxTokens)
	require.NotNil(t, request.MaxCompletionTokens)
	assert.Equal(t, uint(256), *request.MaxCompletionTokens)
	body := modelOperationStoredJSONBody(t, context)
	assert.NotContains(t, body, "max_tokens")
	assert.Equal(t, float64(256), body["max_completion_tokens"])
}

func TestPrepareGeminiImageRequestWritesForced4KToDTOAndBody(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt":       map[string]interface{}{"type": "string", "minLength": float64(1)},
				"aspect_ratio": map[string]interface{}{"type": "string", "enum": []interface{}{"1:1", "9:16"}},
				"resolution":   map[string]interface{}{"type": "string", "enum": []interface{}{"1K", "2K", "4K"}},
			},
			"required": []interface{}{"prompt"},
		},
		RequestContract: model.ModelOperationRequestContract{
			Adapter:  "gemini-image",
			FieldMap: map[string]string{"aspect_ratio": "aspectRatio", "resolution": "imageSize"},
		},
		ParameterDefaults:  map[string]interface{}{"aspect_ratio": "1:1", "resolution": "1K"},
		ParameterOverrides: map[string]interface{}{"resolution": "4K"},
	}
	bodyText := `{"contents":[{"role":"user","parts":[{"text":"draw"}]}],"generationConfig":{"imageConfig":{"aspectRatio":"9:16","imageSize":"2K"}}}`
	context := modelOperationJSONContext(t, bodyText)
	request := &dto.GeminiChatRequest{}
	require.NoError(t, common.Unmarshal([]byte(bodyText), request))

	prepared, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.NoError(t, err)
	assert.Equal(t, "4K", prepared.EffectiveParameters["resolution"])
	imageConfig := map[string]interface{}{}
	require.NoError(t, common.Unmarshal(request.GenerationConfig.ImageConfig, &imageConfig))
	assert.Equal(t, "9:16", imageConfig["aspectRatio"])
	assert.Equal(t, "4K", imageConfig["imageSize"])
	body := modelOperationStoredJSONBody(t, context)
	generationConfig := body["generationConfig"].(map[string]interface{})
	storedImageConfig := generationConfig["imageConfig"].(map[string]interface{})
	assert.Equal(t, "9:16", storedImageConfig["aspectRatio"])
	assert.Equal(t, "4K", storedImageConfig["imageSize"])
}

func TestPrepareVideoRequestWritesEffectiveParametersBeforeBilling(t *testing.T) {
	contract := &model.ModelOperationEffectiveContract{
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"prompt":       map[string]interface{}{"type": "string", "minLength": float64(1)},
				"seconds":      map[string]interface{}{"type": "integer", "enum": []interface{}{float64(4), float64(8)}},
				"aspect_ratio": map[string]interface{}{"type": "string", "enum": []interface{}{"16:9", "9:16"}},
				"resolution":   map[string]interface{}{"type": "string", "enum": []interface{}{"720p"}},
			},
			"required": []interface{}{"prompt"},
		},
		RequestContract: model.ModelOperationRequestContract{
			Adapter: "openai-video",
			FieldMap: map[string]string{
				"seconds": "seconds", "aspect_ratio": "aspect_ratio", "resolution": "resolution",
			},
			Coercions: map[string]string{"seconds": "string"},
		},
		ParameterDefaults: map[string]interface{}{
			"seconds": float64(4), "aspect_ratio": "16:9", "resolution": "720p",
		},
		ParameterOverrides: map[string]interface{}{"seconds": float64(8), "aspect_ratio": "9:16"},
	}
	context := modelOperationJSONContext(t, `{"model":"deepwl/video","prompt":"move","seconds":"4","aspect_ratio":"16:9","resolution":"720p"}`)
	request := &relaycommon.TaskSubmitReq{
		Model: "deepwl/video", Prompt: "move", Seconds: "4", Metadata: map[string]interface{}{},
	}

	prepared, err := prepareModelOperationContractRequestWithContract(context, contract, request)

	require.NoError(t, err)
	assert.Equal(t, float64(8), prepared.EffectiveParameters["seconds"])
	assert.Equal(t, "8", request.Seconds)
	assert.Equal(t, 8, request.Duration)
	assert.Equal(t, "9:16", request.Metadata["aspect_ratio"])
	storedRequest, err := relaycommon.GetTaskRequest(context)
	require.NoError(t, err)
	assert.Equal(t, "8", storedRequest.Seconds)
	assert.Equal(t, "9:16", storedRequest.Metadata["aspect_ratio"])
	body := modelOperationStoredJSONBody(t, context)
	assert.Equal(t, "8", body["seconds"])
	assert.Equal(t, "9:16", body["aspect_ratio"])
	assert.NotContains(t, body, "duration")
}
