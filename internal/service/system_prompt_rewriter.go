package service

import (
	"strings"
)

// System prompt 指纹改写（对齐 system_prompt_rewriter.py）。

const claudeCodeIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
const neutralCLIIdentity = "You are an interactive software engineering assistant operating in a command-line environment."
const mainBranchFingerprint = "Main branch (you will usually use this for PRs):"
const attributionHeaderPrefix = "x-anthropic-billing-header:"

func rewriteText(text string) string {
	return strings.Replace(
		strings.Replace(text, claudeCodeIdentity, neutralCLIIdentity, 1),
		mainBranchFingerprint, "Main branch:", 1,
	)
}

// stripAttributionHeaderLine 移除 attribution 行，返回 (新文本, 是否移除)。
func stripAttributionHeaderLine(text string) (string, bool) {
	if !strings.HasPrefix(text, attributionHeaderPrefix) {
		return text, false
	}
	idx := strings.Index(text, "\n")
	if idx < 0 {
		return "", true
	}
	return text[idx+1:], true
}

// stripAttributionHeaderContent 处理字符串或 block 数组 content。
// 返回 (新 content, 消息整体是否应为空被删除)。
func stripAttributionHeaderContent(content any) (any, bool) {
	if s, ok := content.(string); ok {
		newText, stripped := stripAttributionHeaderLine(s)
		return newText, stripped && newText == ""
	}
	blocks, ok := content.([]any)
	if !ok {
		return content, false
	}
	strippedAny := false
	var retained []any
	for _, block := range blocks {
		bm, ok := block.(map[string]any)
		if !ok || bm["type"] != "text" {
			retained = append(retained, block)
			continue
		}
		text, ok := bm["text"].(string)
		if !ok {
			retained = append(retained, block)
			continue
		}
		newText, stripped := stripAttributionHeaderLine(text)
		if !stripped {
			retained = append(retained, block)
			continue
		}
		strippedAny = true
		if newText != "" {
			bm["text"] = newText
			retained = append(retained, bm)
		}
	}
	return retained, strippedAny && len(retained) == 0
}

func rewriteSystemPromptContent(content any) any {
	if s, ok := content.(string); ok {
		return rewriteText(s)
	}
	blocks, ok := content.([]any)
	if !ok {
		return content
	}
	for _, block := range blocks {
		bm, ok := block.(map[string]any)
		if !ok || bm["type"] != "text" {
			continue
		}
		if text, ok := bm["text"].(string); ok {
			bm["text"] = rewriteText(text)
		}
	}
	return content
}

// RewriteSystemPromptMessages 原位改写 messages 中的 system 消息并移除独立 attribution 块。
func RewriteSystemPromptMessages(payload map[string]any) {
	messages, ok := payload["messages"].([]any)
	if !ok {
		return
	}
	var retained []any
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok || msg["role"] != "system" {
			retained = append(retained, m)
			continue
		}
		newContent, removeMessage := stripAttributionHeaderContent(msg["content"])
		if removeMessage {
			continue
		}
		msg["content"] = rewriteSystemPromptContent(newContent)
		retained = append(retained, msg)
	}
	payload["messages"] = retained
}
