package reapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const ChannelName = "RE"

var endpointByModel = func() map[string]string {
	endpoints := make(map[string]string)
	add := func(endpoint string, models ...string) {
		for _, name := range models {
			endpoints[name] = endpoint
		}
	}
	add("/essay", "ai-essay-writer")
	add("/detect", "ai-text-detector")
	add("/humanize", "humanize")
	add("/audio/generations",
		"audio-multistem", "audio-music-extractor", "audio-stem-separator", "audio-voice-change", "audio-voice-clean",
		"mureka-v9-song", "suno-add-instrumental", "suno-add-vocals", "suno-cover-image", "suno-extend", "suno-lyrics",
		"suno-mashup", "suno-midi", "suno-music", "suno-music-video", "suno-replace-section", "suno-sounds",
		"suno-upload-cover", "suno-upload-extend", "suno-vocal-separation", "suno-voice-generate", "suno-voice-regenerate",
		"suno-voice-validate", "suno-wav",
	)
	add("/images/generations",
		"doubao-seedream-5-0-lite", "doubao-seedream-5-0-pro", "flux-2", "flux-2-flex",
		"gemini-2.5-flash-image-preview", "gemini-2.5-flash-image-preview-official", "gemini-3-pro-image-preview",
		"gemini-3-pro-image-preview-official", "gemini-3.1-flash-image-preview", "gemini-3.1-flash-image-preview-official",
		"gpt-image-2", "gpt-image-2-beta", "gpt-image-2-official", "imagen-4-0", "midjourney", "mj-v7", "mj-v7-edit",
		"mj-v7-enhance", "mj-v7-inpaint", "mj-v7-outpaint", "mj-v7-pan", "mj-v7-remix", "mj-v7-remove-bg",
		"mj-v7-retexture", "mj-v7-upload-paint", "mj-v7-upscale", "mj-v7-variation", "nano-banana-2-lite", "qwen-image-2",
		"wan2.7-image", "wan2.7-image-pro", "z-image",
	)
	add("/videos/generations",
		"doubao-seedance-2.0", "doubao-seedance-2.0-face", "doubao-seedance-2.0-fast", "doubao-seedance-2.0-fast-face",
		"doubao-seedance-2.0-fast-official", "doubao-seedance-2.0-official", "enhance-video-1.0", "gemini-omni", "gemini-omni-beta",
		"grok-imagine-1.0-video", "grok-imagine-video-1.5-beta", "grok-imagine-video-1.5-official", "happyhorse-1-1",
		"happyhorse-1.0", "happyhorse-1.0-official", "kling-3-0", "kling-3-0-turbo", "kling-3-0-turbo-beta",
		"kling-v2-6-motion-control", "kling-v3-motion-control", "midjourney-video", "music-video-1-0", "pixverse-v6",
		"seedance-2.0-beta", "seedance-2.0-fast-beta", "seedance-2.0-mini", "seedance-2.5", "topaz-video-upscaler",
		"veo3.1-fast", "veo3.1-fast-official", "veo3.1-lite", "veo3.1-quality", "veo3.1-quality-official", "viduq3-pro",
		"viduq3-turbo", "wan2.7-video", "wan2.7-video-official",
	)
	return endpoints
}()

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type taskResponse struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	Output    any    `json:"output"`
	Error     any    `json:"error"`
}

func DecodeTaskResponse(body []byte) (id, modelName, status string, createdAt int64, output, taskError any, err error) {
	var task taskResponse
	if err = common.Unmarshal(body, &task); err != nil {
		return "", "", "", 0, nil, nil, err
	}
	return task.ID, task.Model, task.Status, task.CreatedAt, task.Output, task.Error, nil
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if taskErr := relaycommon.ValidateTaskRequest(c, info, constant.TaskActionGenerate, false); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Model) == "" && strings.TrimSpace(info.OriginModelName) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "missing_model", http.StatusBadRequest)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	endpoint, ok := EndpointForModel(info.UpstreamModelName)
	if !ok {
		return "", fmt.Errorf("RE does not support async model %q", info.UpstreamModelName)
	}
	return a.baseURL + endpoint, nil
}

// EndpointForModel returns the RE asynchronous submission path for an upstream
// model identifier. It is exported so the checked-in catalog onboarding tool
// can reject a catalog that has drifted away from the runtime adaptor.
func EndpointForModel(modelName string) (string, bool) {
	endpoint, ok := endpointByModel[modelName]
	return endpoint, ok
}

