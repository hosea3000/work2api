package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// CredentialHandler 管理台凭证管理（含签到、选择、轮换开关、测试）。
type CredentialHandler struct {
	*Handler
	creds        service.CredentialService
	checkin      service.CheckinService
	models       *service.ModelsService
	credExecutor *service.ChatExecutor
}

func NewCredentialHandler(h *Handler, creds service.CredentialService, checkin service.CheckinService, models *service.ModelsService, chat *service.ChatExecutor) *CredentialHandler {
	return &CredentialHandler{Handler: h, creds: creds, checkin: checkin, models: models, credExecutor: chat}
}

func (h *CredentialHandler) List(c *gin.Context) {
	list, err := h.creds.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "list credentials failed"})
		return
	}
	// 对齐 CredentialsResponse / CredentialRecord / CurrentCredential 类型
	records := make([]gin.H, 0, len(list))
	for _, v := range list {
		records = append(records, gin.H{
			"credential_id":      v.Id,
			"filename":           v.Id + ".json",
			"user_id":            v.UserId,
			"time_remaining":     nil,
			"time_remaining_str": "",
			"is_expired":         v.Status != "active",
			"token_type":         "Bearer",
			"auth_source":        v.AuthSource,
			"enterprise_id":      v.Enterprise,
			"has_refresh_token":  false,
			"has_token":          true,
			"token_display":      v.TokenSuffix,
			"nickname":           v.Nickname,
			"preferred_username": v.PreferredUsername,
			"email":              v.Email,
		})
	}
	current := gin.H{"status": "no_credentials"}
	if cur, _ := h.creds.Current(c.Request.Context()); cur != nil {
		current = gin.H{
			"status":        "auto_rotation",
			"credential_id": cur.Id,
			"user_id":       cur.UserId,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"credentials":           records,
		"current":               current,
		"auto_rotation_enabled": h.creds.RotationEnabled(),
	})
}

func (h *CredentialHandler) Create(c *gin.Context) {
	var req struct {
		BearerToken string `json:"bearer_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BearerToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "bearer_token required"})
		return
	}
	cred, err := h.creds.Add(c.Request.Context(), req.BearerToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的凭证 token"})
		return
	}
	h.models.Invalidate()
	c.JSON(http.StatusOK, gin.H{
		"credential": gin.H{
			"credential_id":      cred.Id,
			"filename":           cred.Id + ".json",
			"nickname":           cred.Nickname,
			"preferred_username": cred.PreferredUsername,
			"email":              cred.Email,
			"user_id":            cred.UserId,
			"is_expired":         false,
			"token_type":         "Bearer",
			"auth_source":        cred.AuthSource,
			"has_refresh_token":  false,
			"has_token":          true,
			"token_display":      cred.TokenSuffix,
			"time_remaining_str": "",
		},
	})
}

func (h *CredentialHandler) Delete(c *gin.Context) {
	id := c.Param("credential_id")
	if err := h.creds.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "credential not found"})
		return
	}
	h.models.Invalidate()
	current := gin.H{"status": "no_credentials"}
	if cur, _ := h.creds.Current(c.Request.Context()); cur != nil {
		current = gin.H{"status": "auto_rotation", "credential_id": cur.Id, "user_id": cur.UserId}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "current": current})
}

func (h *CredentialHandler) Select(c *gin.Context) {
	id := c.Param("credential_id")
	cred, disabled, err := h.creds.Select(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "选择凭证失败"})
		return
	}
	h.models.Invalidate()
	c.JSON(http.StatusOK, gin.H{
		"auto_rotation_disabled_by_select": disabled,
		"current": gin.H{
			"status":        "auto_rotation_disabled",
			"credential_id": cred.Id,
			"user_id":       cred.UserId,
		},
	})
}

func (h *CredentialHandler) ToggleRotation(c *gin.Context) {
	enabled, current, err := h.creds.ToggleRotation(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "toggle failed"})
		return
	}
	currentView := gin.H{"status": "no_credentials"}
	if current != nil {
		currentView = gin.H{
			"status":        "auto_rotation",
			"credential_id": current.Id,
			"user_id":       current.UserId,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_rotation_enabled": enabled,
		"current":               currentView,
	})
}

func (h *CredentialHandler) Test(c *gin.Context) {
	id := c.Param("credential_id")
	ok, statusCode, detail := h.creds.Test(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{
		"ok":          ok,
		"status_code": statusCode,
		"detail":      detail,
	})
}

func (h *CredentialHandler) DailyCheckin(c *gin.Context) {
	id := c.Param("credential_id")
	detail, err := h.checkin.ManualCheckin(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "签到请求失败"})
		return
	}
	// 对齐 CredentialDailyCheckin
	resp := gin.H{
		"code":    nil,
		"message": detail["message"],
		"success": detail["success"],
	}
	if code, ok := detail["code"].(*int); ok && code != nil {
		resp["code"] = *code
	}
	if credit, ok := detail["credit"].(*float64); ok && credit != nil {
		resp["credit"] = *credit
	}
	if at, ok := detail["checked_in_at"].(*int64); ok && at != nil {
		resp["checked_in_at"] = *at
	}
	c.JSON(http.StatusOK, resp)
}

var _ = context.Background
