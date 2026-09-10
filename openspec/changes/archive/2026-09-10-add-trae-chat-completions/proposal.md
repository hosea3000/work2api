# add-trae-chat-completions

## Why

TRAE 凭证目前只支持登录导入与保活（自动刷新 + 签到），无法参与聊天消费——`/codebuddy/openai/v1/chat/completions` 整条链路硬编码 codebuddy。参考实现 trae2api-web 已验证 SOLO 通道可反代（`llm_utils_chat` + `function:solo_work_lite`，OpenAI 兼容 + tool calling），且 payload 改写与 SSE 转换均已实测。

本变更新增**独立端点** `/trae/openai/v1/*`（同一套 `sk-` API Key），让客户端通过 `base_url=/trae/openai/v1` 即可用 TRAE SOLO 作为模型后端，codebuddy 业务逻辑零改动（仅路径前缀统一重命名）。

## What Changes

- **新增 TRAE OpenAI 兼容端点**：`POST /trae/openai/v1/chat/completions`、`GET /trae/openai/v1/models`（`APIKeyAuth` 中间件复用），与 `/codebuddy/openai/v1/*` 对称
- **统一端点前缀（破坏性）**：`/openai/v1/*` → `/codebuddy/openai/v1/*`；TRAE 用 `/trae/openai/v1/*`。旧 `/openai/v1` 路径不再保留，现有客户端需改 `base_url`
- **移植 TRAE 聊天上游适配**（`internal/upstream/trae`）：
  - `PrepareChatBody`：OpenAI → SOLO 请求改写（`function=solo_work_lite`、`config_name/model`、messages content 数组化、tools.parameters 字符串化、assistant tool_calls→function_call、tool_choice 归一化）
  - `Stream` / `Aggregate`：SOLO SSE（`output`/`token_usage`/`done`/`error`）→ OpenAI chunk / 聚合响应
  - `SOLOHeaders`：`Cloud-IDE-JWT` + `X-Uid`/`X-Machine-Id`/`X-Device-Id` + 版本头
  - `FetchModels`：`/api/ide/v1/get_detail_param` 模型表
- **新增 `TraeChatExecutor`**：镜像 `ChatExecutor` 骨架，SSE 层直接调 trae 适配器（不做 provider 抽象，codebuddy 路径不动）
- **新增 `TraeCredentialPool`**：独立内存池 + 独立 round-robin；带 `cooldownUntil` 冷却（1005 plan→12h、4011 限流→短冷却、4008 配额→当日、401→MarkExpired）
- **新增 `TraeModelsService`**：`/trae/openai/v1/models` 模型列表，TTL 缓存 + 兜底
- **reload 接线**：`credentialService.refreshPool` 同时刷新 codebuddy 与 trae 两个池；TRAE 刷新 job 落库后触发 reload
- **`stop` 透传**：TRAE 不沿用 codebuddy 的非空 `stop` 拒绝（对齐参考实现，标 `ponytail:` 待实测）

## Capabilities

### New

- `trae-openai-compat-api`: TRAE SOLO 的 OpenAI 兼容聊天补全与模型列表端点——请求改写、SSE 转换、独立凭证池与冷却、错误码映射

### Modified

- `credential-pool`: 新增 TRAE 独立轮换池 requirement（与 codebuddy 池隔离、独立 round-robin、冷却跳过、失效摘除）
- `openai-compat-api`: codebuddy 端点路径重命名为 `/codebuddy/openai/v1/*`
- `api-key-auth`: API Key 校验中间件覆盖的端点路径更新为 `/codebuddy/openai/v1/*` 与 `/trae/openai/v1/*`

## Impact

- **后端新增**：`internal/upstream/trae/chat.go`（payload 改写）、`internal/upstream/trae/sse.go`（Stream/Aggregate）、`internal/service/trae_chat_executor.go`、`internal/service/trae_credential_pool.go`、`internal/service/trae_models_service.go`、`internal/handler/trae_chat_handler.go`
- **后端修改**：`trae/client.go`（加 AgentHost/FetchModels）、`credential_service.go`（refreshPool 双池）、`trae_token_refresh.go`（reload）、`router/gateway.go`（`/trae/openai/v1` 路由）、wire
- **前端**：无（本变更仅 API；TRAE 行 select/rotation UI 仍隐藏）
- **DB**：零迁移（复用 credential 表；冷却状态仅内存）
- **非目标**：模型命名空间路由（方案 A）、TRAE 手动 select/rotation UI、跨 provider 自动兜底
