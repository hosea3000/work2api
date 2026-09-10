## MODIFIED Requirements

### Requirement: 打桩端点返回空数据
对暂不实现的功能，后端 MUST 提供结构与参考实现一致但内容为空的响应，保证前端视图不崩溃：`GET/PUT /api/admin/settings`（返回默认设置）。额度不再打桩，MUST 返回真实探测值或 null。`/api/admin/status` 与 `GET /api/admin/stats/overview` MUST NOT 打桩：前者返回真实服务状态与合并凭证计数，后者返回真实请求统计（详见 `request-metrics` 能力）。

#### Scenario: 额度不再打桩
- **WHEN** 管理员查看凭证列表
- **THEN** 额度字段为真实值或 null，不再是 unknown 快照

#### Scenario: 设置仍打桩
- **WHEN** 管理员调用 GET/PUT `/api/admin/settings`
- **THEN** 返回与默认设置一致的结构，不报错

## ADDED Requirements

### Requirement: 总览页指标卡片真实数据
管理台总览页 SHALL 展示三张指标卡片：服务状态、有效凭证、今日请求数。服务状态 SHALL 在服务进程存活时显示「运行中」（对应 `/api/admin/status` 返回 `status:"healthy"`）；有效凭证 SHALL 展示合并 CodeBuddy 与 TRAE 的真实凭证数量，且当前全部凭证视为有效；今日请求数 SHALL 展示当日（浏览器本地时区）真实 chat completions 调用次数。

#### Scenario: 服务状态运行中
- **WHEN** 管理员打开总览页且服务进程存活
- **THEN** 服务状态卡片显示「运行中」，不显示「异常」

#### Scenario: 有效凭证合并计数
- **WHEN** 系统存在 CodeBuddy 与 TRAE 凭证
- **THEN** 有效凭证卡片的 total/valid 为两个 provider 凭证数量之和

#### Scenario: 今日请求数真实
- **WHEN** 当日发生过 chat completions 调用
- **THEN** 今日请求数卡片显示当日真实调用次数，而非 0
