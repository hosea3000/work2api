# credential-pool 变更规范（增量）

## ADDED Requirements

### Requirement: TRAE 独立轮换池
系统 SHALL 在既有 codebuddy 轮换池之外维护一个独立的 TRAE 内存轮换池，二者完全隔离：codebuddy 池仅装 `provider=codebuddy`（含空 provider 存量）凭证，TRAE 池仅装 `provider=trae` 且 `status=active` 的凭证。TRAE 池 SHALL 支持独立 round-robin 选择（并发安全、原子快照），并 SHALL 在凭证新增/删除/token 刷新时随 codebuddy 池一并重载。TRAE 凭证 MUST NOT 进入 codebuddy 池，codebuddy 凭证 MUST NOT 进入 TRAE 池。

#### Scenario: 两池隔离
- **WHEN** 库内同时存在 codebuddy 与 trae 凭证，且两池均重载
- **THEN** codebuddy 池只含 codebuddy 凭证，TRAE 池只含 trae 凭证，各自 round-robin 互不影响

#### Scenario: 凭证变更双池同步
- **WHEN** TRAE 登录导入、凭证删除或 TRAE token 刷新成功
- **THEN** 两个池都按最新 DB 状态重载

#### Scenario: TRAE 池冷却跳过
- **WHEN** TRAE 池中某凭证处于冷却窗口内
- **THEN** round-robin 选择跳过该凭证，优先返回未冷却的可用凭证
