## ADDED Requirements

### Requirement: 聊天请求记录持久化
系统 MUST 为每一次 chat completions 调用持久化一条请求记录，记录调用发生的 Unix 时间戳（秒）与结果（success / failure）。记录 MUST 覆盖外部入口（`/codebuddy/openai/v1/chat/completions`、`/trae/openai/v1/chat/completions`）与管理台 Playground 入口。系统 MUST NOT 为模型列表、凭证测试、签到等非聊天调用记录。

#### Scenario: 外部调用落库
- **WHEN** 客户端通过 API Key 调用 `/codebuddy/openai/v1/chat/completions` 或 `/trae/openai/v1/chat/completions`
- **THEN** 持久化一条请求记录，包含当时时间戳与结果

#### Scenario: Playground 调用落库
- **WHEN** 管理员通过 Playground 发起聊天请求
- **THEN** 同样持久化一条请求记录

#### Scenario: 非聊天调用不记录
- **WHEN** 调用 `/models`、凭证测试或签到
- **THEN** 不产生请求记录

### Requirement: 按时间范围统计
`GET /api/admin/stats/overview` MUST 依据查询参数 `start_at`、`end_at`（Unix 秒，区间为 `[start_at, end_at)`）返回该范围内的 `totals.request_count` 与 `totals.success_rate`（成功数 / 总数；无请求时为 null）。其余统计字段 MUST 返回空值且响应结构不变。

#### Scenario: 统计今日请求数
- **WHEN** 前端传入当日 `start_at` / `end_at`
- **THEN** `totals.request_count` 等于该区间内持久化的请求记录数

#### Scenario: 成功率计算
- **WHEN** 区间内存在成功与失败记录
- **THEN** `totals.success_rate` 等于成功数除以总数

#### Scenario: 无请求
- **WHEN** 区间内没有请求记录
- **THEN** `request_count` 为 0，`success_rate` 为 null

### Requirement: 记录失败不影响聊天请求
请求记录的写入失败 MUST NOT 导致聊天请求失败或被中断。记录操作 MUST 尽力而为，不得改变聊天响应的状态码或内容。

#### Scenario: 落库失败仍正常响应
- **WHEN** 请求记录写入发生错误
- **THEN** 聊天请求仍按原流程返回正常响应
