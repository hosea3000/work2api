# split-provider-credentials 任务

## 1. 模型与建表

- [x] 1.1 `internal/model`：新增 `CodeBuddyCredential`（TableName `codebuddy_credential`，含 domain/enterprise/oauth 扩展列）、`TraeCredential`（TableName `trae_credential`，含 machine_id/device_id）；移除旧 `Credential` 或标记弃用
- [x] 1.2 `internal/model`：新增 `CodeBuddyCheckinRecord`、`TraeCheckinRecord`（各自唯一索引 `(credential_id, checkin_date)`）
- [x] 1.3 `internal/model`：新增 `PoolState`（provider PK、auto_rotation、current_credential_id）
- [x] 1.4 `repository.EnsureSchema` / `cmd/migration`：AutoMigrate 新 5 张模型（去掉旧 `Credential`/`CheckinRecord`）

## 2. Repository 拆分

- [x] 2.1 新建 `CodeBuddyCredentialRepository`（codebuddy_credential CRUD + codebuddy_checkin_record Get/Save）
- [x] 2.2 新建 `TraeCredentialRepository`（trae_credential CRUD + trae_checkin_record Get/Save）
- [x] 2.3 新建 `PoolStateRepository`（GetByProvider / Upsert auto_rotation / SetCurrent / ClearCurrent）
- [x] 2.4 更新 `GatewayRepository` 接口与 wire provider，移除旧的混合凭证/签到方法 + 单测

## 3. Service 拆分

- [x] 3.1 `TraeCredentialPool` 补齐 `Current()/SelectCurrent()/SetAutoRotation()/AutoRotationEnabled()`；`CredentialPool` 保持
- [x] 3.2 拆 `CodeBuddyCredentialService`（pool + current/rotation 持久化 + SelectByToken/Test/MarkExpired/AddOAuth/Add/Delete/List）+ 单测
- [x] 3.3 拆 `TraeCredentialService`（pool + current/rotation 持久化 + SelectForChat/Test/MarkExpired/Delete/List）+ 单测
- [x] 3.4 拆 `CodeBuddyCheckinService` 与 `TraeCheckinService`（各自 repo/表）+ 单测
- [x] 3.5 `TraeLoginService` 改为写 `trae_credential`（TraeCredentialRepository）
- [x] 3.6 换调用方依赖：`ChatExecutor`/`ModelsService`→CodeBuddy；`TraeChatExecutor`/`TraeModelsService`→Trae；`TokenRefreshService`→CodeBuddy；`TraeTokenRefreshService`→Trae；`Startup`/`CheckinJobServer` 分别 reload 两池；`CheckinJob` 触发两个签到 service
- [x] 3.7 更新 `bootstrap/providers.go` 与 wire 依赖图

## 4. Handler 与路由

- [x] 4.1 拆 `CodeBuddyCredentialHandler`（list/create/delete/select/test/checkin/rotation）
- [x] 4.2 拆 `TraeCredentialHandler`（list/delete/select/test/checkin/rotation）
- [x] 4.3 `router/gateway.go`：注册 `/api/admin/codebuddy/credentials/*` 与 `/api/admin/trae/credentials/*`，移除 `/api/admin/credentials/*`
- [x] 4.4 确认 `/codebuddy/openai/v1/*` 与 `/trae/openai/v1/*` 聊天端点依赖各自 service（行为不变）

## 5. 前端

- [x] 5.1 `admin.ts`：两套凭证 API（codebuddy/trae 各自的 list/select/test/checkin/rotation/delete）；移除旧 `/api/admin/credentials/*` 调用
- [x] 5.2 `types`：拆分/复用凭证记录类型；新增两 provider 的 current/rotation 字段
- [x] 5.3 `CredentialsView.vue`：顶部 provider tab（CodeBuddy / TRAE），切换只挂载选中 provider 的登录面板 + 凭证池卡片（各自表格、当前标记、轮换开关、筛选）
- [x] 5.4 `vue-tsc` + `oxlint` 通过

## 6. 验证

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`（重点：两 service/池、签到、handler；聊天/刷新回归）
- [x] 6.2 清库启动冒烟：全新 DB 建 5 表 → 导入 codebuddy + TRAE 凭证 → 两卡片各自选择当前/切轮换 → 重启验证 current/rotation 持久化 → 两个聊天端点各自命中对应池
