# add-trae-credentials 任务

## 1. 数据模型与过滤基线

- [x] 1.1 `internal/model/gateway.go`：Credential 增加 `Provider`（size:16, default:'codebuddy', index）、`MachineID`、`DeviceID` 三列
- [x] 1.2 `credential_pool.go` LoadAll/Refresh 过滤 `provider == "codebuddy"`，并补测试：池刷新后 trae 凭证不在池内
- [x] 1.3 `token_refresh.go` 扫描跳过 `provider != "codebuddy"`，补测试
- [x] 1.4 `checkin_service.go`/`checkin_job.go` 扫描仅含 codebuddy 凭证，补测试

## 2. TRAE 上游客户端（移植）

- [x] 2.1 新建 `internal/upstream/trae/constants.go`：hosts/ClientID/IdeVersion/端点常量（对齐 trae2api-web upstream/constants.go）
- [x] 2.2 新建 `internal/upstream/trae/login.go`：BuildLoginURL、machineTraceID、ParseCallback（含双层 JSON 解析、TokenExpireAt 毫秒归一化、昵称乱码兜底）+ 单测（用参考实现 callback_test.go 的用例对齐）
- [x] 2.3 新建 `internal/upstream/trae/client.go`：ExchangeToken、GetUserInfo（OAuthHeaders 对齐）+ httptest 单测

## 3. TRAE 登录服务

- [x] 3.1 新建 `internal/service/trae_login_service.go`：pending 状态机（start/result/cancel/取消与回调竞态，互斥锁保护，TTL 10 分钟）
- [x] 3.2 实现回调处理与粘贴导入共享的入库流程：ParseCallback → ExchangeToken（machine/device 取 pending 或回调参数）→ GetUserInfo → CreateCredential(provider=trae, auth_source=web_login, machine_id/device_id/expires_at) → 标记 pending
- [x] 3.3 失败路径：ExchangeToken/GetUserInfo/入库失败标记 pending failed；未知回调（无 trace 匹配）拒绝且无副作用 + 单测

## 4. 路由与 HTTP 编排

- [x] 4.1 config：`CodeBuddyConfig` 或新 `TraeConfig` 增加 `trae.callback_port`（默认 18080）
- [x] 4.2 新建 `internal/handler/trae_auth_handler.go`：start/result/cancel/import 四端点（SessionAuth）+ `GET /authorize` 回调 handler（公共，渲染成功/错误 HTML 页）
- [x] 4.3 `internal/router/gateway.go` 注册 `/api/admin/trae/login/*`（admin 组）与 `GET /authorize`（engine 根级）
- [x] 4.4 bootstrap/server 启动编排：第二 http.Server 监听 callback_port（端口占用仅告警不 fatal），注册同一 /authorize handler
- [x] 4.5 handler 单测：start 返回 login_url/pending_id；import 有效/无效回调 URL；未登录 401

## 5. 凭证列表与调度操作收紧

- [x] 5.1 `credential_service.view`/`credential_handler.List` 响应增加 provider 字段
- [x] 5.2 Select/rotation 相关路径对 provider=trae 凭证返回 400 + 测试；Test/MarkExpired 仅允许 codebuddy（trae 无上游测试能力，二期）

## 6. 前端

- [x] 6.1 `web/src/api/admin.ts`：新增 traeLoginStart/traeLoginResult/traeLoginCancel/traeLoginImport API 与 CredentialRecord.provider 类型
- [x] 6.2 `CredentialsView.vue`：登录区改双等分 panel（左 CodeBuddy 登录不动，右 TRAE 登录：开始认证→新窗口→2s 轮询→成功刷新列表 + 粘贴回调 URL 导入表单）
- [x] 6.3 凭证列表加 provider 标签；trae 行隐藏 select/rotation/签到/测试按钮
- [x] 6.4 前端构建验证（pnpm build）+ 现有测试通过

## 7. 验证收尾

- [x] 7.1 全链路手工验证：本机登录闭环（自动捕获）+ 远程模拟（粘贴导入）+ 列表展示
- [x] 7.2 `make wire && make sqlc && make swag`（如有生成物涉及）+ `go build ./...` + `go test ./...` 全绿
