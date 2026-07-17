package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type publicMaterialTestResolver map[string][]net.IPAddr

type publicMaterialRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip publicMaterialRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type publicMaterialTrackingBody struct {
	reader *bytes.Reader
	read   int
}

func (body *publicMaterialTrackingBody) Read(buffer []byte) (int, error) {
	read, err := body.reader.Read(buffer)
	body.read += read
	return read, err
}

func (body *publicMaterialTrackingBody) Close() error {
	return nil
}

func publicMaterialMP4Bytes(size int) []byte {
	if size < 16 {
		size = 16
	}
	data := make([]byte, size)
	copy(data, []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'})
	return data
}

func publicMaterialPNGBytes(size int) []byte {
	if size < 8 {
		size = 8
	}
	data := make([]byte, size)
	copy(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	return data
}

func (r publicMaterialTestResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if addresses, ok := r[host]; ok {
		return addresses, nil
	}
	return nil, fmt.Errorf("unexpected lookup for %s", host)
}

func newPublicMaterialTestClient(t *testing.T, handler http.Handler, resolver publicMaterialTestResolver) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	actualAddress := server.Listener.Addr().String()
	dialer := &net.Dialer{}
	client := newPublicMaterialHTTPClientWithDialer(
		resolver,
		func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", actualAddress)
		},
	)
	t.Cleanup(client.CloseIdleConnections)
	return client, "http://media.example"
}

func TestValidatePublicMaterialURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantError string
	}{
		{name: "https domain", rawURL: "https://cdn.example/media.mp4?signature=value"},
		{name: "http domain", rawURL: "http://cdn.example/media.mp4"},
		{name: "credentials", rawURL: "https://user:secret@cdn.example/media.mp4", wantError: "credentials"},
		{name: "empty credentials", rawURL: "https://@cdn.example/media.mp4", wantError: "credentials"},
		{name: "public IPv4 literal", rawURL: "https://8.8.8.8/media.mp4", wantError: "IP literal"},
		{name: "IPv6 literal", rawURL: "https://[2606:4700:4700::1111]/media.mp4", wantError: "IP literal"},
		{name: "private IPv4 literal", rawURL: "http://127.0.0.1/media.mp4", wantError: "IP literal"},
		{name: "unsupported scheme", rawURL: "ftp://cdn.example/media.mp4", wantError: "HTTP or HTTPS"},
		{name: "relative URL", rawURL: "/media.mp4", wantError: "absolute"},
		{name: "non web port", rawURL: "https://cdn.example:8443/media.mp4", wantError: "port 8443 is not allowed"},
		{name: "fragment", rawURL: "https://cdn.example/media.mp4#fragment", wantError: "fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePublicMaterialURL(test.rawURL)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestPublicMaterialClientRejectsResolvedPrivateAddress(t *testing.T) {
	var dialed atomic.Bool
	client := newPublicMaterialHTTPClientWithDialer(
		publicMaterialTestResolver{
			"private.example": {{IP: net.ParseIP("169.254.169.254")}},
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, fmt.Errorf("must not dial")
		},
	)
	t.Cleanup(client.CloseIdleConnections)

	request, err := http.NewRequest(http.MethodHead, "http://private.example/media.mp4", nil)
	require.NoError(t, err)
	response, err := client.Do(request)

	require.Error(t, err)
	require.Nil(t, response)
	assert.Contains(t, err.Error(), "private IP address not allowed")
	assert.False(t, dialed.Load())
}

func TestPublicMaterialClientHasStrictTransportSurface(t *testing.T) {
	client := newPublicMaterialHTTPClientWithDialer(publicMaterialTestResolver{}, nil)
	t.Cleanup(client.CloseIdleConnections)
	roundTripper, ok := client.Transport.(*publicMaterialRoundTripper)
	require.True(t, ok)
	require.NotNil(t, roundTripper.transport)
	assert.Nil(t, roundTripper.transport.Proxy)
	assert.Nil(t, roundTripper.transport.TLSClientConfig)

	postRequest, err := http.NewRequest(http.MethodPost, "https://cdn.example/media.mp4", strings.NewReader("body"))
	require.NoError(t, err)
	postResponse, err := client.Do(postRequest)
	require.Error(t, err)
	require.Nil(t, postResponse)
	assert.Contains(t, err.Error(), "method POST is not allowed")

	authRequest, err := http.NewRequest(http.MethodHead, "https://cdn.example/media.mp4", nil)
	require.NoError(t, err)
	authRequest.Header.Set("Authorization", "Bearer secret")
	authResponse, err := client.Do(authRequest)
	require.Error(t, err)
	require.Nil(t, authResponse)
	assert.Contains(t, err.Error(), "header Authorization is not allowed")
}

func TestProbePublicMaterialURLUsesCompleteHeadMetadata(t *testing.T) {
	var headCalls atomic.Int32
	var getCalls atomic.Int32
	client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			headCalls.Add(1)
			assert.Equal(t, "identity", request.Header.Get("Accept-Encoding"))
			assert.Empty(t, request.Header.Get("Range"))
			writer.Header().Set("Content-Type", "Video/MP4; codecs=avc1")
			writer.Header().Set("Content-Length", "4096")
			writer.WriteHeader(http.StatusOK)
		case http.MethodGet:
			getCalls.Add(1)
			assert.Equal(t, "bytes=0-511", request.Header.Get("Range"))
			writer.Header().Set("Content-Type", "video/mp4")
			writer.Header().Set("Content-Length", "512")
			writer.Header().Set("Content-Range", "bytes 0-511/4096")
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = writer.Write(publicMaterialMP4Bytes(512))
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}), publicMaterialTestResolver{
		"media.example": {{IP: net.ParseIP("8.8.8.8")}},
	})

	metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media.mp4")

	require.NoError(t, err)
	assert.Equal(t, PublicMaterialMetadata{MIMEType: "video/mp4", Size: 4096}, metadata)
	assert.Equal(t, int32(1), headCalls.Load())
	assert.Equal(t, int32(1), getCalls.Load())
}

