package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// APIKeyHandler 管理台 API Key 管理。
type APIKeyHandler struct {
	*Handler
	apiKeys service.APIKeyService
}

func NewAPIKeyHandler(h *Handler, apiKeys service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{Handler: h, apiKeys: apiKeys}
}

func (h *APIKeyHandler) List(c *gin.Context) {
	keys, err := h.apiKeys.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "list api keys failed"})
		return
	}
	// 对齐 ApiKeyRecord：preview + created_at(epoch 秒)
	records := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		var created int64
		if t, perr := time.Parse(time.RFC3339, k.CreatedAt); perr == nil {
			created = t.Unix()
		}
		records = append(records, gin.H{
			"id":           k.Id,
			"name":         k.Name,
			"preview":      k.KeySuffix,
			"created_at":   created,
			"last_used_at": nil,
		})
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": records})
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "invalid request"})
		return
	}
	result, err := h.apiKeys.Create(c.Request.Context(), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "create api key failed"})
		return
	}
	// 对齐 ApiKeyCreateResponse：记录字段 + api_key 明文（仅此一次）
	c.JSON(http.StatusOK, gin.H{
		"id":           result.Id,
		"name":         result.Name,
		"preview":      "..." + result.Key[len(result.Key)-4:],
		"created_at":   time.Now().Unix(),
		"last_used_at": nil,
		"api_key":      result.Key,
	})
}

func (h *APIKeyHandler) Delete(c *gin.Context) {
	id := c.Param("key_id")
	if err := h.apiKeys.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "delete api key failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
