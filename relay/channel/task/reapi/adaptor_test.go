package reapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorBuildsRERequestContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", nil)
	context.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: "draw a lighthouse",
		Images: []string{"https://input.example/reference.png"},
		Size:   "1024x1024",
		Metadata: map[string]any{
			"model": "malicious-override",
		},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://reapi.ai/api/v1",
			ApiKey:            "task-key",
			UpstreamModelName: "gpt-image-2",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://reapi.ai/api/v1/images/generations", requestURL)

	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(payload, &decoded))
	assert.Equal(t, "gpt-image-2", decoded["model"])
	assert.Equal(t, "draw a lighthouse", decoded["prompt"])
	assert.Equal(t, "1024x1024", decoded["size"])
	assert.Equal(t, []any{"https://input.example/reference.png"}, decoded["image_urls"])

	req := httptest.NewRequest(http.MethodPost, requestURL, nil)
	require.NoError(t, adaptor.BuildRequestHeader(context, req, info))
	assert.Equal(t, "Bearer task-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestTaskAdaptorPreservesAsyncFieldsWithoutPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{
		"model":"re/audio-multistem",
		"audio_url":"https://input.example/song.wav",
		"stem_list":["vocals","drum"],
		"encoder_format":"mp3",
		"group":"default",
		"metadata":{"duration":12,"media":[{"type":"video","url":"https://input.example/bypass.mp4"}]}
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/audio-multistem",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://reapi.ai/api/v1",
			ApiKey:            "task-key",
			UpstreamModelName: "audio-multistem",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(payload, &decoded))
	assert.Equal(t, "audio-multistem", decoded["model"])
	assert.Equal(t, "https://input.example/song.wav", decoded["audio_url"])
	assert.Equal(t, []any{"vocals", "drum"}, decoded["stem_list"])
	assert.Equal(t, "mp3", decoded["encoder_format"])
	assert.NotContains(t, decoded, "prompt")
	assert.NotContains(t, decoded, "group")
	assert.NotContains(t, decoded, "metadata")
}

func TestTaskAdaptorDoesNotFlattenMetadataIntoPreparedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{
		"model":"re/wan2.7-video",
		"prompt":"animate the scene",
		"aspect_ratio":"16:9",
		"resolution":"720p",
		"metadata":{"duration":12,"media":[{"type":"video","url":"https://input.example/bypass.mp4"}]}
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:   "animate the scene",
		Duration: 12,
		Metadata: map[string]any{
			"duration": 12,
			"media": []any{
				map[string]any{"type": "video", "url": "https://input.example/bypass.mp4"},
			},
		},
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/wan2.7-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://reapi.ai/api/v1",
			ApiKey:            "task-key",
			UpstreamModelName: "wan2.7-video",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(payload, &decoded))
	assert.Equal(t, "wan2.7-video", decoded["model"])
	assert.Equal(t, "animate the scene", decoded["prompt"])
	assert.Equal(t, "16:9", decoded["aspect_ratio"])
	assert.Equal(t, "720p", decoded["resolution"])
	assert.NotContains(t, decoded, "duration")
	assert.NotContains(t, decoded, "media")
	assert.NotContains(t, decoded, "metadata")
}

func TestTaskAdaptorPreservesContractMaterialFieldNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{
		"model":"re/flux-2",
		"prompt":"combine references",
		"images":["https://input.example/one.png","https://input.example/two.png"]
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: "combine references",
		Images: []string{"https://input.example/one.png", "https://input.example/two.png"},
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/flux-2",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://reapi.ai/api/v1",
			ApiKey:            "task-key",
			UpstreamModelName: "flux-2",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(payload, &decoded))
	assert.Equal(t, []any{"https://input.example/one.png", "https://input.example/two.png"}, decoded["images"])
	assert.NotContains(t, decoded, "image_urls")
}

func TestTaskAdaptorPreservesExplicitSeedanceNSFWChecker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{
		"model":"re/doubao-seedance-2.0-face",
		"prompt":"a cinematic scene",
		"duration":5,
		"resolution":"4k",
		"nsfw_checker":false
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("task_request", relaycommon.TaskSubmitReq{Prompt: "a cinematic scene", Duration: 5})
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/doubao-seedance-2.0-face",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://reapi.ai/api/v1",
			ApiKey:            "task-key",
			UpstreamModelName: "doubao-seedance-2.0-face",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(payload, &decoded))
	assert.Equal(t, false, decoded["nsfw_checker"])
	assert.Equal(t, "4k", decoded["resolution"])
}