func TestProbePublicMaterialURLFallsBackToRangeGet(t *testing.T) {
	var headCalls atomic.Int32
	var getCalls atomic.Int32
	client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			headCalls.Add(1)
			writer.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			getCalls.Add(1)
			assert.Equal(t, "bytes=0-511", request.Header.Get("Range"))
			assert.Equal(t, "identity", request.Header.Get("Accept-Encoding"))
			writer.Header().Set("Content-Type", "video/mp4")
			writer.Header().Set("Content-Length", "512")
			writer.Header().Set("Content-Range", "bytes 0-511/8192")
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = writer.Write(publicMaterialMP4Bytes(512))
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}), publicMaterialTestResolver{
		"media.example": {{IP: net.ParseIP("8.8.8.8")}},
	})

	metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media.mp4")

	require.NoError(t, err)
	assert.Equal(t, PublicMaterialMetadata{MIMEType: "video/mp4", Size: 8192}, metadata)
	assert.Equal(t, int32(1), headCalls.Load())
	assert.Equal(t, int32(1), getCalls.Load())
}

func TestProbePublicMaterialURLFallsBackWhenHeadMetadataIsIncomplete(t *testing.T) {
	var getCalls atomic.Int32
	client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			writer.Header().Set("Content-Type", "video/mp4")
			writer.WriteHeader(http.StatusOK)
			return
		}
		getCalls.Add(1)
		writer.Header().Set("Content-Type", "video/mp4")
		writer.Header().Set("Content-Length", "512")
		writer.Header().Set("Content-Range", "bytes 0-511/1024")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write(publicMaterialMP4Bytes(512))
	}), publicMaterialTestResolver{
		"media.example": {{IP: net.ParseIP("8.8.8.8")}},
	})

	metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media.mp4")

	require.NoError(t, err)
	assert.Equal(t, int64(1024), metadata.Size)
	assert.Equal(t, int32(1), getCalls.Load())
}

