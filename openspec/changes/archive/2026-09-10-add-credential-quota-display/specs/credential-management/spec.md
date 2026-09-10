## MODIFIED Requirements

### Requirement: 凭证列表展示
系统 SHALL 提供按 provider 分离的凭证列表端点：`GET /api/admin/codebuddy/credentials` 与 `GET /api/admin/trae/credentials`，各自返回该 provider 的凭证列表（脱敏：bearer_token 仅展示尾部摘要）与该 provider 当前使用中的凭证信息。列表项含：id、user_id、status、auth_source、created_at、签到最后状态、额度字段 `quota`（已探测为 `{total, remaining}`，未探测为 `null`）。codebuddy 端点响应含 `auto_rotation_enabled`；TRAE 端点同。两个列表 MUST NOT 混合对方 provider 的凭证。

#### Scenario: 列表脱敏
- **WHEN** 管理员请求某 provider 凭证列表
- **THEN** 响应中不含完整 bearer_token，仅含尾部若干字符摘要

#### Scenario: 两列表隔离
- **WHEN** 库内同时存在 codebuddy 与 TRAE 凭证
- **THEN** codebuddy 列表只含 codebuddy 凭证，TRAE 列表只含 TRAE 凭证

#### Scenario: 列表携带额度
- **WHEN** 某凭证已探测出额度
- **THEN** 列表该项 quota 为 {total, remaining}
