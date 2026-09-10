# openai-compat-api 变更规范（增量）

## MODIFIED Requirements

### Requirement: OpenAI 兼容聊天补全端点
系统 SHALL 提供 `POST /codebuddy/openai/v1/chat/completions`，接受 OpenAI Chat Completions 格式请求体（model、messages、stream、temperature 等），并返回 OpenAI 格式响应。端点 MUST 仅接受有效的 `sk-...` API Key（Bearer）。

#### Scenario: 非流式请求成功
- **WHEN** 客户端携带有效 API Key 请求 `stream=false`，且上游凭证可用
- **THEN** 系统调用上游 `/v2/chat/completions`，聚合全部 SSE 事件后返回单个 OpenAI Chat Completion JSON（choices、usage 字段齐全）

#### Scenario: 流式请求成功
- **WHEN** 客户端携带有效 API Key 请求 `stream=true`
- **THEN** 系统以 `text/event-stream` 返回 OpenAI chunk 流（`data: {...}` 行 + `data: [DONE]`），内容与上游事件实时对应

#### Scenario: 缺少或无效 API Key
- **WHEN** 请求缺少 Authorization 头、格式非 Bearer、或 API Key 不存在/已禁用
- **THEN** 系统返回 401，响应体为 OpenAI 错误格式 `{"error": {"message", "type": "authentication_error", ...}}`

#### Scenario: 无可用凭证
- **WHEN** 凭证池中没有任何可用凭证（全部失效或为空）
- **THEN** 系统返回 503，错误体说明无可用上游凭证

### Requirement: 模型列表端点
系统 SHALL 提供 `GET /codebuddy/openai/v1/models`，返回配置附加模型与上游 `/v3/config` 实际模型的有序并集（去重、保序），模型查询结果 SHALL 缓存（TTL 30 秒，可配置）。

#### Scenario: 返回合并模型列表
- **WHEN** 客户端携带有效 API Key 请求模型列表且上游可用
- **THEN** 响应包含配置模型（如 glm-5.2）与上游实际模型的并集，`object: "model"` 字段齐全

#### Scenario: 上游不可用时回退
- **WHEN** 上游模型查询失败但缓存存在
- **THEN** 返回缓存的模型列表；无缓存时返回配置模型列表
