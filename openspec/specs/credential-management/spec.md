# credential-management Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: 凭证列表展示
系统 SHALL 提供 `GET /api/admin/credentials`，返回凭证列表（脱敏：bearer_token 仅展示尾部摘要）与当前使用中的凭证信息。列表项含：id、user_id、status、quota 展示字段（一期可为空/未知）、签到最后状态、auth_source、created_at。

#### Scenario: 列表脱敏
- **WHEN** 管理员请求凭证列表
- **THEN** 响应中不含完整 bearer_token，仅含尾部若干字符摘要

### Requirement: 凭证添加（手动）
系统 SHALL 提供 `POST /api/admin/credentials`，接受 `bearer_token`，JWT 解析失败时拒绝（400）；解析成功提取 user_id 并入库。

#### Scenario: 无效 token 被拒绝
- **WHEN** 提交的 bearer_token 不是可解析的合法 JWT
- **THEN** 返回 400，凭证不入库

#### Scenario: 重复凭证去重
- **WHEN** 提交与现有凭证相同 user_id 的 token
- **THEN** 按参考实现行为处理：生成带随机后缀的唯一记录或返回冲突提示（实现时与参考实现对齐，二选一并在设计中记录）

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

