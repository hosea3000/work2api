# Design: add-codebuddy-oauth-flow

## Context

一期（bootstrap-codebuddy-gateway-core）已交付：OpenAI 兼容网关、凭证池（round-robin + SQLite）、签到调度、管理台（前端整搬自 codebuddy2api，其 `useOAuthPolling.ts` 轮询状态机与 CredentialsView 认证 UI 已在库内，对接契约为一期未实现的 3 个 `/codebuddy/auth/*` 端点）。本 change 补齐 OAuth 设备授权流 + refresh_token 续期，消除"凭证 24 小时死亡"问题。

已确认决策：**方案 C**（存全量 token 字段 + 最小刷新：每小时 ticker 扫描，砍掉 Python 版的退避重试/并发信号量/inflight 去重）；不做多账号切换、不做额度联动。

## Goals / Non-Goals

**Goals:**
- 点"开始认证"→ 跳转 CodeBuddy 网页登录 → 凭证自动入库进池，前端零改动
- OAuth 凭证通过 refresh_token 自动续期，长期可用
- 与参考实现协议语义逐项对齐（头集、错误码、字段、pending 判定）

**Non-Goals:**
- 多账号切换（accounts 仅取首个 pluginEnabled 账号的 uid）
- 额度探测联动（一期无额度模块）
- refresh_accounts_pending 标记、5 次指数退避、并发信号量、inflight 去重（单管理员凭证量小，失败记日志下轮再试）
- refresh_token 落盘加密（与现有 bearer_token 同等明文存储策略）

## Decisions

### D1. AuthStateStore 为纯内存结构，挂在 oauth service 内
`map[state]entry{createdAt, consumedAt}` + 两个辅助 map（starting 预约、启动时间窗）。TTL 600s 到期惰性清理（每次操作前 prune）。consume 置 consumedAt 留墓碑，poll/cancel 命中墓碑一律 403。单管理员 → 砍掉 username 归属维度，但 TTL/并发≤3/窗口≤5/墓碑四个语义全保留。
- 备选：落 SQLite → 放弃，state 是短生命周期瞬时态，重启丢失可接受（用户重新点一次）。
- 备选：砍掉启动限流 → 放弃，这是对上游的风控保护，防封号。

### D2. 认证协议调用集中在 `internal/upstream/codebuddy/auth.go`
与 client.go 并列：`StartAuth`（启动头）、`PollToken/PollAccount/PollAccounts`（轮询头）、`RefreshToken`（刷新头）。头集函数复用 headers.go 的 host/uuid 工具新增 `AuthStartHeaders`/`AuthPollHeaders`/`AuthRefreshHeaders`。响应解析为结构体 `AuthState{State, AuthURL}`、`TokenData{AccessToken, ExpiresIn, ExpiresAt, RefreshToken, RefreshExpiresAt, SessionState, Scope, Domain, EnterpriseID}`（对齐 `_token_data` 字段映射，accessToken 与 bearer_token 同值）。
- authUrl 安全校验（绝对 HTTPS、无 userinfo、无控制字符）独立成 `isSafeExternalAuthURL`，单测覆盖。

### D3. 三段瀑布在 oauth service 编排，handler 保持薄
`OAuthService.Poll(authState)` 返回判别结果：`pending{stage,code}` / `success{tokenData}` / `error{name,httpStatus}`。handler 只做三件事：校验 state 归属（store.ValidateOwner）→ 调 Poll → 按结果映射 HTTP 码。成功路径顺序严格为：store.Consume（原子，防并发重放）→ credentialService.AddOAuth → 响应 `{saved:true}`；Consume 失败（并发撞车）返回 409。
- 备选：先入库再 consume → 放弃，失败时已入库但 state 未消费会被重复消费重放。

### D4. credential 表加列 + AddOAuth 路径
AutoMigrate 加列即可（SQLite 兼容）。`AddOAuth(ctx, tokenData, account)`：domain/enterprise_id 取 token 响应字段（与 manual 的 JWT iss 提取路径区分）；account_uid 取 accounts 中第一个 `pluginEnabled==true` 的 uid；expires_at 归一化（≥1000 亿按毫秒除 1000，对齐 `_normalize_epoch`）；expires_at 缺失时用 created_at + expires_in 推算。入库后调用 pool.Refresh 并 invalidate 模型缓存。
- 备选：复用 Add 加可选参数 → 放弃，语义分支多（来源标记、字段来源、账号解析），独立方法更清晰。

### D5. 刷新器为独立 service + ticker，复用 checkin job server 托管
`TokenRefreshService.RunLoop(ctx)`：启动立即扫一轮，之后 `time.Ticker(1h)`。每轮：查 `auth_source='oauth' AND status='active'`，内存过滤 `_should_refresh` 条件（对齐 Python：refresh_token 非空 + refresh_expires_at 未过 + now ≥ expires_at−86400），逐个带 5~20s 抖动调 `RefreshToken`。成功 → 原位 UpdateCredential（保留旧 refresh_token 若响应缺失）→ pool.Refresh → models.Invalidate。401/403 → MarkExpired。启动首轮与 checkin job 一样由 CheckinJobServer 托管 goroutine，不新增 app server。
- 备选：gocron 再挂一个 job → 放弃，ticker 更简单且无需 cron 表达式语义；复用 server 生命周期管理。

### D6. 轮询请求的 HTTP 长度控制
前端轮询间隔 5s、总时长 600s，poll 请求本身每个都是短请求（3 个上游 GET 串行，每个 30s 超时上限）。不引入服务端长轮询/SSE。gin 无默认请求超时，依赖上游 client 的 per-request ctx 超时即可；客户端断开时 `c.Request.Context()` 取消会传播到上游请求。

### D7. 路由挂载与中间件
3 个端点挂 `/codebuddy` 组（`InitGatewayRouter` 内新增），仅 SessionAuth。路径与参考实现一致（`/codebuddy/auth/start|poll|cancel`），前端 `admin.ts` 写死该前缀，不可改。

## Risks / Trade-offs

- [上游认证协议变更] → 头集/端点/字段集中 auth.go 一处；CLI 版本号沿用现有可配置项
- [轮询撞车（多标签页）] → Consume 原子性 + 墓碑；409 路径前端已有兜底提示
- [刷新扫描与聊天请求竞争同一凭证] → 刷新只在临期窗口进行且轮换池快照原子替换，最坏情形是某次请求用旧 token 失败后走既有摘除/重试逻辑
- [SQLite 明文存 refresh_token] → 与一期 bearer_token 同策略；README 已声明仅限本地/内网部署
- [重启丢 AuthState] → 用户重新点开始即可，无持久化需求

## Migration Plan

1. model 加列 → AutoMigrate 自动完成（含存量库）
2. 纯新增路由/服务，wire 重新生成
3. 回滚 = git revert；无破坏性 schema 变更（只加列）

## Open Questions

- 无。方案 C 的边界（砍掉退避/信号量/账号切换）已与用户确认。
