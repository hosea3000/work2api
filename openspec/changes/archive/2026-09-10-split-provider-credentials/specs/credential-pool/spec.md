# credential-pool 变更规范（增量）

## MODIFIED Requirements

### Requirement: 凭证 SQLite 存储
凭证 MUST 按 provider 分表存储：codebuddy 凭证存于 `codebuddy_credential`，TRAE 凭证存于 `trae_credential`，两表互不引用。codebuddy 表含 `id、bearer_token、user_id、account_uid、domain、enterprise_id、department_full_name、auth_source(manual|oauth)、status、expires_at、refresh_token、refresh_expires_at、expires_in、session_state、scope、last_refresh_at、nickname、preferred_username、email、created_at、updated_at`；TRAE 表含 `id、bearer_token、user_id、machine_id、device_id、auth_source(web_login)、status、expires_at、refresh_token、last_refresh_at、nickname、email、created_at、updated_at`。两表 MUST NOT 含 `provider` 列。系统 MUST NOT 提供跨两表的统一凭证查询。

#### Scenario: 手动添加 codebuddy 凭证
- **WHEN** 管理员提交 bearer_token 添加 codebuddy 凭证
- **THEN** 记录写入 `codebuddy_credential`，解码 JWT 提取 user_id，status=active，进入 codebuddy 池

#### Scenario: OAuth 认证成功入库
- **WHEN** codebuddy 设备授权轮询成功
- **THEN** 凭证写入 `codebuddy_credential`（auth_source=oauth，含 refresh_token/expires_at），进入 codebuddy 池

#### Scenario: TRAE 登录入库
- **WHEN** TRAE 网页登录闭环完成
- **THEN** 凭证写入 `trae_credential`（auth_source=web_login，含 machine_id/device_id），进入 TRAE 池

### Requirement: 内存轮换池与 round-robin
系统 SHALL 维护两个完全独立的内存轮换池（codebuddy 池、TRAE 池），各自按 round-robin 选择凭证（支持配置每 N 次请求切换，默认 1）；选择操作 MUST 并发安全（互斥锁），返回凭证快照。两池 MUST NOT 互相感知：codebuddy 池只装 `codebuddy_credential` 的 active 凭证，TRAE 池只装 `trae_credential` 的 active 凭证。

#### Scenario: 两池独立轮换
- **WHEN** codebuddy 池有 A、B，TRAE 池有 X、Y，各自连续发起请求
- **THEN** codebuddy 请求在 A/B 间轮换，TRAE 请求在 X/Y 间轮换，互不影响

#### Scenario: 并发选择安全
- **WHEN** 多个请求并发选择同一池
- **THEN** 每次选择返回一致的凭证快照，计数不丢不重

### Requirement: 失效凭证摘除
当上游返回 401/403 时，系统 SHALL 将该凭证标记为 expired（写对应 provider 表）并立即从对应池的候选中排除，下次选择跳过；当某池内无可用凭证时，对应 provider 的聊天请求返回 503。

#### Scenario: 401 触发摘除
- **WHEN** 某 codebuddy 凭证上游返回 401
- **THEN** 该记录 status=expired，后续 codebuddy 选择不再命中；TRAE 池不受影响

#### Scenario: 池空返回 503
- **WHEN** 某 provider 池内所有凭证失效
- **THEN** 该 provider 的聊天请求返回 503 无可用凭证

### Requirement: 当前凭证与轮换开关持久化
系统 SHALL 提供 `pool_state` 表（`provider` 主键、`auto_rotation`、`current_credential_id`），持久化每个 provider 的当前凭证与轮换开关。管理员手动选择某凭证时 MUST 写入 `current_credential_id` 并将 `auto_rotation` 置 false；切换轮换开关 MUST 更新 `auto_rotation`。服务启动加载池时 MUST 读取 `pool_state` 恢复当前指针与开关；删除当前凭证时 MUST 清空该 provider 的 `current_credential_id`。开关状态与当前选择 MUST 在重启后保持。

#### Scenario: 手动选择并重启保持
- **WHEN** 管理员选择凭证 A 后重启服务
- **THEN** 该 provider 池恢复当前指针到 A，自动轮换保持关闭

#### Scenario: 开启自动轮换
- **WHEN** 管理员开启某 provider 的自动轮换
- **THEN** `pool_state.auto_rotation=true`，池恢复 round-robin，重启后仍为开启

#### Scenario: 删除当前凭证重置
- **WHEN** 删除正在使用的凭证
- **THEN** `pool_state.current_credential_id` 清空，下次选择命中池内下一个可用凭证

## REMOVED Requirements

### Requirement: TRAE 独立轮换池
**Reason**: 被上述"两池完全独立 + 持久化"的通用要求取代——TRAE 池不再是一个附加于 codebuddy 池的特例，而是与 codebuddy 完全对称的一等池。
**Migration**: 无存量迁移（清库重建）。
