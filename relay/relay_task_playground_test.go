package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoExposesVideoURLForPlaygroundPolling(t *testing.T) {
	task := &model.Task{
		TaskID: "task-video-1",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://cdn.example.test/video.mp4",
		},
	}

	payload, err := common.Marshal(dto.TaskResponse[any]{
		Code: dto.TaskSuccessCode,
		Data: TaskModel2Dto(task),
	})

	require.NoError(t, err)
	var response struct {
		Data struct {
			TaskID   string `json:"task_id"`
			Status   string `json:"status"`
			VideoURL string `json:"video_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(payload, &response))
	assert.Equal(t, "task-video-1", response.Data.TaskID)
	assert.Equal(t, string(model.TaskStatusSuccess), response.Data.Status)
	assert.Equal(t, "https://cdn.example.test/video.mp4", response.Data.VideoURL)
}

func TestTaskModel2DtoDoesNotExposeFailureReasonAsVideoURL(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-video-failed",
		Status:     model.TaskStatusFailure,
		FailReason: "upstream rejected the request",
	}

	converted := TaskModel2Dto(task)

	assert.Empty(t, converted.VideoURL)
	assert.Equal(t, "upstream rejected the request", converted.FailReason)
}
