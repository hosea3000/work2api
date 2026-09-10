# api-key-auth 变更规范（增量）

## MODIFIED Requirements

### Requirement: API Key 校验中间件
外部 API 端点（`/codebuddy/openai/v1/*` 与 `/trae/openai/v1/*`）MUST 通过 Bearer Token 提取 API Key，SHA-256 哈希后在数据库比对；不存在、已禁用或格式不符一律 401。

#### Scenario: 禁用的 Key 被拒绝
- **WHEN** 请求携带已禁用 API Key
- **THEN** 返回 401
