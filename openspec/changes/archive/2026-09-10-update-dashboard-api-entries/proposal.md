## Why

管理台总览页当前展示的「OpenAI 客户端入口」只有相对路径 `/codebuddy/openai/v1`，无法直接复制给客户端使用；第二张「Anthropic 客户端入口」的值恒为空，且 Anthropic 协议尚未实现，属于二期遗留的过时占位。网关实际对外提供两个 OpenAI 兼容入口（CodeBuddy 与 TRAE），总览页应准确、完整地展示它们。

## What Changes

- 总览页两张入口卡片改为 **CodeBuddy API 地址** 与 **TRAE API 地址**，分别对应 `/codebuddy/openai/v1` 与 `/trae/openai/v1`。
- 入口地址显示为**完整绝对地址**：以浏览器当前地址的 origin 为前缀（`window.location.origin`）拼接路径，例如 `http://127.0.0.1:8000/codebuddy/openai/v1`。
- 删除「Anthropic 客户端入口」卡片及其 Claude Code 模型 ID 提示。
- **BREAKING**（仅内部前端消费）：`GET /api/admin/status` 响应中不再返回 `api_base_url` 与 `anthropic_api_base_url` 字段，`AdminStatus` 类型同步移除。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `admin-web-frontend`: 总览页入口卡片从过时的 OpenAI/Anthropic 相对路径改为 CodeBuddy/TRAE 完整地址；`/api/admin/status` 契约移除两个不再使用的 URL 字段。

## Impact

- `web/src/views/DashboardView.vue`：入口卡片模板、复制函数、Anthropic 卡片与提示删除。
- `web/src/types/admin.ts`：`AdminStatus` 移除 `api_base_url`、`anthropic_api_base_url`。
- `internal/handler/admin_stub_handler.go`：`Status` 响应移除两个字段。
