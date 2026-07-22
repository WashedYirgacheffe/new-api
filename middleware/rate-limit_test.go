package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBypassGlobalAPIRateLimitOnlyAllowsTokenRuntimeReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "model catalog", method: http.MethodGet, path: "/api/user/models/catalog", want: true},
		{name: "model profile", method: http.MethodGet, path: "/api/user/models/profile", want: true},
		{name: "subsite catalog", method: http.MethodGet, path: "/api/subsites/by-domain/cjzz.top/catalog", want: true},
		{name: "model quote", method: http.MethodPost, path: "/api/user/models/quote", want: true},
		{name: "wrong quote method", method: http.MethodGet, path: "/api/user/models/quote", want: false},
		{name: "subsite mutation", method: http.MethodPost, path: "/api/subsites/claim", want: false},
		{name: "model write", method: http.MethodPost, path: "/api/models/", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(tt.method, tt.path, nil)
			assert.Equal(t, tt.want, bypassGlobalAPIRateLimit(ctx))
		})
	}
}
