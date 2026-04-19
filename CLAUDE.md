# NexusAPI · 项目约定

> 本文档是本仓库的"宪法"。实现任何改动前先读这里。
> 完整工程蓝图：`/Users/stone/.claude/plans/parsed-purring-pebble.md`

## 项目定位

自建 AI 大模型中转网关 + 用户体系 + 按量计费。对标 new-api，但架构更干净。

## 架构约束

### 后端分层（强制）

```
backend/internal/
  domain/      # 纯领域模型 + 仓储接口。零外部依赖。
  app/         # 应用服务。编排 domain。依赖 domain 接口。
  infra/       # 基础设施实现。GORM/Redis/HTTP client/provider。
  interface/   # 入口层。HTTP handler / WS / CLI。
```

**禁止**：
- `domain/` 引用任何第三方库（除了 `time`/`encoding/json` 标准库）
- `app/` 直接 import `gorm.io/gorm` 或具体 provider 包
- `infra/` 反向依赖 `app/`
- `controller` 层混业务逻辑（handler 只做绑定 → 调用 service → 绑定响应）

### 前端分层

- `web-shared/` 共享组件、API client、i18n、hooks
- `web-user/` `web-admin/` 只依赖 `web-shared/`，互不依赖
- 三包都通过 pnpm workspace 管理，Bazel 一致构建

## 设计原则（SOLID/KISS/YAGNI/DRY 落地）

1. **新增供应商**：在 `infra/provider/{name}/` 新增一个包，实现 `relay.Adaptor` 接口。**不改** `app/relay/` 主干。
2. **新增能力**：在 `domain/relay/adaptor.go` 加 `Capability` 常量，在 provider 的 `Supports()` 中返回。
3. **计费唯一入口**：所有扣费通过 `app/billing.Engine`（Reserve→Settle→Refund）。**不允许**在 handler 或 provider 中直接写 DB。
4. **配置**：全部走 viper → `internal/config.Config`。**不允许**散落 `os.Getenv` 在业务代码里。
5. **错误**：用 `pkg/errors` 包装，带错误码 + 用户可见消息。HTTP 响应统一 `{code, message, request_id}`。
6. **日志**：zap structured logging，字段命名用 snake_case。

## 代码注释约定

- 注释语言：**中文**（与项目文档一致）
- 公开 API（exported 函数/类型）必须有注释
- 不写废话注释（`// i := 0 计数器`）
- 设计决策放在包级 `doc.go` 或函数头部

## Bazel 约定

- 任何新 Go 包：运行 `bazel run //:gazelle` 自动生成 `BUILD.bazel`
- 新 Go 依赖：`go get xxx` → 更新 `go.work` → 运行 `bazel run //:gazelle_update_repos` → 在 `MODULE.bazel` `use_repo(go_deps, ...)` 中添加
- 新前端包：加到 `pnpm-workspace.yaml`，运行 `pnpm install` → Bazel 自动识别
- 镜像命名：`ghcr.io/yishuiliunian/nexusapi-{service}`

## 测试

- 后端单测：`_test.go` 并列放
- 集成测试：用 `testcontainers-go` 起 Postgres+Redis
- 前端：Playwright e2e + Vitest 单测（后续里程碑引入）
- 运行：`bazel test //backend/...`

## Git 约定

- 分支：`main`（保护） + `feat/*`、`fix/*`、`chore/*`
- Commit message 用 Conventional Commits（`feat:`、`fix:`、`chore:`、`docs:`、`refactor:`、`test:`）
- PR 需通过 `bazel build //...` + `bazel test //...` 绿灯

## 里程碑

当前：**M0 工程骨架**。见 plan 文件 M0-M5 细节。

## 参考

- new-api（仅读设计，不抠代码）：`/Users/stone/Works/Infra/new-api`
- AIO Bazel 规范（构建宏来源）：`/Users/stone/Works/AIO/AIO/DevOps/BuildSystem/Server/`
