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
	"slices"
	"strings"
	"time"
)

func resolve(cfg *Config) ([]*ResolvedServer, error) {
	timeout, _ := time.ParseDuration(cfg.Global.Timeout)

	var allTestcases []*Testcase
	order := cfg.EndpointOrder
	if len(order) == 0 {
		order = make([]string, 0, len(cfg.Endpoints))
		for name := range cfg.Endpoints {
			order = append(order, name)
		}
		slices.Sort(order)
	}

	for _, endpointName := range order {
		endpoint, ok := cfg.Endpoints[endpointName]
		if !ok {
			continue
		}
		testcases, err := resolveEndpoint(cfg.Global.BaseURL, endpointName, &endpoint)
		if err != nil {
			return nil, err
		}
		allTestcases = append(allTestcases, testcases...)
	}

	serverOrder := cfg.ServerOrder
	if len(serverOrder) == 0 {
		serverOrder = make([]string, 0, len(cfg.Servers))
		for name := range cfg.Servers {
			serverOrder = append(serverOrder, name)
		}
		slices.Sort(serverOrder)
	}

	servers := make([]*ResolvedServer, 0, len(cfg.Servers))
	for _, name := range serverOrder {
		port, ok := cfg.Servers[name]
		if !ok {
			continue
		}
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
			EndpointOrder:       order,
			Warmup:              cfg.Global.Warmup,
			Resources:           cfg.Global.Resources,
			Capacity:            cfg.Global.Capacity,
		})
	}

	return servers, nil
}

func resolveEndpoint(baseURL, endpointName string, endpoint *EndpointConfig) ([]*Testcase, error) {
	endpointFile, err := loadFile(endpoint.File)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q file: %w", endpointName, err)
	}

	if len(endpoint.TestCases) == 0 {
		var tc *Testcase
		tc, err = buildTestcase(baseURL, endpointName, "default", endpoint, nil, endpointFile)
		if err != nil {
			return nil, err
		}
		return []*Testcase{tc}, nil
	}

	testcases := make([]*Testcase, 0, len(endpoint.TestCases))
	for i := range endpoint.TestCases {
		tcConfig := &endpoint.TestCases[i]
		file := endpointFile
		if tcConfig.File != "" {
			file, err = loadFile(tcConfig.File)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q test case %d file: %w", endpointName, i, err)
			}
		}

		var tc *Testcase
		tc, err = buildTestcase(baseURL, endpointName, tcConfig.Name, endpoint, tcConfig, file)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q test case %q: %w", endpointName, tcConfig.Name, err)
		}
		testcases = append(testcases, tc)
	}

	return testcases, nil
}

func buildTestcase(baseURL, endpointName, name string, endpoint *EndpointConfig, tcOverride *TestCaseConfig, file *FileUpload) (*Testcase, error) {
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

	fullURL, err := buildURL(baseURL, path, query)
	if err != nil {
		return nil, err
	}

	tc := &Testcase{
		EndpointName:    endpointName,
		Name:            name,
		Path:            path,
		URL:             fullURL,
		Method:          method,
		Headers:         canonicalizeHeaders(headers),
		ExpectedStatus:  expectedStatus,
		ExpectedHeaders: canonicalizeHeaders(expectedHeaders),
		ExpectedBody:    expectedBody,
		ExpectedText:    expectedText,
	}

	switch {
	case file != nil:
		tc.RequestType = RequestTypeMultipart
		tc.MultipartFields = formData
		tc.FileUpload = file

		tc.CachedMultipartBody, tc.CachedContentType, err = buildMultipartBody(formData, file)
		if err != nil {
			return nil, fmt.Errorf("failed to build multipart body: %w", err)
		}
	case len(formData) > 0:
		tc.RequestType = RequestTypeForm
		tc.FormData = formData
		tc.CachedFormBody = encodeFormBody(formData)
	case body != nil:
		tc.RequestType = RequestTypeJSON
		tc.Body, err = serializeBody(body)
		if err != nil {
			return nil, err
		}
	default:
		tc.RequestType = RequestTypeNone
	}

	return tc, nil
}

func buildMultipartBody(fields map[string]string, file *FileUpload) (body, contentType string, err error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", "", fmt.Errorf("failed to write field %s: %w", key, err)
		}
	}

	if file != nil {
		contentType := file.ContentType
		if contentType == "" {
			contentType = "text/plain"
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`,
			file.FieldName, file.Filename))
		h.Set("Content-Type", contentType)

		part, err := writer.CreatePart(h)
		if err != nil {
			return "", "", fmt.Errorf("failed to create form file: %w", err)
		}
		if _, err := part.Write(file.Content); err != nil {
			return "", "", fmt.Errorf("failed to write file content: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return buf.String(), writer.FormDataContentType(), nil
}

func loadFile(filename string) (*FileUpload, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, nil
	}

	path := filepath.Join("assets", filename)
	content, err := os.ReadFile(path) //nolint:gosec // path is constructed from controlled assets directory
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

func encodeFormBody(formData map[string]string) string {
	if len(formData) == 0 {
		return ""
	}
	values := url.Values{}
	for k, v := range formData {
		values.Set(k, v)
	}
	return values.Encode()
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
