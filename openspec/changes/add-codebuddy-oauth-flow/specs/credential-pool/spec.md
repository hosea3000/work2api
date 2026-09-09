# Spec: credential-pool (delta)

## MODIFIED Requirements

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
