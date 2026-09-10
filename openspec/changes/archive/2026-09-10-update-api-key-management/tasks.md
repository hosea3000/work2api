# update-api-key-management 任务

## 1. 后端

- [x] 1.1 `internal/model/gateway.go`：`APIKey` 加 `Key string`（`gorm:"type:text"`）
- [x] 1.2 `internal/repository/gateway.go`：`EnsureSchema` 在 AutoMigrate 后执行 `CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_name ON api_key(name COLLATE NOCASE)`；加 `GetAPIKeyByName(ctx, name)`（`COLLATE NOCASE`）
- [x] 1.3 `internal/service/apikey_service.go`：新增 `ErrDuplicateName`/`ErrEmptyName`；`Create` 去掉 unnamed 兜底、查重（不区分大小写）、写入 `Key` 明文；`List`/`APIKeyView` 返回明文 `key`；`Validate` 不变
- [x] 1.4 `internal/handler/apikey_handler.go`：`List` 记录带 `key`；`Create` 重复名/空名 → 400 `{"detail":"名称已存在"}` / `{"detail":"名称不能为空"}`

## 2. 前端

- [x] 2.1 `web/src/types/admin.ts`：`ApiKeyRecord` 加 `key: string`
- [x] 2.2 `web/src/views/ApiKeysView.vue`：移除 `pendingApiKeys`/离页警告/`beforeunload`；创建成功直接刷新列表；列表加 Key 列（掩码 + 复制按钮）；重复名 400 → toast
- [x] 2.3 `web/src/utils/apiKeyText.ts`：如需要，调整/移除一次性展示相关文案

## 3. 验证

- [x] 3.1 `go build ./... && go vet ./... && go test ./...`
- [x] 3.2 `web` 侧 `vue-tsc` + `oxlint` + `vite build`
- [x] 3.3 冒烟：创建 key → 列表隐藏显示且可复制 → 复制值与创建时一致 → 用该 key 调 `/codebuddy/openai/v1/models` 通过 → 用同名（含大小写变体）再创建返回 400 → 空名返回 400
