package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/yourname/work2api/internal/upstream/codebuddy"
)

// ChatExecutor 聊天执行器：选择凭证 → 构造头 → 上游流式请求 → 转换/聚合。
type ChatExecutor struct {
	credService    CredentialService
	client         *codebuddy.Client
	policies       *RequestPolicies
	maxRetryCred   int
	firstChunkWait time.Duration
}

func NewChatExecutor(credService CredentialService, client *codebuddy.Client, policies *RequestPolicies) *ChatExecutor {
	return &ChatExecutor{
		credService:    credService,
		client:         client,
		policies:       policies,
		maxRetryCred:   1,
		firstChunkWait: 310 * time.Second,
	}
}

// ResponseContext 客户端可见响应信封。
type ResponseContext struct {
	ResponseID string
	Created    int64
	Model      string
}

func newResponseContext(responseModel string) ResponseContext {
	return ResponseContext{
		ResponseID: "chatcmpl-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Created:    time.Now().Unix(),
		Model:      responseModel,
	}
}

// OpenAIError OpenAI 错误格式。
type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code,omitempty"`
}

func openAIErrorResponse(status int, errType, message string) map[string]any {
	return map[string]any{
		"error": map[string]any{"message": message, "type": errType, "code": nil},
		"status": status,
	}
}

// ExecuteChat 执行聊天请求。
// 流式：写出 SSE chunk 到 http.ResponseWriter；非流式：聚合返回 JSON。
func (x *ChatExecutor) ExecuteChat(
	ctx context.Context,
	requestBody map[string]any,
	conversationIDs codebuddy.ConversationIDs,
	w http.ResponseWriter, // 流式时的 ResponseWriter；非流式传 nil
) (result map[string]any, streamErr *ChatRequestError, done bool) {
	// 1. 校验
	if err := ValidateChatRequest(requestBody); err != nil {
		return nil, &ChatRequestError{Status: err.Status, Message: err.Message}, true
	}
	// 2. 预处理
	payload, responseModel := PrepareChatPayload(requestBody, *x.policies)

	// 3. 选择凭证（有限换凭证重试）
	var cred PoolEntry
	var ok bool
	for attempt := 0; attempt <= x.maxRetryCred; attempt++ {
		sel, selOK := x.credService.SelectByToken(ctx)
		if !selOK {
			return nil, &ChatRequestError{Status: 503, Message: "No available CodeBuddy credential"}, true
		}
		cred = sel.Entry
		ok = true
		break
	}
	if !ok {
		return nil, &ChatRequestError{Status: 503, Message: "No available CodeBuddy credential"}, true
	}

	headers, err := codebuddy.GenerateHeaders(cred.Snapshot, conversationIDs, x.client.CLIVersion)
	if err != nil {
		return nil, &ChatRequestError{Status: 500, Message: "credential header generation failed"}, true
	}

	clientWantsStream := boolOr(requestBody["stream"])
	respCtx := newResponseContext(responseModel)

	// 4. 上游请求
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, x.client.ChatURL(), strings.NewReader(mustJSON(payload)))
	if err != nil {
		return nil, &ChatRequestError{Status: 500, Message: "upstream request build failed"}, true
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := x.client.HTTP.Do(httpReq)
	if err != nil {
		return nil, &ChatRequestError{Status: 502, Message: "Upstream transport error"}, true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw := readLimited(resp.Body, 64*1024)
		ue := mapUpstreamBody(resp.StatusCode, raw)
		if ue.CredInvalid {
			x.credService.MarkExpired(ctx, cred.Credential.Id)
		}
		if clientWantsStream && w != nil {
			// 已按 SSE 头响应前出错：用 SSE error 事件
			return nil, nil, false // handled below via stream path
		}
		return openAIErrorResponse(ue.StatusCode, ue.ErrType, ue.Message), nil, true
	}

	// 5. 消费 SSE
	if clientWantsStream && w != nil {
		x.streamToClient(w, resp, respCtx)
		return nil, nil, false
	}
	result, aggErr := x.aggregate(resp, respCtx)
	if aggErr != nil {
		return nil, &ChatRequestError{Status: aggErr.Status, Message: aggErr.Message}, true
	}
	return result, nil, true
}

