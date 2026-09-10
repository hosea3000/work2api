# trae-credential-auth Specification

## Purpose
TRAE SOLO 账号网页登录认证闭环：登录 URL 生成、pending 状态机、回调捕获与粘贴导入、ExchangeToken 换取凭证入库。
## Requirements

### Requirement: TRAE 登录链接生成
系统 SHALL 提供 `POST /api/admin/trae/login/start`（会话 Cookie 保护）：生成随机 machine_id 与 device_id（各 hex32），创建 pending 登录态（内存存储，TTL 10 分钟），构造 TRAE 官网登录 URL（`https://www.trae.cn/authorization`，携带 client_id、login_trace_id、auth_callback_url 等参数，对齐 trae2api-web BuildLoginURL），返回 `{login_url, pending_id, callback_url}`。

#### Scenario: 启动登录返回链接
- **WHEN** 已登录管理员调用 start
- **THEN** 响应含 login_url（TRAE 官网授权页）、pending_id 与 callback_url（`http://127.0.0.1:{callback_port}/authorize`），且 machine_id/device_id 与 pending 态绑定

#### Scenario: 未登录调用
- **WHEN** 无有效会话调用 start
- **THEN** 返回 401

### Requirement: TRAE 登录回调捕获
系统 SHALL 监听回调端口（默认 18080，可配置 `trae.callback_port`）接收 `GET /authorize` 回调（TRAE 官网登录成功后 302 落点，携带 refreshToken/userInfo/userJwt query 参数）：解析回调 → 用 pending 态的 machine_id/device_id 经 ExchangeToken 换取 access token（refreshToken 轮换）→ GetUserInfo 补全 uid/nickname → 写入 `trae_credential` 表（auth_source=web_login）→ 标记 pending 成功并渲染"登录成功"HTML 页。回调 MUST 通过 loginTraceID（machine_id+device_id 派生）关联 pending 态；ExchangeToken 或入库失败 MUST 标记 pending 失败并向浏览器渲染错误页。入库 MUST 只写 `trae_credential`，绝不触碰 `codebuddy_credential`。

#### Scenario: 回调捕获成功入库
- **WHEN** TRAE 登录成功后浏览器 302 到 /authorize 且 refreshToken 有效
- **THEN** 系统完成 ExchangeToken 与 GetUserInfo，凭证写入 `trae_credential`（含 machine_id/device_id/expires_at），浏览器看到"登录成功"提示页，pending 态变为 success 并含 uid 与昵称

#### Scenario: ExchangeToken 失败
- **WHEN** 回调携带的 refreshToken 无法换取有效 token
- **THEN** pending 态标记为 failed（含错误信息），浏览器看到错误页，无凭证入库

#### Scenario: 未知回调（无匹配 pending）
- **WHEN** /authorize 收到无法关联任何 pending 态的回调
- **THEN** 返回错误提示页，不产生入库副作用

### Requirement: 粘贴回调 URL 导入
系统 SHALL 提供 `POST /api/admin/trae/login/import`（会话 Cookie 保护）：接受 `{callback_url}`（TRAE 登录后浏览器地址栏完整回调 URL），执行与回调捕获相同的解析→ExchangeToken→GetUserInfo→写入 `trae_credential` 流程，machine_id/device_id 取回调参数或新生成。该路径为远程部署（回调固定打 127.0.0.1，服务端无法捕获）的必需兜底。同 uid 重复导入 MUST 原位更新已有 `trae_credential` 记录而非新增。

#### Scenario: 粘贴导入成功
- **WHEN** 管理员提交包含有效 refreshToken 的回调 URL
- **THEN** 完成换 token 与用户信息补全，凭证写入 `trae_credential` 并在 TRAE 列表可见

#### Scenario: 同 uid 重复导入原位更新
- **WHEN** 对已存在的同 uid TRAE 凭证再次导入
- **THEN** 原位更新该记录（不新增），token 与 machine_id/device_id 刷新

#### Scenario: 无效回调 URL
- **WHEN** 提交的 URL 缺失 refreshToken 与 userJwt.Token
- **THEN** 返回 400，无凭证入库

### Requirement: 登录结果轮询
系统 SHALL 提供 `GET /api/admin/trae/login/result?pending_id=`（会话 Cookie 保护）：返回 `{state: pending|success|failed}`，success 时附 uid 与 nickname，failed 时附 error。pending 态过期（10 分钟）或取消后查询返回 404。

#### Scenario: 轮询到成功
- **WHEN** 回调已完成入库后管理员查询 result
- **THEN** 响应 `{state: "success", uid, nickname}`

#### Scenario: pending 过期
- **WHEN** 超过 10 分钟未完成登录后查询
- **THEN** 返回 404

### Requirement: 取消登录
系统 SHALL 提供 `POST /api/admin/trae/login/cancel`（会话 Cookie 保护）：删除指定 pending 态。

#### Scenario: 取消后回调到达
- **WHEN** 管理员取消登录后回调才到达
- **THEN** 回调因无匹配 pending 被拒绝，不产生入库副作用

### Requirement: TRAE 凭证定时刷新
系统 SHALL 以每小时为周期（启动后立即执行首轮，此后 ticker 驱动）扫描全部 `provider=trae && auth_source=web_login` 的凭证：对满足刷新条件（refresh_token 非空、expires_at 临期即 now ≥ expires_at − 24h）的凭证调用 TRAE `ExchangeToken`（refreshToken 轮换）。相邻刷新请求 SHALL 加入随机抖动（5~20 秒，对齐 codebuddy 刷新节奏）。刷新成功 SHALL 原位更新 bearer_token、refresh_token（响应含新值时）、expires_at 与 last_refresh_at；刷新响应缺失 refreshToken 时 SHALL 沿用旧值。刷新判定 MUST 与 codebuddy 刷新（`auth_source=oauth`）相互独立，两套扫描 MUST NOT 触碰对方 provider 的凭证。

#### Scenario: 临期 TRAE 凭证被刷新
- **WHEN** 某 provider=trae 凭证 expires_at 距 now 不足 24 小时且 refresh_token 非空
- **THEN** 每轮扫描中该凭证经 ExchangeToken 刷新，bearer_token/refresh_token/expires_at/last_refresh_at 更新，凭证保持 active

#### Scenario: 刚导入的凭证空转
- **WHEN** 某 TRAE 凭证离过期尚远（> 24h）
- **THEN** 每小时扫描跳过该凭证，不发上游请求

#### Scenario: refreshToken 轮换失效
- **WHEN** ExchangeToken 返回 refresh_failed（上游拒绝 refreshToken）
- **THEN** 凭证被标记 expired，等待管理员重新登录导入（既有"重新登录"链路）

#### Scenario: codebuddy 凭证不被 trae 扫描触碰
- **WHEN** TRAE 刷新扫描遍历凭证列表
- **THEN** 仅处理 provider=trae 的凭证，provider=codebuddy（含空 provider 存量数据）凭证一律跳过
