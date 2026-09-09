package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// CodeBuddyAuthHandler 管理台 CodeBuddy 设备授权端点。
// 契约与前端 useOAuthPolling.ts / admin.ts 对齐，路径 /codebuddy/auth/*。
type CodeBuddyAuthHandler struct {
	*Handler
	oauth service.OAuthService
}

func NewCodeBuddyAuthHandler(h *Handler, oauth service.OAuthService) *CodeBuddyAuthHandler {
	return &CodeBuddyAuthHandler{Handler: h, oauth: oauth}
}

// Start POST /codebuddy/auth/start
func (h *CodeBuddyAuthHandler) Start(c *gin.Context) {
	result, err := h.oauth.Start(c.Request.Context())
	if err != nil {
		var limitErr *service.AuthStartLimitError
		if asLimit(err, &limitErr) {
			c.Header("Retry-After", itoa(limitErr.RetryAfter))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "too_many_attempts",
				"message": "Too many active or recent authentication starts",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "internal_error",
			"message": "认证启动失败，请稍后重试",
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Poll POST /codebuddy/auth/poll
func (h *CodeBuddyAuthHandler) Poll(c *gin.Context) {
	var req struct {
		AuthState string `json:"auth_state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AuthState == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "missing_parameters",
			"error_description": "缺少必要的参数：auth_state",
		})
		return
	}
	result, ok := h.oauth.Poll(c.Request.Context(), req.AuthState)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Invalid or expired auth_state"})
		return
	}
	switch result.Kind {
	case "success":
		c.JSON(http.StatusOK, gin.H{"saved": true, "message": "认证成功"})
	case "pending":
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "authorization_pending",
			"error_description": result.ErrorDesc,
			"code":              result.Code,
			"stage":             result.Stage,
		})
	default:
		status := result.HTTPStatus
		if status == 0 {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{
			"error":             result.ErrorName,
			"error_description": result.ErrorDesc,
		})
	}
}

// Cancel POST /codebuddy/auth/cancel
func (h *CodeBuddyAuthHandler) Cancel(c *gin.Context) {
	var req struct {
		AuthState string `json:"auth_state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AuthState == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Missing auth_state"})
		return
	}
	ok, status := h.oauth.Cancel(req.AuthState)
	if !ok {
		c.JSON(status, gin.H{"detail": "Invalid or expired auth_state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func asLimit(err error, target **service.AuthStartLimitError) bool {
	return errors.As(err, target)
}
