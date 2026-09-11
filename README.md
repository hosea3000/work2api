# work2api — CodeBuddy / TRAE Gateway

将腾讯 CodeBuddy 与 TRAE SOLO 上游服务封装为 OpenAI Chat Completions 兼容接口，提供多凭证轮换、每日自动签到、额度探测与 Web 管理台。基于 Go（nunu 模板：gin + gorm + sqlite）实现，参考 [codebuddy2api](https://github.com/IceeAn/codebuddy2api) 的上游协议逆向成果。

## 核心功能

- 双 provider OpenAI 兼容入口，各自独立凭证池与执行器：
  - `POST /codebuddy/openai/v1/chat/completions`、`GET /codebuddy/openai/v1/models`
  - `POST /trae/openai/v1/chat/completions`、`GET /trae/openai/v1/models`
  - 均支持流式（SSE）与非流式（自动聚合上游流）
- `sk-...` API Key 鉴权（明文持久化 + SHA-256 索引校验，列表可随时复制）
- 凭证池：SQLite 存储 + 内存 round-robin 轮换（每 N 次请求切换），401/403 自动摘除，TRAE 额外带冷却
- 凭证管理：手动粘贴 bearer_token（JWT 解析提取 user_id / 企业信息）、选择、测试、删除、签到、额度刷新、轮换开关
- CodeBuddy OAuth 设备授权 + TRAE 网页登录，凭证自动入库；refresh_token 自动续期
- 额度探测与展示（`剩余 / 总额`，未探测为 null）
- 每日自动签到（服务器本地时区 + 随机抖动防风控）与手动签到，幂等判定（`code==0` 或「已签到」）
- Vue3 管理台：总览 / 凭证 / API Key / API 测试（Playground）/ 设置
- 总览页指标：服务状态、合并的有效凭证数、当日真实请求数与成功率（请求计数持久化于 `request_record`）
- 上游 CLI 伪装头集（x-stainless-*、X-IDE-*、X-Conversation-* 等，版本可配置）

## 快速开始

### 1. 配置

编辑 `config/local.yml`（首启动至少需要 `codebuddy.admin_password`）：

```yaml
codebuddy:
  api_endpoint: https://copilot.tencent.com
  admin_username: admin
  admin_password: admin123       # 仅用于首启动建户
trae:
  callback_port: 18080           # TRAE 网页登录回调端口（远程部署需映射）
```

也可用环境变量 `APP_CONF=/path/to/xxx.yml` 或 `-conf` 覆盖配置路径。

### 2. 初始化数据库

```bash
go run ./cmd/migration
```

> SQLite 库表也会在服务启动时经 `EnsureSchema` 自动迁移，通常可跳过此步。

### 3. 启动服务

```bash
go run ./cmd/server
# 或
make build && ./bin/server
```

访问 `http://127.0.0.1:8000` 打开管理台。

### 4. 验证

```bash
# 登录（拿 Cookie）
curl -c cookies.txt -X POST http://127.0.0.1:8000/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 创建 API Key（列表可随时复制）
curl -b cookies.txt -X POST http://127.0.0.1:8000/api/admin/api-keys \
  -H "Content-Type: application/json" -d '{"name":"my-key"}'

# 添加 CodeBuddy 凭证（方式一：管理台 → 凭证 → 开始认证，OAuth 设备授权，推荐）
# 添加 CodeBuddy 凭证（方式二：手动粘贴）
curl -b cookies.txt -X POST http://127.0.0.1:8000/api/admin/codebuddy/credentials \
  -H "Content-Type: application/json" \
  -d '{"bearer_token":"<你的 CodeBuddy token>"}'

# TRAE 凭证：管理台 → 凭证 → TRAE 网页登录（或粘贴回调地址导入）

# 聊天
curl http://127.0.0.1:8000/codebuddy/openai/v1/chat/completions \
  -H "Authorization: Bearer sk-你的key" \
  -H "Content-Type: application/json" \
  -d '{"model":"glm-5.2","messages":[{"role":"user","content":"你好"}]}'
```

## 配置项（`config/local.yml`）

| 键 | 默认 | 说明 |
|---|---|---|
| `codebuddy.api_endpoint` | `https://copilot.tencent.com` | 上游端点，必须在 `allowed_endpoints` 白名单内 |
| `codebuddy.allowed_endpoints` | `copilot.tencent.com,www.codebuddy.ai` | 上游端点白名单（CSV） |
| `codebuddy.models` | `glm-5.2,deepseek-v4-pro` | 附加模型（与上游实际模型合并） |
| `codebuddy.forced_temperature` | `"1"` | 强制覆盖温度；留空保留客户端值 |
| `codebuddy.rotation_count` | `1` | 每 N 次请求切换凭证 |
| `codebuddy.models_cache_ttl_seconds` | `30` | 模型列表缓存 TTL |
| `codebuddy.auto_checkin_enabled` | `true` | 启用每日自动签到 |
| `codebuddy.checkin_hour` / `checkin_minute` | `9` / `30` | 签到调度时间（服务器本地时区） |
| `codebuddy.background_delay_min/max_seconds` | `5` / `20` | 相邻签到请求的随机抖动区间 |
| `codebuddy.cli_version` | `2.107.0` | CLI 伪装版本号 |
| `codebuddy.admin_username` / `admin_password` | `admin` / — | 首启动建户凭据（password 必填） |
| `trae.callback_port` | `18080` | TRAE 网页登录回调端口（远程部署需映射） |

## 开发

```bash
make build      # 编译后端 → ./bin/server
make swag       # 重新生成 swagger 文档（docs/）
go test ./...   # 全部测试（含 internal/** 单测）
go test -race ./internal/...
# 注意：make test 只跑 ./test/server/...（脚手架测试），不含 internal/** 单测
```

wire 依赖注入**没有 Makefile 目标**，用：

```bash
go generate ./cmd/server/wire/   # 重新生成 cmd/server/wire/wire_gen.go
```

前端（管理台）位于 `web/`，构建产物经 `web/embed.go` 嵌入二进制。**`go build` 前必须先构建前端**，否则 `//go:embed all:dist` 会编译失败：

```bash
cd web && pnpm install && pnpm run build:bundle   # 仅 vite；pnpm build 额外跑 vue-tsc 类型检查
cd .. && go build -o bin/server ./cmd/server
```

## 项目结构

```
cmd/
├── server/         HTTP 服务入口
├── migration/      建表入口
└── task/           定时任务入口
internal/
├── config/         codebuddy / trae 配置段加载与 fail-fast 校验
├── upstream/
│   ├── codebuddy/  CodeBuddy 协议层：伪装头、SSE 解析、错误映射
│   └── trae/       TRAE 协议层
├── service/        凭证池、聊天执行、模型、会话、API Key、签到、额度、统计
├── handler/        HTTP 处理器：OpenAI 兼容、管理台、SPA 静态托管
├── middleware/     API Key / 会话鉴权、请求计数
├── router/         路由挂载
├── model/          gorm 模型（admin_user / api_key / *_credential / *_checkin_record / pool_state / request_record）
├── repository/     数据访问
├── job/            签到调度
└── bootstrap/      启动钩子
web/                Vue3 管理台源码（构建产物嵌入二进制）
openspec/           OpenSpec 变更与规格
```

## 后续计划

- 设置项持久化（当前 `GET/PUT /api/admin/settings` 为打桩）
- 完整用量统计（token / credit / p95 延迟，按模型 / API Key / 凭证维度）

## License

MIT