func TestProbePublicMaterialURLAcceptsRangeGetWithFullResponseLength(t *testing.T) {
	client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		assert.Equal(t, "bytes=0-511", request.Header.Get("Range"))
		writer.Header().Set("Content-Type", "video/mp4")
		writer.Header().Set("Content-Length", "2048")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(publicMaterialMP4Bytes(2048))
	}), publicMaterialTestResolver{
		"media.example": {{IP: net.ParseIP("8.8.8.8")}},
	})

	metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media.mp4")

	require.NoError(t, err)
	assert.Equal(t, PublicMaterialMetadata{MIMEType: "video/mp4", Size: 2048}, metadata)
}

func TestPublicMaterialMetadataRejectsUnknownFullResponseLength(t *testing.T) {
	metadata, err := publicMaterialMetadataFromResponse(&http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"video/mp4"}},
		ContentLength: -1,
	})

	require.Error(t, err)
	assert.Equal(t, PublicMaterialMetadata{}, metadata)
	assert.Contains(t, err.Error(), "Content-Length")
}

func TestDetectPublicMaterialMIMESupportsRequiredFormats(t *testing.T) {
	tests := []struct {
		name     string
		prefix   []byte
		wantMIME string
	}{
		{name: "PNG", prefix: publicMaterialPNGBytes(16), wantMIME: "image/png"},
		{name: "JPEG", prefix: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, wantMIME: "image/jpeg"},
		{name: "WebP", prefix: []byte{'R', 'I', 'F', 'F', 0x10, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}, wantMIME: "image/webp"},
		{name: "GIF", prefix: []byte("GIF89a"), wantMIME: "image/gif"},
		{name: "MP4", prefix: publicMaterialMP4Bytes(16), wantMIME: "video/mp4"},
		{name: "WebM", prefix: []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}, wantMIME: "video/webm"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mimeType, err := detectPublicMaterialMIME(test.prefix)
			require.NoError(t, err)
			assert.Equal(t, test.wantMIME, mimeType)
		})
	}
}

func TestProbePublicMaterialURLRejectsMIMEAndMetadataInconsistency(t *testing.T) {
	tests := []struct {
		name      string
		headMIME  string
		headSize  int
		getMIME   string
		getSize   int
		body      []byte
		wantError string
	}{
		{
			name:      "forged content type",
			headMIME:  "video/mp4",
			headSize:  4096,
			getMIME:   "video/mp4",
			getSize:   4096,
			body:      publicMaterialPNGBytes(512),
			wantError: "does not match detected MIME type",
		},
		{
			name:      "HEAD and GET MIME mismatch",
			headMIME:  "image/png",
			headSize:  4096,
			getMIME:   "video/mp4",
			getSize:   4096,
			body:      publicMaterialMP4Bytes(512),
			wantError: "HEAD MIME type",
		},
		{
			name:      "HEAD and GET length mismatch",
			headMIME:  "video/mp4",
			headSize:  4096,
			getMIME:   "video/mp4",
			getSize:   8192,
			body:      publicMaterialMP4Bytes(512),
			wantError: "HEAD length",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodHead {
					writer.Header().Set("Content-Type", test.headMIME)
					writer.Header().Set("Content-Length", fmt.Sprint(test.headSize))
					writer.WriteHeader(http.StatusOK)
					return
				}
				writer.Header().Set("Content-Type", test.getMIME)
				writer.Header().Set("Content-Length", "512")
				writer.Header().Set("Content-Range", fmt.Sprintf("bytes 0-511/%d", test.getSize))
				writer.WriteHeader(http.StatusPartialContent)
				_, _ = writer.Write(test.body)
			}), publicMaterialTestResolver{
				"media.example": {{IP: net.ParseIP("8.8.8.8")}},
			})

			metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media")

			require.Error(t, err)
			assert.Equal(t, PublicMaterialMetadata{}, metadata)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestProbePublicMaterialURLLimitsIgnoredRangeResponseRead(t *testing.T) {
	payload := publicMaterialMP4Bytes(4096)
	body := &publicMaterialTrackingBody{reader: bytes.NewReader(payload)}
	var requests atomic.Int32
	client := &http.Client{Transport: publicMaterialRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": []string{"video/mp4"}},
				Body:          io.NopCloser(strings.NewReader("")),
				ContentLength: int64(len(payload)),
			}, nil
		}
		assert.Equal(t, "bytes=0-511", request.Header.Get("Range"))
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"video/mp4"}},
			Body:          body,
			ContentLength: int64(len(payload)),
		}, nil
	})}

	metadata, err := probePublicMaterialURL(context.Background(), client, "http://media.example/material.mp4")

	require.NoError(t, err)
	assert.Equal(t, PublicMaterialMetadata{MIMEType: "video/mp4", Size: int64(len(payload))}, metadata)
	assert.Equal(t, int32(2), requests.Load())
	assert.Equal(t, publicMaterialReadLimit, body.read)
}

