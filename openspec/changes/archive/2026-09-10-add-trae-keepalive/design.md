# add-trae-keepalive 设计

## Context

一期（`add-trae-credentials`）落地了 TRAE 凭证登录导入，明确 Non-Goal 延期了三件事：token 自动刷新、签到、聊天调度。本期落地前两件（保活），聊天调度继续二期。参考实现 trae2api-web 已验证全部上游交互（RESEARCH.md 实测）：

- 刷新 = 复用 `ExchangeToken`（与登录闭环同端点，仅 UA 头，无鉴权头），refreshToken 每次轮换，`TokenExpireAt` 毫秒需归一化（work2api 的 `ExpireAtFromExchange` 已处理）
- 签到 = `api.trae.cn/trae/api/v2/ug/checkin_credits/status|claim`，UgHeaders（`Authorization: Cloud-IDE-JWT <at>` + `X-Device-Id` + `X-User-Region: CN`），每日 +200 ide_credits，高峰 9074
- 积分 = `api.trae.cn/trae/api/v2/pay/ide_user_ent_usage`，聚合 entitlement 包 credits_limit − usage.credits_amount（注意：聚合含 work 包，仅为展示参考，不代表 SOLO 真实可用额度）

## Goals / Non-Goals

- Goals: TRAE 凭证不再自然死亡（自动刷新）；ide_credits 每日自动领取（签到）；积分可视化；trae 行签到/测试按钮可用
- Non-Goals: TRAE 聊天调度（二期）；9074 同日重试（用户确认 1A）；刷新失败告警/通知（日志即可）

## Decisions

### D1: 刷新独立文件镜像，不做策略抽象

新建 `internal/service/trae_token_refresh.go`（~80 行），结构镜像 `token_refresh.go`（RunLoop/scanOnce/refreshCredential）。**不**抽公共"刷新策略接口"——两者判定条件完全不同（`oauth`+refresh_expires_at vs `web_login`+仅 expires_at），上游错误形态不同（`AuthError{unauthorized}` vs `refresh_failed`），复制比接口干净，且保住 D2 隔离红线。`ShouldRefresh`（token_refresh.go:34）硬编码 `AuthSource=="oauth"` **一行不动**。

```go
// trae 判定（新函数，不动 codebuddy 的 ShouldRefresh）
func ShouldTraeRefresh(cred *model.Credential, now int64) bool {
    return cred.Provider == "trae" && cred.AuthSource == "web_login" &&
        cred.RefreshToken != nil && *cred.RefreshToken != "" &&
        cred.ExpiresAt != nil && now >= *cred.ExpiresAt-RefreshWindowSeconds
}
```

与 codebuddy 的三点差异：过滤函数（`isTraeView` vs `isCodebuddyView`）；无 refresh_expires_at 概念（上游不返回 RefreshExpireAt 的可靠语义，一期不存，靠"刷新失败即死"兜底）；失败处理（ExchangeToken 返回 `refresh_failed: no token — re-login required` → MarkExpired）。

### D2: 刷新失败即 MarkExpired，死亡兜底走既有链路

ExchangeToken 明确返回 `refresh_failed`（client.go:103）时 MarkExpired。expired 凭证前端已有"重新登录"提示；重新登录走 `trae_login_service.ingest` 同 uid 原位更新复活。零新增链路。

### D3: 签到在 CheckinService 内部分叉，不建第二个 service

`checkin_service.go` 三处各加 provider 分支（~40 行）：

- `ManualCheckin`（:53）：`isCodebuddy` 拒绝处放行 trae，转 trae 签到流程
- `RunScheduledCheckin`（:132）：过滤后按 provider 分派；trae 的幂等判定**查上游 status**（不查 CheckinRecord——两步式中 status 本身就是幂等闸门，且避免"记录表说没签但上游说签了"的不一致）
- `performCheckin` 抹平形态差异：trae 流程 = `CheckinStatus` → (checked_in ? 幂等成功 : `CheckinClaim`) → `EntUsage`（失败仅记日志），产出与 codebuddy 相同的 `CheckinDetail`（success/code/message/credit），credit 填 EntUsage 剩余积分或 nil

