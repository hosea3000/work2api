# add-trae-chat-completions 设计

## Context

一期接入 TRAE 凭证（登录导入 + 保活），二期到此收尾：让 TRAE 凭证进入聊天消费。参考实现 trae2api-web 已验证 SOLO 通道可反代，`payload.go`（OpenAI→SOLO 改写）与 `solosse.go`（SOLO SSE→OpenAI）是实测过的成熟代码，直接移植。

关键约束来自既有设计红线（`add-trae-credentials` D2）：codebuddy 与 trae 两类凭证在消费路径上必须彻底隔离。因此本变更**新增独立端点**而非复用 codebuddy 端点做模型命名空间路由。同时统一两个 provider 的路径前缀（`/codebuddy/openai/v1`、`/trae/openai/v1`），codebuddy 的业务逻辑保持零改动。

已确认决策（用户拍板）：
1. 执行器**镜像**（不抽 provider 接口）
2. **独立池 + 独立轮询**
3. 冷却**一步到位**（1005/4011/4008）
4. 路径 `/trae/openai/v1`
5. `stop` **透传**（不拒绝）

## Goals / Non-Goals

- Goals: `/trae/openai/v1/chat/completions` + `/trae/openai/v1/models` 可用；TRAE 凭证独立轮询并带冷却；codebuddy 逻辑零行为变化（仅路径前缀重命名）
- Non-Goals: 模型命名空间路由（方案 A）、TRAE 手动 select/rotation UI、跨 provider 自动兜底、TRAE 聊天用量统计

## Decisions

### D1: 镜像 TraeChatExecutor，不抽 provider 接口

新建 `internal/service/trae_chat_executor.go`，复制 `ChatExecutor.ExecuteChat` 的调度骨架（校验→改写→选凭证→HTTP→流式/非流式分发→错误映射），专属步骤全部换成 trae 版。**不**定义 `ChatProvider` 接口、**不**重构现有 codebuddy 路径。

理由：共享的只有 ~80 行骨架；两家 SSE 与改写是主体且几乎不共通；抽接口要求先动刀正在工作的 codebuddy 热路径，回归风险与收益不成比例。与仓库既有风格一致（`trae_token_refresh.go` 镜像 `token_refresh.go`）。

### D2: 独立 TraeCredentialPool，随 codebuddy 池一并重载

新建 `internal/service/trae_credential_pool.go`：独立 struct，持有 `[]TraePoolEntry`（含 `model.Credential` + `cooldownUntil int64`）+ round-robin 指针 + 互斥锁。

重载接线（复用现有触发点，避免新增调用方）：
```
credentialService.refreshPool(ctx)         ← 现有唯一 DB→池入口
   ├─ s.pool.Refresh(creds)                ← codebuddy（现有）
   └─ s.traePool.Refresh(creds)            ← trae（新增，过滤 provider=trae && active）
```
`refreshPool` 已在 Add/Delete/Select/AddOAuth/updateExistingOAuth/UpdateCredential/PoolReload 全部触发，因此 trae 池自动跟随。**补一处**：`trae_token_refresh.go` 刷新成功目前只调 `UpdateCredential`（其内部会 refreshPool），无需额外改；确认即可。

TRAE 池的冷却状态是**内存态**（`cooldownUntil`），`Refresh` 重建 entries 时 SHALL 按 credential id 保留未过期的冷却（否则每次刷新都把冷却清空）。

### D3: 冷却语义

| 上游 | 含义 | 动作 |
|---|---|---|
| 401 / session 失效 | 凭证死亡 | `MarkExpired`（写库 expired + 出池） |
| 1005 | plan 权益不足 | `cooldownUntil = now + 12h` |
| 4011 | 请求频率超限 | `cooldownUntil = now + 60s` |
| 4008 | ide_credits 耗尽 | `cooldownUntil = 次日 0 点`（或 now + 1h，取参考实现） |
| 其他 5xx | 上游抖动 | 不冷却，直接返回错误 |

