package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	publicMaterialMaxURLLength       = 8 * 1024
	publicMaterialMaxRedirects       = 5
	publicMaterialProbeTimeout       = 10 * time.Second
	publicMaterialResponseHeaderTime = 5 * time.Second
	publicMaterialMaxResponseHeaders = 64 * 1024
	publicMaterialSniffBytes         = 512
	publicMaterialReadLimit          = publicMaterialSniffBytes + 1
)

var (
	publicMaterialProtection = &common.SSRFProtection{
		AllowPrivateIp:         false,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		AllowedPorts:           []int{httpPort, httpsPort},
		ApplyIPFilterForDomain: true,
	}
	publicMaterialHTTPClient = newPublicMaterialHTTPClientWithDialer(nil, nil)
)

const (
	httpPort  = 80
	httpsPort = 443
)

// PublicMaterialMetadata is the HTTP metadata verified for a public material URL.
type PublicMaterialMetadata struct {
	MIMEType string
	Size     int64
}

// ValidatePublicMaterialURL applies the non-configurable URL policy used for
// user-controlled material probes. DNS answers are checked again by the
// client's protected dialer immediately before a connection is opened.
func ValidatePublicMaterialURL(rawURL string) error {
	_, err := parsePublicMaterialURL(rawURL)
	return err
}

// GetPublicMaterialHTTPClient returns the strict client for probing
// user-controlled public material URLs. Its policy cannot be relaxed by the
// general fetch settings.
func GetPublicMaterialHTTPClient() *http.Client {
	return publicMaterialHTTPClient
}

// ProbePublicMaterialURL verifies HTTP metadata and the leading bytes of a
// public material URL. HEAD is used as an independent metadata signal; a
// bounded range GET is always performed to verify the declared media type.
func ProbePublicMaterialURL(ctx context.Context, rawURL string) (PublicMaterialMetadata, error) {
	return probePublicMaterialURL(ctx, GetPublicMaterialHTTPClient(), rawURL)
}

