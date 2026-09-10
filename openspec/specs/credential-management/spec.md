# credential-management Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 凭证列表展示
系统 SHALL 提供 `GET /api/admin/credentials`，返回凭证列表（脱敏：bearer_token 仅展示尾部摘要）与当前使用中的凭证信息。列表项含：id、user_id、status、quota 展示字段（一期可为空/未知）、签到最后状态、auth_source、provider、created_at。select / rotation-toggle 等调度操作仅对 provider=codebuddy 凭证有效。

#### Scenario: 列表脱敏
- **WHEN** 管理员请求凭证列表
- **THEN** 响应中不含完整 bearer_token，仅含尾部若干字符摘要

#### Scenario: 对 TRAE 凭证执行调度操作
- **WHEN** 管理员对 provider=trae 的凭证调用 select 或 rotation 相关端点
- **THEN** 返回 400（TRAE 凭证不参与 codebuddy 调度）

### Requirement: 凭证删除
系统 SHALL 提供 `DELETE /api/admin/credentials/:id`；删除当前使用中的凭证时 MUST 重置轮换指针。

#### Scenario: 删除当前凭证
- **WHEN** 删除正在使用的凭证
- **THEN** 轮换指针重置，下次选择命中下一个可用凭证

### Requirement: 凭证连通性测试（打桩）
系统 SHALL 提供 `POST /api/admin/credentials/:id/test` 端点：一期实现真实测试（用该凭证请求上游模型列表 `/v3/config`），返回 `{ok, status_code, detail}`。

#### Scenario: 测试有效凭证
- **WHEN** 对 active 凭证执行测试
- **THEN** 响应 `ok: true`，status_code 为上游返回码

#### Scenario: 测试失效凭证
- **WHEN** 对已被上游拒绝的凭证执行测试
- **THEN** 响应 `ok: false`，detail 含错误类别（不透传上游响应体原文）

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
