package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// TraeCredentialHandler 管理台 TRAE 凭证管理。
type TraeCredentialHandler struct {
	*Handler
	creds   service.TraeCredentialService
	checkin service.TraeCheckinService
	models  *service.TraeModelsService
}

func NewTraeCredentialHandler(h *Handler, creds service.TraeCredentialService, checkin service.TraeCheckinService, models *service.TraeModelsService) *TraeCredentialHandler {
	return &TraeCredentialHandler{Handler: h, creds: creds, checkin: checkin, models: models}
}

func (h *TraeCredentialHandler) List(c *gin.Context) {
	list, err := h.creds.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "list credentials failed"})
		return
	}
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
			"provider":           v.Provider,
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
		current = gin.H{"status": "auto_rotation", "credential_id": cur.Id, "user_id": cur.UserId}
	}
	c.JSON(http.StatusOK, gin.H{
		"credentials":           records,
		"current":               current,
		"auto_rotation_enabled": h.creds.RotationEnabled(),
	})
}

func (h *TraeCredentialHandler) Delete(c *gin.Context) {
	id := c.Param("credential_id")
	if err := h.creds.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "credential not found"})
		return
	}
	if h.models != nil {
		h.models.Invalidate()
	}
	current := gin.H{"status": "no_credentials"}
	if cur, _ := h.creds.Current(c.Request.Context()); cur != nil {
		current = gin.H{"status": "auto_rotation", "credential_id": cur.Id, "user_id": cur.UserId}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "current": current})
}

func (h *TraeCredentialHandler) Select(c *gin.Context) {
	id := c.Param("credential_id")
	cred, disabled, err := h.creds.Select(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "选择凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_rotation_disabled_by_select": disabled,
		"current": gin.H{
			"status":        "auto_rotation_disabled",
			"credential_id": cred.Id,
			"user_id":       cred.UserId,
		},
	})
}

func (h *TraeCredentialHandler) ToggleRotation(c *gin.Context) {
	enabled, current, err := h.creds.ToggleRotation(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "toggle failed"})
		return
	}
	currentView := gin.H{"status": "no_credentials"}
	if current != nil {
		currentView = gin.H{"status": "auto_rotation", "credential_id": current.Id, "user_id": current.UserId}
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_rotation_enabled": enabled,
		"current":               currentView,
	})
}

func (h *TraeCredentialHandler) Test(c *gin.Context) {
	id := c.Param("credential_id")
	ok, statusCode, detail := h.creds.Test(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"ok": ok, "status_code": statusCode, "detail": detail})
}

func (h *TraeCredentialHandler) DailyCheckin(c *gin.Context) {
	id := c.Param("credential_id")
	detail, err := h.checkin.ManualCheckin(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "签到请求失败"})
		return
	}
	c.JSON(http.StatusOK, checkinResponse(detail))
}
