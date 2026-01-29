package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func resolveV2(cfg *ConfigV2) ([]*ResolvedServer, error) {
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
		// Skip standalone tests for endpoints that are part of a flow
		// (they use placeholders that are only resolved during flow execution)
		if endpoint.Flow != nil {
			continue
		}
		testcases, err := resolveEndpointV2(cfg.Benchmark.BaseURL, cfg.Databases, endpointName, &endpoint)
		if err != nil {
			return nil, err
		}
		allTestcases = append(allTestcases, testcases...)
	}

	// Resolve flows
	flows, err := resolveFlows(cfg, order)
	if err != nil {
		return nil, err
	}

	servers := make([]*ResolvedServer, 0, len(cfg.Servers))
	for _, server := range cfg.Servers {
		servers = append(servers, &ResolvedServer{
			Name:                      server.Name,
			ImageName:                 server.Name,
			Port:                      server.Port,
			BaseURL:                   cfg.Benchmark.BaseURL,
			Timeout:                   cfg.Benchmark.TimeoutDuration,
			CPULimit:                  cfg.Container.CPU,
			MemoryLimit:               cfg.Container.Memory,
			Workers:                   cfg.Benchmark.Workers,
			RequestsPerEndpoint:       cfg.Benchmark.Requests,
			Testcases:                 allTestcases,
			EndpointOrder:             order,
			WarmupRequestsPerTestcase: cfg.Benchmark.Warmup,
			WarmupEnabled:             cfg.Benchmark.WarmupEnabled,
			ResourcesEnabled:          cfg.Benchmark.ResourcesEnabled,
			Capacity:                  cfg.Capacity,
			Flows:                     flows,
		})
	}

	return servers, nil
}

func resolveFlows(cfg *ConfigV2, order []string) ([]*ResolvedFlow, error) {
	// Group endpoints by flow.id, preserving order
	flowEndpoints := make(map[string][]string) // flow.id -> endpoint names in order
	flowVars := make(map[string]map[string]VarConfig)

	for _, name := range order {
		endpoint, ok := cfg.Endpoints[name]
		if !ok || endpoint.Flow == nil {
			continue
		}
		flowId := endpoint.Flow.Id
		flowEndpoints[flowId] = append(flowEndpoints[flowId], name)

		// Capture vars from first endpoint that has them
		if endpoint.Flow.Vars != nil && flowVars[flowId] == nil {
			flowVars[flowId] = endpoint.Flow.Vars
		}
	}

	var flows []*ResolvedFlow

	for flowId, endpointNames := range flowEndpoints {
		// Check if any endpoint has per_database
		var perDatabase bool
		for _, name := range endpointNames {
			if cfg.Endpoints[name].PerDatabase {
				perDatabase = true
				break
			}
		}

		databases := []string{""}
		if perDatabase && len(cfg.Databases) > 0 {
			databases = cfg.Databases
		}

		for _, db := range databases {
			flow := &ResolvedFlow{
				Id:        flowId,
				Database:  db,
				Vars:      flowVars[flowId],
				Endpoints: make([]*ResolvedFlowEndpoint, 0, len(endpointNames)),
			}

			for _, name := range endpointNames {
				ep := cfg.Endpoints[name]
				path := ep.Path
				if db != "" {
					path = strings.ReplaceAll(path, "{database}", db)
				}

				resolved := &ResolvedFlowEndpoint{
					Name:           name,
					Method:         ep.Method,
					Path:           path,
					Body:           ep.Body,
					Headers:        ep.Headers,
					ExpectedStatus: ep.Expect.Status,
					ExpectedBody:   ep.Expect.Body,
				}
				if ep.Flow != nil {
					resolved.Capture = ep.Flow.Capture
				}
				flow.Endpoints = append(flow.Endpoints, resolved)
			}

			flows = append(flows, flow)
		}
	}

	return flows, nil
}

