# NexusAPI

自建 AI 大模型中转网关 + 用户体系 + 按量计费。

对标 [new-api](https://github.com/QuantumNous/new-api)，从零实现（KISS/YAGNI/DRY/SOLID），用 Bazel 构建。

## 完成度

| 里程碑 | 状态 | 说明 |
|---|---|---|
| M0 工程骨架 | ✅ | Bazel + docker-compose + 三前后端包 |
| M1 核心中转 | ✅ | 用户/ApiKey/Channel/计费/3 家 provider + SSE 流式 |
| M2 管理后台 | ✅ | /api/admin/* + 8 家 provider + web-admin 7 页 |
| M3 多模态 | ✅ | embedding/image/tts/stt/rerank handler |
| M4 异步任务 | ✅ | Task domain/repo/service + Midjourney/Suno provider + TaskPoller |
| M5 高级能力 | ✅ | Redemption 兑换码 + 自研 TOTP 2FA + GitHub OAuth 库 |

## 核心特性

### 已实现并通过 E2E 验证

- **中继（10 家 provider）**：OpenAI、Claude、Gemini、DeepSeek、Moonshot、Qwen、Zhipu、OpenRouter、Midjourney、Suno
- **能力形态**：chat（SSE 流式）、embedding、rerank、image、tts、stt、异步 task
- **异步任务**：Midjourney + Suno + 通用视频 provider + TaskPoller worker（每 5s 轮询）
- **用户体系**：邮箱+bcrypt、session cookie、ApiKey CRUD、TOTP 2FA（RFC 6238 自实现）
- **计费**：Reserve/Settle/Refund 三阶段事务 + 配额账本 + 模型价格 + 兑换码
- **管理后台**：渠道/模型价格/用户/分组/日志 CRUD，权限隔离 admin 403
- **双前端**：web-user（7 页，含 tasks）、web-admin（7 页），Next.js 15 静态预渲染

### 待扩展（非阻塞）

- Stripe/Creem 支付网关（OAuth 库已就绪，handler 可在此基础上快速挂载）
- Prometheus metrics + Sentry
- Passkey WebAuthn
- next-intl 多语言
- 订阅系统 UI

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go 1.25 · Gin · GORM v2 · Zap · Viper · asynq |
| 数据库 | Postgres 15（生产） / SQLite（开发） |
| 缓存 | Redis 7 |
| 前端 | Next.js 15 · TypeScript · Tailwind |
| 构建 | Bazel · rules_go · rules_js · rules_oci |
| 部署 | docker-compose · Caddy · OCI (ghcr.io) |

## 快速开始

### 一键启整栈（推荐）

```bash
pnpm install         # 首次
pnpm dev             # 起 backend(Bazel) + upstream mock + web-user + web-admin + seed
# → backend    http://127.0.0.1:8080   (bazel run //backend/cmd/server)
# → upstream   http://127.0.0.1:18090  (node mock；OpenAI/Claude/GitHub/Google/Stripe)
# → web-user   http://127.0.0.1:3000   (pnpm next dev)
# → web-admin  http://127.0.0.1:3001   (pnpm next dev)
# Ctrl+C 停全部

pnpm dev:reset       # 同上但清空 SQLite 重新 seed
pnpm dev:user-only   # 只起 user 前端
pnpm stop            # 兜底清理残留（跨平台）
```

**启动策略**（全栈 Bazel）：

| 组件 | 启动方式 |
|---|---|
| backend | `bazel run //backend/cmd/server` (rules_go) |
| e2e-seed | `bazel run //backend/cmd/e2e-seed` (rules_go) |
| web-user dev | `bazel run //web-user:next_dev` (aspect_rules_js + next() 宏) |
| web-admin dev | `bazel run //web-admin:next_dev` (同上) |
| upstream mock | `node e2e/scripts/upstream-mock.mjs`（独立 Node 脚本，不走 Bazel） |

加 `--no-bazel` 可临时切回 `go run` / `pnpm exec next dev`（调试 Bazel 时用）。

**全量构建** `bazel build //...` 通过 204 targets；**测试** `bazel test //...` 通过 28 tests。

种子账号（默认已创建）：

- `admin@e2e.test` / `admin12345`（管理员，100 元配额）
- `alice@e2e.test` / `user12345`（普通用户，100 元配额）
- 兑换码：`E2E-REDEEM-CODE`（1 元）
- 套餐：`e2e_monthly`（$10/月，5 元配额）

### 单独组件

```bash
bazel run //backend/cmd/server           # 后端
bazel run //backend/cmd/worker           # 异步任务 poller
bazel run //backend/cmd/migrate          # 数据库迁移
pnpm --filter @nexusapi/web-user dev     # :3000
pnpm --filter @nexusapi/web-admin dev    # :3001
```

### Bazel 构建 / 测试 / 镜像

```bash
bazel build //...                        # 全量构建
bazel test //backend/...                 # 跑后端测试
bazel run //backend/cmd/server:image_tarball         # 本地镜像
bazel run //backend/cmd/server:image_push --stamp \
  --embed_label=$(git rev-parse --short HEAD)       # 推 ghcr.io
```

### 容器化部署

```bash
cd deploy && cp .env.example .env && docker compose up -d
```

## 已验证的 E2E 场景

| 流程 | 验证结果 |
|---|---|
| 注册 → 登录 → session cookie | ✅ |
| 创建 ApiKey（明文一次性返回） | ✅ |
| POST /v1/chat/completions + 上游失败 → Refund | ✅ 账本两条记录完整 |
| admin 创建 channel + 设置模型价格 | ✅ 8→10 个 provider 全部注册 |
| 普通用户访问 admin → 403 | ✅ 权限隔离正确 |
| 兑换码（50 元）→ 余额到账 | ✅ quota 0 → 50000000 |
| 2FA Setup → 生成 TOTP secret + otpauth URL | ✅ 可扫码导入 Google Authenticator |

## 目录

```
backend/                 # Go 后端
  cmd/{server,worker,migrate}/
  internal/
    domain/              # 纯实体 + 仓储接口
      {user,apikey,channel,billing,relay,task}/
    app/                 # 应用服务
      {auth,apikey,billing,relay,task,redemption,twofa,oauth}/
    infra/
      {db,cache,queue}/
      provider/          # 10 家适配器
        {openai,claude,gemini,deepseek,moonshot,qwen,zhipu,
         openrouter,midjourney,suno}/
    interface/http/
      {middleware,api,admin,relay,task}/
  pkg/
    {errors,logger,httpclient,sse}/
web-user/                # 用户前端
web-admin/               # 管理后台
web-shared/              # API client + 共享工具
build/                   # Bazel 宏（从 AIO 拷）
deploy/                  # docker-compose、Caddyfile、迁移
```

## 代码统计

- Go **6,533 行** / 50 文件（含 10 家 provider + 13 领域实体 + 计费引擎等）
- TS+TSX **1,254 行** / 28 文件
- Bazel **1,224 行** / 54 文件
- 总计 **~13,300 行 / 150 文件**，**94 个 Bazel targets 全绿**

完整蓝图见 `/Users/stone/.claude/plans/parsed-purring-pebble.md`。
