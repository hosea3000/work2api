# add-trae-credentials 设计

## Context

work2api 是 CodeBuddy→OpenAI 兼容网关（gin + gorm/SQLite + Vue 管理台），凭证存 `credential` 表，内存 `CredentialPool` 做 codebuddy 聊天调度，另有 OAuth 设备授权流与 token 刷新/签到 job。参考实现 `trae2api-web`（纯 stdlib）已验证 TRAE SOLO 网页登录闭环：登录 URL → 官网登录 → 302 回调 `127.0.0.1:18080/authorize`（或粘贴地址栏 URL）→ ParseCallback → ExchangeToken（轮换 refreshToken）→ GetUserInfo → 落盘。

关键外部事实（来自 trae2api-web RESEARCH.md 实测）：
- TRAE 登录回调**强制 127.0.0.1**，浏览器与服务器不同机时回调必然失败 → 粘贴回调 URL 是刚需路径
- 回调不回传 machine_id/device_id，只回传 loginTraceID（由两者派生）→ 需 trace 反查 pending
- `TokenExpireAt` 上游返回毫秒（>1e12 判定归一化为秒）
- 每次 ExchangeToken 轮换 refreshToken，不刷新则凭证数日后失效
- 回调 userInfo 中文存在双重 URL 编码乱码问题，需兜底处理

## Goals / Non-Goals

**Goals:**
- TRAE 凭证经网页登录闭环（自动捕获 + 粘贴兜底）导入 credential 表，管理台可维护
- 凭证按 provider 隔离消费方：TRAE 凭证绝不进入 codebuddy 调度/刷新/签到路径
- 双 panel 管理台布局：CodeBuddy 登录（左）+ TRAE 登录（右），移除手动添加 UI

**Non-Goals:**
- TRAE 聊天调度（TRAE 凭证进入 OpenAI 兼容请求路径）— 二期
- TRAE token 自动刷新 job、TRAE 签到 — 二期（本期内 refreshToken 轮换失效后需重新登录）
- 多账号切换（codebuddy PollAccounts 已有先例：一期只取当前账号）

## Decisions

### D1: 单表 + provider 列，不建独立 trae_credential 表
`credential` 表加 `provider`（默认 codebuddy）、`machine_id`、`device_id` 三列。两家凭证字段同构（token/uid/nickname/enterprise/expires_at/refresh_token），列表/删除/脱敏/前端组件全复用；隔离点在**消费方查询**而非表。备选独立表被否：代码翻倍，无隔离收益。
注意 `auth_source` 语义不变（provider 内部来源：codebuddy 用 manual/oauth，trae 用 web_login）——token_refresh job 靠 `auth_source=="oauth"` 判定可刷新，绝不能用 auth_source 区分 provider，否则 codebuddy 刷新逻辑会碰 trae 凭证。

### D2: provider 过滤点清单（本次改动最易漏项）
每个 codebuddy 消费方各自过滤，缺一处即 trae token 被错误选中：
- `credential_pool.go` `LoadAll`/`Refresh`：入库前过滤 provider==codebuddy
- `token_refresh.go` 扫描：跳过 provider!=codebuddy
- `checkin_service`/`checkin_job`：仅 codebuddy
- `credential_service` 调度相关路径（Current/Select/Test/MarkExpired）
- `credential_service.List`（管理台）**不过滤**，全量返回（前端按 provider 展示）

### D3: 移植 trae2api-web 的 upstream 包，而非重写
`internal/upstream/trae` 从 trae2api-web 移植：constants.go（hosts/ClientID/IdeVersion/端点）、login.go（BuildLoginURL/ParseCallback/machineTraceID）、client.go（ExchangeToken/GetUserInfo，~200 行）。已实测验证且踩坑已修（毫秒归一化、trace 反查、乱码兜底），重写只会重新踩坑。适配点：原实现直接读写 `auth.Auth` 结构体，改为读写 `model.Credential` 或中间结构。

### D4: pending 登录态复用 AuthStateStore 模式（内存 + TTL）
新建 `trae_login_service.go`：pending 状态机（pending/success/failed/canceled，TTL 10 分钟，内存存储重启即失，符合"登录态瞬时"语义）。反查路径：machine_id 精确匹配 + loginTraceID 派生匹配（复刻参考实现 markPendingByMachine/getPendingByTrace）。备选入库持久化被否：登录是瞬时过程，无需持久化。

### D5: 回调捕获 = 第二 http.Server 监听 callback_port（默认 18080）
`GET /authorize` 同时注册于主 gin engine 与 18080 监听器（gin 同一 handler 复用，照搬参考实现 main.go 的双 server 模式）。config 新增 `trae.callback_port`。端口占用时启动仅告警不 fatal（捕获失败还有粘贴兜底，网关主功能不受影响）。

### D6: /authorize 为公共端点 + trace 归属校验
浏览器 302 落点无法携带管理会话 Cookie 之外的东西，故不走 SessionAuth；安全边界在于回调必须匹配一个 pending 态（loginTraceID 反查）才产生入库副作用，未知回调拒绝。粘贴导入端点（/import）走 SessionAuth。

### D7: 手动添加移除 UI、保留 API
`POST /api/admin/credentials`（bearer_token 手动添加）已测试且零维护成本，API 层保留作逃生舱；UI 移除是产品决策（manual 凭证无 refresh_token 注定过期）。规格层面从 credential-management 移除该 requirement。

### D8: 前端双 panel + provider 标签
CredentialsView 登录区改双等分 panel：左 CodeBuddy 登录认证（现有逻辑不动），右 TRAE 登录（开始认证→新窗口打开→2s 轮询 result→成功刷新列表；panel 内含粘贴回调 URL 导入入口）。凭证列表加 provider 标签列，select/rotation/签到/测试按钮对 trae 行隐藏（二期开放）。

## Risks / Trade-offs

- [TRAE 凭证被 codebuddy 路径选中 → 上游报错] → D2 过滤点清单逐项落实 + 每处过滤有对应测试断言（池刷新后 trae id 不在池内）
- [本期内 TRAE refreshToken 轮换失效，凭证自然死亡] → 接受（Non-Goals 已声明），面板列表对临期/过期 trae 凭证给"重新登录"提示
- [18080 端口被占用或被防火墙拦截] → 捕获路径失效不影响粘贴兜底；启动告警
- [回调乱码（双重 URL 编码昵称）] → 移植参考实现 fix_mojibake 兜底：回转失败回退 `用户+uid末4位`
- [回调竞态：同一 pending 被回调与取消并发命中] → pending 操作全程持互斥锁（复刻参考实现 loginMu 模式）
- [AutoMigrate 加列对存量数据] → provider 有默认值 'codebuddy'，存量凭证自动归入 codebuddy，无需数据迁移

## Migration Plan

1. 部署含新代码的二进制，AutoMigrate 自动加三列（存量 credential.provider 填默认值 codebuddy）
2. 配置补 `trae.callback_port`（默认 18080）；Docker 部署需暴露该端口才能用自动捕获，否则用粘贴导入
3. 回滚：直接回退二进制即可，新增列不影响旧代码读取（gorm 忽略多余列）

## Open Questions

- 无。刷新 job 延期、签到延期、调度延期均已与需求方对齐。
