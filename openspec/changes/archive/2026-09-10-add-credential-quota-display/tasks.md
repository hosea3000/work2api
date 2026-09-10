## 1. 模型与存储

- [x] 1.1 `internal/model/credential.go`：`CodeBuddyCredential` 与 `TraeCredential` 各新增 `QuotaTotal *float64`、`QuotaRemaining *float64`
- [x] 1.2 `internal/server/migration.go` 与 `repository.EnsureSchema` 确认新列随 `AutoMigrate` 自愈（无需额外迁移脚本）
- [x] 1.3 `internal/repository/credential.go`：新增只更新两个 quota 列的方法（`UpdateColumns`，不 bump `updated_at`），两 provider 各一

## 2. 上游额度探测

- [x] 2.1 `internal/upstream/codebuddy`：新增 `FetchQuotaPersonal(ctx, snap) (total, remaining float64, err error)`，`POST /v2/billing/meter/get-user-resource`（body 含 `ProductCode:"p_tcaca"`、`Status:[0,3]`、时间范围），复用 `GenerateHeaders` 并覆盖 `Accept`
- [x] 2.2 codebuddy 解析：遍历 `data.Response.Data.Accounts`，`Status==0` 的套餐按 `CycleCapacitySizePrecise`/`CycleCapacitySize` 求和 total、`CycleCapacityRemainPrecise`/`CycleCapacityRemain` 求和 remaining；结构非法返回受控错误
- [x] 2.3 codebuddy 单测：用 httptest 覆盖正常聚合、空 Accounts、非 200、非法 JSON
- [x] 2.4 trae 复用 `internal/upstream/trae` 的 `EntUsage`（total=limit、remaining=remain），补一条映射单测（若已覆盖则确认）

## 3. 额度服务与定时扫描

- [x] 3.1 新增 `internal/service/quota_service.go`：`Probe(ctx, provider, cred)` 探测并写回；带 `enterprise_id` 的 codebuddy 凭证跳过；失败保留旧值并记日志
- [x] 3.2 实现每小时扫描循环 `RunLoop(ctx)`（写死 1h 常量），遍历两 provider 的 active 凭证
- [x] 3.3 `internal/server/checkin_job_server.go`：把额度扫描 loop 加入启动 goroutine
- [x] 3.4 `internal/bootstrap/providers.go` / wire：注册 QuotaService 依赖
- [x] 3.5 单测：探测成功写回、失败保留旧值、企业凭证跳过

## 4. 接口与路由

- [x] 4.1 `internal/handler/codebuddy_credential_handler.go`：列表每项内嵌 `quota: {total, remaining} | null`；新增 `RefreshQuota` 处理器（成功 `{quota}`、失败 502、不存在 404）
- [x] 4.2 `internal/handler/trae_credential_handler.go`：列表内嵌 `quota`；新增 `RefreshQuota` 处理器
- [x] 4.3 `internal/router/gateway.go`：注册 `POST /api/admin/{provider}/credentials/:id/quota/refresh`；删除 codebuddy 的 `GET .../quota` 桩路由
- [x] 4.4 `internal/handler/admin_stub_handler.go`：移除 `CredentialQuota` 桩方法
- [x] 4.5 `make wire && make sqlc && make swag`（若生成物受影响）

## 5. 前端

- [x] 5.1 `web/src/types/admin.ts`：`CredentialRecord.quota` 简化为 `{ total: number; remaining: number } | null`，移除 `CredentialQuota`/`CredentialQuotaPackage`/`CredentialQuotaProbeMode*` 等未用类型
- [x] 5.2 `web/src/api/admin.ts`：`refreshCredentialQuota(provider, credentialId)` 改为带 provider；移除 `updateCredentialQuotaProbeMode`
- [x] 5.3 `web/src/components/CredentialPoolCard.vue`：新增“额度”列（`剩余 X / 总额 Y`，null 显示 `-`）；接 `onRefreshQuota` mutation（两 provider）
- [x] 5.4 `web/src/components/CredentialActions.vue`：保留“刷新额度”并确保两 provider 可用；移除“额度探测方式”菜单项与相关 props/emits
- [x] 5.5 `cd web && pnpm run build:bundle`（含 vue-tsc / lint），确认无类型错误

## 6. 验证

- [x] 6.1 后端 `go build ./...` 与 `go test ./internal/...` 通过
- [x] 6.2 启动服务，手动刷新某 codebuddy 凭证额度，列表显示真实 `剩余/总额`
- [x] 6.3 手动刷新某 TRAE 凭证额度，列表显示真实 `剩余/总额`
- [x] 6.4 断开上游/构造失败，确认列表保留旧值、不报错
- [x] 6.5 重启服务，确认额度值仍在（SQLite 持久化生效）
