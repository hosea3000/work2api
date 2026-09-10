# add-trae-keepalive

## Why

work2api 一期接入的 TRAE 凭证（`add-trae-credentials`）只有登录导入闭环：access token 过期后无刷新逻辑，凭证自然死亡，只能重新走网页登录。签到也未接入，ide_credits 权益无人维护。参考实现 trae2api-web 已验证两套保活机制（每日整点 token 预刷新 + 每日签到），移植成本极低——DB 列、ExchangeToken 端点、刷新 job 骨架全部就绪。

同时确认的决策：签到高峰 9074 不做重试（次日自然重试）；签到后查询积分（EntUsage）并展示；trae 行解锁"测试"按钮（GetUserInfo 探针）与"签到"按钮。

## What Changes

- **TRAE token 刷新 job**：新增 `trae_token_refresh.go`，镜像 codebuddy 的 `TokenRefreshService`（每小时扫描，临期 24h 窗口内经 ExchangeToken 刷新，RT 失效 MarkExpired），过滤 `provider=trae && auth_source=web_login`
- **TRAE 签到**：`internal/upstream/trae` 新增 UgHeaders、CheckinStatus、CheckinClaim、EntUsage（~90 行）；`CheckinService` 内部按 provider 分叉，trae 走 status→claim 两步式，复用 CheckinRecord 与每日 CheckinJob 调度
- **凭证连通性测试**：`CredentialService.Test` 对 provider=trae 走 GetUserInfo 探针（不再返回 not_supported_for_provider）
- **管理台前端**：trae 行解锁"签到"与"测试"按钮；凭证列表/详情展示 TRAE 剩余积分（签到后刷新）
- 签到成功判定解析响应 `{code, message}`（code==0 成功）；9074（人数太多）记失败不重试

## Capabilities

### Modified

- `trae-credential-auth`: 新增 TRAE token 自动刷新 requirement（每小时扫描、临期刷新、RT 失效标记过期）；原 Non-Goal"刷新延期二期"就此落地
- `daily-checkin`: TRAE 凭证纳入签到调度（status→claim 两步式、复用签到记录、9074 处理）；新增积分查询展示；手动签到与连通性测试对 trae 开放

## Impact

- **后端新增**：`internal/service/trae_token_refresh.go`（刷新扫描 service）；`internal/upstream/trae` 扩展 checkin/ent_usage 方法与 ugHeaders
- **后端修改**：`checkin_job_server.go` 挂载 trae 刷新 RunLoop；`checkin_service.go` 三处 provider 分叉；`credential_service.go` Test 方法 trae 分支
- **路由/配置**：无新增路由；无新增配置项（复用 checkin_hour/抖动配置）
- **前端**：`web/src/views/CredentialsView.vue` trae 行按钮解锁 + 积分展示；`web/src/api/admin.ts` 响应类型补字段
- **DB**：零迁移（复用 CheckinRecord 与 credential 既有列）
