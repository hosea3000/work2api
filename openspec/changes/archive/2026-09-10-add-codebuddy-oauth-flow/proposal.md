# Proposal: add-codebuddy-oauth-flow

## Why

一期凭证只能手动粘贴 bearer_token，且 accessToken 有效期仅约 24 小时，用户每天都要重新抓 token，体验差且凭证池经常整体失效。CodeBuddy 提供了 CLI 设备授权流（跳转网页登录后轮询取 token）和 refresh_token 续期机制，参考实现 codebuddy2api 已完整逆向。本 change 移植该能力，实现"点一下开始认证 → 网页登录 → 凭证自动入库并长期续期"的闭环。

## What Changes

- 新增 3 个管理台端点（会话 Cookie 保护，前端 `useOAuthPolling.ts` 状态机现成对接、零改动）：
  - `POST /codebuddy/auth/start`：向上游 `/v2/plugin/auth/state` 取 auth_state + authUrl，返回 `verification_uri_complete`
  - `POST /codebuddy/auth/poll`：三段瀑布轮询（token → login/account → accounts），成功后解析入库并返回 `{saved:true}`；pending 以 `authorization_pending` 400 表达
  - `POST /codebuddy/auth/cancel`：立即作废 auth_state
- 新增内存 AuthStateStore：TTL 600s、并发活跃 ≤3、60s 窗口启动 ≤5 次、consume 后留墓碑防重放
- 上游客户端新增 5 个调用：auth/state（启动头）、auth/token 与 login/account 与 accounts（轮询头含 b3 埋点头）、auth/token/refresh（刷新头：X-Refresh-Token + X-Auth-Refresh-Source）
- `credential` 表新列：refresh_token、refresh_expires_at、expires_in、session_state、scope、last_refresh_at；新增 AddOAuth 入库路径（auth_source=oauth，domain/enterprise_id 取自 token 响应，account_uid 取第一个 pluginEnabled 账号）
- 每小时刷新扫描：首轮立即执行 + ticker，对 `auth_source=oauth` 且临期（expires_at − 24h）的凭证带抖动刷新；失败记日志下轮再试
- 明确不做：多账号切换（accounts 仅取首个启用账号）、额度联动重探测（一期无额度模块）、refresh_accounts_pending 标记

## Capabilities

### New Capabilities
- `oauth-device-auth`: 管理台发起的 CodeBuddy 设备授权流（start/poll/cancel 端点、AuthStateStore、上游认证协议）

### Modified Capabilities
- `codebuddy-upstream-client`: 新增 5 个上游认证调用（启动头/轮询头/刷新头三类头集与业务错误码判定）
- `credential-pool`: credential 表新增 OAuth token 字段列，新增 AddOAuth 入库路径（auth_source=oauth）
- `daily-checkin`: 调度骨架扩展出每小时刷新扫描（OAuth 凭证临期自动续期）

## Impact

- **新增代码**：`internal/upstream/codebuddy/auth.go`（认证协议）、`internal/service/auth_state_store.go`、`internal/service/oauth_service.go`、`internal/handler/codebuddy_auth_handler.go`、`internal/service/token_refresh.go`，预计 900~1300 行 Go
- **修改**：`internal/model/gateway.go`（credential 加列 + AutoMigrate）、`internal/service/credential_service.go`（AddOAuth）、`internal/router/gateway.go`（挂载 3 端点）、`internal/server/checkin_job_server.go`（挂刷新扫描）、wire 接线
- **前端**：零改动（`useOAuthPolling.ts`、CredentialsView 认证弹窗已随一期搬入）
- **兼容性**：纯增量，现有 manual 凭证与所有既有行为不变；SQLite 加列由 AutoMigrate 完成
- **风险**：认证协议为逆向所得，上游变更即失效；轮询是长循环（最长 600s），需注意 gin 请求超时与 ctx 取消传播
