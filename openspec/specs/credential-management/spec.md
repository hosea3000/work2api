# credential-management Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
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

### Requirement: 凭证删除
系统 SHALL 提供 `DELETE /api/admin/codebuddy/credentials/:id` 与 `DELETE /api/admin/trae/credentials/:id`；删除当前使用中的凭证时 MUST 清空对应 provider 的 `pool_state.current_credential_id` 并重置轮换指针。

#### Scenario: 删除当前凭证
- **WHEN** 删除某 provider 正在使用的凭证
- **THEN** 该 provider 轮换指针重置，下次选择命中下一个可用凭证

### Requirement: 凭证连通性测试
系统 SHALL 提供 `POST /api/admin/codebuddy/credentials/:id/test` 与 `POST /api/admin/trae/credentials/:id/test`，各自返回 `{ok, status_code, detail}`。codebuddy 走 `/v3/config` 模型列表探针；TRAE 走 `GetUserInfo` 探针。失败 detail MUST 为受控错误类别，不透传上游响应体原文。

#### Scenario: 测试有效凭证
- **WHEN** 对 active 凭证执行测试
- **THEN** 响应 `ok: true`，status_code 为上游返回码

#### Scenario: 测试失效凭证
- **WHEN** 对已被上游拒绝的凭证执行测试
- **THEN** 响应 `ok: false`，detail 含错误类别

### Requirement: 凭证调度能力按 provider 对等
系统 SHALL 为两个 provider 提供对等的调度端点：`POST /api/admin/{provider}/credentials/:id/select`（手动设为当前，关闭自动轮换）与 `POST /api/admin/{provider}/credentials/rotation/toggle`（切换自动轮换）。TRAE 凭证 MUST 支持手动选择与轮换开关，与 codebuddy 行为一致。选择后响应 MUST 返回 `auto_rotation_disabled_by_select` 语义与当前凭证。

#### Scenario: TRAE 手动选择
- **WHEN** 管理员对某 TRAE 凭证调用 select
- **THEN** 该凭证成为 TRAE 池当前，`pool_state` 写入 current_credential_id 且 auto_rotation=false，响应含当前凭证

#### Scenario: TRAE 轮换开关
- **WHEN** 管理员对 TRAE 调用 rotation/toggle
- **THEN** TRAE 池自动轮换状态翻转并持久化，响应含 `auto_rotation_enabled`
