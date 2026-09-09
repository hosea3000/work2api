# Spec: admin-web-frontend

## ADDED Requirements

### Requirement: Vue 管理台集成
系统 SHALL 将 codebuddy2api 的 Vue3 管理台源码复制至 `web/`，构建产物由 Go 服务静态托管；前端路由未匹配路径回退 `index.html`（SPA history 模式 hash 路由兼容）。

#### Scenario: 静态托管与回退
- **WHEN** 浏览器访问 `/` 或任意前端路由
- **THEN** 返回 SPA 页面；访问 `/assets/*` 返回构建静态资源

### Requirement: API 契约兼容
后端 MUST 完整实现前端实际调用的端点：`/auth/login`、`/auth/session`、`/auth/logout`、`/api/admin/api-keys`（GET/POST/DELETE）、`/api/admin/credentials`（GET/POST/DELETE）、`/api/admin/credentials/:id/select`、`/api/admin/credentials/:id/test`、`/api/admin/credentials/:id/daily-checkin`、`/api/admin/credentials/rotation/toggle`、`/api/admin/status`。响应 JSON 结构（字段名与类型）与参考实现保持一致。

#### Scenario: 前端凭证页可用
- **WHEN** 管理员登录后进入凭证管理页
- **THEN** 列表加载、添加凭证、选择、签到、删除全部正常工作（字段结构与 Python 版一致）

### Requirement: 打桩端点返回空数据
对暂不实现的功能，后端 MUST 提供结构与参考实现一致但内容为空的响应，保证前端视图不崩溃：`GET/PUT /api/admin/settings`（返回默认设置）、`GET /api/admin/stats/*`（返回空统计）、凭证 quota 相关字段返回 unknown 状态。

#### Scenario: 统计页不崩溃
- **WHEN** 管理员打开统计页面
- **THEN** 页面正常渲染，显示空数据而非报错

### Requirement: API Key 页可用
管理台 API 密钥页面 MUST 支持创建（明文一次性展示）、列表（脱敏）、删除，与参考实现交互一致。

#### Scenario: 创建后一次性展示
- **WHEN** 管理员在页面创建 API Key
- **THEN** 弹出明文提示且刷新列表后仅见摘要
