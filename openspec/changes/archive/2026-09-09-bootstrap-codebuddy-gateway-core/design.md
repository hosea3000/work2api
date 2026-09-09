# Design: bootstrap-codebuddy-gateway-core

## Context

work2api 当前是 nunu 模板（gin + gorm + wire + gocron + viper + zap，sqlite driver 已具备）。参考实现 codebuddy2api（Python/FastAPI，位于 `/root/code/github/codebuddy2api`，只读）完整逆向了 CodeBuddy 上游协议：聊天（仅流式）、模型（/v3/config）、签到、额度、OAuth。其 Vue3 管理台与后端通过一组 REST 契约通信（见 specs/admin-web-frontend）。

已确认的四个决策：**SQLite 全量存储 / 单管理员 / 手动凭证先行（OAuth 二期）/ 签到用服务器本地时区**。

## Goals / Non-Goals

**Goals:**
- API Key 鉴权 + OpenAI 兼容入口（流式/非流式）+ 模型列表
- 凭证池（SQLite + 内存轮换）+ 凭证管理 API + 每日自动签到/手动签到
- Vue 管理台无缝接入（完整实现核心端点，其余打桩）
- 上游协议细节（伪装头、错误码、SSE 语义）与参考实现逐行对齐

**Non-Goals:**
- OAuth 授权流、账号切换（二期）
- Anthropic 协议适配（二期）
- 额度探测/展示、用量统计、playground（二期，前端打桩）
- 多用户隔离（单管理员固定）
- 凭证 JSON 文件存储（统一走 SQLite）

## Decisions

### D1. 分层：沿用 nunu handler → service → repository，新增 `internal/upstream`
`internal/upstream/codebuddy/` 独立包承载伪装头生成 + 端点调用 + SSE 解析（纯函数优先，便于单测）。不引入新的架构模式；wire 继续用于依赖注入。
- 备选：把上游逻辑揉进 service 层 → 放弃，协议层与业务层职责不同，SSE 状态机需要独立可测。

### D2. 聊天代理直接 net/http 透传，不经过 gorm/gin 的 JSON 绑定
`POST /openai/v1/chat/completions` 的流式响应用 `http.Flusher` 逐事件写出；请求体用 `json.Unmarshal` 到宽松结构（未知字段忽略后按需重建上游 payload）。非流式 = 消费完流再聚合。
- 备选：gin 的 `c.Stream()` → 亦可，但底层同样是 Flush，直接用标准库写法更少魔法。
- 关键对齐点：上游 SSE 事件 → OpenAI chunk 的转换（reasoning_content 分离、tool_call 按 index 聚合、usage 事件）必须以 `codebuddy_events.py`（98 行）+ `openai_compat.py`（138 行）+ `stream_service.py` 的 normalizer 为蓝本逐行移植，并为其编写基于录制样本的单测。

### D3. 凭证池 = gorm 表 + 内存 `sync.Mutex` 环形指针
`credential` 表为持久层真相；`CredentialPool` 内存结构持有 active 凭证快照与 round-robin 指针/用量计数，启动时加载、变更时（增删/摘除/手动选择）重建或原位更新。`Select()` 返回 `(id, snapshot)` 原子快照。401/403 时 `MarkExpired(id)` 同步写库 + 更新内存。
- 备选：每次请求查库 → 放弃，轮换计数和指针语义在内存中更自然；快照在变更时刷新。
- 重建时机用"写库后调 pool.Refresh()"，不做文件 watcher。

### D4. 会话：内存 session store + HttpOnly Cookie，复用模板 JWT 包的签名能力或退化为随机 token + 服务端 map
单管理员、单实例部署，内存 map（TTL 7 天，滑动续期）足够；进程重启全员重新登录，可接受。
- 备选：gorm 存 session 表 → 放弃（重启保持登录的价值低于复杂度）；redis → 项目虽带依赖但一期不启。
- 密码哈希用 `golang.org/x/crypto`（bcrypt 或 pbkdf2，与模板已有依赖一致），首启动从配置读初始账密自动建户。

### D5. API Key 哈希：SHA-256（非密码学慢哈希）
key 本身是 256bit 随机数，不存在弱口令爆破面，SHA-256 查库即可（与参考实现一致）。明文只回显一次。
- 备选：bcrypt → 放弃，对随机 key 无增益且无法索引查询。

### D6. 签到：gocron 每天 09:30 本地时区 + 每凭证间随机抖动（5~20s，可配置）
调度循环里遍历 active 凭证，查 `checkin_record` 当日已成功则跳过；签到后触发额度缓存失效占位（一期无额度模块则为 no-op 日志）。手动签到走同一执行函数。
- 备选：time.Ticker 自旋 → 放弃，gocron 是模板既有依赖。
- 幂等判定：`code==0 || strings.Contains(msg,"已签到")`。

### D7. 前端整搬 + Go 静态嵌入
复制 `codebuddy2api/frontend` → `web/`（保留 Vue3+Vite+Tailwind 工程原样），裁掉 e2e/测试等开发目录可后置。构建产物 `web/dist` 用 `embed` 或 gin 静态中间件托管，SPA fallback 到 index.html。打桩端点：settings 返回参考实现默认值结构，stats 返回空聚合结构，quota 字段返回 `status: "unknown"` 快照。
- 打桩响应结构以 `admin_router.py` 的响应模型为准，逐字段核对 `web/src/types` 中的 TS 类型。

### D8. 配置：viper 新增 `codebuddy` 配置段
```
codebuddy:
  api_endpoint: https://copilot.tencent.com
  allowed_endpoints: [...]
  models: "glm-5.2,deepseek-v4-pro"      # 附加模型
  forced_temperature: 1                   # 空=不覆盖
  rotation_count: 1
  auto_checkin_enabled: true
  checkin_hour: 9 / checkin_minute: 30
  background_delay_min/max_seconds: 5/20
  cli_version: "2.107.0"                  # 伪装版本可调
  models_cache_ttl_seconds: 30
```
启动 fail-fast 校验端点在白名单内。

## Risks / Trade-offs

- [上游协议变更（头/端点/字段）] → 伪装版本号与端点全部可配置；SSE 转换层独立成包，改一处即可
- [SSE 语义移植偏差（tool_call/reasoning 边界）] → 先从参考实现代码+实测样本录制事件流，转换为 go 单测 fixture；不对齐就不算完成
- [内存会话重启丢失] → 单管理员场景可接受；二期可落库
- [打桩响应与前端类型漂移] → 以 TS types 为契约核对，允许字段缺失但不允许类型错误
- [轮换池与 DB 状态漂移] → 所有变更走"写库 → Refresh()"单向流，pool 不自行持久化
- [单管理员限制] → 数据模型保留 username 字段但固定值，二期放开成本可控

## Migration Plan

1. 建表 migration（credential、api_key、checkin_record、admin_user）随 gorm AutoMigrate 启动执行
2. 全部为新增路由/包，不触碰模板既有 user 示例代码（模板示例可在本 change 中顺带清理或保留，倾向保留避免无关 diff）
3. 回滚 = git revert；SQLite 单文件，无需数据迁移

## Open Questions

- 凭证重复（相同 user_id 再添加）行为：参考实现是随机后缀并存 → 倾向照搬"并存"，留待实现时确认
- 非流式聚合的最大等待时长：倾向 300s（与参考实现 playground 超时一致），可配置
