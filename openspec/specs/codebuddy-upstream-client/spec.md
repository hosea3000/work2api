# codebuddy-upstream-client Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 上游端点与白名单
上游基础端点 SHALL 可配置（默认 `https://copilot.tencent.com`），支持 `https://www.codebuddy.ai`；配置非法时服务启动必须失败（fail-fast），不得回退到未授权端点。

#### Scenario: 非法端点启动失败
- **WHEN** 配置了白名单之外或格式非法的上游端点并启动服务
- **THEN** 服务启动失败并输出明确错误

### Requirement: CLI 伪装请求头生成
系统 SHALL 生成与参考实现 `codebuddy_api_client.py` 一致的请求头集：`Authorization: Bearer`、`X-User-Id`、`X-Domain`、`X-Product: SaaS`、`X-CodeBuddy-Request: 1`、`X-Agent-Intent: craft`、`X-Agent-Purpose: conversation`、`X-IDE-Type/Name: CLI`、`X-IDE-Version`（可配置，默认 2.107.0）、`x-stainless-*`（lang=js、runtime=node、版本号）、`X-Conversation-ID`/`X-Conversation-Request-ID`/`X-Conversation-Message-ID`/`X-Request-ID`（无传入时随机生成）。企业凭证 MUST 附加 `X-Enterprise-Id`/`X-Tenant-Id`，有 department_full_name 时 MUST URL-encode 后加入 `X-Department-Info`。

#### Scenario: 标准头生成
- **WHEN** 用一个个人凭证构造聊天请求头
- **THEN** 输出头集合包含上述全部字段，值格式与参考实现一致（uuid/hex 长度一致）

#### Scenario: 企业凭证附加头
- **WHEN** 凭证含 enterprise_id
- **THEN** 请求头额外包含 `X-Enterprise-Id` 与 `X-Tenant-Id`

### Requirement: 上游端点清单
系统 MUST 实现以下上游调用（与参考实现对齐）：聊天 `POST /v2/chat/completions`（仅流式）；模型 `GET /v3/config`（附加 `X-IDE-Type/Name: CodeBuddyIDE` 与 `X-Product-Version` 头）；签到 `POST /billing/meter/daily-checkin`；认证启动 `POST /v2/plugin/auth/state?platform=CLI`（启动头集：X-No-Authorization/X-No-User-Id/X-No-Enterprise-Id/X-No-Department-Info 均为 true，User-Agent 为 CLI 伪装版本，无 Authorization）；认证轮询 `GET /v2/plugin/auth/token?state=X`、`GET /v2/plugin/login/account?state=X`、`GET /v2/plugin/accounts`（轮询头集：b3 链路追踪头 X-B3-TraceId/X-B3-SpanId/X-B3-Sampled，无 Authorization 或按需携带 token）；凭证刷新 `POST /v2/plugin/auth/token/refresh`（轮询头集 + Authorization: Bearer 旧 token + X-Refresh-Token + X-Auth-Refresh-Source: plugin）。

#### Scenario: 聊天端点调用
- **WHEN** 发起聊天代理
- **THEN** 请求发往 `{endpoint}/v2/chat/completions`，携带伪装头与转换后的请求体

#### Scenario: 模型端点调用
- **WHEN** 拉取上游模型
- **THEN** 请求 `GET {endpoint}/v3/config` 使用 IDE 变体头集（非 CLI 头集）

#### Scenario: 认证启动调用
- **WHEN** 发起认证启动
- **THEN** 请求携带启动头集（X-No-* 哨兵头、无 Authorization、b3 头不存在）

#### Scenario: 认证轮询调用
- **WHEN** 轮询 token/账号接口
- **THEN** 请求携带轮询头集，包含格式为 `{traceId}-{spanId}-1-` 的 b3 头与三个 X-B3-* 头

#### Scenario: 凭证刷新调用
- **WHEN** 刷新 OAuth 凭证
- **THEN** 请求头包含 Authorization（旧 token）、X-Refresh-Token、X-Auth-Refresh-Source: plugin，请求体为空 JSON 对象

### Requirement: 上游错误处理
上游 401/403 MUST 映射为凭证失效信号（触发摘除）；网络错误/超时 MUST 映射为受控错误类别（如 upstream_unavailable），响应中 MUST NOT 透传上游响应体原文。上游业务错误码（12005/11212/11216/10081）SHALL 映射为可读中文消息。

#### Scenario: 业务错误码映射
- **WHEN** 上游返回 code 12005
- **THEN** 对外错误消息为"企业许可证没有可用席位"（同参考实现映射表）

### Requirement: 超时与重试
上游请求 MUST 设置超时（连接 10s、读 30s，聊天读超时可配置更长）；凭证选择失败（如所选凭证已被摘除）SHALL 支持有限次数的自动换凭证重试（默认 1 次）。

#### Scenario: 换凭证重试
- **WHEN** 首次上游请求返回 401 触发摘除后
- **THEN** 系统自动选择下一凭证重试一次；再失败则返回错误

### Requirement: 认证业务错误码判定
认证轮询/刷新中，上游业务码 SHALL 按以下规则判定：11217 → 等待登录（pending）；12151 → 等待账号（pending）；12005 → 席位限制错误；11212 → 许可证过期；11216 → 试用过期；10081 → IP 限制；其余非 0 → 无效响应错误。

#### Scenario: 刷新时凭证被上游拒绝
- **WHEN** 刷新请求返回 401 或 403
- **THEN** 判定为凭证已失效（unauthorized），调用方摘除该凭证

