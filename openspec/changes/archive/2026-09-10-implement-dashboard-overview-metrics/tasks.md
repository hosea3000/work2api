## 1. 数据层

- [x] 1.1 新增 `internal/model/stats.go`：`RequestRecord{Id, StartedAt, Outcome}`，`TableName()` 返回 `request_record`，`StartedAt` 建索引。
- [x] 1.2 新增 `internal/repository/stats.go`：`StatsRepository` 接口与实现，提供 `Record(ctx, rec)` 与 `CountRequests(ctx, startAt, endAt) (total, success int64, err)`，含 `NewStatsRepository` 构造器。
- [x] 1.3 在 `internal/repository/gateway.go` 的 `EnsureSchema` AutoMigrate 列表中加入 `model.RequestRecord{}`。
- [x] 1.4 为 `StatsRepository` 写单测：插入后范围统计正确、`[start_at, end_at)` 边界、空区间。

## 2. 服务与中间件

- [x] 2.1 新增 `internal/service/stats_service.go`：薄封装仓储，暴露 `Record(ctx, at int64, success bool)` 与 `Overview(ctx, startAt, endAt) (total, success int64, err)`。
- [x] 2.2 新增 `internal/middleware/record.go`：`RecordRequest(stats service.StatsService) gin.HandlerFunc`，在 `c.Next()` 后按 `c.Writer.Status() < 400` 判定并落库，写库错误仅记日志、不改变响应。
- [x] 2.3 中间件单测：2xx/4xx/5xx 的 outcome 判定；写库失败不 panic、响应不受影响。

## 3. Handler

- [x] 3.1 `AdminStubHandler` 注入 `CodeBuddyCredentialService`、`TraeCredentialService` 与统计服务，更新 `NewAdminStubHandler` 签名。
- [x] 3.2 `Status`：`status` 返回 `"healthy"`；`credentials.total` 与 `valid` 改为两个 provider 凭证数量之和；`current.status` 按 `total > 0 ? "auto_rotation" : "no_credentials"`。
- [x] 3.3 `StatsOverview`：解析 `start_at` / `end_at`，填充 `totals.request_count` 与 `totals.success_rate`（无请求为 null），其余字段维持空值，响应结构不变。
- [x] 3.4 handler 单测：`status` 为 `healthy` 且合并计数正确；overview 的计数、成功率与空区间行为。

## 4. 装配

- [x] 4.1 `internal/router/gateway.go`：`RouterDeps` 增加统计服务；在四个 chat completions 路由（外部与 Playground、CodeBuddy 与 TRAE）挂 `RecordRequest` 中间件，`/models` 不挂。
- [x] 4.2 `cmd/server/wire/wire.go`：将新增构造器加入对应 wire set，执行 `make wire` 重新生成 `wire_gen.go`。
- [x] 4.3 检查 `internal/server/http.go` 与 `cmd/migration` 是否需要同步（确保 `EnsureSchema` 覆盖新表）。

## 5. 验证

- [x] 5.1 `go build ./...` 通过。
- [x] 5.2 `go test ./...`（含新增单测）通过。
- [x] 5.3 手动验证：发起一次 chat completions 后，`GET /api/admin/stats/overview?start_at=&end_at=` 的 `request_count` 增 1；`GET /api/admin/status` 返回 `status:"healthy"` 与合并凭证数。（以 middleware 端到端单测代替真实上游调用）
- [x] 5.4 `cd web && pnpm typecheck` 通过，确认前端无回归。
