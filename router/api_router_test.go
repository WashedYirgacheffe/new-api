package router

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenModelQuoteRouteDoesNotUseCriticalRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := engine.Routes()
	for _, route := range routes {
		if route.Method != http.MethodPost || route.Path != "/api/user/models/quote" {
			continue
		}

		assert.Contains(t, route.Handler, "QuoteTokenModel")
		return
	}

	require.Fail(t, "POST /api/user/models/quote route not found")
}

func TestTokenModelQuoteHandlersUseTokenAuthOnly(t *testing.T) {
	handlers := tokenModelQuoteHandlers()
	require.Len(t, handlers, 2)

	names := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		names = append(names, runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name())
	}
	joined := strings.Join(names, "\n")
	assert.Contains(t, joined, "TokenAuth")
	assert.Contains(t, joined, "controller.QuoteTokenModel")
	assert.NotContains(t, joined, "rateLimitFactory")
}
