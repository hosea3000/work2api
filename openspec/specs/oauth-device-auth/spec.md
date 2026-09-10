# oauth-device-auth Specification

## Purpose
TBD - created by archiving change add-codebuddy-oauth-flow. Update Purpose after archive.
## Requirements
### Requirement: 启动认证端点
系统 SHALL 提供 `POST /codebuddy/auth/start`（会话 Cookie 保护）：调用上游 `POST {endpoint}/v2/plugin/auth/state?platform=CLI`（启动头集），成功后返回 `{success: true, auth_state, verification_uri_complete, verification_uri, expires_in: 600, interval: 5, status: "awaiting_login"}`。上游返回的 authUrl MUST 校验为无用户信息、无控制字符的绝对 HTTP(S) URL，非法时按启动失败处理。上游不可用或响应无效时返回 `{success: false, error, message}`（200），MUST NOT 泄露上游响应体原文。

#### Scenario: 启动成功返回认证链接
- **WHEN** 管理员已登录并调用 start，上游返回 code=0 与合法 authUrl
- **THEN** 响应含 auth_state 与 verification_uri_complete，interval=5，expires_in=600

#### Scenario: 上游启动失败
- **WHEN** 上游返回非 200 或 code≠0 或 authUrl 非法
- **THEN** 响应为 `{success:false, error:"auth_start_failed", message:"认证启动失败，请稍后重试"}`

#### Scenario: 未登录调用
- **WHEN** 无有效会话调用 start
- **THEN** 返回 401

### Requirement: 启动频率与并发限制
系统 SHALL 限制认证启动：同一账户 60 秒滑动窗口内最多 5 次启动尝试、同时最多 3 个未完成 auth_state；超限返回 429 与 `Retry-After` 头。

#### Scenario: 窗口内超限
- **WHEN** 60 秒内第 6 次调用 start
- **THEN** 返回 429，响应含 Retry-After 秒数

#### Scenario: 活跃 state 达上限
- **WHEN** 已有 3 个未消费未过期 auth_state 时再次 start
- **THEN** 返回 429

### Requirement: 轮询端点与三段瀑布
系统 SHALL 提供 `POST /codebuddy/auth/poll`（会话保护，请求体 `{auth_state}`），按序查询上游：
1. `GET /v2/plugin/auth/token?state=X`：code=11217 → pending（stage=token）；code=0 → 取 accessToken 继续
2. `GET /v2/plugin/login/account?state=X`：code=12151 → pending（stage=account）；code=0 → 取当前账号继续
3. `GET /v2/plugin/accounts`：code=0 → 过滤 `pluginEnabled==true` 的账号列表

任一业务错误码（12005/11212/11216/10081）→ 返回该错误（HTTP 403 与受控中文描述）；token 与账号均成功后系统 MUST 先原子消费 auth_state（防重放），再解析 token 并入库（AddOAuth 路径），返回 `{saved: true, message: "认证成功"}`。

#### Scenario: 用户尚未登录
- **WHEN** 轮询时上游 token 接口返回 code=11217
- **THEN** 返回 400，体为 `{error: "authorization_pending", error_description, code: 11217, stage: "token"}`

#### Scenario: 认证成功
- **WHEN** 三段查询全部成功
- **THEN** auth_state 被消费，凭证以 auth_source=oauth 入库并进入轮换池，响应 `{saved:true, message:"认证成功"}`

#### Scenario: auth_state 无效或已消费
- **WHEN** 使用不存在、已过期或已消费的 auth_state 轮询
- **THEN** 返回 403

#### Scenario: 企业席位错误
- **WHEN** 上游返回 code=12005
- **THEN** 返回 403，error_description 为"企业许可证没有可用席位"

### Requirement: 取消认证端点
系统 SHALL 提供 `POST /codebuddy/auth/cancel`（会话保护，请求体 `{auth_state}`）：原子消费该 auth_state 并返回 `{cancelled: true}`；state 无效时返回 403，缺失时返回 400。已消费的 state 不产生副作用。

#### Scenario: 取消进行中的认证
- **WHEN** 对待处理 auth_state 调用 cancel
- **THEN** 返回 `{cancelled:true}`，后续对该 state 的 poll 返回 403