func boolOr(v any) bool {
	b, _ := v.(bool)
	return b
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func readLimited(r interface{ Read([]byte) (int, error) }, n int64) string {
	buf := make([]byte, n)
	total, _ := r.Read(buf)
	return string(buf[:total])
}

func mapUpstreamBody(statusCode int, raw string) *codebuddy.UpstreamError {
	message, errType, _ := codebuddy.ParseUpstreamErrorBody(raw)
	ue := codebuddy.MapUpstreamStatus(statusCode, message)
	if errType != "" && ue.ErrType == "upstream_error" {
		ue.ErrType = errType
	}
	return ue
}

// streamToClient 把上游 SSE 转换为 OpenAI chunk 流并实时写出。
func (x *ChatExecutor) streamToClient(w http.ResponseWriter, resp *http.Response, respCtx ResponseContext) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	normalizer := codebuddy.NewStreamNormalizer()
	indexState := codebuddy.NewToolCallIndexState()
	sawDone := false
	var finishSeen bool

	onEvent := func(ev any) bool {
		switch e := ev.(type) {
		case codebuddy.SSEDone:
			sawDone = true
			return false
		case map[string]any:
			if _, hasErr := e["error"]; hasErr {
				writeSSE(w, flusher, codebuddy.FormatSSEError("CodeBuddy upstream stream error", "upstream_error"))
				return false
			}
			event := codebuddy.ParseEvent(e)
			if event.FinishReason != nil {
				finishSeen = true
			}
			converted := codebuddy.AddOpenAIToolCallIndexes(event, indexState)
			converted = codebuddy.NormalizeChunkEnvelope(converted, respCtx.ResponseID, respCtx.Created, respCtx.Model)
			for _, chunk := range normalizer.Normalize(converted) {
				writeSSE(w, flusher, codebuddy.FormatSSEEvent(chunk))
			}
		}
		return true
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	var buffer strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			buffer.Write(buf[:n])
			for {
				s := buffer.String()
				idx := strings.Index(s, "\n")
				if idx < 0 {
					break
				}
				line := s[:idx]
				rest := s[idx+1:]
				buffer.Reset()
				buffer.WriteString(rest)
				parsed, perr := codebuddy.ParseSSELine(line)
				if perr != nil {
					writeSSE(w, flusher, codebuddy.FormatSSEError(perr.Error(), "upstream_protocol_error"))
					return
				}
				if parsed == nil {
					continue
				}
				if !onEvent(parsed) {
					goto streamEnd
				}
			}
		}
		if err != nil {
			break
		}
	}
streamEnd:
	if sawDone || finishSeen {
		writeSSE(w, flusher, codebuddy.FormatSSEDone())
	} else {
		writeSSE(w, flusher, codebuddy.FormatSSEError("CodeBuddy upstream stream ended without a completion marker", codebuddy.ErrCategoryIncomplete))
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, s string) {
	if s == "" {
		return
	}
	_, _ = w.Write([]byte(s))
	if flusher != nil {
		flusher.Flush()
	}
}

// aggregate 消费上游流并聚合为非流式 OpenAI 响应（对齐 StreamResponseAggregator）。
func (x *ChatExecutor) aggregate(resp *http.Response, respCtx ResponseContext) (map[string]any, *ChatRequestError) {
	agg := newAggregator(respCtx)
	sawDone := false
	var finishSeen bool

	onEvent := func(ev any) bool {
		switch e := ev.(type) {
		case codebuddy.SSEDone:
			sawDone = true
			return false
		case map[string]any:
			if _, hasErr := e["error"]; hasErr {
				return false
			}
			event := codebuddy.ParseEvent(e)
			if event.FinishReason != nil {
				finishSeen = true
			}
			agg.Process(event)
		}
		return true
	}
	_ = codebuddy.IterSSEEventsScanner(resp.Body, onEvent)

	if !sawDone && !finishSeen {
		return nil, &ChatRequestError{Status: 502, Message: "CodeBuddy upstream stream ended without a completion marker"}
	}
	return agg.Finalize(), nil
}

