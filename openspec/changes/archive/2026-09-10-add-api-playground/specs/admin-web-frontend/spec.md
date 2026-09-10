# admin-web-frontend 变更规范（增量）

## MODIFIED Requirements

### Requirement: API 契约兼容
后端 MUST 完整实现前端实际调用的端点：`/auth/login`、`/auth/session`、`/auth/logout`、`/api/admin/api-keys`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials/:id/select`、`/api/admin/codebuddy/credentials/:id/test`、`/api/admin/codebuddy/credentials/:id/daily-checkin`、`/api/admin/codebuddy/credentials/rotation/toggle`、`/api/admin/trae/credentials`（GET/DELETE）、`/api/admin/trae/credentials/:id/select`、`/api/admin/trae/credentials/:id/test`、`/api/admin/trae/credentials/:id/daily-checkin`、`/api/admin/trae/credentials/rotation/toggle`、`/api/admin/playground/codebuddy/openai/v1/models`、`/api/admin/playground/codebuddy/openai/v1/chat/completions`、`/api/admin/playground/trae/openai/v1/models`、`/api/admin/playground/trae/openai/v1/chat/completions`、`/api/admin/status`。响应 JSON 结构（字段名与类型）与前端类型定义保持一致。

#### Scenario: 前端凭证页可用
- **WHEN** 管理员登录后进入凭证管理页
- **THEN** 两个 provider 的列表加载、选择、签到、测试、删除、轮换开关全部正常工作

## ADDED Requirements

### Requirement: API 测试（Playground）
管理台 SHALL 提供 API 测试（Playground）模块，允许管理员以**会话身份**（无需 `sk-` API Key）对指定 provider 发起模型列表查询与聊天请求。模块 SHALL 提供 Provider 选择（CodeBuddy / TRAE）；模型列表与聊天 MUST 路由到所选 provider 的上游执行器，与真实端点 `/codebuddy/openai/v1`、`/trae/openai/v1` 复用同一执行器（`OpenAIHandler` / `TraeChatHandler`）。模块 SHALL 支持流式（SSE）与非流式响应。协议仅 OpenAI，MUST NOT 暴露 Anthropic 协议入口。

#### Scenario: 获取所选 provider 的模型
- **WHEN** 管理员在 Playground 选择 CodeBuddy 并加载模型
- **THEN** 返回 codebuddy 模型列表；切换到 TRAE 后返回 trae 模型列表

#### Scenario: 聊天走所选 provider
- **WHEN** 管理员在 Playground 选中某 provider 发送消息
- **THEN** 请求路由到该 provider 的凭证池与上游执行器，返回 OpenAI 格式响应（流式或聚合）

#### Scenario: 会话鉴权
- **WHEN** 无有效管理会话调用 playground 端点
- **THEN** 返回 401

#### Scenario: 无可用凭证
- **WHEN** 所选 provider 的凭证池为空时发送聊天
- **THEN** 返回 503，Playground 显示错误
