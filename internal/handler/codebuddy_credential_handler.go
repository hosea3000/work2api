package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hosea3000/work2api/internal/service"
)

// CodeBuddyCredentialHandler 管理台 CodeBuddy 凭证管理。
type CodeBuddyCredentialHandler struct {
	*Handler
	creds   service.CodeBuddyCredentialService
	checkin service.CodeBuddyCheckinService
	models  *service.ModelsService
	quota   service.QuotaService
}

func NewCodeBuddyCredentialHandler(h *Handler, creds service.CodeBuddyCredentialService, checkin service.CodeBuddyCheckinService, models *service.ModelsService, quota service.QuotaService) *CodeBuddyCredentialHandler {
	return &CodeBuddyCredentialHandler{Handler: h, creds: creds, checkin: checkin, models: models, quota: quota}
}

func (h *CodeBuddyCredentialHandler) List(c *gin.Context) {
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
			"quota":              credentialQuota(v),
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

func (h *CodeBuddyCredentialHandler) Create(c *gin.Context) {
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

func (h *CodeBuddyCredentialHandler) Delete(c *gin.Context) {
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

func (h *CodeBuddyCredentialHandler) Select(c *gin.Context) {
	id := c.Param("credential_id")
	cred, disabled, err := h.creds.Select(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNoCredential) || errors.Is(err, service.ErrCredentialNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "选择凭证失败"})
			return
		}
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

func (h *CodeBuddyCredentialHandler) ToggleRotation(c *gin.Context) {
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

func (h *CodeBuddyCredentialHandler) Test(c *gin.Context) {
	id := c.Param("credential_id")
	ok, statusCode, detail := h.creds.Test(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"ok": ok, "status_code": statusCode, "detail": detail})
}

// RefreshQuota POST /api/admin/codebuddy/credentials/:credential_id/quota/refresh
func (h *CodeBuddyCredentialHandler) RefreshQuota(c *gin.Context) {
	refreshQuota(c, h.quota, "codebuddy")
}

// credentialQuota 把凭证视图的额度两列转成前端 {total, remaining}；未探测为 nil。
func credentialQuota(v service.CredentialView) any {
	if v.QuotaTotal == nil || v.QuotaRemaining == nil {
		return nil
	}
	return gin.H{"total": *v.QuotaTotal, "remaining": *v.QuotaRemaining}
}

// refreshQuota 两 provider 共用的额度刷新响应：成功 {quota}，未找到 404，跳过 409，其余 502。
func refreshQuota(c *gin.Context, svc service.QuotaService, provider string) {
	quota, err := svc.Refresh(c.Request.Context(), provider, c.Param("credential_id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCredentialNotFound):
			c.JSON(http.StatusNotFound, gin.H{"detail": "credential not found"})
		case errors.Is(err, service.ErrQuotaSkipped):
			c.JSON(http.StatusConflict, gin.H{"detail": "该凭证不支持个人额度探测"})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"detail": "额度刷新失败"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"quota": gin.H{"total": quota.Total, "remaining": quota.Remaining}})
}

func (h *CodeBuddyCredentialHandler) DailyCheckin(c *gin.Context) {
	id := c.Param("credential_id")
	detail, err := h.checkin.ManualCheckin(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "签到请求失败"})
		return
	}
	c.JSON(http.StatusOK, checkinResponse(detail))
}

// checkinResponse 把签到明细转为前端 CredentialDailyCheckin 结构。
func checkinResponse(detail map[string]any) gin.H {
	resp := gin.H{"code": nil, "message": detail["message"], "success": detail["success"]}
	if code, ok := detail["code"].(*int); ok && code != nil {
		resp["code"] = *code
	}
	if credit, ok := detail["credit"].(*float64); ok && credit != nil {
		resp["credit"] = *credit
	}
	return resp
}
