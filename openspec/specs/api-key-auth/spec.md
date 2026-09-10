# api-key-auth Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: API Key 生成与存储
系统 SHALL 提供 `sk-` 前缀 API Key 的生成：明文仅在创建响应中返回一次，数据库只存 key 的 SHA-256 哈希、名称、创建时间与启用状态。

#### Scenario: 创建 API Key
- **WHEN** 已登录管理员调用 `POST /api/admin/api-keys` 并提供名称
- **THEN** 响应返回完整明文 key（`sk-...`），数据库记录为哈希值，明文不可再次获取

#### Scenario: 明文仅展示一次
- **WHEN** 管理员调用 `GET /api/admin/api-keys`
- **THEN** 列表仅含 key 尾部摘要（如 `...abcd`）、名称与元数据，不含明文

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

