package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newEdgeOriginAuthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(EdgeOriginAuth())
	router.GET("/api/status", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/private", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func TestEdgeOriginAuthDisabledWithoutSecret(t *testing.T) {
	t.Setenv("EDGE_SHARED_SECRET", "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/private", nil)

	newEdgeOriginAuthTestRouter().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestEdgeOriginAuthAllowsHealthCheckWithoutHeader(t *testing.T) {
	t.Setenv("EDGE_SHARED_SECRET", "expected-secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)

	newEdgeOriginAuthTestRouter().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestEdgeOriginAuthRejectsInvalidSecret(t *testing.T) {
	t.Setenv("EDGE_SHARED_SECRET", "expected-secret")

	for _, providedSecret := range []string{"", "wrong-secret"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/private", nil)
		request.Header.Set(edgeOriginAuthHeader, providedSecret)

		newEdgeOriginAuthTestRouter().ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	}
}

func TestEdgeOriginAuthAllowsMatchingSecret(t *testing.T) {
	t.Setenv("EDGE_SHARED_SECRET", "expected-secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.Header.Set(edgeOriginAuthHeader, "expected-secret")

	newEdgeOriginAuthTestRouter().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
}