func TestProbePublicMaterialURLFailsClosedForIncompleteRangeMetadata(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		contentRange string
		wantError    string
	}{
		{name: "missing content type", contentRange: "bytes 0-511/1024", wantError: "missing Content-Type"},
		{name: "missing content range", contentType: "video/mp4", wantError: "valid Content-Range"},
		{name: "unknown total", contentType: "video/mp4", contentRange: "bytes 0-511/*", wantError: "valid Content-Range"},
		{name: "invalid MIME", contentType: "*/*", contentRange: "bytes 0-511/1024", wantError: "invalid Content-Type"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodHead {
					writer.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				writer.Header().Set("Content-Type", test.contentType)
				if test.contentRange != "" {
					writer.Header().Set("Content-Range", test.contentRange)
				}
				writer.Header().Set("Content-Length", "512")
				writer.WriteHeader(http.StatusPartialContent)
				_, _ = writer.Write(publicMaterialMP4Bytes(512))
			}), publicMaterialTestResolver{
				"media.example": {{IP: net.ParseIP("8.8.8.8")}},
			})

			metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/media.mp4")

			require.Error(t, err)
			assert.Equal(t, PublicMaterialMetadata{}, metadata)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestProbePublicMaterialURLValidatesEveryRedirect(t *testing.T) {
	tests := []struct {
		name      string
		location  string
		wantError string
	}{
		{name: "IP literal", location: "http://127.0.0.1/private", wantError: "IP literal"},
		{name: "credentials", location: "http://user:secret@cdn.example/private", wantError: "credentials"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Location", test.location)
				writer.WriteHeader(http.StatusFound)
			}), publicMaterialTestResolver{
				"media.example": {{IP: net.ParseIP("8.8.8.8")}},
			})

			metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/redirect")

			require.Error(t, err)
			assert.Equal(t, PublicMaterialMetadata{}, metadata)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestProbePublicMaterialURLRejectsRedirectResolvingPrivate(t *testing.T) {
	client, baseURL := newPublicMaterialTestClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Location", "http://private.example/media.mp4")
		writer.WriteHeader(http.StatusFound)
	}), publicMaterialTestResolver{
		"media.example":   {{IP: net.ParseIP("8.8.8.8")}},
		"private.example": {{IP: net.ParseIP("10.0.0.1")}},
	})

	metadata, err := probePublicMaterialURL(context.Background(), client, baseURL+"/redirect")

	require.Error(t, err)
	assert.Equal(t, PublicMaterialMetadata{}, metadata)
	assert.Contains(t, err.Error(), "private IP address not allowed")
}
