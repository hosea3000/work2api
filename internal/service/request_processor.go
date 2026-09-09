package service

import (
	"encoding/json"
	"strings"
)

// RequestPolicies 请求策略参数（来自配置）。
type RequestPolicies struct {
	ForcedTemperature       *float64
	StripModelNamespace     bool
	ForcedReasoningModels   []string
	DefaultModel            string
}

// ValidateChatRequest 验证 OpenAI 聊天请求（对齐 RequestProcessor.validate_request）。
func ValidateChatRequest(body map[string]any) *ChatRequestError {
	if body == nil {
		return &ChatRequestError{Status: 400, Message: "Request body must be a JSON object"}
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		return &ChatRequestError{Status: 400, Message: "Messages field is required and must be an array"}
	}
	for i, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			return &ChatRequestError{Status: 400, Message: "Message must be an object", Item: &i}
		}
		if _, hasRole := msg["role"]; !hasRole {
			return &ChatRequestError{Status: 400, Message: "Message must have 'role' field", Item: &i}
		}
		if _, hasContent := msg["content"]; !hasContent {
			tcs, _ := msg["tool_calls"].([]any)
			valid := false
			if role, _ := msg["role"].(string); role == "assistant" && len(tcs) > 0 {
				valid = true
				for _, tc := range tcs {
					if _, ok := tc.(map[string]any); !ok {
						valid = false
						break
					}
				}
			}
			if !valid {
				return &ChatRequestError{Status: 400, Message: "Message must have 'content' field", Item: &i}
			}
		}
	}
	// 上游无法报告停止序列命中：非空 stop 直接拒绝
	if hasNonEmptyStop(body["stop"]) {
		return &ChatRequestError{Status: 400, Message: "stop sequences are not supported by CodeBuddy upstream"}
	}
	return nil
}

func hasNonEmptyStop(v any) bool {
	switch t := v.(type) {
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	default:
		return false
	}
}

// ChatRequestError 协议校验错误。
type ChatRequestError struct {
	Status  int
	Message string
	Item    *int
}

func (e *ChatRequestError) Error() string { return e.Message }

// PrepareChatPayload 深拷贝并应用产品策略 + 上游协议适配，
// 返回 (上游 payload, 客户端期望的响应 model)。
func PrepareChatPayload(requestBody map[string]any, policies RequestPolicies) (map[string]any, string) {
	payload := deepCloneMap(requestBody)
	responseModel := stringOr(payload["model"], "")
	if responseModel == "" {
		responseModel = "unknown"
	}

	// model 缺省
	if stringOr(payload["model"], "") == "" {
		if policies.DefaultModel != "" {
			payload["model"] = policies.DefaultModel
		}
	}
	// 命名空间剥离 provider/model → model
	if policies.StripModelNamespace {
		if m, ok := payload["model"].(string); ok {
			payload["model"] = stripNamespace(m)
		}
	}
	// 推理模型强制 max / 默认开启 thinking
	if isForcedReasoningModel(stringOr(payload["model"], ""), policies.ForcedReasoningModels) {
		delete(payload, "enable_thinking")
		payload["reasoning_effort"] = "max"
		thinking, _ := payload["thinking"].(map[string]any)
		merged := map[string]any{"type": "enabled"}
		for k, v := range thinking {
			merged[k] = v
		}
		payload["thinking"] = merged
	} else if thinkingExplicitlyDisabled(payload) {
		delete(payload, "enable_thinking")
	} else if _, has := payload["enable_thinking"]; !has {
		payload["enable_thinking"] = true
	}
	// 强制温度
	if policies.ForcedTemperature != nil {
		payload["temperature"] = *policies.ForcedTemperature
	}
	// 单条 user 消息补 system
	if messages, ok := payload["messages"].([]any); ok && len(messages) == 1 {
		if m, ok := messages[0].(map[string]any); ok && m["role"] == "user" {
			system := map[string]any{"role": "system", "content": "You are a helpful assistant."}
			payload["messages"] = []any{system, messages[0]}
		}
	}
	RewriteSystemPromptMessages(payload)

	// 上游只支持流式
	streamOptions, _ := payload["stream_options"].(map[string]any)
	merged := map[string]any{"include_usage": true}
	for k, v := range streamOptions {
		merged[k] = v
	}
	payload["stream_options"] = merged
	payload["stream"] = true

	return payload, responseModel
}

func stripNamespace(model string) string {
	model = strings.TrimSpace(model)
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		return model[idx+1:]
	}
	return model
}

func isForcedReasoningModel(model string, forced []string) bool {
	norm := strings.ToLower(stripNamespace(model))
	for _, m := range forced {
		if strings.ToLower(stripNamespace(m)) == norm {
			return true
		}
	}
	return false
}

func thinkingExplicitlyDisabled(payload map[string]any) bool {
	if v, ok := payload["enable_thinking"]; ok && isFalseLike(v) {
		return true
	}
	if thinking, ok := payload["thinking"].(map[string]any); ok {
		if t, _ := thinking["type"].(string); strings.ToLower(strings.TrimSpace(t)) == "disabled" {
			return true
		}
	}
	return false
}

func isFalseLike(v any) bool {
	switch t := v.(type) {
	case bool:
		return !t
	case float64:
		return t == 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "false" || s == "0" || s == "no" || s == "off" || s == "disabled"
	default:
		return false
	}
}

func stringOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func deepCloneMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
