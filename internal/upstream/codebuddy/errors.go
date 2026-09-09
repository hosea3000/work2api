package codebuddy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 错误类别（受控，不透传上游响应体）。
const (
	ErrCategoryAuth        = "authentication_error" // 401/403 凭证失效
	ErrCategoryRateLimit   = "rate_limit_error"
	ErrCategoryUpstream5xx = "upstream_server_error"
	ErrCategoryProtocol    = "upstream_protocol_error"
	ErrCategoryIncomplete  = "upstream_incomplete"
	ErrCategoryTimeout     = "upstream_timeout"
	ErrCategoryTransport   = "upstream_transport_error"
	ErrCategoryInvalidResp = "invalid_response"
)

// UpstreamError 上游调用失败的受控错误。
type UpstreamError struct {
	StatusCode int    // 映射后对客户端的 HTTP 状态
	ErrType    string // 受控错误类别
	Message    string
	CredInvalid bool // true 时调用方应摘除凭证
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("codebuddy upstream error: status=%d type=%s msg=%s", e.StatusCode, e.ErrType, e.Message)
}

// NewCredInvalidError 凭证失效错误（401/403）。
func NewCredInvalidError() *UpstreamError {
	return &UpstreamError{StatusCode: 401, ErrType: ErrCategoryAuth, Message: "CodeBuddy credential rejected by upstream", CredInvalid: true}
}

// BusinessErrorMessages 与参考实现 AUTH_ERROR_DETAILS 对齐。
var BusinessErrorMessages = map[int]string{
	12005: "企业许可证没有可用席位",
	11212: "CodeBuddy 许可证已过期",
	11216: "CodeBuddy 试用授权已过期",
	10081: "当前 IP 被 CodeBuddy 访问策略限制",
}

// MapUpstreamStatus 将上游 HTTP 状态映射为对客户端状态与错误类别（对齐 _handle_api_error）。
func MapUpstreamStatus(statusCode int, message string) *UpstreamError {
	switch {
	case statusCode == 401:
		return &UpstreamError{StatusCode: 401, ErrType: ErrCategoryAuth, Message: message, CredInvalid: true}
	case statusCode == 403:
		return &UpstreamError{StatusCode: 401, ErrType: ErrCategoryAuth, Message: message, CredInvalid: true}
	case statusCode == 429:
		return &UpstreamError{StatusCode: 429, ErrType: ErrCategoryRateLimit, Message: message}
	case statusCode >= 500:
		return &UpstreamError{StatusCode: 502, ErrType: ErrCategoryUpstream5xx, Message: message}
	case statusCode >= 400:
		return &UpstreamError{StatusCode: statusCode, ErrType: "upstream_error", Message: message}
	default:
		return &UpstreamError{StatusCode: 502, ErrType: ErrCategoryProtocol,
			Message: fmt.Sprintf("CodeBuddy API unexpected status: %d", statusCode)}
	}
}

// ParseUpstreamErrorBody 尽力从错误体提取 message/type/code；失败时用原文。
func ParseUpstreamErrorBody(raw string) (message, errType string, code any) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw, "", nil
	}
	if e, ok := v["error"].(map[string]any); ok {
		if m, ok := e["message"].(string); ok && m != "" {
			message = m
		}
		if t, ok := e["type"].(string); ok {
			errType = t
		}
		code = e["code"]
		return
	}
	// CodeBuddy 业务信封 {code, msg}
	if m, ok := v["msg"].(string); ok && m != "" {
		message = m
	}
	if c, ok := v["code"].(float64); ok {
		code = int(c)
		if int(c) != 0 {
			if bm, ok := BusinessErrorMessages[int(c)]; ok {
				message = bm
			}
		}
	}
	if message == "" {
		message = raw
	}
	return
}

// Timeout 覆盖 connect 10s / 整体读 30s；聊天读超时可按需放大。
func Timeout(read time.Duration) time.Duration {
	if read <= 0 {
		read = 30 * time.Second
	}
	return read
}

var _ = http.StatusOK
