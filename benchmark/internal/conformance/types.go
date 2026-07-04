// Package conformance runs the language-neutral contract cases in the top-level
// contract/ directory against a single base URL, once, sequentially, with strict
// full-body assertions. It is the read-only correctness gate for every server and
// shares nothing with the benchmark's load-generation path except the request
// builder (client.BuildRequest).
package conformance

// Suite is one contract file: a named group of cases.
type Suite struct {
	Name  string `json:"name"`
	Cases []Case `json:"cases"`
}

// Case is a single request+assertion, or (when Flow is set) an ordered group of
// steps that share a capture map. Field semantics are documented in
// contract/README.md.
type Case struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`

	// Request (ignored when Flow is set).
	Method      string            `json:"method,omitempty"` // default GET
	Path        string            `json:"path,omitempty"`
	Query       map[string]string `json:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`        // JSON body, marshaled as-is
	RawBody     *string           `json:"rawBody,omitempty"`     // raw string body (overrides Body)
	ContentType string            `json:"contentType,omitempty"` // override request Content-Type
	Form        map[string]string `json:"form,omitempty"`        // application/x-www-form-urlencoded
	Multipart   *Multipart        `json:"multipart,omitempty"`   // multipart/form-data

	// Response assertions.
	Expect Expect `json:"expect"`

	// Sequencing.
	Capture map[string]string `json:"capture,omitempty"` // {var: responseField} captured after success
	Flow    []Case            `json:"flow,omitempty"`    // ordered steps sharing captures
}

// Multipart describes a multipart/form-data request body.
type Multipart struct {
	Fields map[string]string `json:"fields,omitempty"`
	File   *MultipartFile    `json:"file,omitempty"`
}

// MultipartFile describes one uploaded file part. Exactly one content source
// (Source, Text, or SizeBytes) should be set.
type MultipartFile struct {
	Field       string `json:"field,omitempty"`       // form field name, default "file"
	Filename    string `json:"filename,omitempty"`    // filename in Content-Disposition
	ContentType string `json:"contentType,omitempty"` // part Content-Type header; omitted if empty
	Source      string `json:"source,omitempty"`      // fixture filename in contract/test-files/
	Text        string `json:"text,omitempty"`        // inline literal content
	SizeBytes   int    `json:"sizeBytes,omitempty"`   // synthesize N bytes (for oversized-payload cases)
	FillByte    string `json:"fillByte,omitempty"`    // single char used for synthesized content, default "A"
}

// Expect holds the response assertions for a case.
type Expect struct {
	Status      int               `json:"status"`
	StatusAnyOf []int             `json:"statusAnyOf,omitempty"` // any listed status passes (overrides Status; use without a body assertion)
	Headers     map[string]string `json:"headers,omitempty"`     // substring ("contains") match
	Text        *string           `json:"text,omitempty"`        // exact text body (trimmed)
	Body        any               `json:"body,omitempty"`        // JSON body assertion (matchers supported)
	Match       string            `json:"match,omitempty"`       // "exact" (default) | "subset"
}
