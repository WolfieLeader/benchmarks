package client

import (
	"benchmark-client/internal/config"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func BuildRequest(ctx context.Context, tc *config.Testcase) (*http.Request, error) {
	var bodyReader io.Reader
	var contentType string

	switch tc.RequestType {
	case config.RequestTypeJSON:
		if tc.Body != "" {
			bodyReader = strings.NewReader(tc.Body)
			contentType = "application/json"
		}

	case config.RequestTypeForm:
		if len(tc.FormData) > 0 {
			formData := url.Values{}
			for k, v := range tc.FormData {
				formData.Set(k, v)
			}
			bodyReader = strings.NewReader(formData.Encode())
			contentType = "application/x-www-form-urlencoded"
		}

	case config.RequestTypeMultipart:
		if len(tc.CachedMultipartBody) > 0 {
			bodyReader = bytes.NewReader(tc.CachedMultipartBody)
			contentType = tc.CachedContentType
		}
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, tc.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range tc.Headers {
		req.Header.Set(key, value)
	}

	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	return req, nil
}
