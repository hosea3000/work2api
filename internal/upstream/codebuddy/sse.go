package codebuddy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// SSEDone 表示上游流结束标记（data: [DONE]）。
type SSEDone struct{}

// SSEDataError 上游 SSE data 字段不是可解析 JSON。
type SSEDataError struct{ Message string }

func (e *SSEDataError) Error() string { return e.Message }

// ParseSSELine 解析单行 SSE，返回 JSON 对象(map)、SSEDone{} 或 nil。
// 语义对齐 sse.py parse_sse_event。
func ParseSSELine(line string) (any, error) {
	stripped := strings.TrimSpace(line)
	if stripped == "" || strings.HasPrefix(stripped, ":") {
		return nil, nil
	}
	if stripped != "data:" && !strings.HasPrefix(stripped, "data: ") {
		return nil, nil
	}
	data := strings.TrimSpace(stripped[5:])
	if data == "" {
		return nil, nil
	}
	if data == "[DONE]" {
		return SSEDone{}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return nil, &SSEDataError{Message: "upstream SSE data contains invalid JSON"}
	}
	return v, nil
}

// IterSSEEvents 从任意文本流统一解析 SSE 事件，行为对齐 sse.py iter_sse_events：
// 按行分割、容忍分块边界，流结束后 flush 残余 buffer。
func IterSSEEvents(r io.Reader) ([]any, error) {
	// 一次性读取（上游为 SSE 文本流）；调用方需要真正流式时使用 IterSSEEventsScanner。
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return parseAllLines(string(data))
}

// IterSSEEventsScanner 逐块流式解析，onEvent 返回 false 时提前终止。
func IterSSEEventsScanner(r io.Reader, onEvent func(ev any) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var pending []string
	flush := func(line string) (bool, error) {
		ev, err := ParseSSELine(line)
		if err != nil {
			return false, err
		}
		if ev == nil {
			return true, nil
		}
		return onEvent(ev), nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(pending) == 0 {
			continue
		}
		_ = pending
		ok, err := flush(line)
		if err != nil || !ok {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// flush 残余（无换行结尾）
	if scanner.Text() != "" {
		_, err := flush(scanner.Text())
		return err
	}
	return nil
}

func parseAllLines(s string) ([]any, error) {
	var events []any
	if s == "" {
		return events, nil
	}
	parts := strings.Split(s, "\n")
	for _, line := range parts {
		ev, err := ParseSSELine(line)
		if err != nil {
			return events, err
		}
		if ev != nil {
			events = append(events, ev)
		}
	}
	return events, nil
}

// FormatSSEEvent 格式化 data-only SSE 事件（OpenAI 客户端依赖空行边界）。
func FormatSSEEvent(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return "data: " + string(b) + "\n\n"
}

// FormatSSEDone 结束标记。
func FormatSSEDone() string { return "data: [DONE]\n\n" }

// FormatSSEError 流中错误事件格式。
func FormatSSEError(message, errorType string) string {
	return FormatSSEEvent(map[string]any{
		"error": map[string]any{"message": message, "type": errorType},
	})
}

var _ = fmt.Sprintf
