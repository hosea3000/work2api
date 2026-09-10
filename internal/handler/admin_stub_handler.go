package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hosea3000/work2api/internal/service"
)

// AdminStubHandler 管理台端点：status/stats overview 真实数据 + settings 打桩。
type AdminStubHandler struct {
	*Handler
	cbCreds   service.CodeBuddyCredentialService
	traeCreds service.TraeCredentialService
	stats     service.StatsService
	startTime time.Time
}

func NewAdminStubHandler(h *Handler, cbCreds service.CodeBuddyCredentialService, traeCreds service.TraeCredentialService, stats service.StatsService) *AdminStubHandler {
	return &AdminStubHandler{Handler: h, cbCreds: cbCreds, traeCreds: traeCreds, stats: stats, startTime: time.Now()}
}

// Status GET /api/admin/status — 对齐 AdminStatus 类型。
func (h *AdminStubHandler) Status(c *gin.Context) {
	ctx := c.Request.Context()
	cbList, err := h.cbCreds.List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "list credentials failed"})
		return
	}
	traeList, err := h.traeCreds.List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "list credentials failed"})
		return
	}
	total := len(cbList) + len(traeList)
	currentStatus := "no_credentials"
	if total > 0 {
		currentStatus = "auto_rotation"
	}
	c.JSON(http.StatusOK, gin.H{
		"service":        "work2api",
		"status":         "healthy",
		"username":       c.GetString("admin_username"),
		"source":         "users_file",
		"uptime_seconds": int(time.Since(h.startTime).Seconds()),
		"credentials": gin.H{
			"total":   total,
			"valid":   total,
			"current": gin.H{"status": currentStatus},
		},
	})
}

// GetSettings GET /api/admin/settings — 对齐 SettingsResponse。
func (h *AdminStubHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"settings": gin.H{},
		"fields":   []any{},
	})
}

// SaveSettings PUT /api/admin/settings — 接受但不持久化（二期）。
func (h *AdminStubHandler) SaveSettings(c *gin.Context) {
	var req map[string]any
	_ = c.ShouldBindJSON(&req)
	c.JSON(http.StatusOK, gin.H{
		"settings": gin.H{},
		"fields":   []any{},
		"message":  "设置暂未启用",
	})
}

// statsEmptyTotals 对齐 StatsTotals。
func statsEmptyTotals() gin.H {
	return gin.H{
		"request_count":                0,
		"success_rate":                 nil,
		"input_tokens":                 nil,
		"output_tokens":                nil,
		"total_tokens":                 nil,
		"cache_hit_tokens":             nil,
		"cache_miss_tokens":            nil,
		"total_credit":                 nil,
		"p95_first_output_ms":          nil,
		"p95_first_output_ms_overflow": false,
		"p95_total_ms":                 nil,
		"p95_total_ms_overflow":        false,
		"usage_coverage":               nil,
	}
}

// StatsOverview GET /api/admin/stats/overview — 返回 [start_at, end_at) 内真实请求数与成功率。
func (h *AdminStubHandler) StatsOverview(c *gin.Context) {
	startAt := parseQueryInt64(c.Query("start_at"), 0)
	endAt := parseQueryInt64(c.Query("end_at"), time.Now().Unix())
	total, success, err := h.stats.Overview(c.Request.Context(), startAt, endAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "load stats failed"})
		return
	}
	totals := statsEmptyTotals()
	totals["request_count"] = total
	if total > 0 {
		totals["success_rate"] = float64(success) / float64(total)
	}
	c.JSON(http.StatusOK, gin.H{
		"totals": totals,
		"series": []any{},
		"dimensions": gin.H{
			"models":      []any{},
			"api_keys":    []any{},
			"credentials": []any{},
			"outcomes":    []any{},
		},
		"breakdowns": gin.H{
			"models":      []any{},
			"api_keys":    []any{},
			"credentials": []any{},
		},
		"data_quality": gin.H{
			"coverage": nil,
			"messages": []any{},
		},
	})
}

func parseQueryInt64(raw string, fallback int64) int64 {
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

// StatsRequests GET /api/admin/stats/requests — 空明细。
func (h *AdminStubHandler) StatsRequests(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"requests": []any{}, "total": 0})
}
