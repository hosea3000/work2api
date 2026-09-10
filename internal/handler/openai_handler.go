package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
	"github.com/yourname/work2api/internal/service"
)

// OpenAIHandler OpenAI 兼容入口。
type OpenAIHandler struct {
	*Handler
	chat   *service.ChatExecutor
	models *service.ModelsService
}

func NewOpenAIHandler(h *Handler, chat *service.ChatExecutor, models *service.ModelsService) *OpenAIHandler {
	return &OpenAIHandler{Handler: h, chat: chat, models: models}
}

// ChatCompletions POST /codebuddy/openai/v1/chat/completions
func (h *OpenAIHandler) ChatCompletions(c *gin.Context) {
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
	// 透传会话标识头
	ids := codebuddy.ConversationIDs{
		ConversationID:        c.GetHeader("X-Conversation-ID"),
		ConversationRequestID: c.GetHeader("X-Conversation-Request-ID"),
		ConversationMessageID: c.GetHeader("X-Conversation-Message-ID"),
		RequestID:             c.GetHeader("X-Request-ID"),
	}

	stream, _ := body["stream"].(bool)
	if stream {
		// 流式：直接写 SSE
		_, chatErr, done := h.chat.ExecuteChat(c.Request.Context(), body, ids, c.Writer)
		if done && chatErr != nil {
			h.writeOpenAIError(c, chatErr)
			return
		}
		// 流式路径中校验/上游错误发生在写出前 → 已按错误返回
		if chatErr != nil {
			// headers 可能已写出，只能丢弃
			return
		}
		c.AbortWithStatus(http.StatusOK)
		return
	}

	result, chatErr, done := h.chat.ExecuteChat(c.Request.Context(), body, ids, nil)
	if chatErr != nil {
		if done {
			h.writeOpenAIError(c, chatErr)
		}
		return
	}
	if done && result != nil {
		status := http.StatusOK
		if sc, ok := result["status"].(int); ok {
			status = sc
		}
		delete(result, "status")
		c.JSON(status, result)
	}
}

func (h *OpenAIHandler) writeOpenAIError(c *gin.Context, chatErr *service.ChatRequestError) {
	c.JSON(chatErr.Status, gin.H{
		"error": gin.H{
			"message": chatErr.Message,
			"type":    "invalid_request_error",
			"code":    nil,
		},
	})
}

// Models GET /codebuddy/openai/v1/models
func (h *OpenAIHandler) Models(c *gin.Context) {
	models := h.models.Available(c.Request.Context())
	data := make([]gin.H, 0, len(models))
	for _, m := range models {
		data = append(data, gin.H{
			"id":     m,
			"object": "model",
			"created": 0,
			"owned_by": "codebuddy",
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
