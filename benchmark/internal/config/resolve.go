package config

import (
	"maps"
	"time"
)

func resolve(cfg *Config) (*ResolvedConfig, error) {
	timeout, _ := time.ParseDuration(cfg.Global.Timeout)

	resolved := &ResolvedConfig{
		BaseURL:     cfg.Global.BaseURL,
		Timeout:     timeout,
		Workers:     cfg.Global.Workers,
		Iterations:  cfg.Global.RequestsPerEndpoint,
		Servers:     make([]*ResolvedServer, 0, len(cfg.Servers)),
		CPULimit:    cfg.Global.CPULimit,
		MemoryLimit: cfg.Global.MemoryLimit,
	}

	var allCases []*ResolvedTestCase
	for endpointName, endpoint := range cfg.Endpoints {
		allCases = append(allCases, resolveEndpointTestCases(endpointName, &endpoint)...)
	}

	for name, port := range cfg.Servers {
		resolved.Servers = append(resolved.Servers, &ResolvedServer{
			Name:       name,
			ImageName:  name,
			Port:       port,
			Workers:    cfg.Global.Workers,
			Iterations: cfg.Global.RequestsPerEndpoint,
			TestCases:  allCases,
		})
	}

	return resolved, nil
}

func resolveEndpointTestCases(endpointName string, endpoint *EndpointConfig) []*ResolvedTestCase {
	if len(endpoint.TestCases) == 0 {
		return []*ResolvedTestCase{{
			EndpointName:    endpointName,
			Name:            "default",
			Path:            endpoint.Path,
			Method:          endpoint.Method,
			Headers:         maps.Clone(endpoint.Headers),
			Query:           maps.Clone(endpoint.Query),
			Body:            endpoint.Body,
			FormData:        maps.Clone(endpoint.FormData),
			File:            endpoint.File,
			ExpectedStatus:  endpoint.ExpectedStatus,
			ExpectedHeaders: maps.Clone(endpoint.ExpectedHeaders),
			ExpectedBody:    endpoint.ExpectedBody,
			ExpectedText:    endpoint.ExpectedText,
		}}
	}

	resolved := make([]*ResolvedTestCase, 0, len(endpoint.TestCases))
	for _, tc := range endpoint.TestCases {
		resolved = append(resolved, mergeEndpointWithTestCase(endpointName, endpoint, &tc))
	}
	return resolved
}

func mergeEndpointWithTestCase(endpointName string, endpoint *EndpointConfig, tc *TestCaseConfig) *ResolvedTestCase {
	resolved := &ResolvedTestCase{
		EndpointName:    endpointName,
		Name:            tc.Name,
		Path:            endpoint.Path,
		Method:          endpoint.Method,
		Headers:         maps.Clone(endpoint.Headers),
		Query:           maps.Clone(endpoint.Query),
		Body:            endpoint.Body,
		FormData:        maps.Clone(endpoint.FormData),
		File:            endpoint.File,
		ExpectedStatus:  endpoint.ExpectedStatus,
		ExpectedHeaders: maps.Clone(endpoint.ExpectedHeaders),
		ExpectedBody:    endpoint.ExpectedBody,
		ExpectedText:    endpoint.ExpectedText,
	}

	if tc.Path != "" {
		resolved.Path = tc.Path
	}

	if len(tc.Headers) > 0 {
		if resolved.Headers == nil {
			resolved.Headers = make(map[string]string)
		}
		maps.Copy(resolved.Headers, tc.Headers)
	}

	if len(tc.Query) > 0 {
		if resolved.Query == nil {
			resolved.Query = make(map[string]string)
		}
		maps.Copy(resolved.Query, tc.Query)
	}

	if tc.Body != nil {
		resolved.Body = tc.Body
	}

	if len(tc.FormData) > 0 {
		if resolved.FormData == nil {
			resolved.FormData = make(map[string]string)
		}
		maps.Copy(resolved.FormData, tc.FormData)
	}

	if tc.File != nil {
		resolved.File = tc.File
	}

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
