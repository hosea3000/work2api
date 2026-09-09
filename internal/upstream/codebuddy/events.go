package codebuddy

import (
	"encoding/json"
	"strings"
)

// ResponseEvent 协议中立的单 choice 上游事件语义（对齐 CodeBuddyResponseEvent）。
type ResponseEvent struct {
	ChunkData    map[string]any
	Choice       map[string]any
	Delta        map[string]any
	FinishReason *string
}

// ParseEvent 从 chunk JSON 对象解析共享响应语义。
func ParseEvent(chunkData map[string]any) ResponseEvent {
	ev := ResponseEvent{ChunkData: chunkData, Delta: map[string]any{}}
	if choices, ok := chunkData["choices"].([]any); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]any); ok {
			ev.Choice = c
			if d, ok := c["delta"].(map[string]any); ok {
				ev.Delta = d
			}
			if fr, ok := c["finish_reason"].(string); ok {
				t := strings.TrimSpace(fr)
				if t != "" {
					ev.FinishReason = &t
				}
			}
		}
	}
	return ev
}

func (e ResponseEvent) HasChoice() bool { return e.Choice != nil }

func (e ResponseEvent) ReasoningContent() any { return e.Delta["reasoning_content"] }

func (e ResponseEvent) Content() any { return e.Delta["content"] }

// ToolCalls 返回 delta.tool_calls 列表（非列表返回空）。
func (e ResponseEvent) ToolCalls() []any {
	tcs, ok := e.Delta["tool_calls"].([]any)
	if !ok {
		return nil
	}
	return tcs
}

func (e ResponseEvent) Usage() any { return e.ChunkData["usage"] }

// ToolCallIndexState 优先沿用上游 index，仅在缺失时补齐稳定位置（对齐 ToolCallIndexState）。
type ToolCallIndexState struct {
	IDToIndex    map[string]int
	UsedIndexes  map[int]bool
	CurrentIndex *int
}

func NewToolCallIndexState() *ToolCallIndexState {
	return &ToolCallIndexState{IDToIndex: map[string]int{}, UsedIndexes: map[int]bool{}}
}

// Resolve 返回该 tool_call 应使用的 index。
func (s *ToolCallIndexState) Resolve(toolCall map[string]any) *int {
	toolID, _ := toolCall["id"].(string)
	upstreamIndex, isInt := toolCall["index"].(float64)
	// encoding/json 数字均为 float64；布尔不可能是数字，无需排除
	if isInt {
		idx := int(upstreamIndex)
		s.UsedIndexes[idx] = true
		if toolID != "" {
			s.IDToIndex[toolID] = idx
		}
		s.CurrentIndex = &idx
		return &idx
	}
	if toolID != "" {
		if existing, ok := s.IDToIndex[toolID]; ok {
			s.CurrentIndex = &existing
			return &existing
		}
		generated := 0
		for s.UsedIndexes[generated] {
			generated++
		}
		s.IDToIndex[toolID] = generated
		s.UsedIndexes[generated] = true
		s.CurrentIndex = &generated
	}
	return s.CurrentIndex
}

// AddOpenAIToolCallIndexes 为 chunk 中 tool_calls 补齐 index，返回转换后的 chunk（对齐 add_openai_tool_call_indexes）。
func AddOpenAIToolCallIndexes(ev ResponseEvent, state *ToolCallIndexState) map[string]any {
	tcs := ev.ToolCalls()
	if len(tcs) == 0 {
		return ev.ChunkData
	}
	converted := make([]any, 0, len(tcs))
	for _, tc := range tcs {
		m, ok := tc.(map[string]any)
		if !ok {
			converted = append(converted, tc)
			continue
		}
		cp := cloneMap(m)
		if idx := state.Resolve(cp); idx != nil {
			cp["index"] = *idx
		}
		converted = append(converted, cp)
	}
	delta := cloneMap(ev.Delta)
	delta["tool_calls"] = converted
	return copyFirstChoiceWithDelta(ev.ChunkData, delta)
}

// StreamNormalizer 规范化流式 delta 并补齐 assistant 角色（对齐 OpenAIStreamNormalizer）。
type StreamNormalizer struct {
	roleSent bool
}

func NewStreamNormalizer() *StreamNormalizer { return &StreamNormalizer{} }

