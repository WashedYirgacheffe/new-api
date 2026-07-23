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
		Metadata: map[string]any{
			"model": "malicious-override",
			"size":  "1024x1024",
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
		"group":"default",
		"metadata":{"encoder_format":"mp3"}
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