func probePublicMaterialURL(ctx context.Context, client *http.Client, rawURL string) (PublicMaterialMetadata, error) {
	if ctx == nil {
		return PublicMaterialMetadata{}, errors.New("public material probe context is required")
	}
	if client == nil {
		return PublicMaterialMetadata{}, errors.New("public material HTTP client is required")
	}
	parsedURL, err := parsePublicMaterialURL(rawURL)
	if err != nil {
		return PublicMaterialMetadata{}, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, publicMaterialProbeTimeout)
	defer cancel()

	var headMetadata PublicMaterialMetadata
	var headHasMIME bool
	var headHasSize bool
	headResponse, headErr := doPublicMaterialProbeRequest(probeCtx, client, http.MethodHead, parsedURL.String())
	if headErr == nil {
		statusOK := headResponse.StatusCode >= http.StatusOK && headResponse.StatusCode < http.StatusMultipleChoices
		var metadataErr error
		if statusOK {
			headMetadata, headHasMIME, headHasSize, metadataErr = publicMaterialOptionalMetadataFromHead(headResponse)
		}
		headResponse.Body.Close()
		if metadataErr != nil {
			return PublicMaterialMetadata{}, metadataErr
		}
	} else if probeCtx.Err() != nil {
		return PublicMaterialMetadata{}, probeCtx.Err()
	}

	rangeResponse, err := doPublicMaterialProbeRequest(probeCtx, client, http.MethodGet, parsedURL.String())
	if err != nil {
		return PublicMaterialMetadata{}, fmt.Errorf("public material range probe failed: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer rangeResponse.Body.Close()
	if rangeResponse.StatusCode != http.StatusOK && rangeResponse.StatusCode != http.StatusPartialContent {
		return PublicMaterialMetadata{}, fmt.Errorf("public material range probe returned HTTP %d", rangeResponse.StatusCode)
	}
	metadata, err := publicMaterialMetadataFromResponse(rangeResponse)
	if err != nil {
		return PublicMaterialMetadata{}, err
	}
	detectedMIME, err := sniffPublicMaterialResponse(rangeResponse)
	if err != nil {
		return PublicMaterialMetadata{}, err
	}
	if detectedMIME != metadata.MIMEType {
		return PublicMaterialMetadata{}, fmt.Errorf("public material declared MIME type %q does not match detected MIME type %q", metadata.MIMEType, detectedMIME)
	}
	if headHasMIME && headMetadata.MIMEType != metadata.MIMEType {
		return PublicMaterialMetadata{}, fmt.Errorf("public material HEAD MIME type %q does not match range MIME type %q", headMetadata.MIMEType, metadata.MIMEType)
	}
	if headHasSize && headMetadata.Size != metadata.Size {
		return PublicMaterialMetadata{}, fmt.Errorf("public material HEAD length %d does not match range total length %d", headMetadata.Size, metadata.Size)
	}
	return metadata, nil
}

func doPublicMaterialProbeRequest(ctx context.Context, client *http.Client, method string, rawURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	if method == http.MethodGet {
		request.Header.Set("Range", "bytes=0-511")
	}
	return client.Do(request)
}

func publicMaterialOptionalMetadataFromHead(response *http.Response) (PublicMaterialMetadata, bool, bool, error) {
	mimeType, hasMIME, err := publicMaterialDeclaredMIME(response)
	if err != nil {
		return PublicMaterialMetadata{}, false, false, err
	}
	metadata := PublicMaterialMetadata{MIMEType: mimeType}
	hasSize := false
	if response.StatusCode == http.StatusPartialContent {
		byteRange, err := parsePublicMaterialContentRange(response.Header.Get("Content-Range"))
		if err != nil {
			return PublicMaterialMetadata{}, false, false, err
		}
		metadata.Size = byteRange.total
		hasSize = true
	} else if response.ContentLength > 0 {
		metadata.Size = response.ContentLength
		hasSize = true
	}
	return metadata, hasMIME, hasSize, nil
}

func publicMaterialMetadataFromResponse(response *http.Response) (PublicMaterialMetadata, error) {
	if response == nil {
		return PublicMaterialMetadata{}, errors.New("public material response is required")
	}
	mediaType, hasMIME, err := publicMaterialDeclaredMIME(response)
	if err != nil {
		return PublicMaterialMetadata{}, err
	}
	if !hasMIME {
		return PublicMaterialMetadata{}, errors.New("public material response is missing Content-Type")
	}

	size := response.ContentLength
	if response.StatusCode == http.StatusPartialContent {
		byteRange, rangeErr := parsePublicMaterialContentRange(response.Header.Get("Content-Range"))
		if rangeErr != nil {
			return PublicMaterialMetadata{}, rangeErr
		}
		if byteRange.start != 0 || byteRange.end >= publicMaterialSniffBytes {
			return PublicMaterialMetadata{}, errors.New("public material range response does not cover the requested prefix")
		}
		expectedLength := byteRange.end - byteRange.start + 1
		if response.ContentLength <= 0 || response.ContentLength != expectedLength {
			return PublicMaterialMetadata{}, errors.New("public material range response is missing a consistent Content-Length")
		}
		size = byteRange.total
	}
	if size <= 0 {
		return PublicMaterialMetadata{}, errors.New("public material response is missing a positive Content-Length")
	}
	return PublicMaterialMetadata{
		MIMEType: mediaType,
		Size:     size,
	}, nil
}

func publicMaterialDeclaredMIME(response *http.Response) (string, bool, error) {
	if response == nil {
		return "", false, errors.New("public material response is required")
	}
	rawContentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if rawContentType == "" {
		return "", false, nil
	}
	mediaType, _, err := mime.ParseMediaType(rawContentType)
	if err != nil || !strings.Contains(mediaType, "/") || strings.Contains(mediaType, "*") {
		return "", false, fmt.Errorf("public material response has invalid Content-Type %q", rawContentType)
	}
	return strings.ToLower(mediaType), true, nil
}

func sniffPublicMaterialResponse(response *http.Response) (string, error) {
	prefix, err := io.ReadAll(io.LimitReader(response.Body, publicMaterialReadLimit))
	if err != nil {
		return "", fmt.Errorf("failed to read public material prefix: %w", err)
	}
	expectedRead := response.ContentLength
	if expectedRead > publicMaterialReadLimit {
		expectedRead = publicMaterialReadLimit
	}
	if expectedRead <= 0 || int64(len(prefix)) != expectedRead {
		return "", errors.New("public material response body length does not match Content-Length")
	}
	if len(prefix) > publicMaterialSniffBytes {
		prefix = prefix[:publicMaterialSniffBytes]
	}
	return detectPublicMaterialMIME(prefix)
}

func detectPublicMaterialMIME(prefix []byte) (string, error) {
	if len(prefix) == 0 {
		return "", errors.New("public material response body is empty")
	}
	if isMP4Prefix(prefix) {
		return "video/mp4", nil
	}
	if isWebMPrefix(prefix) {
		return "video/webm", nil
	}
	detected, _, err := mime.ParseMediaType(http.DetectContentType(prefix))
	if err != nil {
		return "", fmt.Errorf("failed to detect public material MIME type: %w", err)
	}
	detected = strings.ToLower(detected)
	if detected == "application/octet-stream" || (!strings.HasPrefix(detected, "image/") && !strings.HasPrefix(detected, "video/") && !strings.HasPrefix(detected, "audio/")) {
		return "", fmt.Errorf("public material MIME type could not be verified from its prefix: %s", detected)
	}
	return detected, nil
}

func isMP4Prefix(prefix []byte) bool {
	return len(prefix) >= 12 && bytes.Equal(prefix[4:8], []byte("ftyp"))
}

func isWebMPrefix(prefix []byte) bool {
	if len(prefix) < 8 || !bytes.Equal(prefix[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return false
	}
	for index := 4; index+3 < len(prefix); index++ {
		if prefix[index] != 0x42 || prefix[index+1] != 0x82 {
			continue
		}
		size, width, ok := parseEBMLVariableInteger(prefix[index+2:])
		valueStart := index + 2 + width
		if ok && size == 4 && valueStart+size <= len(prefix) && bytes.Equal(prefix[valueStart:valueStart+size], []byte("webm")) {
			return true
		}
	}
	return false
}

func parseEBMLVariableInteger(value []byte) (int, int, bool) {
	if len(value) == 0 || value[0] == 0 {
		return 0, 0, false
	}
	marker := byte(0x80)
	width := 1
	for width <= 8 && value[0]&marker == 0 {
		marker >>= 1
		width++
	}
	if width > 8 || len(value) < width {
		return 0, 0, false
	}
	result := int(value[0] &^ marker)
	for index := 1; index < width; index++ {
		result = result<<8 | int(value[index])
	}
	return result, width, true
}

type publicMaterialContentRange struct {
	start int64
	end   int64
	total int64
}

func parsePublicMaterialContentRange(rawContentRange string) (publicMaterialContentRange, error) {
	value := strings.TrimSpace(rawContentRange)
	invalid := func() (publicMaterialContentRange, error) {
		return publicMaterialContentRange{}, errors.New("public material range response is missing a valid Content-Range")
	}
	if len(value) < len("bytes ") || !strings.EqualFold(value[:len("bytes ")], "bytes ") {
		return invalid()
	}
	rangeAndTotal := strings.Split(strings.TrimSpace(value[len("bytes "):]), "/")
	if len(rangeAndTotal) != 2 || rangeAndTotal[1] == "*" {
		return invalid()
	}
	byteRange := strings.Split(strings.TrimSpace(rangeAndTotal[0]), "-")
	if len(byteRange) != 2 {
		return invalid()
	}
	start, startErr := strconv.ParseInt(strings.TrimSpace(byteRange[0]), 10, 64)
	end, endErr := strconv.ParseInt(strings.TrimSpace(byteRange[1]), 10, 64)
	total, totalErr := strconv.ParseInt(strings.TrimSpace(rangeAndTotal[1]), 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || start < 0 || end < start || total <= end {
		return invalid()
	}
	return publicMaterialContentRange{start: start, end: end, total: total}, nil
}

func parsePublicMaterialURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("public material URL is required")
	}
	if len(rawURL) > publicMaterialMaxURLLength {
		return nil, fmt.Errorf("public material URL exceeds %d bytes", publicMaterialMaxURLLength)
	}
	if strings.Contains(rawURL, "#") {
		return nil, errors.New("public material URL must not contain a fragment")
	}
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid public material URL: %w", err)
	}
	if parsedURL.Opaque != "" || !parsedURL.IsAbs() || parsedURL.Host == "" {
		return nil, errors.New("public material URL must be absolute")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("public material URL must use HTTP or HTTPS")
	}
	if parsedURL.User != nil {
		return nil, errors.New("public material URL must not contain credentials")
	}
	if parsedURL.Fragment != "" {
		return nil, errors.New("public material URL must not contain a fragment")
	}

	host := parsedURL.Hostname()
	if host == "" {
		return nil, errors.New("public material URL host is required")
	}
	literalHost := strings.TrimSuffix(host, ".")
	if address, parseErr := netip.ParseAddr(literalHost); parseErr == nil && address.IsValid() {
		return nil, errors.New("public material URL must use a domain name, not an IP literal")
	}

	port := httpPort
	if parsedURL.Scheme == "https" {
		port = httpsPort
	}
	if portText := parsedURL.Port(); portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("public material URL has invalid port %q", portText)
		}
	} else if strings.HasSuffix(parsedURL.Host, ":") {
		return nil, errors.New("public material URL has an empty port")
	}
	if err := publicMaterialProtection.ValidateNetworkTarget(host, port); err != nil {
		return nil, fmt.Errorf("public material URL is not allowed: %w", err)
	}
	return parsedURL, nil
}

