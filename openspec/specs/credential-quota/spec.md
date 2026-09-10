# credential-quota Specification

## Purpose
TBD - created by archiving change add-credential-quota-display. Update Purpose after archive.
## Requirements
### Requirement: 凭证额度探测与存储
系统 SHALL 为两个 provider 的凭证探测额度并持久化到 SQLite：`codebuddy_credential` 与 `trae_credential` 各含 `quota_total`、`quota_remaining` 两个可空列（NULL 表示未探测）。codebuddy 额度 MUST 通过 `POST /v2/billing/meter/get-user-resource`（`ProductCode=p_tcaca`）获取，把 `Status==0` 套餐的周期容量（Precise 字段优先）求和为总额度、周期剩余容量求和为剩余额度；TRAE 额度 MUST 复用现有 `EntUsage`（总额度=limit，剩余额度=remain）。带 `enterprise_id` 的 codebuddy 凭证 MUST 跳过探测并保持 NULL。探测失败（网络错误、非 200、响应结构非法）时 MUST 保留该凭证上次成功写入的值，MUST NOT 写入错误状态或清空。

#### Scenario: codebuddy 个人额度探测成功
- **WHEN** 对一张无 enterprise_id 的 active codebuddy 凭证执行探测
- **THEN** quota_total 与 quota_remaining 被写入该凭证行

#### Scenario: TRAE 额度探测成功
- **WHEN** 对一张 active TRAE 凭证执行探测
- **THEN** quota_total=limit、quota_remaining=remain 被写入该凭证行

#### Scenario: 企业凭证跳过
- **WHEN** 对带 enterprise_id 的 codebuddy 凭证执行探测
- **THEN** 不发起个人额度请求，quota 列保持 NULL

#### Scenario: 探测失败保留旧值
- **WHEN** 某凭证上次探测成功、本次探测上游返回 500
- **THEN** 该凭证 quota_total/quota_remaining 保持上次值不变

### Requirement: 额度定时扫描
系统 SHALL 每小时扫描两个 provider 的全部 active 凭证并各执行一次额度探测，结果写回对应凭证行。扫描 MUST NOT 修改凭证的额度列以外的业务字段，MUST NOT 触发凭证池重载。

#### Scenario: 每小时扫描
- **WHEN** 服务运行满一小时
- **THEN** 两个 provider 的 active 凭证额度被刷新

### Requirement: 额度手动刷新端点
系统 SHALL 提供 `POST /api/admin/{provider}/credentials/:id/quota/refresh`（provider 为 `codebuddy` 或 `trae`），对指定凭证同步执行一次额度探测。成功返回 `{quota: {total, remaining}}`；探测失败返回 502；凭证不存在返回 404。

#### Scenario: 刷新成功
- **WHEN** 管理员对一张可探测凭证调用刷新端点
- **THEN** 返回 200 与最新 `{total, remaining}`，并写回凭证行

#### Scenario: 刷新失败
- **WHEN** 上游额度接口返回非 200
- **THEN** 返回 502，凭证行保持原值

### Requirement: 凭证列表内嵌额度
系统 SHALL 在两个 provider 的凭证列表接口中为每项返回 `quota` 字段：已探测为 `{total, remaining}`，未探测为 `null`。系统 MUST NOT 再为 codebuddy 提供独立的 unknown 额度快照端点。

#### Scenario: 已探测展示
- **WHEN** 某凭证 quota 列已有值
- **THEN** 列表该项 quota 为 {total, remaining}

#### Scenario: 未探测为空
- **WHEN** 某凭证从未探测成功
- **THEN** 列表该项 quota 为 null
