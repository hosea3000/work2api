# admin-web-frontend Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: Vue 管理台集成
系统 SHALL 将 codebuddy2api 的 Vue3 管理台源码复制至 `web/`，构建产物由 Go 服务静态托管；前端路由未匹配路径回退 `index.html`（SPA history 模式 hash 路由兼容）。

#### Scenario: 静态托管与回退
- **WHEN** 浏览器访问 `/` 或任意前端路由
- **THEN** 返回 SPA 页面；访问 `/assets/*` 返回构建静态资源

### Requirement: API 契约兼容
后端 MUST 完整实现前端实际调用的端点：`/auth/login`、`/auth/session`、`/auth/logout`、`/api/admin/api-keys`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials/:id/select`、`/api/admin/codebuddy/credentials/:id/test`、`/api/admin/codebuddy/credentials/:id/daily-checkin`、`/api/admin/codebuddy/credentials/:id/quota/refresh`、`/api/admin/codebuddy/credentials/rotation/toggle`、`/api/admin/trae/credentials`（GET/DELETE）、`/api/admin/trae/credentials/:id/select`、`/api/admin/trae/credentials/:id/test`、`/api/admin/trae/credentials/:id/daily-checkin`、`/api/admin/trae/credentials/:id/quota/refresh`、`/api/admin/trae/credentials/rotation/toggle`、`/api/admin/playground/codebuddy/openai/v1/models`、`/api/admin/playground/codebuddy/openai/v1/chat/completions`、`/api/admin/playground/trae/openai/v1/models`、`/api/admin/playground/trae/openai/v1/chat/completions`、`/api/admin/status`。响应 JSON 结构（字段名与类型）与前端类型定义保持一致。

#### Scenario: 前端凭证页可用
- **WHEN** 管理员登录后进入凭证管理页
- **THEN** 两个 provider 的列表加载、选择、签到、测试、删除、额度刷新、轮换开关全部正常工作

### Requirement: 打桩端点返回空数据
对暂不实现的功能，后端 MUST 提供结构与参考实现一致但内容为空的响应，保证前端视图不崩溃：`GET/PUT /api/admin/settings`（返回默认设置）。额度不再打桩，MUST 返回真实探测值或 null。`/api/admin/status` 与 `GET /api/admin/stats/overview` MUST NOT 打桩：前者返回真实服务状态与合并凭证计数，后者返回真实请求统计（详见 `request-metrics` 能力）。

#### Scenario: 额度不再打桩
- **WHEN** 管理员查看凭证列表
- **THEN** 额度字段为真实值或 null，不再是 unknown 快照

#### Scenario: 设置仍打桩
- **WHEN** 管理员调用 GET/PUT `/api/admin/settings`
- **THEN** 返回与默认设置一致的结构，不报错

### Requirement: API Key 页可用
管理台 API 密钥页面 MUST 支持创建（名称唯一）、列表（key 隐藏显示 + 随时复制）、删除。列表每行 SHALL 以掩码形式展示 key 并提供复制按钮，点击复制即写入剪贴板。创建成功后 SHALL 直接刷新列表（不再有"仅显示一次、关闭后无法查看"的强制告警）。创建时名称重复 SHALL 提示"名称已存在"且不创建。

#### Scenario: 随时复制
- **WHEN** 管理员在列表某行点击复制按钮
- **THEN** 该行完整 key 写入剪贴板

#### Scenario: key 隐藏显示
- **WHEN** 管理员查看列表
- **THEN** key 以掩码形式展示，不直接显示完整明文

#### Scenario: 名称重复提示
- **WHEN** 创建时输入的名称与现有 key 重复
- **THEN** 页面提示"名称已存在"，不创建新 key

#### Scenario: 创建后列表可复制
- **WHEN** 管理员创建成功
- **THEN** 列表刷新，新 key 出现在列表中且可复制，无需一次性保存

### Requirement: 凭证页 provider 切换布局
凭证管理页 SHALL 在顶部提供 provider 切换 tab（CodeBuddy / TRAE），一次只展示选中 provider 的内容：其登录认证区与凭证池卡片。切换 tab MUST 只挂载该 provider 的登录面板与凭证池，另一 provider 的登录与凭证池 MUST NOT 同时渲染。凭证池卡片 SHALL 展示本 provider 的凭证表格、当前凭证标记、自动轮换开关、有效性筛选、额度列（文案 `剩余 X / 总额 Y`，未探测显示 `-`）与“刷新额度”操作。CodeBuddy 视图提供设备授权登录入口；TRAE 视图提供 TRAE 网页登录与粘贴回调导入入口。两个 provider 的操作 MUST NOT 互相影响。凭证池 MUST NOT 展示“额度探测方式”入口。

#### Scenario: 切换 tab 只显示对应 provider
- **WHEN** 管理员在顶部 tab 选择 TRAE
- **THEN** 页面只显示 TRAE 登录面板与 TRAE 凭证池，CodeBuddy 的登录与凭证池不渲染

#### Scenario: 两 provider 操作互不影响
- **WHEN** 管理员在 CodeBuddy 视图切换轮换开关或选择当前凭证
- **THEN** 切到 TRAE 视图后其状态与列表不受影响，反之亦然

#### Scenario: 卡片展示各自当前凭证
- **WHEN** 某 provider 已设置当前凭证
- **THEN** 对应视图的表格中该行标记为"当前"，另一 provider 不标记

#### Scenario: 展示与刷新额度
- **WHEN** 管理员查看凭证池并点击某行"刷新额度"
- **THEN** 该行额度列刷新为最新 `剩余 X / 总额 Y`；未探测的凭证显示 `-`

### Requirement: API 测试（Playground）
管理台 SHALL 提供 API 测试（Playground）模块，允许管理员以**会话身份**（无需 `sk-` API Key）对指定 provider 发起模型列表查询与聊天请求。模块 SHALL 提供 Provider 选择（CodeBuddy / TRAE）；模型列表与聊天 MUST 路由到所选 provider 的上游执行器，与真实端点 `/codebuddy/openai/v1`、`/trae/openai/v1` 复用同一执行器（`OpenAIHandler` / `TraeChatHandler`）。模块 SHALL 支持流式（SSE）与非流式响应。协议仅 OpenAI，MUST NOT 暴露 Anthropic 协议入口。

#### Scenario: 获取所选 provider 的模型
- **WHEN** 管理员在 Playground 选择 CodeBuddy 并加载模型
- **THEN** 返回 codebuddy 模型列表；切换到 TRAE 后返回 trae 模型列表

#### Scenario: 聊天走所选 provider
- **WHEN** 管理员在 Playground 选中某 provider 发送消息
- **THEN** 请求路由到该 provider 的凭证池与上游执行器，返回 OpenAI 格式响应（流式或聚合）

#### Scenario: 会话鉴权
- **WHEN** 无有效管理会话调用 playground 端点
- **THEN** 返回 401

#### Scenario: 无可用凭证
- **WHEN** 所选 provider 的凭证池为空时发送聊天
- **THEN** 返回 503，Playground 显示错误

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

### Requirement: 总览页指标卡片真实数据
管理台总览页 SHALL 展示三张指标卡片：服务状态、有效凭证、今日请求数。服务状态 SHALL 在服务进程存活时显示「运行中」（对应 `/api/admin/status` 返回 `status:"healthy"`）；有效凭证 SHALL 展示合并 CodeBuddy 与 TRAE 的真实凭证数量，且当前全部凭证视为有效；今日请求数 SHALL 展示当日（浏览器本地时区）真实 chat completions 调用次数。

#### Scenario: 服务状态运行中
- **WHEN** 管理员打开总览页且服务进程存活
- **THEN** 服务状态卡片显示「运行中」，不显示「异常」

#### Scenario: 有效凭证合并计数
- **WHEN** 系统存在 CodeBuddy 与 TRAE 凭证
- **THEN** 有效凭证卡片的 total/valid 为两个 provider 凭证数量之和

#### Scenario: 今日请求数真实
- **WHEN** 当日发生过 chat completions 调用
- **THEN** 今日请求数卡片显示当日真实调用次数，而非 0

