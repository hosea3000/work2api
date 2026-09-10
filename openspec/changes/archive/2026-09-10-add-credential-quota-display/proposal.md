## Why

管理台凭证列表目前只有占位数据：CodeBuddy 的 `GET /api/admin/codebuddy/credentials/:id/quota` 是返回 `status:"unknown"` 的桩，前端“刷新额度”动作没有监听者，直接调用还会打到 404。管理员无法判断哪张凭证还有多少额度。TRAE 的权益额度只在签到时写入 `checkin_record.credit`，列表里也不展示。需要一个轻量的额度探测与展示闭环。

## What Changes

- 两个 provider 的凭证表各新增 `quota_total`、`quota_remaining` 两列（TRAE 另有已存在的签到 credit，不冲突）。
- 新增额度探测：CodeBuddy 调 `POST /v2/billing/meter/get-user-resource`（`ProductCode=p_tcaca`）把套餐 `remaining`/`total` 求和；TRAE 复用现有 `EntUsage` 的 `limit`/`remain`。
- 新增每小时后台扫描（两 provider 的 active 凭证），探测结果写回凭证行；探测失败保留上次值。
- 新增手动刷新端点 `POST /api/admin/{provider}/credentials/:id/quota/refresh`，两个 provider 对等。
- 凭证列表接口为每项内嵌 `quota: {total, remaining}`（未探测为 null）。
- 管理台凭证池新增“额度”列（文案 `剩余 1700 / 2000`），操作菜单“刷新额度”接通；隐藏“额度探测方式”。
- 删除 `GET /api/admin/codebuddy/credentials/:id/quota` 的 unknown 桩（由列表内嵌与刷新端点替代）。
- **明确不做**：额度估算、聊天用量扣减、圆环组件、套餐明细（packages）、探测状态机（stale/error）、企业版额度接口、手动凭证的个人/企业探测方式切换。

## Capabilities

### New Capabilities
- `credential-quota`: 两 provider 的额度探测、存储、每小时扫描、手动刷新与列表内嵌。

### Modified Capabilities
- `credential-management`: 凭证列表项的 quota 字段由“可为空占位”变为真实探测值；移除 codebuddy 额度 unknown 桩端点。
- `admin-web-frontend`: 凭证池新增额度列与刷新动作；API 契约更新为 `{provider}/credentials/:id/quota/refresh`，并移除 codebuddy quota 桩。

## Impact

- **模型/存储**：`internal/model/credential.go`（两凭证表加列）；AutoMigrate 与 `EnsureSchema` 自愈建表自动生效。
- **上游**：`internal/upstream/codebuddy`（新增额度探测方法）；复用 `internal/upstream/trae` 的 `EntUsage`。
- **服务/调度**：新增额度 service 与每小时扫描 loop，挂到现有 `CheckinJobServer` 启动流程。
- **路由/处理器**：`internal/router/gateway.go`、`internal/handler/codebuddy_credential_handler.go`、`internal/handler/trae_credential_handler.go`、`internal/handler/admin_stub_handler.go`（移除桩）。
- **前端**：`web/src/components/CredentialPoolCard.vue`、`web/src/components/CredentialActions.vue`、`web/src/api/admin.ts`、`web/src/types/admin.ts`。
- **无新增依赖**；无破坏性 API（新增端点与字段，移除未使用的桩端点）。
