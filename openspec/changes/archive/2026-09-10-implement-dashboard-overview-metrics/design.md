## Context

总览页三张指标卡片（服务状态 / 有效凭证 / 今日请求数）当前的数据来源全部是打桩：

- `AdminStubHandler.Status`（`internal/handler/admin_stub_handler.go`）硬编码 `status:"ok"` 与 `credentials.total/valid = 0`，而前端 `dashboardStatus.ts` 只把 `healthy` 认作「运行中」，导致服务状态永远显示「异常」。
- `AdminStubHandler.StatsOverview` 恒返回 `statsEmptyTotals()`，今日请求数永远为 0。
- 项目中不存在任何请求持久化：`middleware.RequestLogMiddleware` 只写 zap 日志，`CredentialPool.usageCount` 只是内存轮换计数。

四个聊天入口复用两个 handler：`OpenAIHandler.ChatCompletions` 同时服务外部 `/codebuddy/openai/v1` 与 Playground `/api/admin/playground/codebuddy/...`，`TraeChatHandler` 同理。因此「来源」无法从 handler 身份推断，只能从路由取。

前端不改：`DashboardView.vue` 已在消费 `totals.request_count`、`totals.success_rate` 与 `credentials.*`。

## Goals / Non-Goals

**Goals:**

- 服务状态：进程存活即返回 `healthy`，卡片显示「运行中」。
- 有效凭证：合并 CodeBuddy + TRAE 的真实凭证数量，全部视为有效。
- 今日请求数：持久化 chat completions 调用次数，并按 `[start_at, end_at)` 返回真实请求数与成功率。

**Non-Goals:**

- 不实现完整统计子系统（tokens / credit / p95 / 维度明细 / 逐请求明细）。
- 不实现 `settings` 持久化。
- 不区分 external / playground 来源（前端 `traffic` 恒为 `all`）。
- 不修改前端代码。
- 不追求流式场景下 100% 精确的成功率。

## Decisions

### 1. 持久化：新增最小 `request_record` 表

字段：`id`（自增主键）、`started_at`（int64，索引，Unix 秒）、`outcome`（success / failure）。纳入 `EnsureSchema` 的 `AutoMigrate`，启动自愈建表。

- 备选：内存日计数器 —— 重启丢失，与「需要持久化」的要求冲突，否决。
- 备选：完整统计表（对齐旧 `StatsRequestRecord` 的 20+ 字段）—— 统计页已删除，YAGNI，否决。
- 不加 `source`：前端当前只用 `traffic=all`，暂不需要。

### 2. 记录位置：路由级中间件，位于鉴权之后

在四个 chat completions 路由上挂 `middleware.RecordRequest(recorder)`：

```go
admin.POST(".../playground/.../chat/completions", record, h.ChatCompletions)
openai.POST("/chat/completions",                   record, h.ChatCompletions)
trae.POST("/chat/completions",                     record, h.ChatCompletions)
```

- 路由级中间件在 group 鉴权中间件之后执行，鉴权失败的请求不计入。
- `c.Next()` 之后读取 `c.Writer.Status()` 判定 outcome 并落库，因此请求被完整处理后才记录。
- 备选：执行器内记录 —— 能拿到流式真实结果，但需改 `ChatExecutor` / `TraeChatExecutor` 的流式返回签名（当前流式路径写 SSE 后返回 `done=false`），超出「只做卡片」范围，否决。
- 备选：全局中间件 —— 会把 `/models`、鉴权失败等非聊天调用计入，否决。

### 3. outcome 判定：按 HTTP 状态码

`c.Writer.Status() < 400` → `success`，否则 `failure`。已知偏差：流式响应在写出 SSE 头之后才发生上游错误时，状态码仍为 200，会被记成 success，成功率偏高。这是可接受的近似；若后续需要精确，再让执行器回报 outcome。

### 4. 统计查询：handler 解析时间范围，repo 聚合

`StatsOverview` 解析 `start_at` / `end_at`，调用 repository 在 `[start_at, end_at)` 内 `COUNT(*)` 与统计 `outcome = success` 的数量。时区边界完全由前端计算，服务端不处理时区。

### 5. 服务状态：进程存活即 healthy

`Status` 返回 `status:"healthy"` + 现有 `uptime_seconds`，不改语义（对应决策 A）。

### 6. 有效凭证：合并计数

`AdminStubHandler` 注入 `CodeBuddyCredentialService` 与 `TraeCredentialService`，`Status` 中 `total = valid = len(cb.List) + len(trae.List)`（全部视为有效，不过滤 status / 过期）。`current.status`：`total > 0` → `auto_rotation`，否则 `no_credentials`（合并后单个 provider 的轮换开关语义不明确，暂不参与）。

### 7. 记录失败不影响请求

中间件忽略写库错误（仅记日志），不改变聊天响应。

### 8. 依赖注入

新增统计服务 / 仓储，更新 `cmd/server/wire/wire.go` 并执行 `make wire` 重新生成 `wire_gen.go`。

## Risks / Trade-offs

- [流式成功率偏高] → 记录为已知近似，成功率仅作展示；需要精确时改为执行器回报 outcome。
- [表无限增长] → 每行仅几十字节，量小；暂不清理，后续可加按天聚合或保留策略。
- [崩溃漏记] → 记录发生在请求处理之后，进程崩溃时该请求不计；可接受。
- [合并凭证忽略过期] → 符合「先都认为 valid」的临时决策；后续可加有效性过滤。
- [中间件顺序错误导致计数口径偏差] → 测试覆盖鉴权失败不计数、非聊天路由不计数。

## Migration Plan

- 建表由 `EnsureSchema` 的 `AutoMigrate` 在启动时完成，无需手工迁移。
- 回滚：移除中间件挂载与统计服务注入即可，残留表与数据无害。

## Open Questions

- 是否需要按来源（external / playground）或凭证维度统计。
- 请求记录的保留 / 清理策略。