// Normalize 可能返回 0~2 个输出 chunk。
func (n *StreamNormalizer) Normalize(chunkData map[string]any) []map[string]any {
	choices, ok := chunkData["choices"].([]any)
	if !ok || len(choices) == 0 {
		return []map[string]any{chunkData}
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return []map[string]any{chunkData}
	}
	rawDelta, ok := choice["delta"].(map[string]any)
	if !ok {
		return []map[string]any{chunkData}
	}
	delta := cloneMap(rawDelta)
	var finishReason *string
	if fr, ok := choice["finish_reason"].(string); ok && strings.TrimSpace(fr) != "" {
		t := strings.TrimSpace(fr)
		finishReason = &t
	}

	var out []map[string]any
	role, hasRole := delta["role"]
	if !n.roleSent && ((hasRole && (role == "assistant" || role == "")) || hasAssistantDelta(delta)) {
		out = append(out, copyFirstChoiceWithDeltaFinish(chunkData, map[string]any{"role": "assistant"}, nil))
		n.roleSent = true
	}
	removeEmptyDeltaFields(delta)

	if len(delta) == 0 {
		_, hasUsage := chunkData["usage"]
		if finishReason == nil && (!hasUsage || chunkData["usage"] == nil) {
			return out
		}
		out = append(out, copyFirstChoiceWithDeltaFinish(chunkData, map[string]any{}, finishReason))
		return out
	}
	out = append(out, copyFirstChoiceWithDeltaFinish(chunkData, delta, finishReason))
	return out
}

func hasAssistantDelta(delta map[string]any) bool {
	for _, key := range []string{"reasoning_content", "content", "tool_calls"} {
		if _, ok := delta[key]; ok {
			return true
		}
	}
	return false
}

func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		if len(t) == 0 {
			return true
		}
		for _, val := range t {
			switch tv := val.(type) {
			case nil, string:
				if tv == nil || tv == "" {
					continue
				}
				return false
			case []any:
				if len(tv) == 0 {
					continue
				}
				return false
			case map[string]any:
				if len(tv) == 0 {
					continue
				}
				return false
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

func removeEmptyDeltaFields(delta map[string]any) {
	if isEmptyValue(delta["reasoning_content"]) {
		delete(delta, "reasoning_content")
	}
	if isEmptyValue(delta["content"]) {
		delete(delta, "content")
	}
	if isEmptyValue(delta["tool_calls"]) {
		delete(delta, "tool_calls")
	}
	if fc, ok := delta["function_call"]; ok && isEmptyValue(fc) {
		delete(delta, "function_call")
	}
	if isEmptyValue(delta["refusal"]) {
		delete(delta, "refusal")
	}
	if isEmptyValue(delta["extra_fields"]) {
		delete(delta, "extra_fields")
	}
}

func copyFirstChoiceWithDelta(chunkData map[string]any, delta map[string]any) map[string]any {
	copied := cloneMap(chunkData)
	choices, _ := copied["choices"].([]any)
	if len(choices) > 0 {
		if choiceMap, ok := choices[0].(map[string]any); ok {
			nc := cloneMap(choiceMap)
			nc["delta"] = delta
			choices[0] = nc
			copied["choices"] = choices
		}
	}
	return copied
}

func copyFirstChoiceWithDeltaFinish(chunkData map[string]any, delta map[string]any, finishReason *string) map[string]any {
	copied := copyFirstChoiceWithDelta(chunkData, delta)
	choices, _ := copied["choices"].([]any)
	if len(choices) > 0 {
		if choiceMap, ok := choices[0].(map[string]any); ok {
			if finishReason != nil {
				choiceMap["finish_reason"] = *finishReason
			} else {
				choiceMap["finish_reason"] = nil
			}
		}
	}
	return copied
}

// NormalizeChunkEnvelope 统一覆盖流式 chunk 信封（对齐 normalize_openai_stream_chunk_envelope）。
func NormalizeChunkEnvelope(chunkData map[string]any, responseID string, created int64, model string) map[string]any {
	copied := cloneMap(chunkData)
	copied["id"] = responseID
	copied["object"] = "chat.completion.chunk"
	copied["created"] = created
	copied["model"] = model
	return copied
}

func cloneMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
