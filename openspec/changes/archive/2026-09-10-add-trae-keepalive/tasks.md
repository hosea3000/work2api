# add-trae-keepalive 任务

## 1. TRAE 上游客户端扩展（签到/积分）

- [x] 1.1 `internal/upstream/trae/constants.go`：新增 `EpCheckinStatus`、`EpCheckinClaim`、`EpEntUsage`（对齐 trae2api-web constants.go:22-24）
- [x] 1.2 新建 `internal/upstream/trae/checkin.go`：`ugHeaders(accessToken, deviceID)` + `CheckinStatus`（返回 checkedIn/credits/enable）+ `CheckinClaim`（解析 `{code, message}`，code==0 成功；`ponytail:` 标注 schema 未实测）+ httptest 单测（两步幂等、9074 失败、schema 降级）；Client 增加 UgHost 字段
- [x] 1.3 `internal/upstream/trae/checkin.go`：`EntUsage`（聚合 credits_limit − usage.credits_amount，返回 remain/limit/used）+ httptest 单测（空包、多包聚合）

## 2. TRAE token 刷新 job

- [x] 2.1 新建 `internal/service/trae_token_refresh.go`：`ShouldTraeRefresh`（provider=trae && web_login && RT 非空 && 临期 24h）+ `TraeTokenRefreshService`（RunLoop/scanOnce/refreshCredential，抖动 5~20s，成功原位更新 BearerToken/RefreshToken/ExpiresAt/LastRefreshAt，`refresh_failed`/401/403 → MarkExpired）+ 单测（判定边界、成功写回、失败标记、codebuddy 凭证不被触碰）
- [x] 2.2 `internal/server/checkin_job_server.go`：挂载 `traeRefresh.RunLoop`（构造函数加参 + wire 注入，wire.go serviceSet 加 `service.NewTraeTokenRefreshService` 并重新生成 wire_gen.go）

## 3. 签到接入 CheckinService

- [x] 3.1 `internal/service/checkin_service.go`：trae 签到流程（CheckinStatus 幂等闸门 → CheckinClaim → EntUsage 查积分填 credit，EntUsage 失败仅记日志）；`ManualCheckin` 放行 trae；`RunScheduledCheckin` 按 provider 分派（trae 幂等查上游 status，不查 CheckinRecord）+ 单测（trae 手动签到、已签跳过、9074 落库、claim 成功落库）
- [x] 3.2 `ManualCheckin`/`RunScheduledCheckin` 的 isCodebuddy 拒绝逻辑改为 provider 分派（trae 放行，其他未知 provider 仍拒绝）

## 4. 连通性测试

- [x] 4.1 `internal/service/credential_service.go` `Test`：trae 分支走 `trae.Client.GetUserInfo` 探针（401/403 MarkExpired）；新增 `NewCredentialServiceWithTrae` 构造器；单测（trae 成功/失败、codebuddy 行为不变）。注：`NewCredentialService` 内部默认 `trae.New()`，wire 无需改构造签名

## 5. 前端

- [x] 5.1 `web/src/components/CredentialActions.vue`：新增 `canTest` prop（testDisabled 不再绑定 isSchedulable）；`CredentialsView.vue` trae 行传 `canTest: true`、`canCheckIn: !row.is_expired`（移除 enterprise_id 限制）；签到 toast 支持 trae 幂等成功（code=null）与"权益额度"文案
- [x] 5.2 `web/src/types/admin.ts`：`CredentialDailyCheckin.credit` 注释补 trae 语义（权益剩余额度）。vue-tsc + oxlint 通过

## 6. 验证

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`（8 个包全部通过；重点覆盖 trae_token_refresh_test、trae_checkin_test、checkin_service_test、credential_service_test、trae 包 checkin_test）
- [x] 6.2 手动冒烟：导入 TRAE 凭证 → 手动签到（status→claim→积分）→ 连通性测试通过 → 手动把 expires_at 改临期观察刷新扫描写回
