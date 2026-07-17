package controller

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestRelayRetryRequestPathExcludesGatewayQuery(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/v1/chat/completions", relayRetryRequestPath(&relaycommon.RelayInfo{
		RequestURLPath: "/v1/chat/completions?trace=true",
	}))
	assert.Equal(t, "/v1/images/generations", relayRetryRequestPath(&relaycommon.RelayInfo{
		RequestURLPath: "/v1/images/generations",
	}))
}
