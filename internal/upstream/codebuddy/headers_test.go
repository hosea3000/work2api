package codebuddy

import (
	"strings"
	"testing"
)

func TestGenerateHeadersStandard(t *testing.T) {
	cred := CredentialSnapshot{BearerToken: "tok", UserID: "u1"}
	headers, err := GenerateHeaders(cred, ConversationIDs{}, "2.107.0")
	if err != nil {
		t.Fatal(err)
	}
	required := []string{
		"Authorization", "X-User-Id", "X-Domain", "X-Product", "X-CodeBuddy-Request",
		"X-Agent-Intent", "X-Agent-Purpose", "X-IDE-Type", "X-IDE-Name", "X-IDE-Version",
		"x-stainless-lang", "x-stainless-runtime", "x-stainless-runtime-version",
		"X-Conversation-ID", "X-Conversation-Request-ID", "X-Conversation-Message-ID", "X-Request-ID",
	}
	for _, k := range required {
		if headers[k] == "" {
			t.Errorf("missing header %s", k)
		}
	}
	if headers["X-Product"] != "SaaS" || headers["X-Agent-Intent"] != "craft" ||
		headers["X-IDE-Type"] != "CLI" || headers["x-stainless-lang"] != "js" ||
		headers["x-stainless-runtime"] != "node" {
		t.Errorf("constant header mismatch: %v", headers)
	}
	if len(headers["X-Conversation-Request-ID"]) != 32 {
		t.Errorf("conversation request id must be hex16 (32 chars), got %d", len(headers["X-Conversation-Request-ID"]))
	}
	if strings.Contains(headers["X-Conversation-ID"], "-") {
		// uuid 格式正确
	} else if len(headers["X-Conversation-ID"]) != 32 {
		t.Errorf("conversation id must be uuid-like")
	}
}

func TestGenerateHeadersEnterprise(t *testing.T) {
	cred := CredentialSnapshot{BearerToken: "tok", UserID: "u1", EnterpriseID: "e1", DepartmentFullName: "a/b c"}
	headers, err := GenerateHeaders(cred, ConversationIDs{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-Enterprise-Id"] != "e1" || headers["X-Tenant-Id"] != "e1" {
		t.Errorf("enterprise headers missing: %v", headers)
	}
	if headers["X-Department-Info"] != "a%2Fb%20c" {
		t.Errorf("department info not urlencoded: %q", headers["X-Department-Info"])
	}
}

func TestGenerateHeadersAccountUIDPreferred(t *testing.T) {
	headers, _ := GenerateHeaders(CredentialSnapshot{BearerToken: "t", UserID: "u1", AccountUID: "acc"}, ConversationIDs{}, "")
	if headers["X-User-Id"] != "acc" {
		t.Errorf("account_uid should take precedence, got %s", headers["X-User-Id"])
	}
}

func TestGenerateHeadersMissing(t *testing.T) {
	if _, err := GenerateHeaders(CredentialSnapshot{BearerToken: "t"}, ConversationIDs{}, ""); err == nil {
		t.Error("expected error when user_id missing")
	}
	if _, err := GenerateHeaders(CredentialSnapshot{UserID: "u"}, ConversationIDs{}, ""); err == nil {
		t.Error("expected error when bearer_token missing")
	}
}

func TestIDEConfigHeaders(t *testing.T) {
	headers, err := GenerateIDEConfigHeaders(CredentialSnapshot{BearerToken: "t", UserID: "u"}, "2.107.0")
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-IDE-Type"] != "CodeBuddyIDE" || headers["X-Product-Version"] != "2.107.0" {
		t.Errorf("IDE variant headers wrong: %v", headers)
	}
}

func TestParseSSELine(t *testing.T) {
	ev, err := ParseSSELine("data: {\"a\":1}")
	if err != nil || ev == nil {
		t.Fatalf("expected event, got %v %v", ev, err)
	}
	done, _ := ParseSSELine("data: [DONE]")
	if _, ok := done.(SSEDone); !ok {
		t.Error("expected SSEDone")
	}
	comment, _ := ParseSSELine(": comment")
	if comment != nil {
		t.Error("comment should be nil")
	}
	empty, _ := ParseSSELine("")
	if empty != nil {
		t.Error("empty should be nil")
	}
	if _, err := ParseSSELine("data: not-json"); err == nil {
		t.Error("invalid json should error")
	}
}

func TestParseAllLinesFlush(t *testing.T) {
	events, err := parseAllLines("data: {\"x\":1}\n\ndata: [DONE]\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if _, ok := events[1].(SSEDone); !ok {
		t.Error("second event should be DONE")
	}
}

