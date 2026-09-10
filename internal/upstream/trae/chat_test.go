package trae

import (
	"testing"
)

func TestPrepareChatBodyForcesStreamAndFunction(t *testing.T) {
	obj := map[string]any{
		"model":    "glm-5.2",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	out := PrepareChatBody(obj)
	if out["stream"] != true {
		t.Errorf("stream=%v", out["stream"])
	}
	if out["function"] != Function {
		t.Errorf("function=%v", out["function"])
	}
	if out["config_name"] != "glm-5.2" || out["model"] != "glm-5.2" {
		t.Errorf("model fields=%v / %v", out["config_name"], out["model"])
	}
	msgs := out["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "text" || content[0].(map[string]any)["text"] != "hi" {
		t.Errorf("content rewrite=%v", content)
	}
}

func TestPrepareChatBodyDefaultModel(t *testing.T) {
	obj := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}
	out := PrepareChatBody(obj)
	if out["config_name"] != DefaultConfigName || out["model"] != DefaultConfigName {
		t.Errorf("default model=%v / %v", out["config_name"], out["model"])
	}
}

func TestPrepareChatBodyKeepsArrayContent(t *testing.T) {
	obj := map[string]any{
		"model":    "glm-5.2",
		"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}}}},
	}
	out := PrepareChatBody(obj)
	content := out["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 {
		t.Errorf("array content should pass through: %v", content)
	}
}

func TestPrepareChatBodyToolChoiceFunctionObject(t *testing.T) {
	obj := map[string]any{
		"model":       "glm-5.2",
		"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}},
		"tools":       []any{map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}},
	}
	out := PrepareChatBody(obj)
	if out["tool_choice"] != "get_weather" {
		t.Errorf("tool_choice=%v", out["tool_choice"])
	}
	if _, ok := out["tools"]; !ok {
		t.Error("tools should be kept for function choice")
	}
}

func TestPrepareChatBodyToolChoiceNone(t *testing.T) {
	obj := map[string]any{
		"model":       "glm-5.2",
		"tool_choice": "none",
		"tools":       []any{map[string]any{}},
		"functions":   []any{map[string]any{}},
	}
	out := PrepareChatBody(obj)
	if _, ok := out["tool_choice"]; ok {
		t.Error("tool_choice should be deleted")
	}
	if _, ok := out["tools"]; ok {
		t.Error("tools should be deleted")
	}
	if _, ok := out["functions"]; ok {
		t.Error("functions should be deleted")
	}
}

func TestPrepareChatBodyToolsParametersStringified(t *testing.T) {
	obj := map[string]any{
		"model": "glm-5.2",
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name":       "get_weather",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
		}}},
	}
	out := PrepareChatBody(obj)
	tool := out["tools"].([]any)[0].(map[string]any)
	fn := tool["function"].(map[string]any)
	if _, ok := fn["parameters"].(string); !ok {
		t.Fatalf("parameters should be string, got %T", fn["parameters"])
	}
}

func TestPrepareChatBodyToolsInvalidEntriesDropped(t *testing.T) {
	obj := map[string]any{
		"model": "glm-5.2",
		"tools": []any{
			map[string]any{"type": "function", "function": map[string]any{"name": "ok", "parameters": map[string]any{"type": "object"}}},
			map[string]any{"bad": 1},
		},
	}
	out := PrepareChatBody(obj)
	tools := out["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("invalid entry should be dropped, got %d", len(tools))
	}
}

func TestPrepareChatBodyAssistantToolCallsToFunctionCall(t *testing.T) {
	obj := map[string]any{
		"model": "glm-5.2",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
				map[string]any{"id": "call_x", "type": "function", "function": map[string]any{"name": "skill_view", "arguments": `{"name":"x"}`}},
			}},
		},
	}
	out := PrepareChatBody(obj)
	assistant := out["messages"].([]any)[1].(map[string]any)
	tc := assistant["tool_calls"].([]any)[0].(map[string]any)
	if _, has := tc["function"]; has {
		t.Error("function should be converted to function_call")
	}
	fc, ok := tc["function_call"].(map[string]any)
	if !ok || fc["name"] != "skill_view" {
		t.Fatalf("function_call missing: %#v", tc)
	}
}

func TestPrepareChatBodyToolCallWithoutNameDropped(t *testing.T) {
	obj := map[string]any{
		"model": "glm-5.2",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "tool_calls": []any{
				map[string]any{"id": "call_bad", "type": "function", "function": map[string]any{"arguments": "{}"}},
			}},
		},
	}
	out := PrepareChatBody(obj)
	assistant := out["messages"].([]any)[1].(map[string]any)
	if _, has := assistant["tool_calls"]; has {
		t.Error("tool_call without name should be dropped")
	}
}
