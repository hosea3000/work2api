# Proposal: bootstrap-codebuddy-gateway-core

## Why

work2api 是一个空的 Go（nunu 模板）项目，需要在它之上从零构建 CodeBuddy 逆向网关的核心功能。参考实现 codebuddy2api（Python/FastAPI）已经完整逆向了 CodeBuddy 上游协议（聊天、模型、签到、额度），我们把这些已验证的协议知识移植到 Go 技术栈，并复用其 Vue3 管理台前端，得到一个轻量、单二进制部署的等价服务。

## What Changes

- 在 nunu 模板（gin + gorm + sqlite + gocron）上新增 CodeBuddy 网关核心：
  - OpenAI 兼容入口：`POST /openai/v1/chat/completions`（流式 SSE 透传 + 非流式聚合）、`GET /openai/v1/models`（上游 `/v3/config` 拉取 + 配置模型合并）
  - `sk-...` API Key 鉴权中间件（Bearer），管理台会话登录鉴权（HttpOnly Cookie）
  - 凭证池：SQLite 存储凭证，内存轮换池（round-robin，每 N 次请求切换，401/403 摘除）
  - 凭证管理 API：CRUD + 手动添加 bearer_token + 选择当前凭证 + 轮换开关 + 手动签到
  - 每日自动签到：gocron 每天 09:30（服务器本地时区），成功判定 `code==0` 或 msg 含"已签到"，签到后触发额度重探测（一期只记录结果，不做额度展示）
  - 上游客户端：CodeBuddy CLI 伪装头生成（x-stainless-*、X-IDE-*、X-Conversation-* 等）、上游端点白名单
  - SSE 解析：上游流式事件 → OpenAI chunk 格式转换，tool_call 增量索引、reasoning_content 语义与 Python 版逐行对齐
- 复用 codebuddy2api 的 Vue3 管理台（web/ 目录整搬），按 API 契约适配：
  - 完整实现：登录/会话、api-keys、credentials（含 checkin/select/rotation-toggle）、status
  - 打桩返回空数据：settings、stats、test/accounts/quota（视图不崩，后续迭代）
- 明确不做（二期）：OAuth 授权流、Anthropic 协议、额度探测展示、用量统计、playground

## Capabilities

### New Capabilities
- `openai-compat-api`: OpenAI 兼容聊天入口（流式/非流式）与模型列表，含上游 SSE 转换
- `api-key-auth`: sk- API Key 的签发、校验与管理 API
- `admin-session-auth`: 管理台登录会话（HttpOnly Cookie）与登录 API 契约
- `credential-pool`: 凭证存储（SQLite）、轮换池、选择与轮换开关
- `credential-management`: 凭证管理 API（CRUD、手动导入、选择、状态展示）
- `daily-checkin`: 每日自动签到 + 手动签到 + 签到记录
- `codebuddy-upstream-client`: 上游端点、伪装头、超时与错误码处理
- `admin-web-frontend`: Vue 管理台集成与 API 契约兼容（含打桩端点）

### Modified Capabilities

（无 —— 全部为新增能力，项目从零起步）

## Impact

- **新增代码**：`internal/upstream/`、`internal/service/`（chat/credential/checkin）、`internal/handler/`（openai/admin）、`internal/middleware/`（apikey/session），预计 2500~3500 行 Go
- **修改**：`internal/router/`（挂载新路由）、`internal/server/http.go`（SPA 静态托管 + cookie 中间件）、`config/local.yml`（新增 codebuddy 配置段）
- **数据库**：SQLite 新增 3 张表（credential、api_key、checkin_record），gorm migration
- **前端**：`web/` 整体复制 codebuddy2api/frontend 源码，构建产物由 Go 嵌入或静态托管
- **参考实现**：`/root/code/github/codebuddy2api`（只读参考，不修改）
- **上游依赖风险**：CodeBuddy 协议为逆向所得，上游变更会导致功能失效；伪装头版本号（CLI 2.107.0 等）需可配置
