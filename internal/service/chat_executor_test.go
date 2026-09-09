package service

import (
	"github.com/yourname/work2api/internal/upstream/codebuddy"
	"strings"
	"testing"
)

func policies() RequestPolicies {
	ft := 1.0
	return RequestPolicies{
		ForcedTemperature:     &ft,
		StripModelNamespace:   true,
		ForcedReasoningModels: []string{"deepseek-v4-pro", "glm-5.1", "glm-5.2"},
		DefaultModel:          "glm-5.2",
	}
}

func TestValidateChatRequestStopRejected(t *testing.T) {
	err := ValidateChatRequest(map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"stop":     []any{"END"},
	})
	if err == nil || err.Status != 400 || !strings.Contains(err.Message, "stop") {
		t.Errorf("non-empty stop must 400, got %+v", err)
	}
	// 空 stop 数组通过
	if err := ValidateChatRequest(map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"stop":     []any{},
	}); err != nil {
		t.Errorf("empty stop should pass: %v", err)
	}
}

func TestValidateChatRequestMessages(t *testing.T) {
	if err := ValidateChatRequest(map[string]any{}); err == nil {
		t.Error("missing messages must fail")
	}
	if err := ValidateChatRequest(map[string]any{
		"messages": []any{map[string]any{"role": "assistant", "tool_calls": []any{}}},
	}); err == nil {
		t.Error("assistant without content and empty tool_calls must fail")
	}
	if err := ValidateChatRequest(map[string]any{
		"messages": []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "x"}}}},
	}); err != nil {
		t.Errorf("assistant with tool_calls should pass: %v", err)
	}
}

func TestPrepareChatPayloadPolicies(t *testing.T) {
	body := map[string]any{
		"model":       "anthropic/glm-5.2", // namespace stripping
		"messages":    []any{map[string]any{"role": "user", "content": "hi"}},
		"temperature": 0.5,
	}
	payload, responseModel := PrepareChatPayload(body, policies())
	if responseModel != "anthropic/glm-5.2" {
		t.Errorf("response model should keep client value, got %q", responseModel)
	}
	if payload["model"] != "glm-5.2" {
		t.Errorf("namespace not stripped: %v", payload["model"])
	}
	if payload["temperature"] != float64(1) {
		t.Errorf("temperature not forced: %v", payload["temperature"])
	}
	if payload["stream"] != true {
		t.Error("upstream payload must be stream=true")
	}
	opts, _ := payload["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Error("include_usage required")
	}
	// 推理模型强制
	th, ok := payload["thinking"].(map[string]any)
	if !ok || th["type"] != "enabled" || payload["reasoning_effort"] != "max" {
		t.Errorf("forced reasoning missing: %v", payload)
	}
	// 单条 user 消息补 system
	msgs := payload["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("system message not prepended: %v", msgs)
	}
}

func TestPrepareChatPayloadNonReasoning(t *testing.T) {
	ft := 1.0
	p := policies()
	p.ForcedTemperature = &ft
	p.ForcedReasoningModels = nil
	body := map[string]any{
		"model":    "other-model",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	payload, _ := PrepareChatPayload(body, p)
	if payload["enable_thinking"] != true {
		t.Error("default thinking should be enabled")
	}
	if _, has := payload["reasoning_effort"]; has {
		t.Error("non-reasoning model should not set reasoning_effort")
	}
}

func TestPrepareChatPayloadThinkingDisabled(t *testing.T) {
	ft := 1.0
	p := RequestPolicies{ForcedTemperature: &ft}
	body := map[string]any{
		"messages":        []any{map[string]any{"role": "user", "content": "hi"}},
		"enable_thinking": false,
	}
	payload, _ := PrepareChatPayload(body, p)
	if _, has := payload["enable_thinking"]; has {
		t.Error("explicitly disabled thinking must not be re-enabled")
	}
}

func TestSystemPromptRewrite(t *testing.T) {
	payload := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "You are Claude Code, Anthropic's official CLI for Claude.\nMain branch (you will usually use this for PRs): main"},
		map[string]any{"role": "user", "content": "hi"},
	}}
	RewriteSystemPromptMessages(payload)
	msgs := payload["messages"].([]any)
	sys := msgs[0].(map[string]any)["content"].(string)
	if strings.Contains(sys, "Claude Code") || strings.Contains(sys, "you will usually use this for PRs") {
		t.Errorf("fingerprint not rewritten: %q", sys)
	}
	if !strings.Contains(sys, "Main branch: main") {
		t.Errorf("main branch text wrong: %q", sys)
	}
}

func TestSystemPromptAttributionRemoval(t *testing.T) {
	payload := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "x-anthropic-billing-header: xxx"},
		map[string]any{"role": "user", "content": "hi"},
	}}
	RewriteSystemPromptMessages(payload)
	msgs := payload["messages"].([]any)
	if len(msgs) != 1 {
		t.Errorf("attribution-only system message should be removed, got %d", len(msgs))
	}
}

func TestOrderedUnion(t *testing.T) {
	got := orderedUnion([]string{"glm-5.2", "b"}, []string{"b", "c"})
	if len(got) != 3 || got[0] != "glm-5.2" || got[1] != "b" || got[2] != "c" {
		t.Errorf("union wrong: %v", got)
	}
}

func TestAggregatorFinalize(t *testing.T) {
	agg := newAggregator(ResponseContext{ResponseID: "id1", Created: 1, Model: "m"})
	agg.Process(codebuddy.ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "he"}}}}))
	agg.Process(codebuddy.ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"reasoning_content": "think"}}}}))
	agg.Process(codebuddy.ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{
			map[string]any{"index": float64(0), "id": "t1", "type": "function", "function": map[string]any{"name": "f", "arguments": "{\"a\":"}},
			map[string]any{"index": float64(0), "function": map[string]any{"arguments": "1}"}},
		},
	}}}}))
	agg.Process(codebuddy.ParseEvent(map[string]any{"usage": map[string]any{"total_tokens": 5}, "choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "stop"}}}))
	out := agg.Finalize()
	if out["object"] != "chat.completion" || out["id"] != "id1" {
		t.Errorf("envelope wrong: %v", out)
	}
	msg := out["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "he" || msg["reasoning_content"] != "think" {
		t.Errorf("message content wrong: %v", msg)
	}
	tcs := msg["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("expected 1 tool call, got %v", tcs)
	}
	tc := tcs[0].(map[string]any)
	if tc["id"] != "t1" || tc["function"].(map[string]any)["arguments"] != `{"a":1}` {
		t.Errorf("tool call aggregation wrong: %v", tc)
	}
	choice := out["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "stop" {
		t.Errorf("finish reason wrong: %v", choice["finish_reason"])
	}
	if out["usage"].(map[string]any)["total_tokens"] != 5 {
		t.Errorf("usage missing: %v", out["usage"])
	}
}

func TestAggregatorToolCallsFinishReason(t *testing.T) {
	agg := newAggregator(ResponseContext{ResponseID: "i", Created: 1, Model: "m"})
	agg.Process(codebuddy.ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{map[string]any{"index": float64(0), "id": "t", "type": "function", "function": map[string]any{"name": "f"}}},
	}}}}))
	out := agg.Finalize()
	choice := out["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason should default tool_calls, got %v", choice["finish_reason"])
	}
}
