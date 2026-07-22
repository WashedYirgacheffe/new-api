/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayRequestPathNormalizesPlaygroundMediaEndpoints(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/pg/chat/completions":                     "/v1/chat/completions",
		"/pg/images/generations":                   "/v1/images/generations",
		"/pg/videos":                               "/v1/videos",
		"/pg/videos/task-1":                        "/v1/videos/task-1",
		"/pg/v1beta/models/gemini:generateContent": "/v1beta/models/gemini:generateContent",
		"/v1/images/generations":                   "/v1/images/generations",
	}

	for input, expected := range tests {
		assert.Equal(t, expected, relayRequestPath(input), input)
	}
}

func TestRequestedRelayGroupUsesTokenRelayHeaderAndDoesNotForwardIt(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?keep=1&_carlab_route_group=silver", nil)
	ctx.Request.Header.Set(carLabRouteGroupHeader, "gold")

	group := requestedRelayGroup(ctx, &ModelRequest{Group: "untrusted-body-group"})

	assert.Equal(t, "gold", group)
	assert.Empty(t, ctx.Request.Header.Get(carLabRouteGroupHeader))
	assert.Empty(t, ctx.Request.URL.Query().Get(carLabRouteGroupQuery))
	assert.Equal(t, "1", ctx.Request.URL.Query().Get("keep"))
	assert.NotContains(t, ctx.Request.RequestURI, carLabRouteGroupQuery)
	assert.Equal(t, ctx.Request.URL.RequestURI(), ctx.Request.RequestURI)
}

func TestRequestedRelayGroupUsesQueryFallbackAndDoesNotForwardIt(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?keep=1&_carlab_route_group=gold", nil)

	group := requestedRelayGroup(ctx, &ModelRequest{Group: "untrusted-body-group"})

	assert.Equal(t, "gold", group)
	assert.Empty(t, ctx.Request.URL.Query().Get(carLabRouteGroupQuery))
	assert.Equal(t, "1", ctx.Request.URL.Query().Get("keep"))
	assert.NotContains(t, ctx.Request.URL.String(), carLabRouteGroupQuery)
	assert.NotContains(t, ctx.Request.RequestURI, carLabRouteGroupQuery)
	assert.Equal(t, ctx.Request.URL.RequestURI(), ctx.Request.RequestURI)
}

func TestGetModelRequestParsesPlaygroundMediaEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		method              string
		path                string
		body                string
		expectedModel       string
		expectedGroup       string
		expectedRelayMode   int
		shouldSelectChannel bool
	}{
		{
			name:   "chat body group",
			method: http.MethodPost, path: "/pg/chat/completions",
			body:          `{"model":"gpt-4.1","group":"vip"}`,
			expectedModel: "gpt-4.1", expectedGroup: "vip", shouldSelectChannel: true,
		},
		{
			name:   "image body group",
			method: http.MethodPost, path: "/pg/images/generations",
			body:          `{"model":"gpt-image-1","group":"vip"}`,
			expectedModel: "gpt-image-1", expectedGroup: "vip", shouldSelectChannel: true,
		},
		{
			name:   "gemini model path",
			method: http.MethodPost, path: "/pg/v1beta/models/gemini-2.5-flash:generateContent",
			body:          `{}`,
			expectedModel: "gemini-2.5-flash", expectedRelayMode: constant.RelayModeGemini, shouldSelectChannel: true,
		},
		{
			name:   "video submit",
			method: http.MethodPost, path: "/pg/videos",
			body:          `{"model":"sora-2","group":"vip"}`,
			expectedModel: "sora-2", expectedGroup: "vip", expectedRelayMode: constant.RelayModeVideoSubmit, shouldSelectChannel: true,
		},
		{
			name:   "video fetch",
			method: http.MethodGet, path: "/pg/videos/task-1",
			expectedRelayMode: constant.RelayModeVideoFetchByID, shouldSelectChannel: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				ctx.Request.Header.Set("Content-Type", "application/json")
			}

			request, shouldSelectChannel, err := getModelRequest(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedModel, request.Model)
			assert.Equal(t, test.expectedGroup, request.Group)
			assert.Equal(t, test.shouldSelectChannel, shouldSelectChannel)
			if test.expectedRelayMode != 0 {
				assert.Equal(t, test.expectedRelayMode, ctx.GetInt("relay_mode"))
			}
		})
	}
}
