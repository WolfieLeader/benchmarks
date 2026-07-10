package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"html/template"
	"net/http"
	"shared/consts"
	"shared/web"
	"strconv"
	"strings"
	"time"

	"stdlib-server/internal/utils"

	"github.com/golang-jwt/jwt/v5"
)

// htmlPage renders the /html canon: a greeting, a fruit list, and a labeled
// total. html/template escapes interpolated values; none here need it.
var htmlPage = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Benchmark</title></head>
<body>
  <h1>Hello, {{.Name}}</h1>
  <ul>{{range .Fruits}}
    <li>{{.}}</li>{{end}}
  </ul>
  <p>Total: {{.Total}}</p>
</body>
</html>
`))

func RegisterWeb(mux *http.ServeMux, jwtSecret string) {
	mux.HandleFunc("GET /html", handleHTML)
	mux.HandleFunc("GET /jwt/sign", handleJWTSign(jwtSecret))
	mux.HandleFunc("GET /jwt/verify", handleJWTVerify(jwtSecret))
	mux.HandleFunc("POST /validate", handleValidate)
	mux.HandleFunc("GET /compute", handleCompute)
}

func handleHTML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Name   string
		Fruits []string
		Total  int
	}{Name: "Alice", Fruits: []string{"apple", "banana", "cherry"}, Total: 42}
	if err := htmlPage.Execute(w, data); err != nil {
		// The status line is already committed by the first template write, so
		// there is nothing left to recover — the error is inert.
		return
	}
}

func handleJWTSign(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()
		claims := jwt.MapClaims{
			"sub":   web.JWTSubject,
			"name":  web.JWTName,
			"admin": web.JWTAdmin,
			"iat":   now.Unix(),
			"exp":   now.Add(web.JWTTTL).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(secret))
		if err != nil {
			utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
			return
		}
		utils.WriteResponse(w, http.StatusOK, map[string]string{"token": signed})
	}
}

func handleJWTVerify(secret string) http.HandlerFunc {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			utils.WriteError(w, http.StatusUnauthorized, consts.ErrInvalidToken, "missing bearer token")
			return
		}

		claims := jwt.MapClaims{}
		if _, err := parser.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return []byte(secret), nil
		}); err != nil {
			utils.WriteError(w, http.StatusUnauthorized, consts.ErrInvalidToken, err.Error())
			return
		}

		// Echo the verified claims verbatim; the token carries exactly the five
		// canon claims (sub/name/admin/iat/exp), which is what the strict body
		// assertion expects.
		utils.WriteResponse(w, http.StatusOK, claims)
	}
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	var payload web.ValidatePayload
	if err := json.UnmarshalRead(r.Body, &payload, decodeOpts); err != nil {
		utils.WriteBodyError(w, err)
		return
	}

	if err := validate.Struct(&payload); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrValidationFailed, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]bool{"valid": true})
}

func handleCompute(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("n"))
	if err != nil || n < 1 {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidN, "n must be an integer >= 1")
		return
	}
	if n > web.ComputeMaxRounds {
		n = web.ComputeMaxRounds
	}

	state := []byte(web.ComputeSeed)
	for range n {
		sum := sha256.Sum256(state)
		state = sum[:]
	}

	utils.WriteResponse(w, http.StatusOK, map[string]string{"result": hex.EncodeToString(state)})
}
