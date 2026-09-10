package handler

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// TraeAuthHandler 管理台 TRAE 网页登录端点 + 公共 /authorize 回调落点。
type TraeAuthHandler struct {
	*Handler
	svc *service.TraeLoginService
}

func NewTraeAuthHandler(h *Handler, svc *service.TraeLoginService) *TraeAuthHandler {
	return &TraeAuthHandler{Handler: h, svc: svc}
}

// Start POST /api/admin/trae/login/start
func (h *TraeAuthHandler) Start(c *gin.Context) {
	res, err := h.svc.Start()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "生成登录链接失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"login_url":    res.LoginURL,
		"pending_id":   res.PendingID,
		"callback_url": res.CallbackURL,
		"expires_in":   600,
		"instructions": "请在打开的 TRAE 登录页完成登录，登录成功后凭证自动导入",
	})
}

// Result GET /api/admin/trae/login/result?pending_id=
func (h *TraeAuthHandler) Result(c *gin.Context) {
	res, ok := h.svc.Result(c.Query("pending_id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "pending login not found (expired or invalid)"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Cancel POST /api/admin/trae/login/cancel
func (h *TraeAuthHandler) Cancel(c *gin.Context) {
	var req struct {
		PendingID string `json:"pending_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.PendingID == "" {
		req.PendingID = c.Query("pending_id")
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": h.svc.Cancel(req.PendingID)})
}

// Import POST /api/admin/trae/login/import（粘贴回调 URL 兜底）
func (h *TraeAuthHandler) Import(c *gin.Context) {
	var req struct {
		CallbackURL string `json:"callback_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.CallbackURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "callback_url required"})
		return
	}
	cred, err := h.svc.Import(c.Request.Context(), req.CallbackURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"credential": gin.H{"id": cred.Id, "provider": "trae", "user_id": cred.UserId},
	})
}

// Authorize GET /authorize — TRAE 登录成功后浏览器 302 落点（公共端点）。
// 回调无管理会话，安全性由 pending 归属校验（loginTraceID 反查）保证。
func (h *TraeAuthHandler) Authorize(c *gin.Context) {
	rawURL := "http://127.0.0.1" + c.Request.URL.String()
	err := h.svc.HandleAuthorize(c.Request.Context(), rawURL)
	if err == nil {
		authorizeRender(c.Writer, http.StatusOK, "登录成功",
			"TRAE 账号已添加到凭证池，可关闭此窗口返回管理台查看。")
		return
	}
	authorizeRender(c.Writer, http.StatusBadRequest, "登录失败", err.Error())
}

// authorizeRender 渲染面向浏览器的简洁结果页（非 JSON）。
func authorizeRender(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:system-ui;max-width:32rem;margin:12vh auto;text-align:center;color:#333">
<h1>%s</h1><p>%s</p></body></html>`,
		html.EscapeString(title), html.EscapeString(title), html.EscapeString(detail))
}

var _ = context.Background
