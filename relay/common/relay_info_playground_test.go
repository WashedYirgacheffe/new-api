package common

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenBaseRelayInfoNormalizesPlaygroundPathWithoutGatewayQuery(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/pg/images/generations?group=vip":                             "/v1/images/generations",
		"/pg/v1beta/models/gemini-2.5-flash:generateContent?group=vip": "/v1beta/models/gemini-2.5-flash:generateContent",
	}
	for requestURL, expectedPath := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", requestURL, nil)

		info := genBaseRelayInfo(ctx, nil)

		assert.True(t, info.IsPlayground)
		assert.Equal(t, expectedPath, info.RequestURLPath)
	}
}
