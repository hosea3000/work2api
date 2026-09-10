# Spec: codebuddy-upstream-client (delta)

## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: 认证业务错误码判定
认证轮询/刷新中，上游业务码 SHALL 按以下规则判定：11217 → 等待登录（pending）；12151 → 等待账号（pending）；12005 → 席位限制错误；11212 → 许可证过期；11216 → 试用过期；10081 → IP 限制；其余非 0 → 无效响应错误。

#### Scenario: 刷新时凭证被上游拒绝
- **WHEN** 刷新请求返回 401 或 403
- **THEN** 判定为凭证已失效（unauthorized），调用方摘除该凭证
