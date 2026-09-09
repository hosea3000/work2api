package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/middleware"
	"github.com/yourname/work2api/internal/service"
)

// AuthHandler 管理台登录/会话/登出（与参考实现 /auth/* 契约一致）。
type AuthHandler struct {
	*Handler
	sessions    service.SessionService
	rateLimiter *service.RateLimiter
}

func NewAuthHandler(h *Handler, sessions service.SessionService) *AuthHandler {
	return &AuthHandler{
		Handler:     h,
		sessions:    sessions,
		rateLimiter: service.NewRateLimiter(60*time.Second, 60, 10, 5),
	}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"authenticated": false, "detail": "username and password required"})
		return
	}
	ip := service.ClientIP(c.Request)
	if !h.rateLimiter.Allow(ip, req.Username) {
		c.JSON(http.StatusTooManyRequests, gin.H{"authenticated": false, "detail": "Too many login attempts"})
		return
	}
	token, ok, err := h.sessions.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil || !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}
	c.SetCookie(middleware.SessionCookieName, token, 7*24*3600, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "username": req.Username})
}

func (h *AuthHandler) Session(c *gin.Context) {
	token, err := c.Cookie(middleware.SessionCookieName)
	if err == nil && token != "" {
		if username, ok := h.sessions.Validate(c.Request.Context(), token); ok {
			c.JSON(http.StatusOK, gin.H{"authenticated": true, "username": username})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie(middleware.SessionCookieName)
	if token != "" {
		_ = h.sessions.Logout(c.Request.Context(), token)
	}
	c.SetCookie(middleware.SessionCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}
