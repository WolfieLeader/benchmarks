package config

import "maps"

// MergeEndpointWithTestCase merges endpoint defaults with test case overrides
// Precedence: Endpoint config <- TestCase overrides
func MergeEndpointWithTestCase(endpoint *EndpointConfig, tc *TestCaseConfig) *ResolvedTestCase {
	resolved := &ResolvedTestCase{
		Name:            tc.Name,
		Path:            endpoint.Path,
		Method:          endpoint.Method,
		Headers:         cloneMap(endpoint.Headers),
		Query:           cloneMap(endpoint.Query),
		Body:            endpoint.Body,
		FormData:        cloneMap(endpoint.FormData),
		File:            endpoint.File,
		ExpectedStatus:  endpoint.ExpectedStatus,
		ExpectedHeaders: cloneMap(endpoint.ExpectedHeaders),
		ExpectedBody:    endpoint.ExpectedBody,
		ExpectedText:    endpoint.ExpectedText,
	}

	// Apply test case overrides
	if tc.Path != "" {
		resolved.Path = tc.Path
	}

	// Merge headers (test case headers override endpoint headers)
	if len(tc.Headers) > 0 {
		if resolved.Headers == nil {
			resolved.Headers = make(map[string]string)
		}
		maps.Copy(resolved.Headers, tc.Headers)
	}

	// Merge query params (test case query overrides endpoint query)
	if len(tc.Query) > 0 {
		if resolved.Query == nil {
			resolved.Query = make(map[string]string)
		}
		maps.Copy(resolved.Query, tc.Query)
	}

	// Override body if specified
	if tc.Body != nil {
		resolved.Body = tc.Body
	}

	// Merge form data
	if len(tc.FormData) > 0 {
		if resolved.FormData == nil {
			resolved.FormData = make(map[string]string)
		}
		maps.Copy(resolved.FormData, tc.FormData)
	}

	// Override file if specified
	if tc.File != nil {
		resolved.File = tc.File
	}

	// Override expected values if specified
	if tc.ExpectedStatus != 0 {
		resolved.ExpectedStatus = tc.ExpectedStatus
	}
	if len(tc.ExpectedHeaders) > 0 {
		if resolved.ExpectedHeaders == nil {
			resolved.ExpectedHeaders = make(map[string]string)
		}
		maps.Copy(resolved.ExpectedHeaders, tc.ExpectedHeaders)
	}
	if tc.ExpectedBody != nil {
		resolved.ExpectedBody = tc.ExpectedBody
	}
	if tc.ExpectedText != "" {
		resolved.ExpectedText = tc.ExpectedText
	}

	return resolved
}

// ResolveEndpointTestCases resolves all test cases for an endpoint
func ResolveEndpointTestCases(endpoint *EndpointConfig) []*ResolvedTestCase {
	if len(endpoint.TestCases) == 0 {
		// Create a default test case from the endpoint config
		return []*ResolvedTestCase{{
			Name:            "default",
			Path:            endpoint.Path,
			Method:          endpoint.Method,
			Headers:         cloneMap(endpoint.Headers),
			Query:           cloneMap(endpoint.Query),
			Body:            endpoint.Body,
			FormData:        cloneMap(endpoint.FormData),
			File:            endpoint.File,
			ExpectedStatus:  endpoint.ExpectedStatus,
			ExpectedHeaders: cloneMap(endpoint.ExpectedHeaders),
			ExpectedBody:    endpoint.ExpectedBody,
			ExpectedText:    endpoint.ExpectedText,
		}}
	}

	resolved := make([]*ResolvedTestCase, 0, len(endpoint.TestCases))
	for _, tc := range endpoint.TestCases {
		resolved = append(resolved, MergeEndpointWithTestCase(endpoint, &tc))
	}
	return resolved
}

// ApplyServerOverrides applies server-specific overrides to global config
func ApplyServerOverrides(global *GlobalConfig, server *ServerConfig) *ResolvedConfig {
	resolved := &ResolvedConfig{
		BaseURL:    global.BaseURL,
		Timeout:    global.GetTimeout(),
		Workers:    global.Workers,
		Iterations: global.Iterations,
	}

	if server.Overrides != nil {
		if server.Overrides.Workers != nil {
			resolved.Workers = *server.Overrides.Workers
		}
		if server.Overrides.Iterations != nil {
			resolved.Iterations = *server.Overrides.Iterations
		}
	}

	return resolved
}

// cloneMap creates a shallow copy of a string map
func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	maps.Copy(result, m)
	return result
}
