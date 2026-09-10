# trae-credential-auth 变更规范（增量）

## MODIFIED Requirements

### Requirement: TRAE 登录回调捕获
系统 SHALL 监听回调端口（默认 18080，可配置 `trae.callback_port`）接收 `GET /authorize` 回调（TRAE 官网登录成功后 302 落点，携带 refreshToken/userInfo/userJwt query 参数）：解析回调 → 用 pending 态的 machine_id/device_id 经 ExchangeToken 换取 access token（refreshToken 轮换）→ GetUserInfo 补全 uid/nickname → 写入 `trae_credential` 表（auth_source=web_login）→ 标记 pending 成功并渲染"登录成功"HTML 页。回调 MUST 通过 loginTraceID（machine_id+device_id 派生）关联 pending 态；ExchangeToken 或入库失败 MUST 标记 pending 失败并向浏览器渲染错误页。入库 MUST 只写 `trae_credential`，绝不触碰 `codebuddy_credential`。

#### Scenario: 回调捕获成功入库
- **WHEN** TRAE 登录成功后浏览器 302 到 /authorize 且 refreshToken 有效
- **THEN** 系统完成 ExchangeToken 与 GetUserInfo，凭证写入 `trae_credential`（含 machine_id/device_id/expires_at），浏览器看到"登录成功"提示页，pending 态变为 success 并含 uid 与昵称

#### Scenario: ExchangeToken 失败
- **WHEN** 回调携带的 refreshToken 无法换取有效 token
- **THEN** pending 态标记为 failed（含错误信息），浏览器看到错误页，无凭证入库

### Requirement: 粘贴回调 URL 导入
系统 SHALL 提供 `POST /api/admin/trae/login/import`（会话 Cookie 保护）：接受 `{callback_url}`（TRAE 登录后浏览器地址栏完整回调 URL），执行与回调捕获相同的解析→ExchangeToken→GetUserInfo→写入 `trae_credential` 流程，machine_id/device_id 取回调参数或新生成。该路径为远程部署（回调固定打 127.0.0.1，服务端无法捕获）的必需兜底。同 uid 重复导入 MUST 原位更新已有 `trae_credential` 记录而非新增。

#### Scenario: 粘贴导入成功
- **WHEN** 管理员提交包含有效 refreshToken 的回调 URL
- **THEN** 完成换 token 与用户信息补全，凭证写入 `trae_credential` 并在 TRAE 列表可见

#### Scenario: 同 uid 重复导入原位更新
- **WHEN** 对已存在的同 uid TRAE 凭证再次导入
- **THEN** 原位更新该记录（不新增），token 与 machine_id/device_id 刷新
