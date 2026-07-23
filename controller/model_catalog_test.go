package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProfileDispatchReadyAcceptsREAsyncContracts(t *testing.T) {
	tests := []struct {
		operation        string
		responseContract string
	}{
		{operation: "image.generate", responseContract: "re-image-task-v1"},
		{operation: "video.generate", responseContract: "re-video-task-v1"},
		{operation: "audio.generate", responseContract: "re-audio-task-v1"},
		{operation: "text.generate", responseContract: "re-text-task-v1"},
	}

	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			assert.True(t, profileDispatchReady(test.operation, "re-task", "async", test.responseContract))
			assert.False(t, profileDispatchReady(test.operation, "re-task", "sync", test.responseContract))
		})
	}
	assert.False(t, profileDispatchReady("image.generate", "re-task", "async", "re-video-task-v1"))
}
