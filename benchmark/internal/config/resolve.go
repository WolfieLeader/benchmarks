package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func resolve(cfg *Config) ([]*ResolvedServer, error) {
	timeout, _ := time.ParseDuration(cfg.Global.Timeout)

	var allTestcases []*Testcase
	for endpointName, endpoint := range cfg.Endpoints {
		testcases, err := resolveEndpoint(cfg.Global.BaseURL, endpointName, &endpoint)
		if err != nil {
			return nil, err
		}
		allTestcases = append(allTestcases, testcases...)
	}

	servers := make([]*ResolvedServer, 0, len(cfg.Servers))
	for name, port := range cfg.Servers {
		servers = append(servers, &ResolvedServer{
			Name:                name,
			ImageName:           name,
			Port:                port,
			BaseURL:             cfg.Global.BaseURL,
			Timeout:             timeout,
			CPULimit:            cfg.Global.CPULimit,
			MemoryLimit:         cfg.Global.MemoryLimit,
			Workers:             cfg.Global.Workers,
			RequestsPerEndpoint: cfg.Global.RequestsPerEndpoint,
			Testcases:           allTestcases,
		})
	}

	return servers, nil
}

func resolveEndpoint(baseURL, endpointName string, endpoint *EndpointConfig) ([]*Testcase, error) {
	// Load file if specified at endpoint level
	endpointFile, err := loadFile(endpoint.File)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q file: %w", endpointName, err)
	}

	// If no test cases defined, create a default one
	if len(endpoint.TestCases) == 0 {
		tc, err := buildTestcase(baseURL, endpointName, "default", endpoint, nil, endpointFile)
		if err != nil {
			return nil, err
		}
		return []*Testcase{tc}, nil
	}

	// Build test case for each defined test case
	testcases := make([]*Testcase, 0, len(endpoint.TestCases))
	for i, tcConfig := range endpoint.TestCases {
		// Load test case specific file if present
		file := endpointFile
		if tcConfig.File != "" {
			file, err = loadFile(tcConfig.File)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q test case %d file: %w", endpointName, i, err)
			}
		}

		tc, err := buildTestcase(baseURL, endpointName, tcConfig.Name, endpoint, &tcConfig, file)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q test case %q: %w", endpointName, tcConfig.Name, err)
		}
		testcases = append(testcases, tc)
	}

	return testcases, nil
}

func buildTestcase(baseURL, endpointName, name string, endpoint *EndpointConfig, tcOverride *TestCaseConfig, file *FileUpload) (*Testcase, error) {
	// Start with endpoint values
	path := endpoint.Path
	method := strings.ToUpper(endpoint.Method)
	headers := maps.Clone(endpoint.Headers)
	query := maps.Clone(endpoint.Query)
	body := endpoint.Body
	formData := maps.Clone(endpoint.FormData)
	expectedStatus := endpoint.ExpectedStatus
	expectedHeaders := maps.Clone(endpoint.ExpectedHeaders)
	expectedBody := endpoint.ExpectedBody
	expectedText := endpoint.ExpectedText

	// Apply test case overrides if present
	if tcOverride != nil {
		if tcOverride.Path != "" {
			path = tcOverride.Path
		}
		if len(tcOverride.Headers) > 0 {
			if headers == nil {
				headers = make(map[string]string)
			}
			maps.Copy(headers, tcOverride.Headers)
		}
		if len(tcOverride.Query) > 0 {
			if query == nil {
				query = make(map[string]string)
			}
			maps.Copy(query, tcOverride.Query)
		}
		if tcOverride.Body != nil {
			body = tcOverride.Body
		}
		if len(tcOverride.FormData) > 0 {
			if formData == nil {
				formData = make(map[string]string)
			}
			maps.Copy(formData, tcOverride.FormData)
		}
		if tcOverride.ExpectedStatus != 0 {
			expectedStatus = tcOverride.ExpectedStatus
		}
		if len(tcOverride.ExpectedHeaders) > 0 {
			if expectedHeaders == nil {
				expectedHeaders = make(map[string]string)
			}
			maps.Copy(expectedHeaders, tcOverride.ExpectedHeaders)
		}
		if tcOverride.ExpectedBody != nil {
			expectedBody = tcOverride.ExpectedBody
		}
		if tcOverride.ExpectedText != "" {
			expectedText = tcOverride.ExpectedText
		}
	}

	// Build full URL with query params
	fullURL, err := buildURL(baseURL, path, query)
	if err != nil {
		return nil, err
	}

	tc := &Testcase{
		EndpointName:    endpointName,
		Name:            name,
		URL:             fullURL,
		Method:          method,
		Headers:         canonicalizeHeaders(headers),
		ExpectedStatus:  expectedStatus,
		ExpectedHeaders: canonicalizeHeaders(expectedHeaders),
		ExpectedBody:    expectedBody,
		ExpectedText:    expectedText,
	}

	// Determine request type and prepare body
	if file != nil {
		tc.RequestType = RequestTypeMultipart
		tc.MultipartFields = formData
		tc.FileUpload = file
		// Pre-build multipart body to avoid rebuilding per request
		tc.CachedMultipartBody, tc.CachedContentType, err = buildMultipartBody(formData, file)
		if err != nil {
			return nil, fmt.Errorf("failed to build multipart body: %w", err)
		}
	} else if len(formData) > 0 {
		tc.RequestType = RequestTypeForm
		tc.FormData = formData
	} else if body != nil {
		tc.RequestType = RequestTypeJSON
		tc.Body, err = serializeBody(body)
		if err != nil {
			return nil, err
		}
	} else {
		tc.RequestType = RequestTypeNone
	}

	return tc, nil
}

func buildMultipartBody(fields map[string]string, file *FileUpload) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("failed to write field %s: %w", key, err)
		}
	}

	if file != nil {
		contentType := file.ContentType
		if contentType == "" {
			contentType = "text/plain"
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			escapeQuotes(file.FieldName), escapeQuotes(file.Filename)))
		h.Set("Content-Type", contentType)

		part, err := writer.CreatePart(h)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create form file: %w", err)
		}
		if _, err := part.Write(file.Content); err != nil {
			return nil, "", fmt.Errorf("failed to write file content: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return buf.Bytes(), writer.FormDataContentType(), nil
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func loadFile(filename string) (*FileUpload, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, nil
	}

	path := filepath.Join("testdata", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	return &FileUpload{
		FieldName:   "file",
		Filename:    filename,
		Content:     content,
		ContentType: "text/plain",
	}, nil
}

func buildURL(baseURL, path string, query map[string]string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	pathURL, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	fullURL := base.ResolveReference(pathURL)

	if len(query) > 0 {
		q := fullURL.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		fullURL.RawQuery = q.Encode()
	}

	return fullURL.String(), nil
}

func canonicalizeHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(k))
		if key != "" {
			result[key] = strings.TrimSpace(v)
		}
	}
	return result
}

func serializeBody(body any) (string, error) {
	if body == nil {
		return "", nil
	}
	if s, ok := body.(string); ok {
		return s, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal body: %w", err)
	}
	return string(data), nil
}