`CheckinRecord` 表零改动。`CheckinJob`（每日 checkin_hour:checkin_minute 调度）零改动——trae 凭证自然进入既有遍历。

trae 分支不依赖凭证池快照（池仅装 codebuddy），直接用 `model.Credential` 组装 trae 请求参数（access_token/device_id）。

### D4: trae 包扩展 ~90 行

`internal/upstream/trae` 新增：

- 常量：`EpCheckinStatus/EpCheckinClaim/EpEntUsage`（对齐 trae2api-web constants.go:22-24）
- `ugHeaders(req, accessToken, deviceID)`：Cloud-IDE-JWT + X-Device-Id + X-User-Region: CN + UA
- `CheckinStatus(ctx, accessToken, deviceID) (checkedIn, credits int64, enable, err)` — body `{}`
- `CheckinClaim(ctx, accessToken, deviceID) (*CheckinResult, error)` — 解析 `{code, message}`，code==0 成功（**响应 schema 未实测**，按 ug 域惯例推断；实测不符时只改解析，端点/头不动——`ponytail:` 标注）
- `EntUsage(ctx, accessToken, deviceID) (remain, limit, used int64, err)` — 聚合 entitlement 包（含 work 包，仅展示参考）

Client 已有 HTTP/doJSON/oauthHeaders，直接复用。UgHost 常量已在 constants.go。

### D5: Test 探针按 provider 分派

`credential_service.go` `Test`（:243）：trae 分支改用 `trae.Client.GetUserInfo(accessToken, host)`（已存在，登录闭环在用），成功即通过；codebuddy 分支不动。`bootstrap/providers.go` 需把 trae client 注入 CredentialService（构造函数加参）。

### D6: 刷新 RunLoop 挂载点

`checkin_job_server.go` Start（:38）加一行 `go s.traeRefresh.RunLoop(ctx)`，构造函数加 `*TraeTokenRefreshService` 参数（wire 注入）。两个 RunLoop 并行各自扫库，互不感知——SQLite 读多写少，无锁竞争风险。

### D7: 前端解锁 + 积分展示

- `CredentialsView.vue`：trae 行移除"签到/测试"按钮隐藏逻辑；凭证卡片/行展示 TRAE 剩余积分（来自签到响应或列表接口透传）
- `admin.ts`：签到响应类型补 trae 字段（credit 语义 = 剩余积分）；无新端点
- 积分数据源：签到时查的 EntUsage 随 CheckinRecord.credit 落库，列表接口透传当日记录即可，不加独立积分端点（YAGNI，等聊天调度上线再考虑实时额度）

## Risks / Trade-offs

- [ug 域 claim 响应 schema 未实测] → 解析层收敛在 `CheckinClaim` 单函数，实测不符仅改一处；code==0 判定失败时按"HTTP 200 即成功"降级（参考实现行为）
- [EntUsage 聚合含 work 包，数字偏大] → 前端标注"权益额度"而非"剩余对话次数"；RESEARCH.md 已确认 SOLO 真实额度看 notify_usage，留二期聊天调度时精确化
- [两个刷新 RunLoop 同库并发] → 各自只写本 provider 凭证，gorm/SQLite 单写者模型下串行化，无冲突；抖动天然错峰
- [status 与 CheckinRecord 双幂等源] → trae 只信上游 status（D3），记录表仅作展示，不做判定

## Migration Plan

无 DB 迁移。部署后首轮刷新扫描立即执行（临期凭证 24h 内被救活）；次日 09:30 起自动签到。已在库中的临期/过期凭证：临期的自动救活，RT 已失效的走 MarkExpired → 重新登录。

## Open Questions

- 无（1A/2B/3B/4B/5 已由用户确认）
