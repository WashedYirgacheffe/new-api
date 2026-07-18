package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNodyHubVideoURLFromStoredTaskData(t *testing.T) {
	channel := &model.Channel{
		Type:            constant.ChannelTypeOpenAI,
		ChannelProvider: "nodyhub",
	}
	task := &model.Task{
		TaskID: "public-task",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "/v1/videos/public-task/content",
		},
		Data: []byte(`{"task_id":"upstream-task","status":"SUCCESS","progress":"100%","data":{"result":{"videos":[{"url":["https://media.example/result.mp4"]}]}}}`),
	}

	videoURL, err := getNodyHubVideoURLFromTaskData(channel, task)
	require.NoError(t, err)
	assert.Equal(t, "https://media.example/result.mp4", videoURL)
}

func TestGetNodyHubVideoURLPrefersStoredDirectResult(t *testing.T) {
	channel := &model.Channel{
		Type:            constant.ChannelTypeOpenAI,
		ChannelProvider: "nodyhub",
	}
	task := &model.Task{
		TaskID: "public-task",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://media.example/direct.mp4",
		},
	}

	videoURL, err := getNodyHubVideoURLFromTaskData(channel, task)
	require.NoError(t, err)
	assert.Equal(t, "https://media.example/direct.mp4", videoURL)
}

func TestGetNodyHubVideoURLRejectsMissingStoredResult(t *testing.T) {
	channel := &model.Channel{
		Type:            constant.ChannelTypeOpenAI,
		ChannelProvider: "nodyhub",
	}
	task := &model.Task{
		TaskID: "public-task",
		PrivateData: model.TaskPrivateData{
			ResultURL: "/v1/videos/public-task/content",
		},
		Data: []byte(`{"task_id":"upstream-task","status":"SUCCESS","progress":"100%","data":{}}`),
	}

	_, err := getNodyHubVideoURLFromTaskData(channel, task)
	require.Error(t, err)
	assert.ErrorContains(t, err, "NodyHub video URL not found")
}
