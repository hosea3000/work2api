# Tasks: bootstrap-codebuddy-gateway-core

## 1. 基础设施与数据模型

- [x] 1.1 viper 配置新增 `codebuddy` 段（api_endpoint/allowed_endpoints/models/forced_temperature/rotation_count/checkin 调度/抖动/cli_version/models_cache_ttl），含启动 fail-fast 白名单校验
- [x] 1.2 gorm 模型与 AutoMigrate：`admin_user`、`credential`、`api_key`、`checkin_record` 四张表
- [x] 1.3 首启动初始化单管理员账户（从配置读初始账密，bcrypt 哈希入库）
- [x] 1.4 wire 依赖注入接线：upstream/service/handler/pool 全部纳入 ProviderSet，`make wire` 通过

## 2. 上游协议层 `internal/upstream/codebuddy`

- [x] 2.1 伪装头生成器：标准 CLI 头集 + 企业凭证附加头 + CodeBuddyIDE 变体头（模型接口用），uuid/hex 生成规则与参考实现对齐
- [x] 2.2 上游客户端：聊天（POST /v2/chat/completions，流式）、模型（GET /v3/config）、签到（POST /billing/meter/daily-checkin），统一超时（connect 10s / read 30s）
- [x] 2.3 SSE 解析器：逐行解析 `data:` 事件 → 结构化事件（content/reasoning_content/tool_call 增量/usage/finish/done），语义以 `codebuddy_events.py` 为蓝本
- [x] 2.4 错误处理：401/403 → 凭证失效信号；业务错误码映射表（12005/11212/11216/10081 → 中文消息）；受控错误类别（不透传上游响应体）
- [x] 2.5 单测：伪装头字段断言（用参考实现输出做 fixture）、SSE 事件解析（录制样本回放）、错误码映射表全覆盖

## 3. 凭证池与服务

- [x] 3.1 `CredentialPool`：内存 active 快照 + mutex 保护的 round-robin（每 N 次切换）、`Select()` 原子快照、`MarkExpired()`、`Refresh()`（写库后调用）
- [x] 3.2 凭证 service：手动添加（JWT 解析提取 user_id，无效拒绝）、删除（重置指针）、select（关闭自动轮换）、rotation toggle、列表（bearer 尾部脱敏）
- [x] 3.3 换凭证重试：上游 401 摘除后自动换下一凭证重试 1 次，再失败返回受控错误
- [x] 3.4 单测：轮换计数/并发 Select（`-race`）、摘除后跳过、池空信号、JWT 解析失败拒绝

## 4. 聊天代理与 OpenAI 兼容层

- [x] 4.1 请求转换：OpenAI messages → 上游 payload（temperature 强制覆盖逻辑、stop 非空 400）
- [x] 4.2 SSE → OpenAI chunk 流式转换（reasoning_content 分离、tool_call 按 index 聚合、usage 事件），`http.Flusher` 实时写出
- [x] 4.3 非流式聚合：消费完上游流聚合成完整 Chat Completion（含 tool_calls arguments 拼接、usage），客户端断连取消上游
- [x] 4.4 模型列表：配置模型 ∪ 上游 `/v3/config` 实际模型（有序去重），TTL 缓存 + 失败回退
- [x] 4.5 单测：stop 拒绝、温度覆盖、chunk 转换（fixture 回放）、非流式聚合、断连取消

## 5. 鉴权层

- [x] 5.1 API Key service/中间件：SHA-256 存储、Bearer 校验、创建一次性明文/列表脱敏/删除立即失效；OpenAI 错误格式 401
- [x] 5.2 会话：内存 session store（TTL 7 天滑动）、HttpOnly Cookie、`/auth/login|session|logout`、bcrypt 校验
- [x] 5.3 登录限流：全局/IP/用户名三滑动窗口（60s 内 60/10/5），超限 429
- [x] 5.4 中间件隔离：`/api/admin/*` 仅认会话 Cookie，`/openai/v1/*` 仅认 API Key，互不通用
- [x] 5.5 单测：key 失效即 401、未登录 401、sk- 不能过 admin、限流窗口计数

## 6. 签到

- [x] 6.1 签到执行函数：构造头 → 请求上游 → 幂等判定（code==0 或"已签到"）→ 写 `checkin_record`
- [x] 6.2 gocron 调度：每天 09:30 本地时区，遍历 active 未签到凭证 + 凭证间随机抖动（可配置区间）
- [x] 6.3 手动签到端点 `POST /api/admin/credentials/:id/daily-checkin`（实时返回结果明细）
- [x] 6.4 单测：幂等判定三分支（成功/已签到/失败）、当日跳过、抖动区间边界

## 7. 管理台前端集成

- [x] 7.1 复制 codebuddy2api/frontend → `web/`，裁剪 e2e 等非必需目录，`pnpm build` 产出 `web/dist`
- [x] 7.2 Go 静态托管：SPA fallback + 静态资源；`/api`、`/auth`、`/openai` 路由优先级高于 SPA
- [x] 7.3 核心端点契约核对：逐字段对照 `web/src/types` + `admin_router.py` 响应模型（status/api-keys/credentials/checkin/select/toggle）
- [x] 7.4 打桩端点：settings 返回默认结构、stats 返回空聚合、quota 字段 unknown 快照
- [x] 7.5 (API 层已 curl 实测；浏览器端联调待用户确认) 手动联调：登录 → 凭证页（添加/选择/签到/删除）→ API Key 页（创建/删除）→ 模型/聊天实测 → Dashboard/统计页不崩

## 8. 端到端验证

- [x] 8.1 curl 全链路：登录 → 建 key → 添加凭证 → 非流式聊天 → 流式聊天 → 删 key 后 401
- [x] 8.2 全部 go test 通过（含 `-race`）、`go vet` 干净
- [x] 8.3 README 快速开始更新（建户、配置、启动、验证命令）