func TestNormalizeChunkEnvelope(t *testing.T) {
	got := NormalizeChunkEnvelope(map[string]any{"choices": []any{}}, "chatcmpl-abc", 123, "glm-5.2")
	if got["id"] != "chatcmpl-abc" || got["object"] != "chat.completion.chunk" || got["created"] != int64(123) || got["model"] != "glm-5.2" {
		t.Errorf("envelope wrong: %v", got)
	}
}

func TestStreamNormalizerRoleAndEmpty(t *testing.T) {
	n := NewStreamNormalizer()
	chunk := map[string]any{
		"choices": []any{map[string]any{
			"delta":         map[string]any{"role": "assistant", "content": "hi"},
			"finish_reason": nil,
		}},
	}
	out := n.Normalize(chunk)
	if len(out) != 2 {
		t.Fatalf("expected role chunk + content chunk, got %d", len(out))
	}
	first := out[0]["choices"].([]any)[0].(map[string]any)
	if first["delta"].(map[string]any)["role"] != "assistant" {
		t.Error("first chunk should carry role")
	}

	// empty delta without finish/usage → no output
	empty := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": nil}}}
	if out := n.Normalize(empty); len(out) != 0 {
		t.Errorf("empty delta should produce no chunks, got %d", len(out))
	}
}

func TestToolCallIndexResolution(t *testing.T) {
	state := NewToolCallIndexState()
	// explicit upstream index
	ev := ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{map[string]any{"index": float64(2), "id": "call_a", "function": map[string]any{"name": "f", "arguments": ""}}},
	}}}})
	out := AddOpenAIToolCallIndexes(ev, state)
	tc := out["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if tc["index"] != 2 {
		t.Errorf("expected index 2, got %v", tc["index"])
	}
	// same id without index → reuse
	ev2 := ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{map[string]any{"id": "call_a", "function": map[string]any{"arguments": "{}"}}},
	}}}})
	out2 := AddOpenAIToolCallIndexes(ev2, state)
	tc2 := out2["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if tc2["index"] != 2 {
		t.Errorf("expected reused index 2, got %v", tc2["index"])
	}
	// new id without index → next free
	ev3 := ParseEvent(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
		"tool_calls": []any{map[string]any{"id": "call_b", "function": map[string]any{"name": "g"}}},
	}}}})
	_ = AddOpenAIToolCallIndexes(ev3, state)
	if state.CurrentIndex == nil || *state.CurrentIndex != 0 {
		t.Errorf("expected generated index 0, got %v", state.CurrentIndex)
	}
}

func TestMapUpstreamStatus(t *testing.T) {
	if e := MapUpstreamStatus(401, "x"); !e.CredInvalid || e.StatusCode != 401 {
		t.Errorf("401 mapping wrong: %+v", e)
	}
	if e := MapUpstreamStatus(403, "x"); !e.CredInvalid {
		t.Errorf("403 should mark credential invalid")
	}
	if e := MapUpstreamStatus(429, "x"); e.StatusCode != 429 || e.ErrType != ErrCategoryRateLimit {
		t.Errorf("429 mapping wrong: %+v", e)
	}
	if e := MapUpstreamStatus(502, "x"); e.StatusCode != 502 || e.ErrType != ErrCategoryUpstream5xx {
		t.Errorf("5xx mapping wrong: %+v", e)
	}
	if e := MapUpstreamStatus(200, "x"); e.StatusCode != 502 || e.ErrType != ErrCategoryProtocol {
		t.Errorf("unexpected status mapping wrong: %+v", e)
	}
}

func TestParseUpstreamErrorBodyBusiness(t *testing.T) {
	msg, _, code := ParseUpstreamErrorBody(`{"code":12005,"msg":"err"}`)
	if code != 12005 || msg != "企业许可证没有可用席位" {
		t.Errorf("business mapping wrong: %q %v", msg, code)
	}
	msg2, _, _ := ParseUpstreamErrorBody(`{"error":{"message":"bad","type":"invalid_request_error"}}`)
	if msg2 != "bad" {
		t.Errorf("openai-style error parse wrong: %q", msg2)
	}
	msg3, _, _ := ParseUpstreamErrorBody("plain text")
	if msg3 != "plain text" {
		t.Errorf("fallback wrong: %q", msg3)
	}
}

func TestExtractModelIDs(t *testing.T) {
	got := ExtractModelIDs([]any{"glm-5.2", map[string]any{"id": "glm-5.3"}, map[string]any{"model": "other"}, "glm-5.2", map[string]any{}})
	want := []string{"glm-5.2", "glm-5.3", "other"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
