package helper

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type modelOperationMaterialInput struct {
	mimeType string
	size     int64
	role     string
	known    bool
}

var probeModelOperationPublicMaterialURL = service.ProbePublicMaterialURL

func ModelOperationRequestParameters(c *gin.Context, request interface{}) (map[string]interface{}, error) {
	parameters, err := ModelOperationParameters(request)
	if err != nil || c == nil || c.Request == nil {
		return parameters, err
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return parameters, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return parameters, nil
	}
	raw := map[string]interface{}{}
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	for key, value := range raw {
		parameters[key] = value
	}
	return parameters, nil
}

func materialRuleNumber(rule map[string]interface{}, field string) (float64, bool) {
	value, exists := rule[field]
	if !exists {
		return 0, false
	}
	number, ok := value.(float64)
	if ok {
		return number, true
	}
	return 0, false
}

func modelOperationMIMETypeAllowed(actual string, allowed map[string]struct{}) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	if _, ok := allowed[actual]; ok {
		return true
	}
	mediaType, _, found := strings.Cut(actual, "/")
	if !found {
		return false
	}
	_, all := allowed["*/*"]
	_, family := allowed[mediaType+"/*"]
	return all || family
}

func materialValues(value interface{}) []interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []interface{}{typed}
	case []interface{}:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, materialValues(item)...)
		}
		return values
	case []string:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, materialValues(item)...)
		}
		return values
	default:
		return []interface{}{value}
	}
}

func inspectDataURI(value string) (modelOperationMaterialInput, error) {
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return modelOperationMaterialInput{}, nil
	}
	header, payload, found := strings.Cut(value[len("data:"):], ",")
	if !found {
		return modelOperationMaterialInput{}, errors.New("data URI is missing its payload separator")
	}
	parts := strings.Split(header, ";")
	mimeType := strings.ToLower(strings.TrimSpace(parts[0]))
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
		}
	}
	var (
		size    int64
		sniffed string
	)
	if isBase64 {
		decoded := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
		buffer := make([]byte, 512)
		read, readErr := io.ReadFull(decoded, buffer)
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return modelOperationMaterialInput{}, fmt.Errorf("data URI contains invalid base64: %w", readErr)
		}
		remaining, err := io.Copy(io.Discard, decoded)
		if err != nil {
			return modelOperationMaterialInput{}, fmt.Errorf("data URI contains invalid base64: %w", err)
		}
		size = int64(read) + remaining
		if read > 0 {
			sniffed = strings.ToLower(http.DetectContentType(buffer[:read]))
		}
	} else {
		decoded, err := url.PathUnescape(payload)
		if err != nil {
			return modelOperationMaterialInput{}, fmt.Errorf("data URI contains invalid escaping: %w", err)
		}
		size = int64(len(decoded))
		if decoded != "" {
			buffer := []byte(decoded)
			if len(buffer) > 512 {
				buffer = buffer[:512]
			}
			sniffed = strings.ToLower(http.DetectContentType(buffer))
		}
	}
	if size <= 0 {
		return modelOperationMaterialInput{}, errors.New("data URI payload is empty")
	}
	if mimeType == "" {
		mimeType = sniffed
	} else if sniffed != "" && sniffed != "application/octet-stream" && mimeType != sniffed {
		return modelOperationMaterialInput{}, fmt.Errorf("data URI declared MIME type %q but content is %q", mimeType, sniffed)
	}
	return modelOperationMaterialInput{mimeType: mimeType, size: size, known: true}, nil
}

func inspectMultipartFile(file *multipart.FileHeader) (modelOperationMaterialInput, error) {
	if file == nil {
		return modelOperationMaterialInput{}, errors.New("multipart file is required")
	}
	opened, err := file.Open()
	if err != nil {
		return modelOperationMaterialInput{}, err
	}
	defer opened.Close()
	buffer := make([]byte, 512)
	read, readErr := opened.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return modelOperationMaterialInput{}, readErr
	}
	if read == 0 {
		return modelOperationMaterialInput{}, errors.New("multipart file is empty")
	}
	mimeType := strings.ToLower(http.DetectContentType(buffer[:read]))
	return modelOperationMaterialInput{mimeType: mimeType, size: file.Size, known: true}, nil
}

func modelOperationMaterialContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func inspectMaterialValue(ctx context.Context, value interface{}, transport string) (modelOperationMaterialInput, error) {
	role := ""
	if object, ok := value.(map[string]interface{}); ok {
		for key := range object {
			if key != "url" && key != "role" {
				return modelOperationMaterialInput{}, fmt.Errorf("material object contains unsupported field %s", key)
			}
		}
		urlValue, ok := object["url"].(string)
		if !ok || strings.TrimSpace(urlValue) == "" {
			return modelOperationMaterialInput{}, errors.New("material object requires a non-empty url")
		}
		value = urlValue
		if rawRole, exists := object["role"]; exists {
			parsedRole, ok := rawRole.(string)
			if !ok || strings.TrimSpace(parsedRole) == "" {
				return modelOperationMaterialInput{}, errors.New("material object role must be a non-empty string")
			}
			role = strings.TrimSpace(parsedRole)
		}
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return modelOperationMaterialInput{}, errors.New("material must be a URL string or {url, role} object")
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(strings.ToLower(text), "data:") {
		if transport == "url" {
			return modelOperationMaterialInput{}, errors.New("transport url does not allow data URIs")
		}
		input, err := inspectDataURI(text)
		input.role = role
		return input, err
	}
	metadata, err := probeModelOperationPublicMaterialURL(ctx, text)
	if err != nil {
		return modelOperationMaterialInput{}, fmt.Errorf("probe public material URL: %w", err)
	}
	return modelOperationMaterialInput{
		mimeType: metadata.MIMEType,
		size:     metadata.Size,
		role:     role,
		known:    true,
	}, nil
}

func materialInputs(
	c *gin.Context,
	requestField string,
	transport string,
	parameters map[string]interface{},
	minimum float64,
	hasMinimum bool,
	maximum float64,
	hasMaximum bool,
) ([]modelOperationMaterialInput, int, error) {
	if c != nil && c.Request != nil && strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, 0, err
		}
		count := len(form.Value[requestField]) + len(form.File[requestField])
		if (hasMinimum && float64(count) < minimum) || (hasMaximum && float64(count) > maximum) {
			return nil, count, nil
		}
		if transport == "url" && len(form.File[requestField]) > 0 {
			return nil, count, fmt.Errorf("material field %s transport url does not allow multipart files", requestField)
		}
		inputs := make([]modelOperationMaterialInput, 0, count)
		for _, value := range form.Value[requestField] {
			for _, item := range materialValues(value) {
				input, err := inspectMaterialValue(modelOperationMaterialContext(c), item, transport)
				if err != nil {
					return nil, 0, err
				}
				inputs = append(inputs, input)
			}
		}
		for _, file := range form.File[requestField] {
			input, err := inspectMultipartFile(file)
			if err != nil {
				return nil, 0, err
			}
			inputs = append(inputs, input)
		}
		return inputs, count, nil
	}

	values := materialValues(parameters[requestField])
	count := len(values)
	if (hasMinimum && float64(count) < minimum) || (hasMaximum && float64(count) > maximum) {
		return nil, count, nil
	}
	inputs := make([]modelOperationMaterialInput, 0, len(values))
	for _, value := range values {
		input, err := inspectMaterialValue(modelOperationMaterialContext(c), value, transport)
		if err != nil {
			return nil, 0, err
		}
		inputs = append(inputs, input)
	}
	return inputs, count, nil
}

func ValidateModelOperationContractMaterials(c *gin.Context, contract *model.ModelOperationEffectiveContract, parameters map[string]interface{}) error {
	if contract == nil {
		return nil
	}
	for _, materialType := range []string{"image", "video", "audio"} {
		rule, ok := contract.MaterialSchema[materialType].(map[string]interface{})
		if !ok {
			continue
		}
		requestField, _ := rule["request_field"].(string)
		requestField = strings.TrimSpace(requestField)
		if requestField == "" {
			requestField = materialType
		}
		transport, _ := rule["transport"].(string)
		transport = strings.TrimSpace(transport)
		minimum, hasMinimum := materialRuleNumber(rule, "min_items")
		maximum, hasMaximum := materialRuleNumber(rule, "max_items")
		inputs, count, err := materialInputs(c, requestField, transport, parameters, minimum, hasMinimum, maximum, hasMaximum)
		if err != nil {
			return fmt.Errorf("material_schema.%s: %w", materialType, err)
		}
		if hasMinimum && float64(count) < minimum {
			return fmt.Errorf("material_schema.%s requires at least %s item(s)", materialType, strconvFormatNumber(minimum))
		}
		if hasMaximum && float64(count) > maximum {
			return fmt.Errorf("material_schema.%s allows at most %s item(s)", materialType, strconvFormatNumber(maximum))
		}
		if _, hasDurationLimit := materialRuleNumber(rule, "max_total_duration"); hasDurationLimit && count > 0 {
			return fmt.Errorf("material_schema.%s max_total_duration cannot be verified without server-side media parsing", materialType)
		}
		allowedRoles := make(map[string]struct{})
		if rawRoles, exists := rule["roles"].([]interface{}); exists {
			for _, rawRole := range rawRoles {
				if role, ok := rawRole.(string); ok {
					allowedRoles[strings.TrimSpace(role)] = struct{}{}
				}
			}
		}
		allowedMimeTypes := make(map[string]struct{})
		if rawMimeTypes, exists := rule["mime_types"].([]interface{}); exists {
			for _, rawMimeType := range rawMimeTypes {
				if mimeType, ok := rawMimeType.(string); ok {
					allowedMimeTypes[strings.ToLower(strings.TrimSpace(mimeType))] = struct{}{}
				}
			}
		}
		maximumSizeMB, hasMaximumSize := materialRuleNumber(rule, "max_size_mb")
		maximumSize := int64(math.Ceil(maximumSizeMB * 1024 * 1024))
		for _, input := range inputs {
			if len(allowedRoles) > 0 {
				if input.role == "" {
					return fmt.Errorf("material_schema.%s requires each item to declare a role", materialType)
				}
				if _, allowed := allowedRoles[input.role]; !allowed {
					return fmt.Errorf("material_schema.%s does not allow role %q", materialType, input.role)
				}
			}
			if len(allowedMimeTypes) > 0 {
				if !modelOperationMIMETypeAllowed(input.mimeType, allowedMimeTypes) {
					return fmt.Errorf("material_schema.%s does not allow MIME type %q", materialType, input.mimeType)
				}
			}
			if hasMaximumSize && input.size > maximumSize {
				return fmt.Errorf("material_schema.%s item exceeds maximum size %s MB", materialType, strconvFormatNumber(maximumSizeMB))
			}
		}
	}
	return nil
}

func strconvFormatNumber(value float64) string {
	return fmt.Sprintf("%g", value)
}
