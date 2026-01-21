package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// RequestType indicates the type of request body
type RequestType int

const (
	RequestTypeNone RequestType = iota
	RequestTypeJSON
	RequestTypeForm
	RequestTypeMultipart
)

// ExecutableTestcase represents a fully prepared test case ready for execution
type ExecutableTestcase struct {
	Name            string
	URL             string
	Method          string
	Headers         map[string]string
	RequestType     RequestType
	Body            string            // JSON body
	FormData        map[string]string // URL-encoded form data
	MultipartFields map[string]string // Multipart form fields
	FileUpload      *FileUpload       // File for multipart upload
	ExpectedStatus  int
	ExpectedHeaders map[string]string
	ExpectedBody    any    // JSON body match
	ExpectedText    string // Plain text match
}

// FileUpload represents a file to upload
type FileUpload struct {
	FieldName   string
	Filename    string
	Content     []byte
	ContentType string
}

// BuildRequest creates an HTTP request from an executable testcase
func (c *Client) BuildRequest(ctx context.Context, tc *ExecutableTestcase) (*http.Request, error) {
	var bodyReader io.Reader
	var contentType string

	switch tc.RequestType {
	case RequestTypeJSON:
		if tc.Body != "" {
			bodyReader = strings.NewReader(tc.Body)
			contentType = "application/json"
		}

	case RequestTypeForm:
		if len(tc.FormData) > 0 {
			formData := url.Values{}
			for k, v := range tc.FormData {
				formData.Set(k, v)
			}
			bodyReader = strings.NewReader(formData.Encode())
			contentType = "application/x-www-form-urlencoded"
		}

	case RequestTypeMultipart:
		body, ct, err := c.buildMultipartBody(tc)
		if err != nil {
			return nil, fmt.Errorf("failed to build multipart body: %w", err)
		}
		bodyReader = body
		contentType = ct
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, tc.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers from testcase
	for key, value := range tc.Headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			req.Header.Set(key, value)
		}
	}

	// Set content type if not already set
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Set Accept header if not already set
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	return req, nil
}

func (c *Client) buildMultipartBody(tc *ExecutableTestcase) (io.Reader, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Write form fields
	for key, value := range tc.MultipartFields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("failed to write field %s: %w", key, err)
		}
	}

	// Write file if present
	if tc.FileUpload != nil {
		// Create part with correct Content-Type header (not application/octet-stream)
		contentType := tc.FileUpload.ContentType
		if contentType == "" {
			contentType = "text/plain"
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			escapeQuotes(tc.FileUpload.FieldName), escapeQuotes(tc.FileUpload.Filename)))
		h.Set("Content-Type", contentType)

		part, err := writer.CreatePart(h)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create form file: %w", err)
		}
		if _, err := part.Write(tc.FileUpload.Content); err != nil {
			return nil, "", fmt.Errorf("failed to write file content: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return &buf, writer.FormDataContentType(), nil
}

// escapeQuotes escapes quotes in a string for use in Content-Disposition header
func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// BuildURLWithQuery builds a URL with query parameters
func BuildURLWithQuery(baseURL, path string, query map[string]string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	pathURL, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	fullURL := base.ResolveReference(pathURL)

	// Add query parameters
	if len(query) > 0 {
		q := fullURL.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		fullURL.RawQuery = q.Encode()
	}

	return fullURL.String(), nil
}

// PrepareJSONBody serializes a body to JSON
func PrepareJSONBody(body any) (string, error) {
	if body == nil {
		return "", nil
	}

	// If it's already a string, return as-is (might be pre-serialized)
	if s, ok := body.(string); ok {
		return s, nil
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal body: %w", err)
	}
	return string(data), nil
}
