## Context

管理台凭证列表的额度展示是参考实现 `codebuddy2api` 的遗留契约：前端 `web/` 已带完整的 `CredentialQuota` 类型、`refreshCredentialQuota` / `updateCredentialQuotaProbeMode` API 调用与 `CredentialActions` 的“刷新额度”菜单项，但后端只有 `GET /api/admin/codebuddy/credentials/:id/quota` 返回 `status:"unknown"` 的桩，刷新端点未注册（404），列表也不返回 quota。TRAE 的额度只在签到时取一次 `EntUsage`，存进 `checkin_record.credit`，列表不展示。

约束：本仓库是单管理员、Go(gin+gorm+sqlite) 单二进制；参考实现的额度模块（内存缓存、每小时扫描、并发限速、generation 竞态保护、请求后用量估算、packages 明细、个人/企业探测方式）大部分是为多用户多凭证服务设计的，在这里属于过度设计。本次只做“总额度 + 剩余额度”的最小闭环。

## Goals / Non-Goals

**Goals:**
- 两个 provider 的凭证列表都能看到 `剩余 X / 总额 Y`。
- 每小时后台扫描 + 手动刷新按钮，两条路径都能更新额度。
- 额度持久化到 SQLite，重启后列表立即可见。
- 探测失败时静默保留上次成功值。

**Non-Goals:**
- 额度估算（`estimated_credit_since_sync`）与聊天 `usage.credit` 扣减。
- 圆环组件、`packages` 套餐明细、`status/stale/error` 状态机、时间戳字段。
- 企业版额度接口（`get-enterprise-user-usage`）与手动凭证的个人/企业探测方式切换。
- 新增独立额度表。

## Decisions

### 1. 存储：给两张凭证表各加 2 列，不建新表

`codebuddy_credential` 与 `trae_credential` 各新增 `quota_total *float64`、`quota_remaining *float64`（NULL = 未探测）。

- **为什么**：只要两个数，加列的改动比建表更小；凭证行天然按 provider 分离，无需复合主键。
- **备选**：独立 `credential_quota(provider, credential_id, snapshot)` 表——当字段只有两个时收益不足，且要多一套 model/repo。
- **注意**：每小时写回用 `UpdateColumns`（或 `Select` 指定列），避免无谓 bump `updated_at`。

### 2. 探测源：codebuddy 走 get-user-resource，trae 复用 EntUsage

| provider | 上游 | 映射 |
|---|---|---|
| codebuddy | `POST /v2/billing/meter/get-user-resource`，body `{PageNumber:1, PageSize:200, ProductCode:"p_tcaca", Status:[0,3], PackageEndTimeRangeBegin: now, PackageEndTimeRangeEnd:"2127-01-01 00:00:00"}` | 遍历 `data.Response.Data.Accounts`，`Status==0` 的套餐把 `CycleCapacitySize`(Precise 优先) 求和为 total、`CycleCapacityRemain`(Precise 优先) 求和为 remaining |
| trae | 复用 `internal/upstream/trae` 的 `EntUsage` | total = `limit`，remaining = `remain` |

- **为什么**：TRAE 的 `EntUsage` 已实现且有测试，不重复造。
- **Header**：codebuddy 复用 `GenerateHeaders`（把 `Accept` 覆盖为 `application/json, text/plain, */*`），与企业/个人探测无关。

### 3. 调度：新增独立 hourly loop，挂在 `CheckinJobServer`

仿照现有 `TokenRefreshService.RunLoop`，新增一个每 1 小时的循环（写死常量，不进 config），遍历两 provider 的 active 凭证并探测写回。

- **为什么**：复用现有 server 启动方式，不引入 gocron；间隔是产品决定，写死最省。
- **备选**：配置项 `quota_scan_interval`——YAGNI，需要再加。

### 4. 失败处理：保留旧值，不引入状态

探测失败（网络/非 200/解析失败）时日志记录，不更新凭证行；列表继续显示上次成功的值；从未成功过则显示 `-`。

- **为什么**：用户明确不需要错误状态；避免 `status/stale/error` 状态机。

### 5. 接口：provider 对等，列表内嵌 + 独立刷新

- `GET /api/admin/{provider}/credentials`：每项新增 `quota: {total, remaining} | null`。
- `POST /api/admin/{provider}/credentials/:id/quota/refresh`：同步探测，成功返回 `{quota:{total,remaining}}`，失败返回 502。
- 删除 `GET /api/admin/codebuddy/credentials/:id/quota` 桩。
- 前端 `adminApi.refreshCredentialQuota` 改为带 `provider` 参数。

### 6. 展示：纯文本列，不用圆环

凭证池表格新增“额度”列，渲染 `剩余 1700 / 2000`；未探测显示 `-`。操作菜单“刷新额度”对两 provider 都接通；“额度探测方式”菜单项隐藏（`canEditQuotaProbeMode` 保持 false）。

## Risks / Trade-offs

- [企业凭证走个人接口会失败] → 探测时跳过带 `enterprise_id` 的 codebuddy 凭证，保持 NULL，不产生失败日志噪音。
- [每小时写凭证行可能与 OAuth 刷新/池加载竞争] → 只 `UpdateColumns` 两个 quota 列，不触发 `pool.Refresh`；额度列不参与任何池逻辑。
- [探测给上游带来额外流量] → 每小时 × 凭证数，量级很小；手动刷新仅在管理员点击时发生。
- [sqlite 无列迁移版本管理] → 沿用现有 `AutoMigrate` + `EnsureSchema` 自愈建表，新列自动追加（sqlite 支持 ADD COLUMN）。
- [TRAE 额度语义是“权益额度”非 SOLO 真实可用额度] → 与现有签到展示口径一致，文案仅“额度”，不宣称精确。

## Migration Plan

1. 上线后首次启动 `EnsureSchema` 自动为两表追加两列（NULL）。
2. 每小时扫描或管理员手动刷新后填充。
3. 回滚：删除新端点与前端列即可，新增列留空不影响旧逻辑（无破坏性）。

## Open Questions

- 无。范围与展示口径已确认（个人版、仅总额/剩余、失败保留旧值、纯文本）。