type publicMaterialRoundTripper struct {
	transport *http.Transport
}

func (t *publicMaterialRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("public material request is invalid")
	}
	if request.Method != http.MethodHead && request.Method != http.MethodGet {
		return nil, fmt.Errorf("public material request method %s is not allowed", request.Method)
	}
	if err := ValidatePublicMaterialURL(request.URL.String()); err != nil {
		return nil, err
	}
	for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
		if request.Header.Get(header) != "" {
			return nil, fmt.Errorf("public material request header %s is not allowed", header)
		}
	}

	cleanRequest := request.Clone(request.Context())
	cleanRequest.Header = request.Header.Clone()
	cleanRequest.Header.Del("Referer")
	return t.transport.RoundTrip(cleanRequest)
}

func (t *publicMaterialRoundTripper) CloseIdleConnections() {
	if t != nil && t.transport != nil {
		t.transport.CloseIdleConnections()
	}
}

func checkPublicMaterialRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= publicMaterialMaxRedirects {
		return fmt.Errorf("public material request stopped after %d redirects", publicMaterialMaxRedirects)
	}
	if request == nil || request.URL == nil {
		return errors.New("public material redirect is invalid")
	}
	if err := ValidatePublicMaterialURL(request.URL.String()); err != nil {
		return fmt.Errorf("public material redirect blocked: %w", err)
	}
	request.Header.Del("Referer")
	return nil
}

func newPublicMaterialHTTPClientWithDialer(resolver ssrfResolver, dialContext func(context.Context, string, string) (net.Conn, error)) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialContext == nil {
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialContext = dialer.DialContext
	}
	protectedDialer := &protectedFetchDialer{
		resolver:    resolver,
		dialContext: dialContext,
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return publicMaterialProtection, true, nil
		},
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            protectedDialer.DialContext,
		ForceAttemptHTTP2:      true,
		DisableCompression:     true,
		MaxIdleConns:           16,
		MaxIdleConnsPerHost:    4,
		MaxConnsPerHost:        8,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  publicMaterialResponseHeaderTime,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: publicMaterialMaxResponseHeaders,
	}
	return &http.Client{
		Transport:     &publicMaterialRoundTripper{transport: transport},
		CheckRedirect: checkPublicMaterialRedirect,
		Timeout:       publicMaterialProbeTimeout,
	}
}
