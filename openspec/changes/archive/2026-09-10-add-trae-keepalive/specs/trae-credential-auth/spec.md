# trae-credential-auth 变更规范（增量）

## ADDED Requirements

### Requirement: TRAE 凭证定时刷新
系统 SHALL 以每小时为周期（启动后立即执行首轮，此后 ticker 驱动）扫描全部 `provider=trae && auth_source=web_login` 的凭证：对满足刷新条件（refresh_token 非空、expires_at 临期即 now ≥ expires_at − 24h）的凭证调用 TRAE `ExchangeToken`（refreshToken 轮换）。相邻刷新请求 SHALL 加入随机抖动（5~20 秒，对齐 codebuddy 刷新节奏）。刷新成功 SHALL 原位更新 bearer_token、refresh_token（响应含新值时）、expires_at 与 last_refresh_at；刷新响应缺失 refreshToken 时 SHALL 沿用旧值。刷新判定 MUST 与 codebuddy 刷新（`auth_source=oauth`）相互独立，两套扫描 MUST NOT 触碰对方 provider 的凭证。

#### Scenario: 临期 TRAE 凭证被刷新
- **WHEN** 某 provider=trae 凭证 expires_at 距 now 不足 24 小时且 refresh_token 非空
- **THEN** 每轮扫描中该凭证经 ExchangeToken 刷新，bearer_token/refresh_token/expires_at/last_refresh_at 更新，凭证保持 active

#### Scenario: 刚导入的凭证空转
- **WHEN** 某 TRAE 凭证离过期尚远（> 24h）
- **THEN** 每小时扫描跳过该凭证，不发上游请求

#### Scenario: refreshToken 轮换失效
- **WHEN** ExchangeToken 返回 refresh_failed（上游拒绝 refreshToken）
- **THEN** 凭证被标记 expired，等待管理员重新登录导入（既有"重新登录"链路）

#### Scenario: codebuddy 凭证不被 trae 扫描触碰
- **WHEN** TRAE 刷新扫描遍历凭证列表
- **THEN** 仅处理 provider=trae 的凭证，provider=codebuddy（含空 provider 存量数据）凭证一律跳过
