package trae

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// 捕获的 SOLO llm_utils_chat SSE 样例（真实结构，token 无）。
const soloSSEFixture = "id:1\nevent:metadata\ndata:{\"model\":\"\",\"session_id\":\"897f0f3f-935a-4f42-a0fc-60f5140ccd02\",\"prompt_completion_id\":0}\n\n" +
	"id:2\nevent:timing_cost\ndata:{\"name\":\"llm_raw_chat_v2\",\"preprocess_timing\":71}\n\n" +
	"event:output\ndata:{\"response\":\"中国\",\"reasoning_content\":\"让我想想\",\"tool_calls\":null}\n\n" +
	"event:output\ndata:{\"response\":\"的首都是北京。\",\"reasoning_content\":\"\",\"tool_calls\":null}\n\n" +
	"event:extra_info\ndata:{\"reasoning_content\":\"让我想想\"}\n\n" +
	"event:token_usage\ndata:{\"prompt_tokens\":21,\"completion_tokens\":142,\"total_tokens\":163,\"reasoning_tokens\":135}\n\n" +
	"event:done\ndata:{\"finish_reason\":\"stop\"}\n\n"

func TestAggregateSOLO(t *testing.T) {
	resp, err := Aggregate(strings.NewReader(soloSSEFixture))
	if err != nil {
		t.Fatal(err)
	}
	if resp["object"] != "chat.completion" {
		t.Errorf("object=%v", resp["object"])
	}
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "中国的首都是北京。" {
		t.Errorf("content=%q", msg["content"])
	}
	if msg["reasoning_content"] != "让我想想" {
		t.Errorf("reasoning=%q", msg["reasoning_content"])
	}
	if choices[0].(map[string]any)["finish_reason"] != "stop" {
		t.Errorf("finish_reason=%v", choices[0].(map[string]any)["finish_reason"])
	}
	usage := resp["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 163 {
		t.Errorf("usage=%v", usage)
	}
}

func TestAggregateSOLOError(t *testing.T) {
	raw := "event:error\ndata:{\"code\":4001,\"message\":\"param is invalid\"}\n\n" +
		"event:done\ndata:{\"finish_reason\":\"stop\"}\n\n"
	_, err := Aggregate(strings.NewReader(raw))
	if err == nil {
		t.Fatal("want error from event:error")
	}
	se, ok := err.(*SOLOStreamError)
	if !ok || se.Code != 4001 {
		t.Fatalf("err=%#v", err)
	}
}

func TestParseSOLOLine(t *testing.T) {
	ev, err := ParseSOLOLine("output", `{"response":"hi","reasoning_content":"think","tool_calls":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Response != "hi" || ev.Reasoning != "think" {
		t.Errorf("ev=%+v", ev)
	}
	if string(ev.ToolCalls) != "null" {
		t.Errorf("tool_calls=%s", ev.ToolCalls)
	}
	ev, err = ParseSOLOLine("done", `{"finish_reason":"stop"}`)
	if err != nil || ev.FinishReason != "stop" {
		t.Errorf("done: %+v %v", ev, err)
	}
}

func TestAggregateSOLOToolCalls(t *testing.T) {
	raw := "event:output\ndata:{\"response\":\"\",\"reasoning_content\":\"\",\"tool_calls\":[{\"id\":\"call_a\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"city\\\":\\\"北京\\\"}\"},\"index\":0}]}\n\n" +
		"event:done\ndata:{\"finish_reason\":\"tool_calls\"}\n\n"
	resp, err := Aggregate(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	choice := resp["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason=%v", choice["finish_reason"])
	}
	msg := choice["message"].(map[string]any)
	calls, ok := msg["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls=%#v", msg["tool_calls"])
	}
	if calls[0]["id"] != "call_a" {
		t.Errorf("call id=%v", calls[0]["id"])
	}
	fn := calls[0]["function"].(map[string]any)
	if fn["name"] != "get_weather" || fn["arguments"] != `{"city":"北京"}` {
		t.Errorf("fn=%v", fn)
	}
}

func TestStreamConvertsToOpenAIChunks(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Stream(rec, strings.NewReader(soloSSEFixture)); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Errorf("missing chunk object: %q", body)
	}
	if !strings.Contains(body, `"content":"中国"`) {
		t.Errorf("missing content delta: %q", body)
	}
	if !strings.Contains(body, `"reasoning_content"`) {
		t.Errorf("missing reasoning delta: %q", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("missing [DONE]: %q", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("content-type=%q", ct)
	}
}

func TestStreamGuaranteesDone(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Stream(rec, strings.NewReader("event:output\ndata:{\"response\":\"x\",\"reasoning_content\":\"\",\"tool_calls\":null}\n\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Errorf("missing [DONE]: %q", rec.Body.String())
	}
}

func TestStreamErrorCallback(t *testing.T) {
	rec := httptest.NewRecorder()
	var got *SOLOStreamError
	raw := "event:error\ndata:{\"code\":1005,\"message\":\"plan limit\"}\n\n"
	if err := StreamWithError(rec, strings.NewReader(raw), func(e *SOLOStreamError) { got = e }); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Code != 1005 {
		t.Fatalf("onErr not called: %#v", got)
	}
	if !strings.Contains(rec.Body.String(), "event: error") {
		t.Errorf("missing error event: %q", rec.Body.String())
	}
}

func TestMergeToolCallSOLOFunctionCall(t *testing.T) {
	toolCalls := map[int]map[string]any{}
	var order []int
	m1 := json.RawMessage(`[{"index":0,"id":"call_x","type":"function","function_call":{"name":"get_weather","arguments":"{\"city\":\"北京\""}}]`)
	m2 := json.RawMessage(`[{"index":0,"id":"","type":"function","function_call":{"name":"","arguments":"}"}}]`)
	mergeToolCallJSON(toolCalls, &order, m1)
	mergeToolCallJSON(toolCalls, &order, m2)
	if len(order) != 1 || order[0] != 0 {
		t.Fatalf("order=%v", order)
	}
	m := toolCalls[0]
	if m["id"] != "call_x" || m["type"] != "function" {
		t.Fatalf("id/type: %#v", m)
	}
	fn := m["function"].(map[string]any)
	if fn["name"] != "get_weather" || fn["arguments"] != `{"city":"北京"}` {
		t.Fatalf("fn=%#v", fn)
	}
}

func TestMergeToolCallStripsSOLOFields(t *testing.T) {
	toolCalls := map[int]map[string]any{}
	var order []int
	m := json.RawMessage(`[{"index":0,"id":"call_y","type":"function","function_call":{"name":"skill_view","arguments":"{\"name\":\"x\"}","namespace":"trae","partial_arguments":null}}]`)
	mergeToolCallJSON(toolCalls, &order, m)
	fn := toolCalls[0]["function"].(map[string]any)
	if _, has := fn["namespace"]; has {
		t.Error("namespace should be stripped")
	}
	if _, has := fn["partial_arguments"]; has {
		t.Error("partial_arguments should be stripped")
	}
	if fn["name"] != "skill_view" {
		t.Errorf("name=%v", fn["name"])
	}
}
