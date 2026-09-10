# credential-management 变更规范（增量）

## ADDED Requirements

### Requirement: 凭证 provider 区分
credential 表 SHALL 增加 `provider` 字段（`codebuddy` | `trae`，默认 `codebuddy`）与 TRAE 专用字段 `machine_id`、`device_id`。凭证列表响应 SHALL 包含 provider 字段供前端区分展示。

#### Scenario: TRAE 凭证入库标记
- **WHEN** TRAE 登录闭环完成入库
- **THEN** 凭证记录 provider=trae，且 machine_id/device_id 与登录时生成的一致

#### Scenario: 列表展示 provider
- **WHEN** 管理员请求凭证列表
- **THEN** 每条记录含 provider 字段（codebuddy 或 trae）

### Requirement: codebuddy 调度路径排除 TRAE 凭证
所有 codebuddy 消费方（内存轮换池刷新、token 刷新 job、签到 job、聊天执行路径的凭证查询）MUST 仅纳入 provider=codebuddy 的凭证；TRAE 凭证不得被 codebuddy 上游请求路径选中。

#### Scenario: TRAE 凭证不进入轮换池
- **WHEN** 凭证池刷新时库内同时存在 codebuddy 与 trae 凭证
- **THEN** 仅 provider=codebuddy 的凭证进入池内，聊天请求只会命中 codebuddy 凭证

#### Scenario: 定时任务跳过 TRAE 凭证
- **WHEN** token 刷新与每日签到 job 扫描凭证
- **THEN** 仅处理 provider=codebuddy 的凭证，TRAE 凭证不被 codebuddy 刷新接口触碰

## MODIFIED Requirements

### Requirement: 凭证列表展示
系统 SHALL 提供 `GET /api/admin/credentials`，返回凭证列表（脱敏：bearer_token 仅展示尾部摘要）与当前使用中的凭证信息。列表项含：id、user_id、status、quota 展示字段（一期可为空/未知）、签到最后状态、auth_source、provider、created_at。select / rotation-toggle 等调度操作仅对 provider=codebuddy 凭证有效。

#### Scenario: 列表脱敏
- **WHEN** 管理员请求凭证列表
- **THEN** 响应中不含完整 bearer_token，仅含尾部若干字符摘要

#### Scenario: 对 TRAE 凭证执行调度操作
- **WHEN** 管理员对 provider=trae 的凭证调用 select 或 rotation 相关端点
- **THEN** 返回 400（TRAE 凭证不参与 codebuddy 调度）

## REMOVED Requirements

### Requirement: 凭证添加（手动）
**Reason**: 手动粘贴的裸 bearer_token 无 refresh_token，过期后凭证即失效，属于制造注定死亡的凭证；管理台 UI 已改为双 panel 登录入口（CodeBuddy OAuth + TRAE 网页登录）。后端 `POST /api/admin/credentials` API 保留作为逃生舱，但不再作为规格要求的行为。
**Migration**: 管理员通过 CodeBuddy OAuth 设备授权流或 TRAE 网页登录导入凭证。
