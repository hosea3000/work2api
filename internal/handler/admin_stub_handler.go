package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AdminStubHandler 管理台端点：status 真实数据 + settings/stats 打桩（结构与前端 TS 类型逐字段对齐）。
type AdminStubHandler struct {
	*Handler
	startTime time.Time
}

func NewAdminStubHandler(h *Handler) *AdminStubHandler {
	return &AdminStubHandler{Handler: h, startTime: time.Now()}
}

// Status GET /api/admin/status — 对齐 AdminStatus 类型。
func (h *AdminStubHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":                   "work2api",
		"status":                    "ok",
		"username":                  c.GetString("admin_username"),
		"source":                    "users_file",
		"uptime_seconds":            int(time.Since(h.startTime).Seconds()),
		"api_base_url":              "/codebuddy/openai/v1",
		"anthropic_api_base_url":    "",
		"credentials": gin.H{
			"total":   0,
			"valid":   0,
			"current": gin.H{"status": "no_credentials"},
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
		"request_count":                 0,
		"success_rate":                  nil,
		"input_tokens":                  nil,
		"output_tokens":                 nil,
		"total_tokens":                  nil,
		"cache_hit_tokens":              nil,
		"cache_miss_tokens":             nil,
		"total_credit":                  nil,
		"p95_first_output_ms":           nil,
		"p95_first_output_ms_overflow":  false,
		"p95_total_ms":                  nil,
		"p95_total_ms_overflow":         false,
		"usage_coverage":                nil,
	}
}

// StatsOverview GET /api/admin/stats/overview — 对齐 StatsOverviewResponse 空数据。
func (h *AdminStubHandler) StatsOverview(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"totals": statsEmptyTotals(),
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

// StatsRequests GET /api/admin/stats/requests — 空明细。
func (h *AdminStubHandler) StatsRequests(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"requests": []any{}, "total": 0})
}

// CredentialQuota GET /api/admin/credentials/:id/quota — 对齐 CredentialQuota unknown 快照。
func (h *AdminStubHandler) CredentialQuota(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"quota": gin.H{
			"status":                      "unknown",
			"quota_type":                  "personal",
			"quota_available":             nil,
			"total":                       nil,
			"remaining":                   nil,
			"remaining_percent":           nil,
			"estimated":                   false,
			"estimated_credit_since_sync": 0,
			"last_attempt_at":             nil,
			"last_success_at":             nil,
			"last_estimated_at":           nil,
			"error_type":                  nil,
			"packages":                    []any{},
		},
	})
}
