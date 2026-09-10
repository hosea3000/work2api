# credential-pool Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 凭证 SQLite 存储
凭证 MUST 存储于 SQLite `credential` 表，字段包含：id（UUID）、bearer_token、user_id、account_uid、domain、enterprise_id、department_full_name、auth_source（manual/oauth）、status（active/expired/disabled）、created_at、updated_at、expires_at，以及 OAuth 扩展列：refresh_token、refresh_expires_at、expires_in、session_state、scope、last_refresh_at。

#### Scenario: 手动添加凭证
- **WHEN** 管理员提交 bearer_token 添加凭证
- **THEN** 系统解码 JWT 提取 user_id（参考实现 `_extract_user_info` 逻辑），入库为 active 状态，进入轮换池

#### Scenario: OAuth 认证成功入库
- **WHEN** 设备授权轮询成功
- **THEN** 凭证以 auth_source=oauth 入库：bearer_token=accessToken，expires_at/expires_in 取自 token 响应，domain/enterprise_id 取自 token 响应（非 JWT），account_uid 取第一个 pluginEnabled 账号的 uid，refresh_token 与 refresh_expires_at 原样保存，入库后进入轮换池

#### Scenario: 刷新成功更新凭证
- **WHEN** 临期 OAuth 凭证刷新成功
- **THEN** 原记录原位更新：bearer_token/expires_at/expires_in/last_refresh_at 替换为新值；响应未含新 refresh_token 时保留旧 refresh_token

### Requirement: 内存轮换池与 round-robin
系统 SHALL 维护内存凭证池，按 round-robin 选择凭证：支持配置每 N 次请求切换（默认 1）；选择操作 MUST 并发安全（互斥锁），返回凭证 ID + 凭证数据的原子快照。

#### Scenario: 每 N 次请求轮换
- **WHEN** 轮换计数配置为 2，连续发起 4 次聊天请求
- **THEN** 请求 1、2 使用凭证 A，请求 3、4 使用凭证 B（池内有 A、B 两个 active 凭证）

#### Scenario: 并发选择安全
- **WHEN** 多个请求并发选择凭证
- **THEN** 每次选择返回一致的 (id, credential) 快照，计数不丢不重

### Requirement: 失效凭证摘除
当上游返回 401/403 时，系统 SHALL 将该凭证标记为 expired（或按错误类型 disabled）并立即从轮换候选中排除，下次选择跳过；当池内无可用凭证时聊天请求返回 503。

#### Scenario: 401 触发摘除
- **WHEN** 某次请求上游返回 401
- **THEN** 该凭证状态变为 expired，后续选择不再命中；若池中还有其他凭证则自动切换重试或由下一次请求命中新凭证

#### Scenario: 池空返回 503
- **WHEN** 所有凭证均失效后发起新请求
- **THEN** 返回 503 无可用凭证错误

### Requirement: 轮换开关与手动选择
系统 SHALL 提供 `POST /api/admin/credentials/rotation/toggle` 切换自动轮换开关；提供 `POST /api/admin/credentials/:id/select` 手动指定当前凭证（选择后自动轮换关闭，与参考实现 `auto_rotation_disabled_by_select` 行为一致）。

#### Scenario: 手动选择关闭自动轮换
- **WHEN** 管理员选择某凭证
- **THEN** 响应含 `auto_rotation_disabled_by_select: true`，后续请求固定使用该凭证

#### Scenario: 重新开启自动轮换
- **WHEN** 管理员调用 rotation/toggle 开启轮换
- **THEN** 恢复 round-robin 策略，响应含当前凭证信息

