package client

import (
	"encoding/json/v2"
	"net/textproto"
	"strings"
)

type Body map[string]any
type Headers map[string]string
type Method = string

const (
	GET     Method = "GET"
	POST    Method = "POST"
	PUT     Method = "PUT"
	DELETE  Method = "DELETE"
	PATCH   Method = "PATCH"
	HEAD    Method = "HEAD"
	OPTIONS Method = "OPTIONS"
)

var methods = []Method{GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS}

type Endpoint struct {
	Path     string
	Method   Method
	Headers  Headers
	Body     Body
	Expected *Expected
}

type Expected struct {
	StatusCode int
	Body       any
	Headers    Headers
}

func newExpected(statusCode int, body Body, headers Headers) *Expected {
	expectedBytes, err := json.Marshal(body)
	if err != nil {
		return nil
	}

	var normalizedBody any
	if err := json.Unmarshal(expectedBytes, &normalizedBody); err != nil {
		return nil
	}

	for key, value := range headers {
		key = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if key == "" {
			delete(headers, key)
			continue
		}
		headers[key] = strings.TrimSpace(value)
	}

	return &Expected{StatusCode: statusCode, Body: normalizedBody, Headers: headers}
}
