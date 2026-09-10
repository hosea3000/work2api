# add-trae-credentials

## Why

work2api 目前只支持 CodeBuddy 单一上游凭证。参考 trae2api-web 的成熟实现，接入 TRAE SOLO 凭证可让同一网关管理两类上游账号；本期限于凭证管理（登录导入 + 列表维护），聊天调度等消费能力二期再做。

## What Changes

- 新增 TRAE 上游客户端包（`internal/upstream/trae`）：登录 URL 构造、回调解析、ExchangeToken、GetUserInfo（移植自 trae2api-web）
- 新增 TRAE 网页登录闭环：管理台发起 → 浏览器新窗口打开 TRAE 官网登录 → 回调捕获（127.0.0.1:18080 第二监听器）或粘贴回调 URL 兜底 → 换 token → GetUserInfo → 入库
- credential 表新增 `provider`（`codebuddy` | `trae`，默认 `codebuddy`）与 `machine_id`、`device_id` 三列
- 所有 codebuddy 消费方（凭证池、token 刷新 job、签到 job、聊天执行路径）按 `provider = 'codebuddy'` 过滤，TRAE 凭证不进入调度
- 管理台凭证页改为双 panel 布局：左侧 CodeBuddy 登录认证，右侧 TRAE 登录（含粘贴回调 URL 导入）；移除 CodeBuddy 手动添加（粘贴裸 bearer_token）UI 入口，后端 API 保留
- TRAE 凭证 token 自动刷新延期至二期：本期内 refreshToken 轮换失效后需重新登录导入

## Capabilities

### New Capabilities

- `trae-credential-auth`: TRAE 网页登录认证闭环——登录 URL 生成、pending 状态机、回调捕获（/authorize + 18080 监听器）、粘贴回调 URL 导入、ExchangeToken 换取凭证入库

### Modified Capabilities

- `credential-management`: 凭证列表增加 provider 维度展示；手动添加不再由管理台 UI 暴露（API 保留）；凭证按 provider 区分且 TRAE 凭证不进入 codebuddy 调度池

## Impact

- **后端新增**：`internal/upstream/trae`（constants/login/client）、`internal/service/trae_login_service.go`（pending 状态机）、`internal/handler/trae_auth_handler.go`、18080 回调监听器（bootstrap/server 启动编排）
- **数据模型**：`internal/model/gateway.go` Credential 加列；AutoMigrate 自动迁移，无需 SQL 脚本
- **过滤点改动**（遗漏即事故）：`credential_pool.go` Refresh/LoadAll、`token_refresh.go` 扫描、`checkin_job`/`checkin_service` 扫描、`credential_service.List` 相关 codebuddy 调度路径
- **路由**：`/api/admin/trae/login/*`（SessionAuth）、`GET /authorize`（公共，浏览器 302 落点）；config 新增 `trae.callback_port`
- **前端**：`web/src/views/CredentialsView.vue` 双 panel 改版、`web/src/api/admin.ts` 新增 TRAE 登录 API、凭证列表加 provider 标签
- **依赖**：无新第三方依赖（移植代码纯标准库 + 现有 gin 栈）
