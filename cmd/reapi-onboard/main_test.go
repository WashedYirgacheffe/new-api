package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredKeysAllowsIndependentChannelOnboarding(t *testing.T) {
	t.Setenv("REAPI_CHAT_API_KEY", "")
	t.Setenv("REAPI_TASK_API_KEY", "")
	_, _, err := configuredKeys()
	require.Error(t, err)

	t.Setenv("REAPI_TASK_API_KEY", "task-key")
	chatKey, taskKey, err := configuredKeys()
	require.NoError(t, err)
	assert.Empty(t, chatKey)
	assert.Equal(t, "task-key", taskKey)

	t.Setenv("REAPI_TASK_API_KEY", "")
	t.Setenv("REAPI_CHAT_API_KEY", "chat-key")
	chatKey, taskKey, err = configuredKeys()
	require.NoError(t, err)
	assert.Equal(t, "chat-key", chatKey)
	assert.Empty(t, taskKey)
}
