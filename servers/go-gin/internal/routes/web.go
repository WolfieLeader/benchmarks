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

	"gin-server/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// htmlTemplate renders the /html canon: a greeting, a fruit list, and a labeled
// total. html/template escapes interpolated values; none here need it. It is
// registered on the engine (SetHTMLTemplate) so handlers render it with c.HTML,
// gin's idiomatic template path.
const htmlTemplate = `<!DOCTYPE html>
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
`

func RegisterWeb(r *gin.Engine, jwtSecret string) {
	r.SetHTMLTemplate(template.Must(template.New("page").Parse(htmlTemplate)))

	r.GET("/html", handleHTML)
	r.GET("/jwt/sign", handleJWTSign(jwtSecret))
	r.GET("/jwt/verify", handleJWTVerify(jwtSecret))
	r.POST("/validate", handleValidate)
	r.GET("/compute", handleCompute)
}

func handleHTML(c *gin.Context) {
	c.HTML(http.StatusOK, "page", struct {
		Name   string
		Fruits []string
		Total  int
	}{Name: "Alice", Fruits: []string{"apple", "banana", "cherry"}, Total: 42})
}

func handleJWTSign(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
			return
		}
		utils.WriteResponse(c, http.StatusOK, gin.H{"token": signed})
	}
}

func handleJWTVerify(secret string) gin.HandlerFunc {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	return func(c *gin.Context) {
		tokenStr, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(tokenStr) == "" {
			utils.WriteError(c, http.StatusUnauthorized, consts.ErrInvalidToken, "missing bearer token")
			return
		}

		claims := jwt.MapClaims{}
		if _, err := parser.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return []byte(secret), nil
		}); err != nil {
			utils.WriteError(c, http.StatusUnauthorized, consts.ErrInvalidToken, err.Error())
			return
		}

		// Echo the verified claims verbatim; the token carries exactly the five
		// canon claims (sub/name/admin/iat/exp), which is what the strict body
		// assertion expects.
		utils.WriteResponse(c, http.StatusOK, claims)
	}
}

func handleValidate(c *gin.Context) {
	var payload web.ValidatePayload
	if err := utils.BindJSON(c, &payload); err != nil {
		utils.WriteBodyError(c, err)
		return
	}

	if err := validate.Struct(&payload); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrValidationFailed, err.Error())
		return
	}

	utils.WriteResponse(c, http.StatusOK, gin.H{"valid": true})
}

func handleCompute(c *gin.Context) {
	n, err := strconv.Atoi(c.Query("n"))
	if err != nil || n < 1 {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidN, "n must be an integer >= 1")
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

	utils.WriteResponse(c, http.StatusOK, gin.H{"result": hex.EncodeToString(state)})
}
