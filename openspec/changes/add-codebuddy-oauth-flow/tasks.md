# Tasks: add-codebuddy-oauth-flow

## 1. 数据模型与上游协议层

- [x] 1.1 credential 表加列：refresh_token / refresh_expires_at / expires_in / session_state / scope / last_refresh_at，AutoMigrate 通过（含存量库加列）
- [x] 1.2 `headers.go` 新增认证三类头集：AuthStartHeaders（X-No-* 哨兵 + 无 Authorization）、AuthPollHeaders（b3 头 `{traceId}-{spanId}-1-` + X-B3-*）、AuthRefreshHeaders（轮询头 + Authorization + X-Refresh-Token + X-Auth-Refresh-Source: plugin）
- [x] 1.3 `auth.go` 上游调用：StartAuth（state?platform=CLI + authUrl 安全校验）、PollToken（11217→pending、token 字段映射）、PollAccount（12151→pending）、PollAccounts（过滤 pluginEnabled）、RefreshToken（空 JSON 体、401/403→unauthorized）；`_normalize_epoch` 毫秒归一化
- [x] 1.4 业务错误码表：12005/11212/11216/10081 → 受控中文描述 + 403；单测覆盖全部判定分支

## 2. AuthStateStore 与 OAuth 服务

- [x] 2.1 内存 AuthStateStore：TTL 600s 惰性清理、并发活跃 ≤3、60s 窗口 ≤5 次启动（429 + Retry-After）、BeginStart/FinishStart 原子预约、Consume 墓碑防重放；`-race` 单测
- [x] 2.2 `OAuthService.Poll` 三段瀑布编排：token → account → accounts，返回 pending/success/error 判别结果；单测用 httptest 模拟上游（pending 两阶段、全成功、业务错误码、账号过滤）
- [x] 2.3 `credentialService.AddOAuth`：auth_source=oauth、domain/enterprise_id 取 token 响应、account_uid 取首个 pluginEnabled 账号、expires_at 归一化、入库后 pool.Refresh + models.Invalidate；单测

## 3. HTTP 端点

- [x] 3.1 `codebuddy_auth_handler.go`：start（429 Retry-After、失败 `{success:false}` 200）、poll（归属校验 403 → Consume 原子化 → AddOAuth，撞车 409）、cancel（400/403/{cancelled:true}）
- [x] 3.2 路由挂载 `/codebuddy/auth/start|poll|cancel`（SessionAuth）+ wire 接线 + 与前端 admin.ts 契约核对（路径、方法、请求/响应字段逐一对齐）

## 4. 刷新器（方案 C）

- [x] 4.1 `TokenRefreshService`：RunLoop（首轮立即 + 1h ticker）、`_should_refresh` 条件判定（oauth 来源、refresh_token 非空、refresh_expires_at 未过、临期 24h）、带抖动逐个刷新、成功原位更新（缺新 refresh_token 沿用旧的）+ pool.Refresh + models.Invalidate、401/403 摘除、manual 跳过
- [x] 4.2 挂入 CheckinJobServer 生命周期（goroutine + ctx 取消）；单测：should_refresh 条件矩阵（含 manual 跳过、refresh 过期）、刷新成功更新、刷新失败保留原值

## 5. 端到端验证

- [x] 5.1 curl 全链路：start 返回合法 authUrl → 手工/模拟 poll（pending → 成功入库）→ 凭证列表可见 auth_source=oauth → cancel 后 poll 403 → 重复消费 403
- [x] 5.2 (curl 层全过；浏览器端联调待用户确认) 浏览器联调：凭证页点"开始认证"→ CodeBuddy 登录页弹出 → 登录后自动入库 → toast"认证成功"→ 凭证进入轮换池
- [x] 5.3 `go build` / `go test -race` / `go vet` / `openspec validate` 全绿；README 补 OAuth 使用说明
