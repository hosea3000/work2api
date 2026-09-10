# trae-openai-compat-api 变更规范（增量）

## ADDED Requirements

### Requirement: TRAE OpenAI 兼容聊天补全端点
系统 SHALL 提供 `POST /trae/openai/v1/chat/completions`，接受与 `/codebuddy/openai/v1/chat/completions` 相同的 OpenAI Chat Completions 请求体（model、messages、stream、tools、tool_choice、temperature 等），返回 OpenAI 格式响应。端点 MUST 通过 `APIKeyAuth` 校验 `sk-...` API Key（与 `/codebuddy/openai/v1/*` 同一套 key，不绑定 provider）。请求 MUST 路由到 TRAE 凭证池，绝不触碰 codebuddy 池；codebuddy 现有路径 MUST NOT 因本变更改变行为。

#### Scenario: 非流式请求成功
- **WHEN** 客户端携带有效 API Key 请求 `stream=false`，且 TRAE 池有可用凭证
- **THEN** 系统调用 SOLO `llm_utils_chat`，聚合全部 SSE 事件后返回单个 OpenAI Chat Completion JSON（choices、usage、model 回显客户端请求值）

#### Scenario: 流式请求成功
- **WHEN** 客户端携带有效 API Key 请求 `stream=true`
- **THEN** 系统以 `text/event-stream` 返回 OpenAI chunk 流（`data: {...}` + `data: [DONE]`），`reasoning_content` 与 `content` 分离，tool_call 增量按 index 聚合

#### Scenario: 无可用 TRAE 凭证
- **WHEN** TRAE 池为空（无 active 凭证或全部冷却/失效）
- **THEN** 返回 503，错误体说明无可用 TRAE 上游凭证

#### Scenario: 无效 API Key
- **WHEN** 请求缺少/无效 API Key
- **THEN** 返回 401（OpenAI 错误格式）

### Requirement: OpenAI 请求到 SOLO 的改写
系统 SHALL 把 OpenAI 请求改写为 SOLO `llm_utils_chat` 格式：`function` 固定为 `solo_work_lite`；`model` 同时写入 `config_name` 与 `model`（空则默认模型）；`stream` 强制 true（非流式由服务端聚合）；messages 的字符串 content 转 `[{"type":"text","text":...}]`（已是数组则透传）；`tools[].function.parameters` 对象序列化为 JSON 字符串；assistant 消息的 `tool_calls[].function` 转为上游 `function_call`（无 name 的剔除）；`tool_choice` 按上游标量语义归一化（none 删 tools；auto/required 转字符串；function 提取 name）。改写规则 MUST 与参考实现 trae2api-web `PrepareBody` 对齐。

#### Scenario: 字符串 content 数组化
- **WHEN** 请求 messages 含 `content` 字符串
- **THEN** 上游 payload 中该 content 变为 `[{"type":"text","text":"..."}]`

#### Scenario: tools 参数序列化
- **WHEN** 请求携带 `tools[].function.parameters` 对象
- **THEN** 上游 payload 中该字段为 JSON 字符串，且 tools 条目缺 function 的整体剔除

#### Scenario: tool_choice none 抑制
- **WHEN** 请求 `tool_choice` 为 "none" 或 `{"type":"none"}`
- **THEN** 上游 payload 删除 tool_choice 与 tools/functions

### Requirement: SOLO SSE 到 OpenAI 的转换
系统 SHALL 解析 SOLO SSE 事件并转换为 OpenAI 语义：`output` 事件的 `response` → `content` 增量、`reasoning_content` → `reasoning_content` 增量、`tool_calls` → 按 index 聚合的 `delta.tool_calls`（`function_call` 字段兼容、arguments 拼接、剔除 `namespace`/`partial_arguments` 上游专属字段）；`token_usage` → OpenAI usage；`done` 的 `finish_reason` 透传；`error` 事件作为流内错误处理。流式 MUST 每 chunk flush，且保证至少一个 `[DONE]` 或 error 事件收尾。

#### Scenario: reasoning 与正文分离
- **WHEN** 上游 output 事件同时含 response 与 reasoning_content
- **THEN** chunk 的 delta 分别输出 content 与 reasoning_content

#### Scenario: tool_call 增量聚合
- **WHEN** 上游流式输出多个 tool_call 分片
- **THEN** delta.tool_calls 按 index 对应，非流式聚合中 arguments 拼接为完整字符串，字段名为标准 `function`

#### Scenario: 流内 error 事件
- **WHEN** 上游发出 `event:error`
- **THEN** 系统按错误码分类（1005 plan 限流 / 其他客户端错误），冷却或摘除对应凭证，并向客户端注入 SSE error 事件

### Requirement: TRAE 凭证池与冷却
系统 SHALL 维护独立的 TRAE 内存轮换池：仅装载 `provider=trae` 且 `status=active` 的凭证，独立 round-robin（与 codebuddy 池互不影响）。凭证 SHALL 携带 `cooldownUntil` 冷却时间戳，选池时跳过冷却中的凭证。上游错误 SHALL 映射为冷却/失效：`1005` plan 权益不足 → 12h 冷却；`4011` 频率超限 → 短冷却（如 60s）；`4008` 配额耗尽 → 当日冷却；`401`/session 失效 → MarkExpired（写库 + 出池）。池内无可选凭证时聊天返回 503。

#### Scenario: 冷却凭证被跳过
- **WHEN** 某 TRAE 凭证因 1005 进入 12h 冷却，池内还有另一可用凭证
- **THEN** round-robin 选择跳过冷却凭证，命中可用凭证

#### Scenario: 全部冷却返回 503
- **WHEN** 池内所有凭证均在冷却中
- **THEN** 聊天请求返回 503 无可用凭证

#### Scenario: 401 摘除凭证
- **WHEN** 上游返回 401 session 失效
- **THEN** 凭证标记 expired 并立即从池移除，后续选择不再命中

#### Scenario: 凭证变更后池同步
- **WHEN** TRAE 凭证新增/删除/刷新（token 更新）
- **THEN** TRAE 池随之重载，选中凭证的快照为最新 token

### Requirement: TRAE 模型列表端点
系统 SHALL 提供 `GET /trae/openai/v1/models`（APIKeyAuth），返回 TRAE 上游 `get_detail_param` 的模型列表（`config_name` 作为 id），结果 SHALL 缓存（TTL 30 秒，可配置）；上游不可用时回退缓存，无缓存则返回空列表。模型条目 `owned_by` SHALL 为 `trae`。

#### Scenario: 返回 TRAE 模型
- **WHEN** 客户端携带有效 API Key 请求 `/trae/openai/v1/models` 且上游可用
- **THEN** 响应为 `object: "list"`，data 含上游 config_name 列表，`owned_by: "trae"`

#### Scenario: 上游不可用回退
- **WHEN** 模型查询失败但存在缓存
- **THEN** 返回缓存列表；无缓存返回空列表且不报 5xx

### Requirement: TRAE 停止序列处理
TRAE 端点 SHALL 透传客户端 `stop` / `stop_sequences` 至上游（不沿用 codebuddy 的非空拒绝）。若实测上游忽略 stop 导致语义不符，再改为拒绝。

#### Scenario: stop 透传
- **WHEN** 请求携带非空 `stop`
- **THEN** 系统不返回 400，将其纳入上游 payload
