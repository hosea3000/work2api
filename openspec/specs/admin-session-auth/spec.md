# admin-session-auth Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 管理员登录
系统 SHALL 提供 `POST /auth/login`（username + password），校验通过后设置 HttpOnly、SameSite=Lax 会话 Cookie，返回 `{authenticated: true, username}`。密码哈希存储于 SQLite（单管理员账户，首启动可用环境变量/配置初始化）。

#### Scenario: 登录成功
- **WHEN** 提交正确的用户名与密码
- **THEN** 响应 200，Set-Cookie 为 HttpOnly 会话 Cookie，体为 `{"authenticated": true, "username": "..."}`

#### Scenario: 登录失败
- **WHEN** 提交错误凭据
- **THEN** 返回 401，体为 `{"authenticated": false}`，不设置会话 Cookie

### Requirement: 会话查询与登出
系统 SHALL 提供 `GET /auth/session`（返回当前会话状态 `{authenticated, username}`）与 `POST /auth/logout`（撤销会话并清除 Cookie）。

#### Scenario: 未登录查询会话
- **WHEN** 无有效会话 Cookie 时请求 `GET /auth/session`
- **THEN** 返回 `{"authenticated": false}`（200，不报错）

#### Scenario: 登出
- **WHEN** 已登录用户请求 `POST /auth/logout`
- **THEN** 会话被撤销，Cookie 被清除，后续 `/api/admin/*` 请求返回 401

### Requirement: 管理端点会话保护
所有 `/api/admin/*` 端点 MUST 要求有效会话 Cookie；未登录返回 401。`sk-` API Key MUST NOT 能访问管理端点，反之会话 Cookie MUST NOT 能访问外部 API 端点（两套鉴权严格隔离）。

#### Scenario: 未登录访问管理端点
- **WHEN** 无会话 Cookie 请求 `GET /api/admin/credentials`
- **THEN** 返回 401

#### Scenario: API Key 不能访问管理端点
- **WHEN** 请求 `GET /api/admin/credentials` 携带 `Authorization: Bearer sk-...`
- **THEN** 返回 401

### Requirement: 登录暴力破解防护
系统 SHALL 对登录实施速率限制：全局、按 IP、按用户名三个独立滑动窗口（默认 60s 窗口内 60/10/5 次），超限返回 429。

#### Scenario: 同一用户名连续失败触发限流
- **WHEN** 同一用户名在窗口内连续失败达到阈值后再次尝试
- **THEN** 返回 429

