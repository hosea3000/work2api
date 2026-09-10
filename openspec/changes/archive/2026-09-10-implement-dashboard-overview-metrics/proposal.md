## Why

总览页三张卡片（服务状态 / 有效凭证 / 今日请求数）目前全部由打桩数据驱动：`/api/admin/status` 返回 `status:"ok"` 且凭证计数写死为 0，`/api/admin/stats/overview` 恒返回空统计。结果是服务状态永远显示「异常」，凭证数与今日请求数永远是 0，卡片形同虚设。需要让这三张卡片反映真实运行状态。

## What Changes

- 新增持久化的聊天请求记录：每次 chat completions 调用落一行，支持按时间范围统计。
- `/api/admin/status`：`status` 返回 `healthy`（进程存活即健康）；`credentials.total/valid` 改为合并 CodeBuddy 与 TRAE 的真实凭证计数（全部视为有效）。
- `/api/admin/stats/overview`：解析 `start_at`/`end_at`，返回真实的 `request_count` 与 `success_rate`；其余统计字段维持空值。
- 四个 chat completions 入口（外部 + Playground，CodeBuddy + TRAE）挂请求记录中间件；`/models`、凭证测试、签到不计数。
- 前端不改：`DashboardView` 已在消费 `totals.request_count`、`totals.success_rate` 与 `credentials.*`。
- `GET/PUT /api/admin/settings` 维持打桩不变。

## Capabilities

### New Capabilities
- `request-metrics`: 持久化聊天 API 调用记录，并按时间范围提供请求数与成功率统计。

### Modified Capabilities
- `admin-web-frontend`: 总览页服务状态、有效凭证、今日请求数三项的数据契约由打桩改为真实数据；原「打桩端点返回空数据」中关于 `status` 与 `stats/*` 的要求相应调整（settings 仍打桩）。

## Impact

- 新增代码：`internal/model`（请求记录表）、`internal/repository`（记录/统计）、`internal/service`（薄统计服务）、`internal/middleware`（记录中间件）。
- 修改代码：`internal/handler/admin_stub_handler.go`、`internal/router/gateway.go`、`internal/repository/gateway.go`（`EnsureSchema`）、`cmd/server/wire`（依赖注入，`make wire`）。
- API：`/api/admin/status`、`/api/admin/stats/overview` 返回结构不变，字段语义由打桩变为真实。
- 依赖：无新增第三方依赖，复用现有 GORM/SQLite。
