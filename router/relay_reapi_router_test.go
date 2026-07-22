package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRelayRouterRegistersRETaskRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, hasSubmission := routes["POST /v1/re/generations"]
	_, hasFetch := routes["GET /v1/re/tasks/:task_id"]
	assert.True(t, hasSubmission)
	assert.True(t, hasFetch)
}
