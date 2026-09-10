# admin-web-frontend Specification

## Purpose
TBD - created by archiving change bootstrap-codebuddy-gateway-core. Update Purpose after archive.
## Requirements
### Requirement: Vue 管理台集成
系统 SHALL 将 codebuddy2api 的 Vue3 管理台源码复制至 `web/`，构建产物由 Go 服务静态托管；前端路由未匹配路径回退 `index.html`（SPA history 模式 hash 路由兼容）。

#### Scenario: 静态托管与回退
- **WHEN** 浏览器访问 `/` 或任意前端路由
- **THEN** 返回 SPA 页面；访问 `/assets/*` 返回构建静态资源

### Requirement: API 契约兼容
后端 MUST 完整实现前端实际调用的端点：`/auth/login`、`/auth/session`、`/auth/logout`、`/api/admin/api-keys`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials/:id/select`、`/api/admin/codebuddy/credentials/:id/test`、`/api/admin/codebuddy/credentials/:id/daily-checkin`、`/api/admin/codebuddy/credentials/rotation/toggle`、`/api/admin/trae/credentials`（GET/DELETE）、`/api/admin/trae/credentials/:id/select`、`/api/admin/trae/credentials/:id/test`、`/api/admin/trae/credentials/:id/daily-checkin`、`/api/admin/trae/credentials/rotation/toggle`、`/api/admin/status`。响应 JSON 结构（字段名与类型）与前端类型定义保持一致。

#### Scenario: 前端凭证页可用
- **WHEN** 管理员登录后进入凭证管理页
- **THEN** 两个 provider 的列表加载、选择、签到、测试、删除、轮换开关全部正常工作

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

### Requirement: 凭证页 provider 切换布局
凭证管理页 SHALL 在顶部提供 provider 切换 tab（CodeBuddy / TRAE），一次只展示选中 provider 的内容：其登录认证区与凭证池卡片。切换 tab MUST 只挂载该 provider 的登录面板与凭证池，另一 provider 的登录与凭证池 MUST NOT 同时渲染。凭证池卡片 SHALL 展示本 provider 的凭证表格、当前凭证标记、自动轮换开关、有效性筛选。CodeBuddy 视图提供设备授权登录入口；TRAE 视图提供 TRAE 网页登录与粘贴回调导入入口。两个 provider 的操作 MUST NOT 互相影响。

#### Scenario: 切换 tab 只显示对应 provider
- **WHEN** 管理员在顶部 tab 选择 TRAE
- **THEN** 页面只显示 TRAE 登录面板与 TRAE 凭证池，CodeBuddy 的登录与凭证池不渲染

#### Scenario: 两 provider 操作互不影响
- **WHEN** 管理员在 CodeBuddy 视图切换轮换开关或选择当前凭证
- **THEN** 切到 TRAE 视图后其状态与列表不受影响，反之亦然

#### Scenario: 卡片展示各自当前凭证
- **WHEN** 某 provider 已设置当前凭证
- **THEN** 对应视图的表格中该行标记为"当前"，另一 provider 不标记