选池时跳过 `cooldownUntil > now` 的 entry；若全部冷却则返回 `ok=false` → 503。冷却只作用于内存池，不写 DB。

### D4: 端点与路由

```
openai := s.Group("/codebuddy/openai/v1"); openai.Use(APIKeyAuth)   ← 现有
   POST /chat/completions  OpenAIHandler.ChatCompletions
   GET  /models            OpenAIHandler.Models

trae := s.Group("/trae/openai/v1"); trae.Use(APIKeyAuth)          ← 新增
   POST /chat/completions  TraeChatHandler.ChatCompletions
   GET  /models            TraeChatHandler.Models
```
`TraeChatHandler` 镜像 `OpenAIHandler`（同样的 body 读取、流式/非流式分支、错误格式）。

### D5: stop 透传

`TraeChatExecutor` **不**调用 codebuddy 的 `ValidateChatRequest` 的 stop 拒绝分支。抽一个共享的 `validateMessages(body)` 做 messages 校验，stop 检查仅 codebuddy 使用。代码标 `// ponytail: trae 透传 stop（对齐参考实现）；实测上游忽略 stop 再补 400`。

### D6: 上游适配移植（internal/upstream/trae）

- `chat.go`：`PrepareChatBody(src []byte) []byte` —— 移植 trae2api-web `payload.go`（含 normalizeTools/normalizeToolChoice）
- `sse.go`：`Stream(w, r)` / `StreamWithError(w, r, onErr)` / `Aggregate(r)` + `ParseSOLOLine` / `SOLOEvent` / `SOLOStreamError` —— 移植 `solosse.go`
- `client.go` 增：`AgentHost` 字段（已存在于 constants，Client 结构补字段）、`ChatURL()`、`SOLOHeaders(cred, stream)`、`FetchModels(accessToken, machineID, deviceID)`（`get_detail_param`）
- 常量：`EpChat = /api/agent/v3/llm_utils_chat`、`EpModels = /api/ide/v1/get_detail_param`、`Function = solo_work_lite`、`DefaultConfigName`

Client 结构需补 `MachineID`/`DeviceID` 来源——它们来自凭证，不进 Client，由调用方传入 headers。

### D7: TraeModelsService

镜像 `ModelsService`：`Available(ctx)` 取 TRAE 池一个凭证 → `FetchModels` → 缓存 TTL 30s → 失败回退缓存。`owned_by: "trae"`。无需"配置模型"并集（TRAE 模型由上游动态下发）。

### D8: 错误分类与响应 model 回显

`TraeChatExecutor` 把上游错误映射为 OpenAI 错误响应；`responseModel` 用客户端请求的 model（非空），空则默认 `DefaultConfigName`。SSE 转换里聚合响应的 `model` 由 executor 填入。

## Risks / Trade-offs

- [SOLO 请求/SSE 细节未在本仓库实测] → 全部逻辑集中在 `chat.go`/`sse.go`，且移植自实测实现；冒烟阶段用真实凭证打一次流式+非流式校准
- [两处执行器骨架重复，将来骨架 bug 要改两处] → 接受（D1）；待出现第三个 provider 再抽
- [冷却状态内存态，重启即失] → 接受；重启后上游若仍限流会再次冷却，无数据损失
- [codebuddy 逻辑零回归的验证] → 保留 codebuddy 现有测试全绿作为回归闸门
- [TRAE 模型名与 codebuddy 不同] → 独立 `/trae/openai/v1/models`，客户端按 base_url 各取各的，不混淆

## Migration Plan

无 DB 迁移。部署后：`/trae/openai/v1/models` 立即可用（用池内凭证拉模型）；`/trae/openai/v1/chat/completions` 走独立池 round-robin。TRAE 凭证的刷新/签到/保活沿用上一变更。

## Open Questions

- 无（5 项决策已确认）