func OperationForModel(modelName string) (string, bool) {
	endpoint, ok := EndpointForModel(modelName)
	if !ok {
		return "", false
	}
	switch endpoint {
	case "/images/generations":
		return "image.generate", true
	case "/videos/generations":
		return "video.generate", true
	case "/audio/generations":
		return "audio.generate", true
	case "/essay", "/detect", "/humanize":
		return "text.generate", true
	default:
		return "", false
	}
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body := make(map[string]any, len(req.Metadata)+8)
	if c != nil && c.Request != nil && strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		storage, storageErr := common.GetBodyStorage(c)
		if storageErr != nil {
			return nil, storageErr
		}
		raw, readErr := storage.Bytes()
		if readErr != nil {
			return nil, readErr
		}
		if len(raw) > 0 {
			if decodeErr := common.Unmarshal(raw, &body); decodeErr != nil {
				return nil, decodeErr
			}
		}
	}
	for key, value := range req.Metadata {
		if key != "model" {
			if _, exists := body[key]; exists {
				continue
			}
			body[key] = value
		}
	}
	delete(body, "metadata")
	delete(body, "group")
	delete(body, "image")
	delete(body, "images")
	delete(body, "input_reference")
	body["model"] = info.UpstreamModelName
	if _, exists := body["prompt"]; !exists && strings.TrimSpace(req.Prompt) != "" {
		body["prompt"] = req.Prompt
	}
	if _, exists := body["size"]; !exists && strings.TrimSpace(req.Size) != "" {
		body["size"] = req.Size
	}
	if _, exists := body["seconds"]; !exists && strings.TrimSpace(req.Seconds) != "" {
		body["seconds"] = req.Seconds
	}
	if _, hasDuration := body["duration"]; !hasDuration && req.Duration > 0 {
		if _, hasSeconds := body["seconds"]; !hasSeconds {
			body["duration"] = req.Duration
		}
	}
	if _, exists := body["image_urls"]; !exists && len(req.Images) > 0 {
		body["image_urls"] = req.Images
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	id, _, status, createdAt, output, taskError, err := DecodeTaskResponse(body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(id) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("RE task id is empty"), "invalid_response", http.StatusBadGateway)
	}
	c.JSON(http.StatusOK, PublicTaskResponse(
		info.PublicTaskID,
		info.OriginModelName,
		status,
		createdAt,
		RedactUpstreamTaskID(output, id),
		RedactUpstreamTaskID(taskError, id),
	))
	return id, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.NewProxyHttpClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var task taskResponse
	if err := common.Unmarshal(body, &task); err != nil {
		return nil, err
	}
	result := &relaycommon.TaskInfo{Code: 0}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "queued", "pending":
		result.Status = model.TaskStatusQueued
		result.Progress = taskcommon.ProgressQueued
	case "processing", "running", "in_progress":
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	case "completed", "succeeded", "success":
		result.Status = model.TaskStatusSuccess
		result.Progress = taskcommon.ProgressComplete
		result.Url = firstResultURL(task.Output)
	case "failed", "cancelled", "canceled":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = taskErrorReason(task.Error)
		if result.Reason == "" {
			result.Reason = "RE task failed"
		}
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	}
	return result, nil
}

func (a *TaskAdaptor) GetModelList() []string { return nil }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func StatusFromTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "completed"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func PublicTaskResponse(publicID, modelName, status string, createdAt int64, output, taskError any) map[string]any {
	return map[string]any{
		"id":         publicID,
		"model":      modelName,
		"status":     status,
		"created_at": createdAt,
		"output":     output,
		"error":      taskError,
	}
}

// RedactUpstreamTaskID removes an upstream task ID from provider response
// payloads before they are sent to a client. RE output formats vary by model,
// so this walks nested JSON objects without assuming a fixed response shape.
func RedactUpstreamTaskID(value any, upstreamTaskID string) any {
	if upstreamTaskID == "" {
		return value
	}
	switch v := value.(type) {
	case string:
		if v == upstreamTaskID {
			return ""
		}
		return v
	case []any:
		redacted := make([]any, len(v))
		for index, item := range v {
			redacted[index] = RedactUpstreamTaskID(item, upstreamTaskID)
		}
		return redacted
	case map[string]any:
		redacted := make(map[string]any, len(v))
		for key, item := range v {
			redacted[key] = RedactUpstreamTaskID(item, upstreamTaskID)
		}
		return redacted
	default:
		return value
	}
}

func taskErrorReason(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"message", "detail", "error", "code"} {
			if message, ok := v[key].(string); ok && strings.TrimSpace(message) != "" {
				return strings.TrimSpace(message)
			}
		}
	}
	return ""
}

func firstResultURL(value any) string {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
	case []any:
		for _, item := range v {
			if result := firstResultURL(item); result != "" {
				return result
			}
		}
	case map[string]any:
		visited := make(map[string]struct{}, len(v))
		for _, key := range []string{"url", "urls", "result_url", "video_url", "image_url", "audio_url", "download_url", "data", "output", "result", "files"} {
			if item, ok := v[key]; ok {
				visited[key] = struct{}{}
				if result := firstResultURL(item); result != "" {
					return result
				}
			}
		}
		for key, item := range v {
			if _, known := visited[key]; known {
				continue
			}
			if result := firstResultURL(item); result != "" {
				return result
			}
		}
	}
	return ""
}
