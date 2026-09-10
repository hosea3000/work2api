## ADDED Requirements

### Requirement: 总览页 API 入口地址展示

管理台总览页 SHALL 展示两张 API 入口卡片，分别标注 CodeBuddy 与 TRAE，对应网关的两个 OpenAI 兼容入口 `/codebuddy/openai/v1` 与 `/trae/openai/v1`。每个卡片 SHALL 展示完整绝对地址，前缀取自浏览器当前地址的 origin（`window.location.origin`），MUST NOT 仅展示相对路径。入口地址 MUST 由前端基于浏览器 origin 拼接，不依赖 `/api/admin/status` 返回的 URL 字段。总览页 MUST NOT 展示 Anthropic 入口卡片或 Claude Code 模型 ID 提示。

#### Scenario: 展示两个入口的完整地址

- **WHEN** 管理员打开总览页
- **THEN** 页面展示 CodeBuddy 入口 `http://<当前访问 host>/codebuddy/openai/v1` 与 TRAE 入口 `http://<当前访问 host>/trae/openai/v1`，前缀与浏览器地址栏 origin 一致

#### Scenario: 复制入口地址

- **WHEN** 管理员点击某入口卡片的复制按钮
- **THEN** 该入口的完整绝对地址写入剪贴板

#### Scenario: 不再展示 Anthropic 入口

- **WHEN** 管理员打开总览页
- **THEN** 页面不出现「Anthropic 客户端入口」卡片，也不出现 Claude Code 模型 ID 提示
