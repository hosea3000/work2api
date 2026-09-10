# add-api-playground

## Why

管理台"API 测试"（Playground）模块的前端 `ApiConsoleView.vue` 调用 `/api/admin/playground/openai/v1/*`（模型列表 + 聊天），但后端**从未注册过 playground 路由**（`grep playground internal/` 零命中）。因此模型获取、聊天发送、Anthropic 全部 404——整个模块不可用，不是 provider 拆分导致的回归。

同时，凭证已拆为 codebuddy / trae 两套独立端点，Playground 需要能选择要测试的 provider。

## What Changes

- **后端**：在已有 `SessionAuth` 的 `/api/admin` 组下新增 playground 路由，**直接复用现有聊天/模型 handler**（handler 自身不校验 key，鉴权在中间件）：
  - `GET /api/admin/playground/codebuddy/openai/v1/models` → `OpenAIHandler.Models`
  - `POST /api/admin/playground/codebuddy/openai/v1/chat/completions` → `OpenAIHandler.ChatCompletions`
  - `GET /api/admin/playground/trae/openai/v1/models` → `TraeChatHandler.Models`
  - `POST /api/admin/playground/trae/openai/v1/chat/completions` → `TraeChatHandler.ChatCompletions`
- **前端**：Playground 加 **Provider 选择（CodeBuddy / TRAE）**，模型列表与聊天按所选 provider 走对应端点；**移除 Anthropic 协议开关**（后端从未实现 Anthropic 端点）
- **路径**：统一为 `/api/admin/playground/{provider}/openai/v1/...`（与真实端点对称，不做旧路径兼容）

## Capabilities

### Modified

- `admin-web-frontend`: 新增 API 测试（Playground）能力——session 鉴权、provider 可选、复用上游执行器；API 契约补 playground 端点

## Impact

- **后端**：`internal/router/gateway.go` 加 4 条 playground 路由（复用 `OpenAIHandler`/`TraeChatHandler`），无新 handler/service
- **前端**：`web/src/api/admin.ts`（playground API 加 provider 参数、移除 anthropicPlaygroundApi）、`web/src/utils/adminQueryKeys.ts`（playgroundModels 加 provider）、`web/src/views/ApiConsoleView.vue`（加 provider 选择、移除 Anthropic 分支）、`web/src/types`（清理 Anthropic 相关未用类型，视需要）
- **DB**：无
- **非目标**：Anthropic 协议兼容、Playground 用量统计归类（stats 仍为打桩）
