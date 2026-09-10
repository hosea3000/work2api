# daily-checkin 变更规范（增量）

## MODIFIED Requirements

### Requirement: 定时自动签到
系统 SHALL 在每天服务器本地时间 09:30（gocron 调度）对全部 active 且未签到的凭证执行签到：codebuddy 凭证（含空 provider 存量数据）走 `POST {endpoint}/billing/meter/daily-checkin` 一发式；TRAE 凭证走两步式——先 `POST {ug_host}/trae/api/v2/ug/checkin_credits/status` 查询当日签到状态（`checked_in=true` 则跳过），未签到且 `enable=true` 时调 `POST {ug_host}/trae/api/v2/ug/checkin_credits/claim` 领取。两类凭证 MUST 各自仅使用本 provider 的上游端点与请求头（codebuddy 用 CLI 伪装头，trae 用 UgHeaders：Cloud-IDE-JWT + X-Device-Id + X-User-Region: CN）。相邻凭证签到请求 SHALL 加入随机抖动间隔（可配置，默认 5~20 秒）。TRAE claim 高峰返回 code=9074（人数太多）时 SHALL 记录一次失败，不做同日重试。

#### Scenario: 每日定时触发
- **WHEN** 服务器时间到达 09:30 且存在 active 凭证
- **THEN** 系统逐一（带抖动）对每个未签到凭证按其 provider 发起签到请求

#### Scenario: TRAE 当日已签到跳过
- **WHEN** 某 TRAE 凭证 status 接口返回 checked_in=true
- **THEN** 不调用 claim，记一次成功（幂等）

#### Scenario: TRAE 高峰人数过多
- **WHEN** TRAE claim 响应 code=9074
- **THEN** 记录一次失败（code=9074，message 含"人数过多"类上游文案），当日不再重试

#### Scenario: 无凭证时静默跳过
- **WHEN** 到达调度时间但无 active 凭证
- **THEN** 不发任何请求，仅记录日志

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
每次签到尝试 MUST 写入 `checkin_record` 表（codebuddy 与 TRAE 凭证共用，含凭证 id、日期、成功与否、code、message、credit、attempted_at、checked_in_at）。同凭证同日期不允许产生第二次自动签到（当日已签到则跳过——codebuddy 查记录表判定，TRAE 查上游 status 判定）。

#### Scenario: 当日已签到跳过
- **WHEN** 自动调度发现某凭证当天已成功签到（codebuddy 查记录表 / TRAE 查上游 status）
- **THEN** 跳过该凭证，不重复请求上游

### Requirement: 手动签到
系统 SHALL 提供 `POST /api/admin/credentials/:id/daily-checkin` 立即对指定凭证签到（无视当日自动记录，上游幂等"已签到"仍算成功）：codebuddy 走既有一发式，TRAE 走 status→claim 两步式。对 provider=trae 凭证调用 SHALL 正常执行（不再拒绝）。

#### Scenario: 手动签到立即执行
- **WHEN** 管理员对某凭证（codebuddy 或 trae）触发手动签到
- **THEN** 实时执行对应 provider 的签到流程，响应返回本次签到结果明细

### Requirement: TRAE 积分查询
系统 SHALL 在 TRAE 签到完成后调用 `POST {ug_host}/trae/api/v2/pay/ide_user_ent_usage` 聚合积分（user_entitlement_pack_list 中 credits_limit 求和减 usage.credits_amount 已用），查询失败仅记日志不影响签到结果。查询结果 SHALL 存入凭证上下文供管理台展示（剩余积分）。

#### Scenario: 签到后积分随列表展示
- **WHEN** 某 TRAE 凭证完成签到且 EntUsage 查询成功
- **THEN** 管理台凭证列表可见该凭证剩余积分

#### Scenario: 积分查询失败不阻塞签到
- **WHEN** EntUsage 请求失败
- **THEN** 签到结果照常落库，仅记录查询失败日志

### Requirement: 凭证连通性测试按 provider 分派
`CredentialService.Test` SHALL 按 provider 分派探针：codebuddy 凭证走上游模型列表（既有行为），TRAE 凭证走 `GetUserInfo`（携带 access token 查询账号信息，成功即视为连通）。对 provider=trae 凭证调用测试 SHALL 正常执行（不再返回 not_supported_for_provider）。

#### Scenario: TRAE 凭证连通性测试
- **WHEN** 管理员对某 provider=trae 凭证触发连通性测试
- **THEN** 系统调用 GetUserInfo，成功返回测试通过，失败返回错误信息
