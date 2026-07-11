package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"shared/consts"
	"shared/web"
	"strconv"
	"strings"
	"time"

	"fiber-server/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

// htmlPage renders the /html canon: a greeting, a fruit list, and a labeled
// total. html/template escapes interpolated values; none here need it. fiber has
// no view engine configured, so the handler executes the template into a buffer
// and sends it with the text/html content type.
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

func RegisterWeb(r fiber.Router, jwtSecret string) {
	r.Get("/html", handleHTML)
	r.Get("/jwt/sign", handleJWTSign(jwtSecret))
	r.Get("/jwt/verify", handleJWTVerify(jwtSecret))
	r.Post("/validate", handleValidate)
	r.Get("/compute", handleCompute)
}

func handleHTML(c fiber.Ctx) error {
	var buf strings.Builder
	data := struct {
		Name   string
		Fruits []string
		Total  int
	}{Name: "Alice", Fruits: []string{"apple", "banana", "cherry"}, Total: 42}
	if err := htmlPage.Execute(&buf, data); err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.SendString(buf.String())
}

func handleJWTSign(secret string) fiber.Handler {
	return func(c fiber.Ctx) error {
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
			return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
		}
		return c.JSON(fiber.Map{"token": signed})
	}
}

func handleJWTVerify(secret string) fiber.Handler {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	return func(c fiber.Ctx) error {
		tokenStr, ok := strings.CutPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			return utils.WriteError(c, fiber.StatusUnauthorized, consts.ErrInvalidToken, "missing bearer token")
		}

		claims := jwt.MapClaims{}
		if _, err := parser.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return []byte(secret), nil
		}); err != nil {
			return utils.WriteError(c, fiber.StatusUnauthorized, consts.ErrInvalidToken, err.Error())
		}

		// Echo the verified claims verbatim; the token carries exactly the five
		// canon claims (sub/name/admin/iat/exp), which is what the strict body
		// assertion expects.
		return c.JSON(claims)
	}
}

func handleValidate(c fiber.Ctx) error {
	var payload web.ValidatePayload
	if err := c.Bind().Body(&payload); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	if err := validate.Struct(&payload); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrValidationFailed, err.Error())
	}

	return c.JSON(fiber.Map{"valid": true})
}

func handleCompute(c fiber.Ctx) error {
	n, err := strconv.Atoi(c.Query("n"))
	if err != nil || n < 1 {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidN, "n must be an integer >= 1")
	}
	if n > web.ComputeMaxRounds {
		n = web.ComputeMaxRounds
	}

	state := []byte(web.ComputeSeed)
	for range n {
		sum := sha256.Sum256(state)
		state = sum[:]
	}

	return c.JSON(fiber.Map{"result": hex.EncodeToString(state)})
}
