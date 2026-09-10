## MODIFIED Requirements

### Requirement: API 契约兼容
后端 MUST 完整实现前端实际调用的端点：`/auth/login`、`/auth/session`、`/auth/logout`、`/api/admin/api-keys`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials`（GET/POST/DELETE）、`/api/admin/codebuddy/credentials/:id/select`、`/api/admin/codebuddy/credentials/:id/test`、`/api/admin/codebuddy/credentials/:id/daily-checkin`、`/api/admin/codebuddy/credentials/:id/quota/refresh`、`/api/admin/codebuddy/credentials/rotation/toggle`、`/api/admin/trae/credentials`（GET/DELETE）、`/api/admin/trae/credentials/:id/select`、`/api/admin/trae/credentials/:id/test`、`/api/admin/trae/credentials/:id/daily-checkin`、`/api/admin/trae/credentials/:id/quota/refresh`、`/api/admin/trae/credentials/rotation/toggle`、`/api/admin/playground/codebuddy/openai/v1/models`、`/api/admin/playground/codebuddy/openai/v1/chat/completions`、`/api/admin/playground/trae/openai/v1/models`、`/api/admin/playground/trae/openai/v1/chat/completions`、`/api/admin/status`。响应 JSON 结构（字段名与类型）与前端类型定义保持一致。

#### Scenario: 前端凭证页可用
- **WHEN** 管理员登录后进入凭证管理页
- **THEN** 两个 provider 的列表加载、选择、签到、测试、删除、额度刷新、轮换开关全部正常工作

### Requirement: 打桩端点返回空数据
对暂不实现的功能，后端 MUST 提供结构与参考实现一致但内容为空的响应，保证前端视图不崩溃：`GET/PUT /api/admin/settings`（返回默认设置）、`GET /api/admin/stats/*`（返回空统计）。额度不再打桩，MUST 返回真实探测值或 null。

#### Scenario: 统计页不崩溃
- **WHEN** 管理员打开统计页面
- **THEN** 页面正常渲染，显示空数据而非报错

#### Scenario: 额度不再打桩
- **WHEN** 管理员查看凭证列表
- **THEN** 额度字段为真实值或 null，不再是 unknown 快照

### Requirement: 凭证页 provider 切换布局
凭证管理页 SHALL 在顶部提供 provider 切换 tab（CodeBuddy / TRAE），一次只展示选中 provider 的内容：其登录认证区与凭证池卡片。切换 tab MUST 只挂载该 provider 的登录面板与凭证池，另一 provider 的登录与凭证池 MUST NOT 同时渲染。凭证池卡片 SHALL 展示本 provider 的凭证表格、当前凭证标记、自动轮换开关、有效性筛选、额度列（文案 `剩余 X / 总额 Y`，未探测显示 `-`）与“刷新额度”操作。CodeBuddy 视图提供设备授权登录入口；TRAE 视图提供 TRAE 网页登录与粘贴回调导入入口。两个 provider 的操作 MUST NOT 互相影响。凭证池 MUST NOT 展示“额度探测方式”入口。

#### Scenario: 切换 tab 只显示对应 provider
- **WHEN** 管理员在顶部 tab 选择 TRAE
- **THEN** 页面只显示 TRAE 登录面板与 TRAE 凭证池，CodeBuddy 的登录与凭证池不渲染

#### Scenario: 两 provider 操作互不影响
- **WHEN** 管理员在 CodeBuddy 视图切换轮换开关或选择当前凭证
- **THEN** 切到 TRAE 视图后其状态与列表不受影响，反之亦然

#### Scenario: 卡片展示各自当前凭证
- **WHEN** 某 provider 已设置当前凭证
- **THEN** 对应视图的表格中该行标记为“当前”，另一 provider 不标记

#### Scenario: 展示与刷新额度
- **WHEN** 管理员查看凭证池并点击某行“刷新额度”
- **THEN** 该行额度列刷新为最新 `剩余 X / 总额 Y`；未探测的凭证显示 `-`
