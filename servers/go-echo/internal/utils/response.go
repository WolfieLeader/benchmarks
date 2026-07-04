package utils

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"net/http"
	"shared/consts"

	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// decodeOpts keeps json/v2 request decoding aligned with every other server in the
// suite: duplicate keys take the last value (JSON.parse semantics in the JS/Python
// stacks), where json/v2 alone would reject them by default.
var decodeOpts = jsontext.AllowDuplicateNames(true)

// JSONSerializer plugs encoding/json/v2 into echo's c.JSON / JSONSerializer hooks so
// the whole server marshals and decodes through json/v2 — the repo convention — while
// handlers keep echo's idiomatic c.JSON. echo's DefaultJSONSerializer uses the stdlib
// encoding/json (v1) Encoder; going through json/v2 directly matches every other Go
// server here (and their duplicate-key decode semantics).
type JSONSerializer struct{}

// Serialize marshals straight to the response writer with json/v2. The indent argument
// (echo's ?pretty / Debug pretty-printing) is intentionally ignored: the benchmark
// always emits compact JSON, identical across frameworks.
func (JSONSerializer) Serialize(c echo.Context, i any, _ string) error {
	return json.MarshalWrite(c.Response(), i)
}

// Deserialize decodes the request body with json/v2 (duplicate-key last-wins). It
// returns the raw decode error so handlers can map it (e.g. *http.MaxBytesError -> 413)
// rather than echo's binder wrapping it into a generic 400 HTTPError.
func (JSONSerializer) Deserialize(c echo.Context, i any) error {
	return json.UnmarshalRead(c.Request().Body, i, decodeOpts)
}

// BindJSON decodes the request body through the configured json/v2 serializer, keeping
// request decoding out of echo's DefaultBinder so body-error mapping stays in the
// handler (see WriteBodyError).
func BindJSON(c echo.Context, out any) error {
	return c.Echo().JSONSerializer.Deserialize(c, out)
}

func WriteError(c echo.Context, status int, message string, detail ...any) error {
	resp := ErrorResponse{Error: message}
	if len(detail) > 0 {
		switch v := detail[0].(type) {
		case string:
			if v != "" {
				resp.Details = v
			}
		case error:
			if v != nil {
				resp.Details = v.Error()
			}
		}
	}
	return c.JSON(status, resp)
}

// WriteBodyError renders a JSON request-body decode failure. A body over the global cap
// surfaces as *http.MaxBytesError from the MaxBytesReader-wrapped body and becomes 413
// "request body too large"; everything else is a plain 400 "invalid JSON body".
func WriteBodyError(c echo.Context, err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return WriteError(c, http.StatusRequestEntityTooLarge, consts.ErrRequestTooLarge)
	}
	return WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
}
