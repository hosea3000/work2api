# split-provider-credentials 设计

## Context

前序变更把 TRAE 接进了网关（登录、保活、聊天），但凭证子系统一直是"半拆"：两个内存池共用一个 `credential` 表、一个 `credentialService`、一张管理台表。`Current()` 只读 codebuddy 池、`Select()` 只收 codebuddy、管理台只有一个轮换开关，导致 TRAE 无法手动指定当前凭证，且用户心智模型（一个列表 vs 两个池）错位。

本变更彻底拆分为两个对等的凭证子系统。用户已确认：checkin_record 也拆；current/rotation 持久化；TRAE 也提供轮换开关；**不做存量迁移**（清库重建）；先归档前序 change。

## Goals / Non-Goals

- Goals: 两 provider 的表、repo、service、池、API、UI 完全独立且对称；当前凭证与轮换开关持久化；TRAE 具备与 codebuddy 相同的手动选择/轮换能力
- Non-Goals: 存量数据迁移（清库）、跨 provider 统一视图、凭证互转

## Decisions

### D1: 存储按 provider 分表，去掉 provider 列

```
codebuddy_credential          trae_credential
  id                            id
  bearer_token                  bearer_token
  user_id                       user_id
  account_uid                   machine_id
  domain                        device_id
  enterprise_id                 auth_source (web_login)
  department_full_name          status
  auth_source (manual|oauth)    expires_at
  status                        refresh_token
  expires_at                    last_refresh_at
  refresh_token                 nickname
  refresh_expires_at            email
  expires_in                    created_at / updated_at
  session_state / scope
  last_refresh_at
  nickname / preferred_username / email
  created_at / updated_at
```
各自专属列互不携带（codebuddy 无 machine_id/device_id，trae 无 domain/enterprise/oauth 扩展列）。GORM 两个 struct，各自 `TableName()`。

### D2: 签到记录也分表

`checkin_record` → `codebuddy_checkin_record` + `trae_checkin_record`，结构相同（credential_id、checkin_date、success、code、message、credit、attempted_at、checked_in_at，`(credential_id, checkin_date)` 唯一）。签到服务按 provider 拆为两个，各查各表。

### D3: `pool_state` 表持久化 current + rotation

```sql
pool_state(
  provider             TEXT PRIMARY KEY,   -- 'codebuddy' | 'trae'
  auto_rotation        BOOLEAN NOT NULL DEFAULT true,
  current_credential_id TEXT
)
```
- 启动 / 池 Refresh：读 `pool_state`，把 current 指针定位到 `current_credential_id`，恢复 `auto_rotation`
- `Select(id)`：写 `current_credential_id=id` 且 `auto_rotation=false`（upsert）
- `rotation/toggle`：翻转 `auto_rotation`（upsert）
- 删除当前凭证：清空该 provider 的 `current_credential_id`

放在独立小表而非给凭证表加 `is_current` 列：一个 provider 只有一行状态，避免"多行 is_current 真值"的约束维护。

### D4: Repository 拆分

`GatewayRepository` 的凭证/签到方法拆为 `CodeBuddyCredentialRepository`（codebuddy_credential + codebuddy_checkin_record）与 `TraeCredentialRepository`（trae_credential + trae_checkin_record）。各含 List/Get/GetByAccountUid/Create/Update/Delete + 签到记录 Get/Save。`pool_state` 的读写单独一个 `PoolStateRepository`（或并入各自 repo，各查自己 provider 行）。

### D5: Service 拆分

```
CodeBuddyCredentialService          TraeCredentialService
  pool *CredentialPool                pool *TraeCredentialPool
  Current/Select/ToggleRotation       Current/Select/ToggleRotation
  SelectByToken (聊天)                SelectForChat (聊天)
  Test/MarkExpired/AddOAuth           Test/MarkExpired
  Add/Delete/List                     Delete/List
  （持久化 pool_state）               （持久化 pool_state）

CodeBuddyCheckinService             TraeCheckinService
  ManualCheckin/RunScheduled          ManualCheckin/RunScheduled
```
`CredentialPool` 与 `TraeCredentialPool` 各自加 `Current()/SelectCurrent()/SetAutoRotation()/AutoRotationEnabled()`（Trae 池补齐对称）。

调用方换依赖：`ChatExecutor`→CodeBuddyCredentialService；`TraeChatExecutor`/`TraeModelsService`→TraeCredentialService；`ModelsService`→CodeBuddy；`TokenRefreshService`→CodeBuddy；`TraeTokenRefreshService`→Trae；`TraeLoginService`→Trae repo/service；`Startup`/`CheckinJobServer` 分别 reload 两个池。

### D6: Admin API 拆命名空间

```
/api/admin/codebuddy/credentials          GET / POST
/api/admin/codebuddy/credentials/:id      DELETE
/api/admin/codebuddy/credentials/:id/select
/api/admin/codebuddy/credentials/:id/test
/api/admin/codebuddy/credentials/:id/daily-checkin
/api/admin/codebuddy/credentials/rotation/toggle
/api/admin/trae/credentials               GET
/api/admin/trae/credentials/:id           DELETE
/api/admin/trae/credentials/:id/select
/api/admin/trae/credentials/:id/test
/api/admin/trae/credentials/:id/daily-checkin
/api/admin/trae/credentials/rotation/toggle
```
两个独立 handler。旧 `/api/admin/credentials/*` 移除。TRAE 无 `POST`（添加走网页登录）。

### D7: 管理台 provider tab

`CredentialsView.vue` 顶部加 provider tab（CodeBuddy / TRAE），用 `v-if/v-else` 一次只挂载选中 provider 的登录面板 + 凭证池卡片，避免两套内容同页拥挤。凭证池卡片各含：本 provider 表格、当前标记、轮换开关、可用/过期筛选、操作列。数据来自两个独立 query（`/codebuddy/credentials`、`/trae/credentials`）。

### D8: 无存量迁移

部署前清库（用户确认）。`EnsureSchema` 的 AutoMigrate 会按新模型建 5 张表；旧 `credential`/`checkin_record` 表不再被模型引用，新库中不存在。不写数据搬迁逻辑。

### D9: 共享的上游客户端不变

`trae.Client`、`codebuddy.Client` 不动；变的只是它们背后读取凭证的来源（repo/service）。聊天/刷新/签到/模型列表的上游协议逻辑保持不变。

## Risks / Trade-offs

- [大面积重构，回归面广] → 分阶段：先建模型/repo/迁移建表，再拆 service，再拆 API/UI；每阶段跑全量测试；保留 codebuddy 现有测试作为行为基准
- [两套 handler/service 有重复] → 接受（与"完全分开"一致；重复换来零耦合）
- [清库意味着现有凭证丢失] → 用户已确认；文档/发布说明需注明
- [pool_state 与凭证删除的竞态] → 删除时清 current 与池摘除在同一 service 方法内顺序执行
- [checkin job 要跑两个任务] → 一个 job 触发两个 service 的 RunScheduled，或两个 job；取实现最简

## Migration Plan

清库重建：删除旧 SQLite 文件 → 启动时 `EnsureSchema` 建 5 张新表 → 管理员重新导入 codebuddy（OAuth/手动）与 TRAE（网页登录）凭证 → 设置各自当前凭证。

## Open Questions

- 无（1~5 决策已确认）
