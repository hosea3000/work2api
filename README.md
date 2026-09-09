# work2api — CodeBuddy Gateway

将腾讯 CodeBuddy 上游服务封装为 OpenAI Chat Completions 兼容接口，提供多凭证轮换、每日自动签到和 Web 管理台。基于 Go (nunu 模板：gin + gorm + sqlite) 实现，参考 [codebuddy2api](https://github.com/IceeAn/codebuddy2api) 的上游协议逆向成果。

## 核心功能

- `POST /openai/v1/chat/completions`：OpenAI 兼容聊天入口，支持流式（SSE）与非流式（自动聚合上游流）
- `GET /openai/v1/models`：配置模型 ∪ 上游实际模型（`/v3/config`，带 TTL 缓存）
- `sk-...` API Key 鉴权（SHA-256 存储，明文仅创建时展示一次）
- 凭证池：SQLite 存储 + 内存 round-robin 轮换（每 N 次请求切换，可配置），401/403 自动摘除
- 凭证管理：手动粘贴 bearer_token（JWT 解析提取 user_id / 企业信息）、选择、测试、删除
- **OAuth 设备授权**：管理台点"开始认证"→ 跳转 CodeBuddy 网页登录 → 凭证自动入库；refresh_token 每小时自动续期（临期 24h 窗口），凭证长期可用
- 每日自动签到（09:30 本地时区 + 随机抖动防风控）与手动签到，幂等判定（`code==0` 或"已签到"）
- Vue3 管理台（来自 codebuddy2api 前端）：登录 / 凭证 / API Key / 状态页
- 上游 CLI 伪装头集（x-stainless-*、X-IDE-*、X-Conversation-* 等，版本可配置）

## 快速开始

### 1. 配置

编辑 `config/local.yml`：

```yaml
codebuddy:
  api_endpoint: https://copilot.tencent.com
  admin_username: admin
  admin_password: admin123       # 仅用于首启动建户
```

### 2. 初始化数据库

```bash
go run ./cmd/migration
```

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

# 创建 API Key（明文只展示一次）
curl -b cookies.txt -X POST http://127.0.0.1:8000/api/admin/api-keys \
  -H "Content-Type: application/json" -d '{"name":"my-key"}'

# 添加凭证（方式一：OAuth 设备授权，推荐）
# 管理台 → 凭证管理 → "开始认证" → 在弹出的 CodeBuddy 页面登录即可，
# 凭证自动入库并进入轮换池；后台每小时自动续期。

# 添加凭证（方式二：手动粘贴）
curl -b cookies.txt -X POST http://127.0.0.1:8000/api/admin/credentials \
  -H "Content-Type: application/json" \
  -d '{"bearer_token":"<你的 CodeBuddy token>"}'

# 聊天
curl http://127.0.0.1:8000/openai/v1/chat/completions \
  -H "Authorization: Bearer sk-你的key" \
  -H "Content-Type: application/json" \
  -d '{"model":"glm-5.2","messages":[{"role":"user","content":"你好"}]}'
```

## 配置项（config/local.yml → codebuddy 段）

| 键 | 默认 | 说明 |
|---|---|---|
| `api_endpoint` | `https://copilot.tencent.com` | 上游端点，必须在 `allowed_endpoints` 白名单内 |
| `models` | `glm-5.2,deepseek-v4-pro` | 附加模型（与上游实际模型合并） |
| `forced_temperature` | `"1"` | 强制覆盖温度；留空保留客户端值 |
| `rotation_count` | `1` | 每 N 次请求切换凭证 |
| `auto_checkin_enabled` | `true` | 启用每日自动签到 |
| `checkin_hour` / `checkin_minute` | `9` / `30` | 签到调度时间（服务器本地时区） |
| `background_delay_min/max_seconds` | `5` / `20` | 相邻签到请求的随机抖动区间 |
| `cli_version` | `2.107.0` | CLI 伪装版本号 |
| `admin_username` / `admin_password` | — | 首启动建户凭据 |

## 开发

```bash
make wire       # 重新生成 wire 依赖注入
make swag       # 重新生成 swagger 文档
make test       # 跑测试
go test -race ./internal/...
```

前端（管理台）位于 `web/`，改动后需重新构建并同步到 `internal/server/webdist/dist/`：

```bash
cd web && pnpm install && pnpm run build:bundle
cp -r dist ../internal/server/webdist/
```

## 项目结构

```
internal/
├── config/          codebuddy 配置段加载与 fail-fast 校验
├── upstream/codebuddy/  上游协议层：伪装头、SSE 解析、端点、错误映射
├── service/         凭证池、聊天执行、模型、会话、API Key、签到
├── handler/         HTTP 处理器：OpenAI 兼容、管理台、SPA 静态托管
├── middleware/      API Key / 会话鉴权
├── router/          路由挂载
├── model/           gorm 模型（admin_user/credential/api_key/checkin_record）
├── repository/      数据访问
├── job/             签到调度
└── bootstrap/       启动钩子
web/                 Vue3 管理台源码（构建产物嵌入二进制）
```

## 二期计划

OAuth 授权流、Anthropic 协议适配、额度探测与展示、用量统计、playground。

## License

MIT
