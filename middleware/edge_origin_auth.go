package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const edgeOriginAuthHeader = "X-Edge-Origin-Auth"

func EdgeOriginAuth() gin.HandlerFunc {
	expectedSecret := []byte(strings.TrimSpace(os.Getenv("EDGE_SHARED_SECRET")))

	return func(c *gin.Context) {
		if len(expectedSecret) == 0 || c.Request.URL.Path == "/api/status" {
			c.Next()
			return
		}

		providedSecret := []byte(c.GetHeader(edgeOriginAuthHeader))
		if len(providedSecret) != len(expectedSecret) || subtle.ConstantTimeCompare(providedSecret, expectedSecret) != 1 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Next()
	}
}
