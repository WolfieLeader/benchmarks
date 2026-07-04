package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"benchmark-client/internal/config"
)

// BuildRequest builds an *http.Request for tc against baseURL. The absolute URL
// is baseURL + tc.RequestURI, so the same testcase can target a server on any
// (dynamically mapped) host port. baseURL must not have a trailing slash.
func BuildRequest(ctx context.Context, baseURL string, tc *config.Testcase) (*http.Request, error) {
	var bodyReader io.Reader
	var contentType string

	switch tc.RequestType {
	case config.RequestTypeNone:
		// No request body — bodyReader stays nil and no Content-Type is set.

	case config.RequestTypeJSON:
		if tc.Body != "" {
			bodyReader = strings.NewReader(tc.Body)
			contentType = "application/json"
		}

	case config.RequestTypeForm:
		if tc.CachedFormBody != "" {
			bodyReader = strings.NewReader(tc.CachedFormBody)
			contentType = "application/x-www-form-urlencoded"
			break
		}
		if len(tc.FormData) > 0 {
			formData := url.Values{}
			for k, v := range tc.FormData {
				formData.Set(k, v)
			}
			bodyReader = strings.NewReader(formData.Encode())
			contentType = "application/x-www-form-urlencoded"
		}

	case config.RequestTypeMultipart:
		if tc.CachedMultipartBody != "" {
			bodyReader = strings.NewReader(tc.CachedMultipartBody)
			contentType = tc.CachedContentType
		}
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, baseURL+tc.RequestURI, bodyReader)
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
		if tc.ExpectedText != "" {
			req.Header.Set("Accept", "text/plain")
		} else if tc.ExpectedBody != nil {
			req.Header.Set("Accept", "application/json")
		}
	}

	return req, nil
}