func TestOperationForModelMatchesAsyncCapability(t *testing.T) {
	tests := map[string]string{
		"gpt-image-2":      "image.generate",
		"veo3.1-fast":      "video.generate",
		"audio-multistem":  "audio.generate",
		"ai-text-detector": "text.generate",
	}
	for modelName, expected := range tests {
		operation, ok := OperationForModel(modelName)
		assert.True(t, ok, modelName)
		assert.Equal(t, expected, operation, modelName)
	}
	_, ok := OperationForModel("unknown-model")
	assert.False(t, ok)
}

func TestTaskAdaptorEstimatesREBillingQuantities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		model    string
		body     string
		expected map[string]float64
	}{
		{name: "video seconds", model: "grok-imagine-1.0-video", body: `{"model":"re/grok-imagine-1.0-video","duration":6}`, expected: map[string]float64{"seconds": 6}},
		{name: "image count", model: "gpt-image-2", body: `{"model":"re/gpt-image-2","n":3}`, expected: map[string]float64{"images": 3}},
		{name: "essay flat price", model: "ai-essay-writer", body: `{"model":"re/ai-essay-writer","length":"long"}`, expected: nil},
		{name: "text word floor", model: "humanize", body: `{"model":"re/humanize","text":"make this sound natural"}`, expected: map[string]float64{"thousand_words": 0.05}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(test.body))
			context.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{
				OriginModelName: "re/" + test.model,
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: test.model},
			}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
			assert.Equal(t, test.expected, adaptor.EstimateBilling(context, info))
		})
	}
}

func TestTaskAdaptorRejectsUnboundedImageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{"model":"re/gpt-image-2","n":999}`))
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/gpt-image-2",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-image-2"},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_n", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestTaskAdaptorRequiresDurationForPerSecondBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{"model":"re/grok-imagine-1.0-video","prompt":"a moving train"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/grok-imagine-1.0-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "re/grok-imagine-1.0-video"},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "missing_duration", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestTaskAdaptorRejectsConflictingDurationFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(`{"model":"re/grok-imagine-1.0-video","prompt":"a moving train","duration":4,"seconds":"8"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/grok-imagine-1.0-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "re/grok-imagine-1.0-video"},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_duration", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestTaskAdaptorRejectsStringMetadataDurationConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	metadata, err := common.Marshal(`{"seconds":8}`)
	require.NoError(t, err)
	body := `{"model":"re/grok-imagine-1.0-video","prompt":"a moving train","duration":4,"metadata":` + string(metadata) + `}`
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/re/generations", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/grok-imagine-1.0-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "re/grok-imagine-1.0-video"},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_duration", taskErr.Code)
}

func TestTaskAdaptorDoesNotExposeUpstreamTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream_secret_123","model":"gpt-image-2","status":"processing","created_at":123,"output":{"task_id":"upstream_secret_123"}}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "re/gpt-image-2",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_123"},
	}

	upstreamID, _, taskErr := adaptor.DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_secret_123", upstreamID)
	assert.Contains(t, recorder.Body.String(), `"id":"task_public_123"`)
	assert.NotContains(t, recorder.Body.String(), "upstream_secret_123")
}

func TestTaskAdaptorParsesNestedResultAndFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	success, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed",
		"output":{"nested":{"items":[{"url":"https://cdn.example/result.mp4"}]}}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, success.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", success.Url)

	failure, err := adaptor.ParseTaskResult([]byte(`{"status":"failed","error":{"detail":"upstream rejected input"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failure.Status)
	assert.Equal(t, "upstream rejected input", failure.Reason)
}

func TestTaskAdaptorFetchEscapesUpstreamTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/tasks/id%2Fwith%3Fquery", req.URL.EscapedPath())
		assert.Equal(t, "Bearer task-key", req.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	response, err := adaptor.FetchTask(server.URL, "task-key", map[string]any{"task_id": "id/with?query"}, "")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestStatusFromTaskStatus(t *testing.T) {
	assert.Equal(t, "completed", StatusFromTaskStatus(model.TaskStatusSuccess))
	assert.Equal(t, "failed", StatusFromTaskStatus(model.TaskStatusFailure))
	assert.Equal(t, "queued", StatusFromTaskStatus(model.TaskStatusQueued))
	assert.Equal(t, "processing", StatusFromTaskStatus(model.TaskStatusInProgress))
}
