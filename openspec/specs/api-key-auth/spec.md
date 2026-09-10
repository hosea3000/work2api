# api-key-auth Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: API Key 生成与存储
系统 SHALL 提供 `sk-` 前缀 API Key 的生成与存储：明文 key SHALL 持久化保存（可随时取回），同时保留 SHA-256 哈希作为校验索引。名称 MUST 非空且全局唯一（不区分大小写）。

#### Scenario: 创建 API Key
- **WHEN** 已登录管理员调用 `POST /api/admin/api-keys` 并提供非空且未占用的名称
- **THEN** 响应返回完整明文 key（`sk-...`），数据库同时保存明文与哈希

#### Scenario: 名称重复被拒绝
- **WHEN** 管理员用已存在的名称（含仅大小写不同，如 `Prod` vs `prod`）创建
- **THEN** 返回 400，不创建新 key

#### Scenario: 名称为空被拒绝
- **WHEN** 管理员提交空名称或仅空白字符
- **THEN** 返回 400，不创建新 key

#### Scenario: 列表返回明文 key
- **WHEN** 管理员调用 `GET /api/admin/api-keys`
- **THEN** 每条记录含完整明文 `key`（前端以掩码显示，供随时复制）

### Requirement: API Key 校验中间件
外部 API 端点（`/codebuddy/openai/v1/*` 与 `/trae/openai/v1/*`）MUST 通过 Bearer Token 提取 API Key，SHA-256 哈希后在数据库比对；不存在、已禁用或格式不符一律 401。

#### Scenario: 禁用的 Key 被拒绝
- **WHEN** 请求携带已禁用 API Key
- **THEN** 返回 401

### Requirement: API Key 删除
系统 SHALL 支持按 key_id 删除 API Key，删除后立即失效。

#### Scenario: 删除后立即失效
- **WHEN** 管理员删除某 API Key 后，客户端再用该 Key 请求 `/openai/v1/models`
- **THEN** 返回 401

