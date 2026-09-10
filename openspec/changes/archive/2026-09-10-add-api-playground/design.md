# add-api-playground 设计

## Context

`ApiConsoleView.vue` 从参考实现 codebuddy2api 移植，调用 `/api/admin/playground/*`，但 work2api 后端从未实现这些路由，导致模块全 404。凭证拆分后，Playground 还需能选 provider。

关键事实：`OpenAIHandler.ChatCompletions/Models` 与 `TraeChatHandler.*` **不自行校验鉴权**——key 校验在 `APIKeyAuth` 中间件。因此同一批 handler 可挂到 `SessionAuth` 组下复用，无需新 handler/service。

## Goals / Non-Goals

- Goals: 恢复 API 测试模块（模型列表 + 聊天）；支持 provider 选择；会话鉴权，无需 sk- key
- Non-Goals: Anthropic 协议兼容；Playground 用量统计归类

## Decisions

### D1: 复用现有 handler，只加路由

在 `/api/admin`（已 `SessionAuth`）组下新增：
```
GET  /api/admin/playground/codebuddy/openai/v1/models           → OpenAIHandler.Models
POST /api/admin/playground/codebuddy/openai/v1/chat/completions → OpenAIHandler.ChatCompletions
GET  /api/admin/playground/trae/openai/v1/models                → TraeChatHandler.Models
POST /api/admin/playground/trae/openai/v1/chat/completions      → TraeChatHandler.ChatCompletions
```
无新 handler、无新 service、无新业务逻辑。playground 与真实端点的唯一差异是鉴权中间件（SessionAuth vs APIKeyAuth）。

### D2: 路径带 provider，与真实端点对称

`/api/admin/playground/{provider}/openai/v1/...`。不做旧 `/api/admin/playground/openai/v1/...` 兼容（模块本就不可用）。

### D3: 砍掉 Anthropic

后端从未有 Anthropic 端点（`anthropic_api_base_url` 为空），前端开关只会继续 404。移除 `anthropicPlaygroundApi` 与 `ApiConsoleView` 的 Anthropic 分支。若将来要支持，另开 change 做 Anthropic ⇄ OpenAI 翻译。

### D4: 前端 provider 选择

`ApiConsoleView.vue` 把 `protocol: 'openai'|'anthropic'` 换成 `provider: 'codebuddy'|'trae'`：
- `openaiPlaygroundApi.models(provider, signal)` → `GET /api/admin/playground/{provider}/openai/v1/models`
- `openaiPlaygroundApi.chat(provider, body, signal)` → `POST /api/admin/playground/{provider}/openai/v1/chat/completions`
- `adminQueryKeys.playgroundModels(provider)`（去掉 protocol 维度）
- 切换 provider 时清空所选模型、重置输出（沿用现有 `watch`）

## Risks / Trade-offs

- [playground 与真实端点共用执行器，未来若执行器要按来源区分行为会耦合] → 目前无此需求；需要时再注入来源标记
- [会话身份可无限调用上游聊天，无速率限制] → 管理台本已是高权限面；接受
- [移除 Anthropic 后前端类型 `AnthropicMessageRequest` 可能变为未使用] → 实现时清理未用类型，避免 lint 报错

## Open Questions

- 无（决策已确认）
