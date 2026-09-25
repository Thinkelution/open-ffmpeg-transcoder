package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
)

const sessionCookieName = "oft_session"

type SessionAuth struct {
	user     string
	password string
	secret   string
	apiKey   string
}

func NewSessionAuth(cfg *config.Config) *SessionAuth {
	secret := cfg.SessionSecret
	if secret == "" {
		secret = cfg.APIKey
	}
	if secret == "" {
		secret = "open-ffmpeg-transcoder-dev-session"
	}

	password := cfg.DashboardPassword
	if password == "" {
		password = cfg.APIKey
	}

	return &SessionAuth{
		user:     cfg.DashboardUser,
		password: password,
		secret:   secret,
		apiKey:   cfg.APIKey,
	}
}

func (a *SessionAuth) Login(c *gin.Context, user, password string) bool {
	if a.password == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) != 1 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) != 1 {
		return false
	}

	expires := time.Now().Add(24 * time.Hour)
	token := a.sign(user, expires)
	c.SetCookie(sessionCookieName, token, int(time.Until(expires).Seconds()), "/", "", false, true)
	return true
}

func (a *SessionAuth) Logout(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func (a *SessionAuth) RequireDashboard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.ValidSession(c) {
			c.Next()
			return
		}
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
	}
}

func (a *SessionAuth) RequireAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.ValidAPIKey(c) || a.ValidSession(c) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid or missing API key",
		})
	}
}

func (a *SessionAuth) ValidAPIKey(c *gin.Context) bool {
	if a.apiKey == "" {
		return false
	}
	key := c.GetHeader("X-API-Key")
	if key == "" {
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(a.apiKey)) == 1
}

func (a *SessionAuth) ValidSession(c *gin.Context) bool {
	token, err := c.Cookie(sessionCookieName)
	if err != nil || token == "" {
		return false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	userBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiresUnix {
		return false
	}
	user := string(userBytes)
	expected := a.signature(user, parts[1])
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expected)) != 1 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) == 1
}

func (a *SessionAuth) sign(user string, expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	return fmt.Sprintf("%s.%s.%s", base64.RawURLEncoding.EncodeToString([]byte(user)), exp, a.signature(user, exp))
}

func (a *SessionAuth) signature(user, expires string) string {
	mac := hmac.New(sha256.New, []byte(a.secret))
	mac.Write([]byte(user))
	mac.Write([]byte("."))
	mac.Write([]byte(expires))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
