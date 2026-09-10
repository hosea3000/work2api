package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/service"
)

// TraeChatHandler TRAE SOLO 的 OpenAI 兼容入口（独立端点 /trae/openai/v1/*）。
type TraeChatHandler struct {
	*Handler
	chat   *service.TraeChatExecutor
	models *service.TraeModelsService
}

func NewTraeChatHandler(h *Handler, chat *service.TraeChatExecutor, models *service.TraeModelsService) *TraeChatHandler {
	return &TraeChatHandler{Handler: h, chat: chat, models: models}
}

// ChatCompletions POST /trae/openai/v1/chat/completions
func (h *TraeChatHandler) ChatCompletions(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 32*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "unable to read request body", "type": "invalid_request_error"}})
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "Invalid JSON request body", "type": "invalid_request_error"}})
		return
	}

	stream, _ := body["stream"].(bool)
	if stream {
		result, chatErr, done := h.chat.ExecuteChat(c.Request.Context(), body, c.Writer)
		if chatErr != nil {
			h.writeOpenAIError(c, chatErr)
			return
		}
		if done && result != nil {
			h.writeResult(c, result)
		}
		return
	}

	result, chatErr, done := h.chat.ExecuteChat(c.Request.Context(), body, nil)
	if chatErr != nil {
		h.writeOpenAIError(c, chatErr)
		return
	}
	if done && result != nil {
		h.writeResult(c, result)
	}
}

func (h *TraeChatHandler) writeResult(c *gin.Context, result map[string]any) {
	status := http.StatusOK
	if sc, ok := result["status"].(int); ok {
		status = sc
	}
	delete(result, "status")
	c.JSON(status, result)
}

func (h *TraeChatHandler) writeOpenAIError(c *gin.Context, chatErr *service.ChatRequestError) {
	c.JSON(chatErr.Status, gin.H{
		"error": gin.H{
			"message": chatErr.Message,
			"type":    "invalid_request_error",
			"code":    nil,
		},
	})
}

// Models GET /trae/openai/v1/models
func (h *TraeChatHandler) Models(c *gin.Context) {
	models := h.models.Available(c.Request.Context())
	data := make([]gin.H, 0, len(models))
	for _, m := range models {
		data = append(data, gin.H{
			"id":       m,
			"object":   "model",
			"created":  0,
			"owned_by": "trae",
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
