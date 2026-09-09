# Spec: daily-checkin (delta)

## MODIFIED Requirements

### Requirement: 定时自动签到
系统 SHALL 在每天服务器本地时间 09:30（gocron 调度）对全部 active 且未签到的凭证执行签到：`POST {endpoint}/billing/meter/daily-checkin`，携带标准伪装头。相邻凭证签到请求 SHALL 加入随机抖动间隔（可配置，默认 5~20 秒）。

#### Scenario: 每日定时触发
- **WHEN** 服务器时间到达 09:30 且存在 active 凭证
- **THEN** 系统逐一（带抖动）对每个未签到凭证发起签到请求

#### Scenario: 无凭证时静默跳过
- **WHEN** 到达调度时间但无 active 凭证
- **THEN** 不发任何请求，仅记录日志

## ADDED Requirements

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
