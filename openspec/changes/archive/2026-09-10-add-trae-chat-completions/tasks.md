# add-trae-chat-completions 任务

## 1. TRAE 上游聊天适配（移植）

- [x] 1.1 `internal/upstream/trae/constants.go`：加 `EpChat`、`EpModels`、`Function = "solo_work_lite"`、`DefaultConfigName`
- [x] 1.2 新建 `internal/upstream/trae/chat.go`：`PrepareChatBody`（function/config_name/model、content 数组化、tools.parameters 字符串化、assistant tool_calls→function_call、tool_choice 归一化）+ 单测（对齐 trae2api-web payload.go 用例：字符串 content、tools 序列化、tool_choice none/auto/function、无 name 剔除）
- [x] 1.3 新建 `internal/upstream/trae/sse.go`：`SOLOEvent`/`ParseSOLOLine`/`SOLOStreamError`/`Stream`/`StreamWithError`/`Aggregate`（output/token_usage/done/error，tool_call 按 index 聚合，function_call 兼容）+ httptest 单测（流式 chunk、非流式聚合、error 事件）
- [x] 1.4 `internal/upstream/trae/client.go`：补 `AgentHost` 字段 + `ChatURL()` + `SOLOHeaders(accessToken, uid, machineID, deviceID, stream)` + `FetchModels(accessToken, machineID, deviceID)`（`get_detail_param`）+ 单测（头齐全、模型解析）

## 2. TRAE 独立凭证池

- [x] 2.1 新建 `internal/service/trae_credential_pool.go`：`TraePoolEntry`（Credential + cooldownUntil）+ `TraeCredentialPool`（Refresh 保留未过期冷却 / Select 跳过冷却 / MarkExpired / Cooldown / Len）+ 单测（round-robin、隔离 codebuddy、冷却跳过、全冷却空选、Refresh 保留冷却）
- [x] 2.2 `internal/service/credential_service.go`：`credentialService` 加 `traePool`，`refreshPool` 同时 `s.traePool.Refresh(creds)`；构造函数注入/初始化 trae 池

## 3. TRAE 聊天执行器

- [x] 3.1 新建 `internal/service/trae_chat_executor.go`：镜像 `ChatExecutor.ExecuteChat`（校验 messages → `trae.PrepareChatBody` → trae 池选凭证 → `SOLOHeaders` → POST `ChatURL` → 非200 分类冷却/摘除 → `trae.Stream`/`trae.Aggregate`），response model 回显；抽共享 `validateMessages`（stop 检查仅 codebuddy 用）+ 单测（非流式聚合、流式透传、无凭证 503、1005 冷却、401 摘除）
- [x] 3.2 新建 `internal/service/trae_models_service.go`：`TraeModelsService.Available`（池内凭证 → `FetchModels` → TTL 缓存 → 失败回退）+ 单测

## 4. 路由与处理器

- [x] 4.1 新建 `internal/handler/trae_chat_handler.go`：`ChatCompletions`（流式/非流式分支，镜像 OpenAIHandler）+ `Models`（`owned_by: trae`）
- [x] 4.2 `internal/router/gateway.go`：加 `/trae/openai/v1` group（`APIKeyAuth`）+ 两端点；`RouterDeps` 加 `TraeChatHandler`
- [x] 4.3 wire：`bootstrap/providers.go` 构造 trae 聊天依赖；`cmd/server/wire/wire.go` serviceSet/handlerSet 注册并重新生成 wire_gen.go
- [x] 4.4 统一端点前缀（破坏性）：`/openai/v1/*` → `/codebuddy/openai/v1/*`，`/trae/v1/*` → `/trae/openai/v1/*`；同步 handler 注释、admin `api_base_url`、README、ApiDocsView、specs（openai-compat-api / api-key-auth delta）

## 5. 验证

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`（重点：trae chat/sse 单测、trae 池单测、trae executor 单测；codebuddy 现有测试全绿作为零回归闸门）
- [ ] 5.2 手动冒烟：`/trae/openai/v1/models` 列出 TRAE 模型 → 用真实凭证发流式与非流式 chat（含 tools/tool_choice）→ 确认 reasoning_content 分离、tool_call 聚合、usage 字段 → 构造 1005/401 观察冷却与摘除
