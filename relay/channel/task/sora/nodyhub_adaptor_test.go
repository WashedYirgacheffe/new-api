package sora_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorRoutesSubmitAndPollByChannelProvider(t *testing.T) {
	service.InitHttpClient()

	testCases := []struct {
		name               string
		channelType        int
		channelProvider    string
		expectedSubmitPath string
		expectedPollMethod string
		expectedPollPath   string
	}{
		{
			name:               "NodyHub OpenAI channel",
			channelType:        constant.ChannelTypeOpenAI,
			channelProvider:    "nodyhub",
			expectedSubmitPath: "/v2/videos/generations",
			expectedPollMethod: http.MethodGet,
			expectedPollPath:   "/v2/videos/generations/upstream-task",
		},
		{
			name:               "other OpenAI channel",
			channelType:        constant.ChannelTypeOpenAI,
			channelProvider:    "deepwl",
			expectedSubmitPath: "/v1/videos",
			expectedPollMethod: http.MethodGet,
			expectedPollPath:   "/v1/videos/upstream-task",
		},
		{
			name:               "Sora channel",
			channelType:        constant.ChannelTypeSora,
			channelProvider:    "",
			expectedSubmitPath: "/v1/videos",
			expectedPollMethod: http.MethodGet,
			expectedPollPath:   "/v1/videos/upstream-task",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			type capturedRequest struct {
				method        string
				path          string
				authorization string
				contentType   string
			}
			captured := make(chan capturedRequest, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured <- capturedRequest{
					method:        r.Method,
					path:          r.URL.Path,
					authorization: r.Header.Get("Authorization"),
					contentType:   r.Header.Get("Content-Type"),
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()

			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:     testCase.channelType,
					ChannelProvider: testCase.channelProvider,
					ChannelBaseUrl:  server.URL,
					ApiKey:          "submit-key",
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionGenerate},
			}
			adaptor := &sora.TaskAdaptor{}
			adaptor.Init(info)

			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
			context.Request.Header.Set("Content-Type", "application/json")
			submitResponse, err := adaptor.DoRequest(context, info, strings.NewReader(`{}`))
			require.NoError(t, err)
			require.NoError(t, submitResponse.Body.Close())

			pollResponse, err := adaptor.FetchTask(server.URL, "poll-key", map[string]any{
				"task_id": "upstream-task",
			}, "")
			require.NoError(t, err)
			require.NoError(t, pollResponse.Body.Close())

			submitRequest := <-captured
			assert.Equal(t, http.MethodPost, submitRequest.method)
			assert.Equal(t, testCase.expectedSubmitPath, submitRequest.path)
			assert.Equal(t, "Bearer submit-key", submitRequest.authorization)
			assert.Equal(t, "application/json", submitRequest.contentType)

			pollRequest := <-captured
			assert.Equal(t, testCase.expectedPollMethod, pollRequest.method)
			assert.Equal(t, testCase.expectedPollPath, pollRequest.path)
			assert.Equal(t, "Bearer poll-key", pollRequest.authorization)
			assert.Empty(t, pollRequest.contentType)
		})
	}
}

func TestBuildRequestBodyNormalizesNodyHubSecondsOnly(t *testing.T) {
	testCases := []struct {
		name            string
		channelProvider string
		seconds         string
		expectedSeconds any
		expectError     bool
	}{
		{
			name:            "NodyHub integer string becomes number",
			channelProvider: "nodyhub",
			seconds:         "8",
			expectedSeconds: float64(8),
		},
		{
			name:            "other channel keeps string",
			channelProvider: "deepwl",
			seconds:         "8",
			expectedSeconds: "8",
		},
		{
			name:            "NodyHub rejects non integer string",
			channelProvider: "nodyhub",
			seconds:         "8.5",
			expectError:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := `{"model":"client-model","prompt":"move","seconds":"` + testCase.seconds + `"}`
			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
			context.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(context) })

			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelProvider:   testCase.channelProvider,
				UpstreamModelName: "upstream-model",
			}}
			adaptor := &sora.TaskAdaptor{}
			adaptor.Init(info)

			outbound, err := adaptor.BuildRequestBody(context, info)
			if testCase.expectError {
				require.Error(t, err)
				assert.ErrorContains(t, err, "invalid seconds: must be an integer")
				return
			}

			require.NoError(t, err)
			outboundBody, err := io.ReadAll(outbound)
			require.NoError(t, err)
			decoded := map[string]interface{}{}
			require.NoError(t, common.Unmarshal(outboundBody, &decoded))
			assert.Equal(t, "upstream-model", decoded["model"])
			assert.Equal(t, testCase.expectedSeconds, decoded["seconds"])
		})
	}
}

func TestParseTaskResultSupportsNodyHubBareTask(t *testing.T) {
	testCases := []struct {
		name             string
		responseBody     string
		expectedStatus   string
		expectedProgress string
		expectedReason   string
		expectedURL      string
	}{
		{
			name:             "submitted",
			responseBody:     `{"task_id":"upstream-task","status":"SUBMITTED","progress":"0%","data":{}}`,
			expectedStatus:   model.TaskStatusSubmitted,
			expectedProgress: "0%",
		},
		{
			name:             "queued",
			responseBody:     `{"task_id":"upstream-task","status":"QUEUED","progress":"10%","data":{}}`,
			expectedStatus:   model.TaskStatusQueued,
			expectedProgress: "10%",
		},
		{
			name:             "in progress",
			responseBody:     `{"task_id":"upstream-task","status":"IN_PROGRESS","progress":"45%","data":{}}`,
			expectedStatus:   model.TaskStatusInProgress,
			expectedProgress: "45%",
		},
		{
			name:             "success with result URL",
			responseBody:     `{"task_id":"upstream-task","status":"SUCCESS","progress":"100%","result_url":"https://media.example/result.mp4","data":{}}`,
			expectedStatus:   model.TaskStatusSuccess,
			expectedProgress: "100%",
			expectedURL:      "https://media.example/result.mp4",
		},
		{
			name:             "success with video URL fallback",
			responseBody:     `{"task_id":"upstream-task","status":"SUCCESS","progress":"100%","video_url":"https://media.example/video.mp4","data":{}}`,
			expectedStatus:   model.TaskStatusSuccess,
			expectedProgress: "100%",
			expectedURL:      "https://media.example/video.mp4",
		},
		{
			name:             "failure",
			responseBody:     `{"task_id":"upstream-task","status":"FAILURE","progress":"100%","fail_reason":"upstream rejected request","data":{}}`,
			expectedStatus:   model.TaskStatusFailure,
			expectedProgress: "100%",
			expectedReason:   "upstream rejected request",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelProvider: "nodyhub",
			}}
			adaptor := &sora.TaskAdaptor{}
			adaptor.Init(info)

			result, err := adaptor.ParseTaskResult([]byte(testCase.responseBody))
			require.NoError(t, err)
			assert.Equal(t, "upstream-task", result.TaskID)
			assert.Equal(t, testCase.expectedStatus, result.Status)
			assert.Equal(t, testCase.expectedProgress, result.Progress)
			assert.Equal(t, testCase.expectedReason, result.Reason)
			assert.Equal(t, testCase.expectedURL, result.Url)
		})
	}
}

func TestParseTaskResultKeepsOtherSoraChannelsUnchanged(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelProvider: "deepwl",
	}}
	adaptor := &sora.TaskAdaptor{}
	adaptor.Init(info)

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream-task","status":"processing","progress":42}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, "42%", result.Progress)
}
