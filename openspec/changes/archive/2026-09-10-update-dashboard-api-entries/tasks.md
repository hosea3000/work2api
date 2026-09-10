## 1. 前端总览页

- [x] 1.1 在 `web/src/views/DashboardView.vue` 增加基于 `window.location.origin` 拼接的两个入口地址计算属性：`/codebuddy/openai/v1`、`/trae/openai/v1`
- [x] 1.2 将「OpenAI 客户端入口」卡片改为「CodeBuddy API 地址」，绑定完整地址
- [x] 1.3 新增「TRAE API 地址」卡片，绑定完整地址
- [x] 1.4 复制函数改用新的完整地址（CodeBuddy / TRAE），移除对 `anthropic_api_base_url` 的引用
- [x] 1.5 删除「Anthropic 客户端入口」卡片及 Claude Code 模型 ID 提示

## 2. 类型与后端契约

- [x] 2.1 从 `web/src/types/admin.ts` 的 `AdminStatus` 移除 `api_base_url`、`anthropic_api_base_url`
- [x] 2.2 从 `internal/handler/admin_stub_handler.go` 的 `Status` 响应移除 `api_base_url`、`anthropic_api_base_url`

## 3. 验证

- [x] 3.1 前端构建通过（`web/` 下 `pnpm run build:bundle`）
- [x] 3.2 后端编译通过（`go build ./...`）
- [x] 3.3 打开总览页确认两个入口显示完整地址、复制可用、无 Anthropic 卡片
