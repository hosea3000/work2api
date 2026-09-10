# daily-checkin Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 定时自动签到
系统 SHALL 在每天服务器本地时间 09:30 调度两个独立的签到任务：codebuddy 任务扫描 `codebuddy_credential` 的 active 凭证，走 `POST {endpoint}/billing/meter/daily-checkin` 一发式；TRAE 任务扫描 `trae_credential` 的 active 凭证，走两步式——先 `POST {ug_host}/trae/api/v2/ug/checkin_credits/status` 查询当日签到状态（`checked_in=true` 则跳过），未签到且 `enable=true` 时调 `POST {ug_host}/trae/api/v2/ug/checkin_credits/claim` 领取。两任务各自仅使用本 provider 的上游端点与请求头（codebuddy 用 CLI 伪装头，trae 用 UgHeaders）。相邻凭证签到请求 SHALL 加入随机抖动间隔（可配置，默认 5~20 秒）。TRAE claim 高峰返回 code=9074 时 SHALL 记录一次失败，不做同日重试。两任务 MUST NOT 读取对方的凭证表。

#### Scenario: 每日定时触发
- **WHEN** 服务器时间到达 09:30 且存在 active 凭证
- **THEN** 两个 provider 的签到任务各自（带抖动）对其凭证发起签到请求

#### Scenario: TRAE 当日已签到跳过
- **WHEN** 某 TRAE 凭证 status 接口返回 checked_in=true
- **THEN** 不调用 claim，记一次成功（幂等）

#### Scenario: TRAE 高峰人数过多
- **WHEN** TRAE claim 响应 code=9074
- **THEN** 记录一次失败（code=9074），当日不再重试

#### Scenario: 无凭证时静默跳过
- **WHEN** 到达调度时间但某 provider 无 active 凭证
- **THEN** 该 provider 任务不发任何请求，仅记录日志

### Requirement: 签到成功判定
签到结果判定 MUST 与参考实现一致：codebuddy 凭证 `code == 0` 或响应 `msg` 包含"已签到"均算成功，成功时记录返回的 credit 数值（如有）与 checked_in_at 时间戳。TRAE 凭证解析 claim 响应体 `{code, message}`，`code == 0` 算成功；status 显示已签到亦算成功（幂等）。响应 schema 与上游实测不符时以实测为准校准解析。

#### Scenario: 重复签到视为成功
- **WHEN** 上游返回 `msg` 含"已签到"且 code 非 0
- **THEN** 该次签到记录为成功（幂等）

#### Scenario: TRAE 未签到且领取成功
- **WHEN** TRAE status 返回未签到且 claim 响应 code=0
- **THEN** 该次签到记录为成功

#### Scenario: 网络失败记录失败
- **WHEN** 签到请求超时或网络错误
- **THEN** 记录一次失败（message 为受控错误类别，不存上游响应体）

### Requirement: 签到记录持久化
每次签到尝试 MUST 写入对应 provider 的签到记录表：codebuddy 写入 `codebuddy_checkin_record`，TRAE 写入 `trae_checkin_record`（各自含凭证 id、日期、成功与否、code、message、credit、attempted_at、checked_in_at，`(credential_id, checkin_date)` 唯一）。同凭证同日期不允许产生第二次自动签到（codebuddy 查记录表判定，TRAE 查上游 status 判定）。两表 MUST NOT 混用。

#### Scenario: 当日已签到跳过
- **WHEN** 自动调度发现某凭证当天已成功签到
- **THEN** 跳过该凭证，不重复请求上游

#### Scenario: 记录写入对应表
- **WHEN** 某 TRAE 凭证签到完成
- **THEN** 记录写入 `trae_checkin_record`，`codebuddy_checkin_record` 不受影响

### Requirement: 手动签到
系统 SHALL 提供按 provider 分离的手动签到端点：`POST /api/admin/codebuddy/credentials/:id/daily-checkin` 与 `POST /api/admin/trae/credentials/:id/daily-checkin`，各自立即对指定凭证签到（无视当日自动记录，上游幂等"已签到"仍算成功）。codebuddy 走一发式，TRAE 走 status→claim 两步式。

#### Scenario: 手动签到立即执行
- **WHEN** 管理员对某凭证触发手动签到
- **THEN** 实时执行对应 provider 的签到流程，响应返回本次签到结果明细

### Requirement: OAuth 凭证定时刷新
系统 SHALL 以每小时为周期（启动后立即执行首轮，此后 ticker 驱动）扫描全部 `auth_source=oauth` 的凭证：对满足刷新条件（refresh_token 非空、refresh_expires_at 未过期、expires_at 临期即 now ≥ expires_at − 24h 或缺失）的凭证调用上游刷新端点。相邻刷新请求带随机抖动；刷新成功原位更新凭证并刷新轮换池快照；401/403 将凭证摘除；其余失败记日志等待下轮。手动添加（manual）的凭证 MUST NOT 参与刷新。

#### Scenario: 临期凭证被刷新
- **WHEN** 某 OAuth 凭证 expires_at 距 now 不足 24 小时且 refresh_token 有效
- **THEN** 每轮扫描中该凭证被刷新，bearer_token 与 expires_at 更新，凭证保持 active

#### Scenario: refresh_token 已过期
- **WHEN** 某 OAuth 凭证 refresh_expires_at 已过
- **THEN** 不发起刷新请求；待 access token 自然过期后按既有摘除规则处理

#### Scenario: 手动凭证不刷新
- **WHEN** 扫描遍历到 auth_source=manual 的凭证
- **THEN** 直接跳过

### Requirement: TRAE 积分查询
系统 SHALL 在 TRAE 签到完成后调用 `POST {ug_host}/trae/api/v2/pay/ide_user_ent_usage` 聚合积分（user_entitlement_pack_list 中 credits_limit 求和减 usage.credits_amount 已用），查询失败仅记日志不影响签到结果。查询结果 SHALL 存入凭证上下文供管理台展示（剩余积分）。

#### Scenario: 签到后积分随列表展示
- **WHEN** 某 TRAE 凭证完成签到且 EntUsage 查询成功
- **THEN** 管理台凭证列表可见该凭证剩余积分

#### Scenario: 积分查询失败不阻塞签到
- **WHEN** EntUsage 请求失败
- **THEN** 签到结果照常落库，仅记录查询失败日志

