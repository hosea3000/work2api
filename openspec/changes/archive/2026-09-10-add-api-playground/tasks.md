# add-api-playground 任务

## 1. 后端

- [x] 1.1 `internal/router/gateway.go`：在 `/api/admin` 组下新增 4 条 playground 路由，复用 `deps.OpenAIHandler` / `deps.TraeChatHandler` 的 `Models` / `ChatCompletions`

## 2. 前端

- [x] 2.1 `web/src/api/admin.ts`：`openaiPlaygroundApi.models(provider, signal)` 与 `chat(provider, body, signal)` 路径带 provider；移除 `anthropicPlaygroundApi`
- [x] 2.2 `web/src/utils/adminQueryKeys.ts`：`playgroundModels(provider)`（去掉 protocol 维度）
- [x] 2.3 `web/src/views/ApiConsoleView.vue`：`protocol` 换成 `provider`（CodeBuddy / TRAE）单选；模型/聊天按 provider 调用；移除 Anthropic 分支与 `message_stop` 处理
- [x] 2.4 清理未使用的 Anthropic 相关类型/导入（如 `AnthropicMessageRequest`），确保 `vue-tsc`/`oxlint` 通过

## 3. 验证

- [x] 3.1 `go build ./... && go vet ./... && go test ./...`
- [x] 3.2 `web` 侧 `vue-tsc` + `oxlint` + `vite build`
- [x] 3.3 冒烟：Playground 选 CodeBuddy 加载模型 → 发送（流式/非流式）成功；切 TRAE 加载 trae 模型 → 发送成功；无凭证时显示 503 错误；未登录访问 playground 端点返回 401
