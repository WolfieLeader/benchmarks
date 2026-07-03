package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
)

// buildRequest turns a resolved case into an *http.Request, reusing the
// benchmark's client.BuildRequest so the wire format matches real runs exactly.
func buildRequest(ctx context.Context, baseURL, testFilesDir string, c *Case) (*http.Request, error) {
	fullURL, err := buildURL(baseURL, c.Path, c.Query)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(c.Headers)+1)
	maps.Copy(headers, c.Headers)
	if c.ContentType != "" {
		headers["Content-Type"] = c.ContentType
	}

	tc := &config.Testcase{
		Url:     fullURL,
		Method:  method(c.Method),
		Headers: headers,
		// Populate expectations so client.BuildRequest derives the same Accept
		// header the benchmark path sends (text/plain vs application/json).
		ExpectedBody: c.Expect.Body,
	}
	if c.Expect.Text != nil {
		tc.ExpectedText = *c.Expect.Text
	}

	switch {
	case c.RawBody != nil:
		tc.RequestType = config.RequestTypeJSON
		tc.Body = *c.RawBody
	case c.Multipart != nil:
		body, contentType, mErr := buildMultipart(c.Multipart, testFilesDir)
		if mErr != nil {
			return nil, mErr
		}
		tc.RequestType = config.RequestTypeMultipart
		tc.CachedMultipartBody = body
		tc.CachedContentType = contentType
	case len(c.Form) > 0:
		tc.RequestType = config.RequestTypeForm
		tc.CachedFormBody = encodeForm(c.Form)
	case c.Body != nil:
		encoded, jErr := json.Marshal(c.Body)
		if jErr != nil {
			return nil, fmt.Errorf("marshal body: %w", jErr)
		}
		tc.RequestType = config.RequestTypeJSON
		tc.Body = string(encoded)
	default:
		tc.RequestType = config.RequestTypeNone
	}

	return client.BuildRequest(ctx, tc)
}

func method(m string) string {
	if m == "" {
		return http.MethodGet
	}
	return strings.ToUpper(m)
}

func buildURL(baseURL, path string, query map[string]string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", path, err)
	}
	full := base.ResolveReference(ref)
	if len(query) > 0 {
		q := full.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		full.RawQuery = q.Encode()
	}
	return full.String(), nil
}

func encodeForm(form map[string]string) string {
	values := url.Values{}
	for k, v := range form {
		values.Set(k, v)
	}
	return values.Encode()
}

func buildMultipart(mp *Multipart, testFilesDir string) (body, contentType string, err error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for k, v := range mp.Fields {
		if err := writer.WriteField(k, v); err != nil {
			return "", "", fmt.Errorf("write field %q: %w", k, err)
		}
	}

	if mp.File != nil {
		content, cErr := fileContent(mp.File, testFilesDir)
		if cErr != nil {
			return "", "", cErr
		}
		field := mp.File.Field
		if field == "" {
			field = "file"
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, mp.File.Filename))
		if mp.File.ContentType != "" {
			h.Set("Content-Type", mp.File.ContentType)
		}
		part, pErr := writer.CreatePart(h)
		if pErr != nil {
			return "", "", fmt.Errorf("create part: %w", pErr)
		}
		if _, wErr := part.Write(content); wErr != nil {
			return "", "", fmt.Errorf("write part: %w", wErr)
		}
	}

	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("close multipart: %w", err)
	}
	return buf.String(), writer.FormDataContentType(), nil
}

func fileContent(f *MultipartFile, testFilesDir string) ([]byte, error) {
	switch {
	case f.SizeBytes > 0:
		fill := byte('A')
		if f.FillByte != "" {
			fill = f.FillByte[0]
		}
		return bytes.Repeat([]byte{fill}, f.SizeBytes), nil
	case f.Source != "":
		if strings.Contains(f.Source, "..") || strings.ContainsRune(f.Source, filepath.Separator) {
			return nil, fmt.Errorf("invalid fixture name %q", f.Source)
		}
		data, err := os.ReadFile(filepath.Join(testFilesDir, f.Source)) //nolint:gosec // fixture name validated above
		if err != nil {
			return nil, fmt.Errorf("read fixture %q: %w", f.Source, err)
		}
		return data, nil
	default:
		return []byte(f.Text), nil
	}
}
