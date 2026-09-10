# update-api-key-management

## Why

当前 API Key 只存 SHA-256 哈希，明文仅创建响应返回一次——管理员一旦没及时复制，就永久丢失，只能删了重建。同时名称无唯一约束，可以创建多个同名 key，管理台无法区分。

本变更把 API Key 改为**可随时复制**（明文持久化 + 列表隐藏显示 + 复制按钮），并给名称加**全局唯一（不区分大小写）**约束。这会**覆盖**既有 `api-key-auth` spec 中"只存哈希、明文仅一次"的约定。

## What Changes

- **存储改为可逆**：`api_key` 表新增明文 `key` 列（持久化保存），保留 `key_hash` 作为校验索引；创建后明文可随时取回
- **名称唯一**：`name` 加唯一索引（`COLLATE NOCASE`，不区分大小写），空名称拒绝；重复创建返回 400
- **列表返回明文**：`GET /api/admin/api-keys` 每条记录带完整 `key`
- **管理台 UI**：列表每行以掩码（`sk-••••••••`）显示 key + 复制按钮（随时复制）；移除"仅显示一次、关闭后无法查看"的强制告警；创建重复名时提示"名称已存在"
- **spec 覆盖**：`api-key-auth` 的"明文仅展示一次"改为"可随时复制"；新增名称唯一要求

## Capabilities

### Modified

- `api-key-auth`: 生成与存储改为明文可逆 + 名称唯一；校验中间件行为不变（仍走哈希索引）
- `admin-web-frontend`: API Key 页改为 key 隐藏显示 + 随时复制；创建后不再有一次性告警

## Impact

- **后端**：`model.APIKey`（加 `Key` 列、`Name` 唯一索引）、`repository`（加按名查重）、`service`（Create 查重 + 存明文；List 返回明文）、`handler`（List 带 key；Create 重复名 400）
- **前端**：`ApiKeysView.vue`（列表加掩码 key + 复制列；移除 pending "仅显示一次" 告警；重复名错误提示）、`types`（`ApiKeyRecord` 加 `key`）
- **DB**：新增 `key` 列 + `name` 唯一索引；无数据迁移（清库或 AutoMigrate 增量建列）
- **安全**：sk 明文落库，与同库既有明文 `bearer_token` 的信任模型一致；`key_hash` 保留为定长校验索引
- **非目标**：key 轮换/过期、按 key 的用量统计、`last_used_at` 追踪
