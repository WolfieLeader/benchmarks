package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/http"
	"shared/consts"
	"shared/web"
	"strconv"
	"strings"
	"time"

	"echo-server/internal/utils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// htmlPage renders the /html canon: a greeting, a fruit list, and a labeled
// total. html/template escapes interpolated values; none here need it. echo has
// no template renderer configured, so the handler executes the template into a
// buffer and sends it with c.HTML.
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

func RegisterWeb(e *echo.Echo, jwtSecret string) {
	e.GET("/html", handleHTML)
	e.GET("/jwt/sign", handleJWTSign(jwtSecret))
	e.GET("/jwt/verify", handleJWTVerify(jwtSecret))
	e.POST("/validate", handleValidate)
	e.GET("/compute", handleCompute)
}

func handleHTML(c echo.Context) error {
	var buf strings.Builder
	data := struct {
		Name   string
		Fruits []string
		Total  int
	}{Name: "Alice", Fruits: []string{"apple", "banana", "cherry"}, Total: 42}
	if err := htmlPage.Execute(&buf, data); err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	return c.HTML(http.StatusOK, buf.String())
}

func handleJWTSign(secret string) echo.HandlerFunc {
	return func(c echo.Context) error {
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
			return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]string{"token": signed})
	}
}

func handleJWTVerify(secret string) echo.HandlerFunc {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	return func(c echo.Context) error {
		tokenStr, ok := strings.CutPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			return utils.WriteError(c, http.StatusUnauthorized, consts.ErrInvalidToken, "missing bearer token")
		}

		claims := jwt.MapClaims{}
		if _, err := parser.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return []byte(secret), nil
		}); err != nil {
			return utils.WriteError(c, http.StatusUnauthorized, consts.ErrInvalidToken, err.Error())
		}

		// Echo the verified claims verbatim; the token carries exactly the five
		// canon claims (sub/name/admin/iat/exp), which is what the strict body
		// assertion expects.
		return c.JSON(http.StatusOK, claims)
	}
}

func handleValidate(c echo.Context) error {
	var payload web.ValidatePayload
	if err := utils.BindJSON(c, &payload); err != nil {
		return utils.WriteBodyError(c, err)
	}

	if err := validate.Struct(&payload); err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrValidationFailed, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]bool{"valid": true})
}

func handleCompute(c echo.Context) error {
	n, err := strconv.Atoi(c.QueryParam("n"))
	if err != nil || n < 1 {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidN, "n must be an integer >= 1")
	}
	if n > web.ComputeMaxRounds {
		n = web.ComputeMaxRounds
	}

	state := []byte(web.ComputeSeed)
	for range n {
		sum := sha256.Sum256(state)
		state = sum[:]
	}

	return c.JSON(http.StatusOK, map[string]string{"result": hex.EncodeToString(state)})
}
