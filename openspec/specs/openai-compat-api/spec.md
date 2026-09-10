# openai-compat-api Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
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

### Requirement: 非流式请求聚合上游流
由于上游仅提供流式响应，系统 MUST 对 `stream=false` 的请求消费完上游 SSE 流后聚合为完整响应返回，且 SHALL 在聚合中支持客户端主动断连时取消上游请求。

#### Scenario: 客户端断连取消上游
- **WHEN** 非流式请求进行中客户端断开连接
- **THEN** 系统取消上游 HTTP 请求，不再继续消费 SSE 流

### Requirement: 上游 SSE 事件到 OpenAI chunk 的转换
系统 SHALL 将上游 SSE 事件转换为 OpenAI chunk 格式，语义与参考实现（codebuddy2api 的 `codebuddy_events.py` + `openai_compat.py`）对齐，包括：`reasoning_content` 与正文 `content` 分离、tool_call 增量索引（按 index 聚合 id/name/arguments 分片）、finish_reason 透传、usage 事件转 OpenAI usage 字段。

#### Scenario: reasoning 内容分离
- **WHEN** 上游事件包含推理内容片段与正文内容片段
- **THEN** 对应 chunk 的 delta 中分别输出 `reasoning_content` 与 `content` 字段

#### Scenario: tool_call 增量聚合
- **WHEN** 上游流式输出多个 tool_call 分片（含 index、id、function.name、arguments 增量）
- **THEN** 输出 chunk 的 `delta.tool_calls` 按 index 正确对应，非流式聚合结果中 arguments 拼接为完整字符串

### Requirement: 停止序列与温度处理
系统 SHALL 对非空 `stop` / `stop_sequences` 请求返回 400（上游无法正确报告停止序列命中）。系统 SHALL 支持配置强制覆盖 `temperature`（默认覆盖为 1，空配置则保留客户端值）。

#### Scenario: 非空停止序列被拒绝
- **WHEN** 请求体携带非空 `stop` 数组
- **THEN** 系统返回 400，错误说明不支持停止序列

### Requirement: 模型列表端点
系统 SHALL 提供 `GET /codebuddy/openai/v1/models`，返回配置附加模型与上游 `/v3/config` 实际模型的有序并集（去重、保序），模型查询结果 SHALL 缓存（TTL 30 秒，可配置）。

#### Scenario: 返回合并模型列表
- **WHEN** 客户端携带有效 API Key 请求模型列表且上游可用
- **THEN** 响应包含配置模型（如 glm-5.2）与上游实际模型的并集，`object: "model"` 字段齐全

#### Scenario: 上游不可用时回退
- **WHEN** 上游模型查询失败但缓存存在
- **THEN** 返回缓存的模型列表；无缓存时返回配置模型列表

