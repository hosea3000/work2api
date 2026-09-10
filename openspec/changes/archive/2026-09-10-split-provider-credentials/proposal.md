# split-provider-credentials

## Why

当前凭证子系统是"半拆"状态：内存里有两个池（codebuddy / trae），但**存储共用一张 `credential` 表（provider 列区分）、服务共用一个 `credentialService`、管理台共用一张表和一个 "current" 概念**。结果是概念错位——管理台一个列表，但"当前凭证/轮换开关"只属于 codebuddy 池，TRAE 侧无对应 UI，用户无法手动指定 TRAE 用哪个凭证。

本变更是**一次性彻底拆分**：codebuddy 与 TRAE 各自拥有独立的表、repo、service、内存池、管理 API、管理台卡片，并新增持久化的"当前凭证 + 轮换开关"。拆完两个 provider 从存储到 UI 完全对称、互不感知。

## What Changes

- **存储拆分**：`credential` → `codebuddy_credential` + `trae_credential`（各自专属列，去掉 `provider` 列）；`checkin_record` → `codebuddy_checkin_record` + `trae_checkin_record`
- **新增 `pool_state` 表**：`provider`(PK) + `auto_rotation` + `current_credential_id`，持久化每个 provider 的当前凭证与轮换开关（重启不丢）
- **Repository 拆分**：`CodeBuddyCredentialRepository` / `TraeCredentialRepository`（含各自签到记录）
- **Service 拆分**：`CodeBuddyCredentialService` / `TraeCredentialService`（各持自己的池 + current + rotation，读写 `pool_state`）；签到服务按 provider 拆分；刷新/聊天/登录服务各自依赖对应 provider 的 service
- **Admin API 拆分**：`/api/admin/codebuddy/credentials/*` 与 `/api/admin/trae/credentials/*`（list/create/delete/select/test/rotation/checkin 各自一套）；移除旧的 `/api/admin/credentials/*`
- **管理台 UI 拆分**：凭证页顶部加 provider tab（CodeBuddy / TRAE），一次只显示选中 provider 的登录认证与凭证池，各有自己的表格、当前凭证、轮换开关、筛选
- **TRAE 获得完整调度能力**：手动设当前 + 轮换开关，与 codebuddy 对称
- **不做存量数据迁移**：部署前清库，全新初始化（由 `EnsureSchema` 建新表）

## Capabilities

### Modified

- `credential-management`: 存储/API 按 provider 拆分为两套；新增持久化的当前凭证与轮换开关
- `credential-pool`: 两个完全独立的内存池（各自 round-robin/current/rotation，状态持久化）
- `daily-checkin`: 签到记录按 provider 拆表，签到服务按 provider 拆分
- `trae-credential-auth`: TRAE 登录入库写入 `trae_credential`
- `admin-web-frontend`: 凭证页改为顶部 provider tab（CodeBuddy / TRAE），一次只显示一个 provider

## Impact

- **DB**：`credential`/`checkin_record` 两张旧表废弃，新增 5 张表（`codebuddy_credential`、`trae_credential`、`codebuddy_checkin_record`、`trae_checkin_record`、`pool_state`）；无数据迁移（清库）
- **后端**：`model`、`repository`、`service`（credential/checkin/chat/models/token_refresh/trae_login）、`handler`、`router`、`bootstrap`、wire 全面改动
- **前端**：`CredentialsView.vue` 拆两张卡片、`admin.ts` 两套凭证 API、`types` 拆分
- **非目标**：存量数据迁移、跨 provider 统一视图、codebuddy 与 TRAE 凭证互转
