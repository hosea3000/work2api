# AGENTS.md

CodeBuddy / TRAE 网关：Go（Gin + GORM + SQLite）后端 + `web/` Vue3 管理台（构建产物嵌入二进制）。两个上游 provider（codebuddy / trae）完全平行——分表、分凭证池、分执行器，改一个通常要同步另一个。

## 常用命令

- 启动服务：`go run ./cmd/server`（默认读 `config/local.yml`，可用 `APP_CONF` 或 `-conf` 覆盖；默认端口 8000）
- 初始化 / 迁移数据库：`go run ./cmd/migration`
- 构建：`make build` → `./bin/server`
- 测试：`go test ./...`（`go test -race ./internal/...`）。注意 `make test` 只跑 `./test/server/...`（nunu 脚手架测试），`internal/**` 的单测不在其中
- 前端：`cd web && pnpm install && pnpm run build:bundle`；另有 `pnpm typecheck` / `pnpm lint`(oxlint) / `pnpm format`(prettier)

## 前端嵌入（易踩坑）

- `web/embed.go` 用 `//go:embed all:dist`，**`go build` 前 `web/dist` 必须已构建**，否则编译直接失败；全新 clone 没有 `web/dist`。
- 顺序：先 `cd web && pnpm install && pnpm run build:bundle`，再 `go build ./...`。
- `pnpm build` = vue-tsc + vite（含类型检查）；`build:bundle` = 仅 vite。

## 代码生成

- **`make wire` 在本仓库不存在**：wire 用 `go generate ./cmd/server/wire/` 重新生成 `cmd/server/wire/wire_gen.go`。改了任何构造函数（尤其 DI 参数）后必须重跑。
- 无 sqlc（数据访问是手写 GORM repository）。
- swagger：`make swag` → `docs/`（`docs.go` / `swagger.json` / `swagger.yaml` 被 gitignore）。

## 数据库 / 模型

- SQLite；启动时 `bootstrap.Startup` → `EnsureSchema` 自动 AutoMigrate 自愈建表。
- 新增 model 必须**同时**加到 `internal/repository/gateway.go` 的 `EnsureSchema` 与 `internal/server/migration.go` 的 `AutoMigrate`。

## 结构

- `internal/service` 业务（凭证池、聊天执行器）；`internal/upstream/{codebuddy,trae}` 上游协议；`internal/handler` + `internal/router` HTTP；`internal/repository` 数据访问；`internal/bootstrap` 启动钩子；`internal/middleware` 鉴权等。
- 聊天请求计数：`middleware.RecordRequest` 只挂在 4 个 chat completions 路由（外部 + Playground，两 provider），`/models` 不计数。

## OpenSpec 工作流

- 变更用 `openspec` CLI 管理：change 在 `openspec/changes/`，主 specs 在 `openspec/specs/`。
- 归档：移动到 `openspec/changes/archive/YYYY-MM-DD-<name>/`，并把 delta specs 同步到主 specs。

## 约定

- Commit：Conventional Commits + 中文描述，如 `feat(admin): 实现总览页指标`。
- 代码注释用中文。
- 配置：`config/local.yml`（gitignore）/ `config/prod.yml`。
- 全局 CLAUDE.md 里的 `make wire` / `make sqlc` / `make buildx env=...` 在本仓库都不存在，勿照抄。
