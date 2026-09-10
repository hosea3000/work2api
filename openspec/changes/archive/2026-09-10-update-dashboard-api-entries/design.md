## Context

总览页（`web/src/views/DashboardView.vue`）的两张入口卡片数据来自 `GET /api/admin/status`：`api_base_url` 为相对路径 `/codebuddy/openai/v1`，`anthropic_api_base_url` 恒为空字符串（`internal/handler/admin_stub_handler.go:28-29`）。网关实际暴露两个 OpenAI 兼容入口：`/codebuddy/openai/v1`（`internal/router/gateway.go:136`）与 `/trae/openai/v1`（`gateway.go:144`）。管理台与网关由同一 Go 二进制、同一 origin 提供服务。

## Goals / Non-Goals

**Goals:**
- 总览页准确展示两个真实入口（CodeBuddy / TRAE）的完整可复制地址。
- 移除过时的 Anthropic 入口与提示。
- 不引入新的后端 URL 构造逻辑或配置项。

**Non-Goals:**
- 不新增 Anthropic 协议入口。
- 不支持管理台与 API 网关分属不同域名时自动推导公网地址。
- 不改变两个入口的路由或鉴权行为。

## Decisions

### 决策 1：完整地址由前端基于浏览器 origin 拼接

`window.location.origin` 拼接常量路径 `/codebuddy/openai/v1`、`/trae/openai/v1`。

- 理由：管理台与网关同源（单二进制），浏览器 origin 即对外入口地址，后端零改动、无新配置。
- 备选：后端从 `c.Request.Host` + `X-Forwarded-Proto` 构造——多代码且需处理 TLS 终止；配置 `public_base_url`——需新增配置与回退，当前无分域需求，YAGNI。

### 决策 2：两个路径作为前端常量，不再由 status 接口提供

- 理由：路径是前端展示契约的一部分，仅两处常量；避免后端字段命名与实际含义不符（`anthropic_api_base_url` 复用给 TRAE 会造成误导）。
- 备选：保留 `api_base_url` 作为路径来源、新增 trae 字段——增加后端字段与前端解构，收益低。

### 决策 3：移除 status 响应中不再使用的 URL 字段

删除 `api_base_url`、`anthropic_api_base_url`（后端 handler 与前端 `AdminStatus` 类型同步）。

- 理由：删除优于保留死字段，避免后续误用。
- 影响面：仅管理台前端消费该接口，无外部消费者。

## Risks / Trade-offs

- [反向代理挂子路径部署时 origin 不含子路径] → 当前 vite base 为根路径、应用按根部署；如未来支持子路径，改用配置化 base 或后端构造。
- [管理台域名与 API 公网域名不一致时复制到的是管理台地址] → 当前部署为单域名；若分域再引入 `public_base_url` 配置。
- [前后端字段删除需同步] → 同仓同版本发布，前端不再读取该字段，后端删除无兼容风险。

## Migration Plan

1. 前端改为 origin 拼接并删除 Anthropic 卡片。
2. 后端 `Status` 移除两个字段、前端类型同步移除。
3. 回滚：还原上述文件即可，无数据迁移。

## Open Questions

（无）
