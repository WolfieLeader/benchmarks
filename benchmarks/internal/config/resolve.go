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

func resolve(cfg *Config) ([]*ResolvedServer, error) {
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
		if endpoint.Sequence != nil {
			continue
		}
		testcases, err := resolveEndpoint(cfg.Benchmark.BaseURL, cfg.Databases, endpointName, &endpoint)
		if err != nil {
			return nil, err
		}
		allTestcases = append(allTestcases, testcases...)
	}

	sequences := resolveSequences(cfg, order)

	servers := make([]*ResolvedServer, 0, len(cfg.Servers))
	for _, server := range cfg.Servers {
		servers = append(servers, &ResolvedServer{
			Name:                server.Name,
			ImageName:           server.Image,
			Port:                server.Port,
			BaseURL:             cfg.Benchmark.BaseURL,
			RequestTimeout:      cfg.Benchmark.RequestTimeout,
			CPULimit:            cfg.Container.CPULimit,
			MemoryLimit:         cfg.Container.MemoryLimit,
			Concurrency:         cfg.Benchmark.Concurrency,
			DurationPerEndpoint: cfg.Benchmark.DurationPerEndpoint,
			Testcases:           allTestcases,
			EndpointOrder:       order,
			WarmupDuration:      cfg.Benchmark.WarmupDuration,
			WarmupPause:         cfg.Benchmark.WarmupPause,
			Sequences:           sequences,
		})
	}

	return servers, nil
}

func resolveSequences(cfg *Config, order []string) []*ResolvedSequence {
	seqEndpoints := make(map[string][]string)
	seqVars := make(map[string]map[string]VarConfig)

	for _, name := range order {
		endpoint, ok := cfg.Endpoints[name]
		if !ok || endpoint.Sequence == nil {
			continue
		}
		seqId := endpoint.Sequence.Id
		seqEndpoints[seqId] = append(seqEndpoints[seqId], name)

		if endpoint.Sequence.Vars != nil && seqVars[seqId] == nil {
			seqVars[seqId] = endpoint.Sequence.Vars
		}
	}

	maxDbs := 1
	maxDbs = max(maxDbs, len(cfg.Databases))
	sequences := make([]*ResolvedSequence, 0, len(seqEndpoints)*maxDbs)

	for seqId, endpointNames := range seqEndpoints {
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
			seq := &ResolvedSequence{
				Id:        seqId,
				Database:  db,
				Vars:      seqVars[seqId],
				Endpoints: make([]*ResolvedSequenceEndpoint, 0, len(endpointNames)),
			}

			for _, name := range endpointNames {
				ep := cfg.Endpoints[name]
				path := ep.Path
				if db != "" {
					path = strings.ReplaceAll(path, "{database}", db)
				}

				resolved := &ResolvedSequenceEndpoint{
					Name:           name,
					Method:         ep.Method,
					Path:           path,
					Body:           ep.Body,
					Headers:        ep.Headers,
					ExpectedStatus: ep.Expect.Status,
					ExpectedBody:   ep.Expect.Body,
				}
				if ep.Sequence != nil {
					resolved.Capture = ep.Sequence.Capture
				}
				seq.Endpoints = append(seq.Endpoints, resolved)
			}

			sequences = append(sequences, seq)
		}
	}

	return sequences
}

func resolveEndpoint(baseURL string, databases []string, endpointName string, endpoint *EndpointConfig) ([]*Testcase, error) {
	endpointFile, err := loadFile(endpoint.File)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q file: %w", endpointName, err)
	}

	var testcases []*Testcase

	if endpoint.PerDatabase && len(databases) > 0 {
		for _, db := range databases {
			tc, tcErr := buildTestcase(baseURL, endpointName, db, endpoint, nil, endpointFile, db)
			if tcErr != nil {
				return nil, tcErr
			}
			testcases = append(testcases, tc)

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
				tc, tcErr = buildTestcase(baseURL, endpointName, db+"/"+variationName, endpoint, variation, file, db)
				if tcErr != nil {
					return nil, fmt.Errorf("endpoint %q database %q variation %d: %w", endpointName, db, i, tcErr)
				}
				testcases = append(testcases, tc)
			}
		}
	} else {
		if len(endpoint.Variations) == 0 {
			tc, tcErr := buildTestcase(baseURL, endpointName, "default", endpoint, nil, endpointFile, "")
			if tcErr != nil {
				return nil, tcErr
			}
			return []*Testcase{tc}, nil
		}

		tc, tcErr := buildTestcase(baseURL, endpointName, "default", endpoint, nil, endpointFile, "")
		if tcErr != nil {
			return nil, tcErr
		}
		testcases = append(testcases, tc)

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
			tc, err = buildTestcase(baseURL, endpointName, variationName, endpoint, variation, file, "")
			if err != nil {
				return nil, fmt.Errorf("endpoint %q variation %d: %w", endpointName, i, err)
			}
			testcases = append(testcases, tc)
		}
	}

	return testcases, nil
}

func buildTestcase(baseURL, endpointName, name string, endpoint *EndpointConfig, variation *VariationConfig, file *FileUpload, database string) (*Testcase, error) {
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

	if strings.Contains(filename, "..") {
		return nil, errors.New("invalid filename: path traversal not allowed")
	}

	testFilesDir := filepath.Join("..", "test-files")
	path := filepath.Join(testFilesDir, filename)

	absTestFilesDir, err := filepath.Abs(testFilesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve test-files directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(absPath, absTestFilesDir+string(filepath.Separator)) {
		return nil, errors.New("invalid filename: path must be within test-files directory")
	}

	content, err := os.ReadFile(absPath) //nolint:gosec // path is validated to be within test-files directory
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
