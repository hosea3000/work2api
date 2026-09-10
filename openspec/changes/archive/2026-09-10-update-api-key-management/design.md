# update-api-key-management 设计

## Context

`api_key` 表原本只存 SHA-256 哈希，明文一次性返回。用户要求"随时可复制 + 页面隐藏显示"以及"名称不可重复"。前者在数学上要求服务器能还原明文，因此必须持久化明文（或可逆密文）。

系统内 `codebuddy_credential`/`trae_credential` 的 `bearer_token` 本就明文落库，DB 文件即信任边界。故本变更直接存明文，与既有信任模型一致，不引入 master key 管理。

## Goals / Non-Goals

- Goals: API Key 明文可随时取回并在管理台隐藏显示 + 复制；名称全局唯一（不区分大小写）且非空
- Non-Goals: key 轮换/过期、加密存储、`last_used_at` 追踪、按 key 的统计

## Decisions

### D1: 存明文 `key`，保留 `key_hash` 作校验索引

`model.APIKey` 新增 `Key string`（`type:text`）持久化明文；保留 `KeyHash`。
- `Create`：生成明文后同时写入 `Key` 与 `KeyHash`
- `Validate`：**不变**，仍按 `KeyHash` 查（哈希成为定长、带唯一索引的查找键）
- `key` 列加普通索引（可选）

理由：明文既然已落库，哈希不再是安全措施，但保留它可让校验热路径与既有索引零改动；删除哈希需改 `Validate`/repo，收益不大。

### D2: 列表返回明文，前端掩码 + 复制

`GET /api/admin/api-keys` 每条记录带完整 `key`。前端渲染为 `sk-••••••••` + 复制按钮，点击复制拿缓存明文。不做独立 reveal 端点（省一次往返；管理台本身已是高权限面）。

### D3: 名称唯一（不区分大小写）+ 非空

- `model.APIKey.Name`：`size:128;not null`（不加 gorm uniqueIndex，改由显式索引控制大小写语义）
- `EnsureSchema`：AutoMigrate 后执行
  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_name ON api_key(name COLLATE NOCASE)
  ```
- `service.Create`：`name = strings.TrimSpace(name)`；为空 → 返回 `ErrDuplicateName` 之外的 `ErrEmptyName`（或统一 400）
- 查重：`SELECT count(*) FROM api_key WHERE name = ? COLLATE NOCASE`（或 repo `GetAPIKeyByName`）；命中 → `ErrDuplicateName`
- 兜底：插入若触发 DB 唯一冲突，也映射为 `ErrDuplicateName`
- 移除旧的"空名兜底成 unnamed"逻辑

`Prod` 与 `prod` 视为重复。

### D4: 校验中间件不变

`APIKeyService.Validate` 保持哈希比对，无行为变化。

### D5: 管理台去掉"仅显示一次"

`ApiKeysView.vue`：
- 移除 `pendingApiKeys` 的一次性告警、离页警告、`beforeunload` 逻辑
- 创建成功 → 直接刷新列表 + toast
- 列表新增"Key"列：掩码显示（如 `sk-••••••••` 或 `preview`）+ 复制按钮（`Copy` 图标）
- 创建错误 400（重复名）→ toast `名称已存在`

`ApiKeyRecord` 加 `key: string` 字段。

### D6: 迁移

- `EnsureSchema`/`cmd/migration` 的 AutoMigrate 会为 `api_key` 增量添加 `key` 列（SQLite 加列，不删列）
- 唯一索引在 EnsureSchema 里显式创建；若存量数据有同名，创建会失败——清库或先手动去重
- `key_hash` 列保留不动

## Risks / Trade-offs

- [明文落库，DB 泄露即 key 泄露] → 与同库 bearer_token 同级；接受（用户已确认）
- [列表响应携带全部明文 key] → 仅管理员会话可访问；接受（用户选了简单方案）
- [存量同名导致唯一索引创建失败] → 文档注明；清库或去重
- [GORM collate 语义不确定] → 用显式 `CREATE UNIQUE INDEX ... COLLATE NOCASE` + 应用层查重双保险

## Open Questions

- 无（决策已确认）