func resolveEndpointV2(baseURL string, databases []string, endpointName string, endpoint *EndpointConfigV2) ([]*Testcase, error) {
	endpointFile, err := loadFile(endpoint.File)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q file: %w", endpointName, err)
	}

	var testcases []*Testcase

	// If per_database is true, expand for each database
	if endpoint.PerDatabase && len(databases) > 0 {
		for _, db := range databases {
			// Create base testcase with database substitution
			tc, tcErr := buildTestcaseV2(baseURL, endpointName, db, endpoint, nil, endpointFile, db)
			if tcErr != nil {
				return nil, tcErr
			}
			testcases = append(testcases, tc)

			// Add variations for this database
			for i := range endpoint.Variations {
				variation := &endpoint.Variations[i]
				file := endpointFile
				if variation.File != "" {
					file, tcErr = loadFile(variation.File)
					if tcErr != nil {
						return nil, fmt.Errorf("endpoint %q variation %d file: %w", endpointName, i, tcErr)
					}
				}

				variationName := fmt.Sprintf("variation_%d", i)
				tc, tcErr = buildTestcaseV2(baseURL, endpointName, db+"/"+variationName, endpoint, variation, file, db)
				if tcErr != nil {
					return nil, fmt.Errorf("endpoint %q database %q variation %d: %w", endpointName, db, i, tcErr)
				}
				testcases = append(testcases, tc)
			}
		}
	} else {
		// No per_database expansion - create single base testcase
		if len(endpoint.Variations) == 0 {
			tc, tcErr := buildTestcaseV2(baseURL, endpointName, "default", endpoint, nil, endpointFile, "")
			if tcErr != nil {
				return nil, tcErr
			}
			return []*Testcase{tc}, nil
		}

		// Create base testcase
		tc, tcErr := buildTestcaseV2(baseURL, endpointName, "default", endpoint, nil, endpointFile, "")
		if tcErr != nil {
			return nil, tcErr
		}
		testcases = append(testcases, tc)

		// Add variations
		for i := range endpoint.Variations {
			variation := &endpoint.Variations[i]
			file := endpointFile
			if variation.File != "" {
				file, err = loadFile(variation.File)
				if err != nil {
					return nil, fmt.Errorf("endpoint %q variation %d file: %w", endpointName, i, err)
				}
			}

			variationName := fmt.Sprintf("variation_%d", i)
			tc, err = buildTestcaseV2(baseURL, endpointName, variationName, endpoint, variation, file, "")
			if err != nil {
				return nil, fmt.Errorf("endpoint %q variation %d: %w", endpointName, i, err)
			}
			testcases = append(testcases, tc)
		}
	}

	return testcases, nil
}

func buildTestcaseV2(baseURL, endpointName, name string, endpoint *EndpointConfigV2, variation *VariationConfig, file *FileUpload, database string) (*Testcase, error) {
	path := endpoint.Path
	method := strings.ToUpper(endpoint.Method)
	headers := maps.Clone(endpoint.Headers)
	query := maps.Clone(endpoint.Query)
	body := endpoint.Body
	formData := maps.Clone(endpoint.FormData)
	expectedStatus := endpoint.Expect.Status
	expectedHeaders := maps.Clone(endpoint.Expect.Headers)
	expectedBody := endpoint.Expect.Body
	expectedText := endpoint.Expect.Text

	// Apply variation overrides
	if variation != nil {
		if variation.Path != "" {
			path = variation.Path
		}
		if len(variation.Headers) > 0 {
			if headers == nil {
				headers = make(map[string]string)
			}
			maps.Copy(headers, variation.Headers)
		}
		if len(variation.Query) > 0 {
			if query == nil {
				query = make(map[string]string)
			}
			maps.Copy(query, variation.Query)
		}
		if variation.Body != nil {
			body = variation.Body
		}
		if len(variation.FormData) > 0 {
			if formData == nil {
				formData = make(map[string]string)
			}
			maps.Copy(formData, variation.FormData)
		}
		if variation.Expect != nil {
			if variation.Expect.Status != 0 {
				expectedStatus = variation.Expect.Status
			}
			if len(variation.Expect.Headers) > 0 {
				if expectedHeaders == nil {
					expectedHeaders = make(map[string]string)
				}
				maps.Copy(expectedHeaders, variation.Expect.Headers)
			}
			if variation.Expect.Body != nil {
				expectedBody = variation.Expect.Body
			}
			if variation.Expect.Text != "" {
				expectedText = variation.Expect.Text
			}
		}
	}

	// Replace {database} placeholder if database is provided
	if database != "" {
		path = strings.ReplaceAll(path, "{database}", database)
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

	// Prevent path traversal attacks
	if strings.Contains(filename, "..") {
		return nil, errors.New("invalid filename: path traversal not allowed")
	}

	assetsDir := filepath.Join("..", "assets")
	path := filepath.Join(assetsDir, filename)

	// Verify the resolved path is still within assets directory
	absAssetsDir, err := filepath.Abs(assetsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve assets directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(absPath, absAssetsDir+string(filepath.Separator)) {
		return nil, errors.New("invalid filename: path must be within assets directory")
	}

	content, err := os.ReadFile(absPath) //nolint:gosec // path is validated to be within assets directory
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