// aggregator 非流式聚合状态。
type aggregator struct {
	ctx            ResponseContext
	content        strings.Builder
	reasoning      strings.Builder
	finishReason   *string
	usage          any
	systemFp       any
	indexState     *codebuddy.ToolCallIndexState
	toolCallMap    map[int]map[string]any
}

func newAggregator(ctx ResponseContext) *aggregator {
	return &aggregator{
		ctx:         ctx,
		indexState:  codebuddy.NewToolCallIndexState(),
		toolCallMap: map[int]map[string]any{},
	}
}

func (a *aggregator) Process(event codebuddy.ResponseEvent) {
	if fp, ok := event.ChunkData["system_fingerprint"]; ok && fp != nil {
		a.systemFp = fp
	}
	if u := event.Usage(); u != nil {
		a.usage = u
	}
	if !event.HasChoice() {
		return
	}
	if rc, ok := event.ReasoningContent().(string); ok && rc != "" {
		a.reasoning.WriteString(rc)
	}
	if c, ok := event.Content().(string); ok && c != "" {
		a.content.WriteString(c)
	}
	for _, tc := range event.ToolCalls() {
		tcm, ok := tc.(map[string]any)
		if !ok {
			continue
		}
		fn, ok := tcm["function"].(map[string]any)
		if !ok {
			continue
		}
		idx := a.indexState.Resolve(tcm)
		if idx == nil {
			continue
		}
		toolID, _ := tcm["id"].(string)
		cur, exists := a.toolCallMap[*idx]
		if !exists {
			typ, _ := tcm["type"].(string)
			if typ == "" {
				typ = "function"
			}
			cur = map[string]any{
				"id":       toolID,
				"type":     typ,
				"function": map[string]any{"name": "", "arguments": ""},
			}
			a.toolCallMap[*idx] = cur
		} else if toolID != "" {
			cur["id"] = toolID
		}
		if t, ok := tcm["type"].(string); ok && t != "" {
			cur["type"] = t
		}
		fnMap := cur["function"].(map[string]any)
		if name, ok := fn["name"].(string); ok && name != "" {
			fnMap["name"] = name
		}
		if args, ok := fn["arguments"].(string); ok && args != "" {
			fnMap["arguments"] = fnMap["arguments"].(string) + args
		}
	}
	if event.FinishReason != nil {
		a.finishReason = event.FinishReason
	}
}

func (a *aggregator) Finalize() map[string]any {
	toolCalls := make([]any, 0)
	for i := 0; ; i++ {
		tc, ok := a.toolCallMap[i]
		if !ok {
			break
		}
		toolCalls = append(toolCalls, tc)
	}
	// 容忍 index 空洞：按序收集剩余
	if len(a.toolCallMap) != len(toolCalls) {
		toolCalls = toolCalls[:0]
		for i := 0; i < maxToolIndex(a.toolCallMap)+1; i++ {
			if tc, ok := a.toolCallMap[i]; ok {
				toolCalls = append(toolCalls, tc)
			}
		}
	}

	finalMessage := map[string]any{"role": "assistant", "content": a.content.String()}
	if a.reasoning.Len() > 0 {
		finalMessage["reasoning_content"] = a.reasoning.String()
	}
	if len(toolCalls) > 0 {
		finalMessage["tool_calls"] = toolCalls
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if a.finishReason != nil {
		finishReason = *a.finishReason
	}

	resp := map[string]any{
		"id":     a.ctx.ResponseID,
		"object": "chat.completion",
		"created": a.ctx.Created,
		"model":  a.ctx.Model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       finalMessage,
			"finish_reason": finishReason,
			"logprobs":      nil,
		}},
	}
	if a.usage != nil {
		resp["usage"] = a.usage
	}
	if a.systemFp != nil {
		resp["system_fingerprint"] = a.systemFp
	}
	return resp
}

func maxToolIndex(m map[int]map[string]any) int {
	max := -1
	for k := range m {
		if k > max {
			max = k
		}
	}
	return max
}

var _ = fmt.Sprintf
var _ = unicode.IsLetter
