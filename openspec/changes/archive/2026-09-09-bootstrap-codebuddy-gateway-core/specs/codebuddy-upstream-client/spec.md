# Spec: codebuddy-upstream-client

## ADDED Requirements

### Requirement: 上游端点与白名单
上游基础端点 SHALL 可配置（默认 `https://copilot.tencent.com`），支持 `https://www.codebuddy.ai`；配置非法时服务启动必须失败（fail-fast），不得回退到未授权端点。

#### Scenario: 非法端点启动失败
- **WHEN** 配置了白名单之外或格式非法的上游端点并启动服务
- **THEN** 服务启动失败并输出明确错误

### Requirement: CLI 伪装请求头生成
系统 SHALL 生成与参考实现 `codebuddy_api_client.py` 一致的请求头集：`Authorization: Bearer`、`X-User-Id`、`X-Domain`、`X-Product: SaaS`、`X-CodeBuddy-Request: 1`、`X-Agent-Intent: craft`、`X-Agent-Purpose: conversation`、`X-IDE-Type/Name: CLI`、`X-IDE-Version`（可配置，默认 2.107.0）、`x-stainless-*`（lang=js、runtime=node、版本号）、`X-Conversation-ID`/`X-Conversation-Request-ID`/`X-Conversation-Message-ID`/`X-Request-ID`（无传入时随机生成）。企业凭证 MUST 附加 `X-Enterprise-Id`/`X-Tenant-Id`，有 department_full_name 时 MUST URL-encode 后加入 `X-Department-Info`。

#### Scenario: 标准头生成
- **WHEN** 用一个个人凭证构造聊天请求头
- **THEN** 输出头集合包含上述全部字段，值格式与参考实现一致（uuid/hex 长度一致）

#### Scenario: 企业凭证附加头
- **WHEN** 凭证含 enterprise_id
- **THEN** 请求头额外包含 `X-Enterprise-Id` 与 `X-Tenant-Id`

### Requirement: 上游端点清单
系统 MUST 实现以下上游调用（与参考实现对齐）：聊天 `POST /v2/chat/completions`（仅流式）；模型 `GET /v3/config`（附加 `X-IDE-Type/Name: CodeBuddyIDE` 与 `X-Product-Version` 头）；签到 `POST /billing/meter/daily-checkin`。

#### Scenario: 聊天端点调用
- **WHEN** 发起聊天代理
- **THEN** 请求发往 `{endpoint}/v2/chat/completions`，携带伪装头与转换后的请求体

#### Scenario: 模型端点调用
- **WHEN** 拉取上游模型
- **THEN** 请求 `GET {endpoint}/v3/config` 使用 IDE 变体头集（非 CLI 头集）

### Requirement: 上游错误处理
上游 401/403 MUST 映射为凭证失效信号（触发摘除）；网络错误/超时 MUST 映射为受控错误类别（如 upstream_unavailable），响应中 MUST NOT 透传上游响应体原文。上游业务错误码（12005/11212/11216/10081）SHALL 映射为可读中文消息。

#### Scenario: 业务错误码映射
- **WHEN** 上游返回 code 12005
- **THEN** 对外错误消息为"企业许可证没有可用席位"（同参考实现映射表）

### Requirement: 超时与重试
上游请求 MUST 设置超时（连接 10s、读 30s，聊天读超时可配置更长）；凭证选择失败（如所选凭证已被摘除）SHALL 支持有限次数的自动换凭证重试（默认 1 次）。

#### Scenario: 换凭证重试
- **WHEN** 首次上游请求返回 401 触发摘除后
- **THEN** 系统自动选择下一凭证重试一次；再失败则返回错误
