# daily-checkin 变更规范（增量）

## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: 凭证连通性测试按 provider 分派
**Reason**: 连通性测试已按 provider 拆为独立端点（见 `credential-management`），不再需要一个在单一 service 内按 provider 分派的要求。
**Migration**: 无。
