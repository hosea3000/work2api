# daily-checkin Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 定时自动签到
系统 SHALL 在每天服务器本地时间 09:30（gocron 调度）对全部 active 且未签到的凭证执行签到：`POST {endpoint}/billing/meter/daily-checkin`，携带标准伪装头。相邻凭证签到请求 SHALL 加入随机抖动间隔（可配置，默认 5~20 秒）。

#### Scenario: 每日定时触发
- **WHEN** 服务器时间到达 09:30 且存在 active 凭证
- **THEN** 系统逐一（带抖动）对每个未签到凭证发起签到请求

#### Scenario: 无凭证时静默跳过
- **WHEN** 到达调度时间但无 active 凭证
- **THEN** 不发任何请求，仅记录日志

### Requirement: 签到成功判定
签到结果判定 MUST 与参考实现一致：`code == 0` 或响应 `msg` 包含"已签到"均算成功；成功时记录返回的 credit 数值（如有）与 checked_in_at 时间戳。

#### Scenario: 重复签到视为成功
- **WHEN** 上游返回 `msg` 含"已签到"且 code 非 0
- **THEN** 该次签到记录为成功（幂等）

#### Scenario: 网络失败记录失败
- **WHEN** 签到请求超时或网络错误
- **THEN** 记录一次失败（message 为受控错误类别，不存上游响应体）

### Requirement: 签到记录持久化
每次签到尝试 MUST 写入 `checkin_record` 表：凭证 id、日期（本地时区当天）、成功与否、code、message、credit、attempted_at、checked_in_at。同凭证同日期不允许产生第二次自动签到（当日已签到则跳过）。

#### Scenario: 当日已签到跳过
- **WHEN** 自动调度发现某凭证当天已成功签到
- **THEN** 跳过该凭证，不重复请求上游

### Requirement: 手动签到
系统 SHALL 提供 `POST /api/admin/credentials/:id/daily-checkin` 立即对指定凭证签到（无视当日自动记录，但上游幂等"已签到"仍算成功），并触发一次额度重探测信号（一期：刷新额度缓存占位）。

#### Scenario: 手动签到立即执行
- **WHEN** 管理员对某凭证触发手动签到
- **THEN** 实时执行请求上游，响应返回本次签到结果明细

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

