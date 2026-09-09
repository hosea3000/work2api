package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
	"github.com/yourname/work2api/pkg/log"
)

const SessionCookieName = "work2api_session"
const UsernameKey = "admin_username"

// APIKeyAuth 外部 API 鉴权：仅接受 sk- Bearer。
func APIKeyAuth(logger *log.Logger, apiKeys service.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			abortOpenAIAuth(c)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		ok, err := apiKeys.Validate(c.Request.Context(), raw)
		if err != nil || !ok {
			if err != nil {
				logger.WithContext(c).Error("api key validate error")
			}
			abortOpenAIAuth(c)
			return
		}
		c.Next()
	}
}

func abortOpenAIAuth(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"message": "Invalid API key provided",
			"type":    "authentication_error",
			"code":    nil,
		},
	})
}

// SessionAuth 管理端鉴权：仅接受会话 Cookie。
func SessionAuth(logger *log.Logger, sessions service.SessionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			abortSession(c)
			return
		}
		username, ok := sessions.Validate(c.Request.Context(), token)
		if !ok {
			abortSession(c)
			return
		}
		c.Set(UsernameKey, username)
		c.Next()
	}
}

func abortSession(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"authenticated": false,
		"detail":        "Not authenticated",
	})
}
