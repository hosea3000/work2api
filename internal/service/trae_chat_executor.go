package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/yourname/work2api/internal/upstream/trae"
)

// TraeChatExecutor TRAE 聊天执行器：镜像 ChatExecutor 骨架，上游协议走 trae 适配器。
// 独立于 codebuddy 路径，使用 TRAE 独立凭证池。
type TraeChatExecutor struct {
	credService TraeCredentialService
	client      *trae.Client
}

func NewTraeChatExecutor(credService TraeCredentialService, client *trae.Client) *TraeChatExecutor {
	return &TraeChatExecutor{credService: credService, client: client}
}

// ExecuteChat 执行 TRAE 聊天请求。
// 流式成功：写出 SSE，返回 (nil, nil, false)；非流式：返回聚合 result；
// 上游错误（未写 SSE 头）：返回 (nil, chatErr, true) 由 handler 写 OpenAI 错误。
func (x *TraeChatExecutor) ExecuteChat(
	ctx context.Context,
	requestBody map[string]any,
	w http.ResponseWriter,
) (result map[string]any, streamErr *ChatRequestError, done bool) {
	if err := validateMessages(requestBody); err != nil {
		return nil, err, true
	}
	responseModel := stringOr(requestBody["model"], trae.DefaultConfigName)
	payload := trae.PrepareChatBody(deepCloneMap(requestBody))

	entry, ok := x.credService.SelectForChat(ctx)
	if !ok {
		return nil, &ChatRequestError{Status: 503, Message: "No available TRAE credential"}, true
	}
	cred := entry.Credential

	clientWantsStream := boolOr(requestBody["stream"])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.client.ChatURL(), strings.NewReader(mustJSON(payload)))
	if err != nil {
		return nil, &ChatRequestError{Status: 500, Message: "upstream request build failed"}, true
	}
	trae.SOLOHeaders(req, cred.BearerToken, cred.UserId, derefStr(cred.MachineID), derefStr(cred.DeviceID), clientWantsStream)
	// 用无总超时的流客户端，避免长 SSE 流被 HTTP.Timeout 截断。
	hc := x.client.HTTP
	if x.client.StreamHTTP != nil {
		hc = x.client.StreamHTTP
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &ChatRequestError{Status: 502, Message: "TRAE upstream transport error"}, true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw := readLimited(resp.Body, 64*1024)
		x.applyHTTPError(ctx, cred.Id, resp.StatusCode)
		status, errType, msg := classifyTraeHTTP(resp.StatusCode, raw)
		return nil, &ChatRequestError{Status: status, Message: errType + ": " + msg}, true
	}

	if clientWantsStream && w != nil {
		_ = trae.StreamWithError(w, resp.Body, func(se *trae.SOLOStreamError) {
			x.applySOLOError(ctx, cred.Id, se.Code)
		})
		return nil, nil, false
	}

	agg, err := trae.Aggregate(resp.Body)
	if err != nil {
		if se, ok := err.(*trae.SOLOStreamError); ok {
			x.applySOLOError(ctx, cred.Id, se.Code)
			return nil, &ChatRequestError{Status: traeErrorStatus(se.Code), Message: se.Error()}, true
		}
		return nil, &ChatRequestError{Status: 502, Message: "TRAE upstream stream error"}, true
	}
	agg["model"] = responseModel
	return agg, nil, true
}

// applyHTTPError HTTP 层错误：401/403 → 摘除凭证。
func (x *TraeChatExecutor) applyHTTPError(ctx context.Context, id string, status int) {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		x.credService.MarkExpired(ctx, id)
	}
}

// applySOLOError SOLO 流内错误码 → 冷却/摘除。
func (x *TraeChatExecutor) applySOLOError(ctx context.Context, id string, code int64) {
	now := time.Now()
	switch code {
	case 401:
		x.credService.MarkExpired(ctx, id)
	case 1005: // plan 权益不足 → 硬冷却 12h
		x.credService.Cooldown(ctx, id, now.Add(12*time.Hour).Unix())
	case 4011: // 频率超限 → 短冷却
		x.credService.Cooldown(ctx, id, now.Add(60*time.Second).Unix())
	case 4008: // ide_credits 耗尽 → 次日 0 点
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		x.credService.Cooldown(ctx, id, next.Unix())
	}
}

// traeErrorStatus SOLO 错误码 → OpenAI 响应状态。
func traeErrorStatus(code int64) int {
	switch code {
	case 1005, 4008, 4011:
		return http.StatusTooManyRequests
	case 401:
		return http.StatusUnauthorized
	default:
		return http.StatusBadRequest
	}
}

// classifyTraeHTTP 把上游非 2xx 响应映射为 (OpenAI 状态, 错误类型, 消息)。
func classifyTraeHTTP(status int, raw string) (int, string, string) {
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return http.StatusUnauthorized, "authentication_error", msg
	case status == http.StatusTooManyRequests:
		return http.StatusTooManyRequests, "rate_limit_error", msg
	case status >= 500:
		return http.StatusBadGateway, "upstream_error", msg
	default:
		return http.StatusBadRequest, "invalid_request_error", msg
	}
}
