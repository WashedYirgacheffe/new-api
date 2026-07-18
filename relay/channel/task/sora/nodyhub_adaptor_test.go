package sora_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
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
			expectedPollMethod: http.MethodPost,
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
			if testCase.expectedPollMethod == http.MethodPost {
				assert.Equal(t, "application/json", pollRequest.contentType)
			} else {
				assert.Empty(t, pollRequest.contentType)
			}
		})
	}
}
