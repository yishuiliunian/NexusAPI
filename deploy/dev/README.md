# NexusAPI · Dev 环境

一键拉起本地开发栈，支持 **多 worktree 并发开发** 互不冲突。**全部走 Bazel target**，与项目整体口径一致。

## 快速开始

```bash
# 启动完整 dev（infra + backend + 双前端）
bazel run //deploy/dev:up
# 或：pnpm dev

# 只起 infra + backend（CI / 跑 e2e 用）
bazel run //deploy/dev:backend_only

# 全部清理（容器/卷/host 进程/.env）
bazel run //deploy/dev:clean
# 或：pnpm dev:clean

# 跑 e2e（探测到 dev 已起就复用；否则自动起 backend_only）
bazel run //e2e:test
# 或：pnpm e2e
```

也可直接 `bash deploy/dev/dev.sh`，三个 sh_binary 是 bazel 入口的封装。

## 端口分配规则

`port = 20000 + offset * 50`，offset 由 worktree 名决定：

| Worktree | offset | backend | upstream | web-user | web-admin | postgres | redis |
|---|---|---|---|---|---|---|---|
| `main` / `master` | 0 | 20000 | 20001 | 20002 | 20003 | 20004 | 20005 |
| 其他（哈希分配） | 1–500 | 20050+ | ... | ... | ... | ... | ... |

哈希：`md5(worktree_name) % 500 + 1`，步长 50，最多 500 个并发 worktree。

## 架构

```
docker compose ──────────────────────────────┐
  • postgres:15-alpine                       │
  • redis:7-alpine                           │ 基础设施
  • upstream-mock (node:20-alpine 挂源码)    │ 都在 docker
  • adminer:4 (postgres web UI)              │
                                              │
host (bazel run) ─────────────────────────────┤
  • bazel run //backend/cmd/server           │
  • bazel run //web-user:next_dev            │ 业务
  • bazel run //web-admin:next_dev           │ 在 host
```

业务为什么不进 docker：保留 next dev / rules_go 增量编译的热重载与调试器直连。

## Worktree 隔离机制

每个 worktree 自动拿到：

1. **独立端口段** — 不会和别的 worktree 撞
2. **独立 `COMPOSE_PROJECT_NAME`** — `nexusapi-<worktree>`，docker 容器/卷/网络全部加前缀
3. **独立 `.env`**（`deploy/dev/.env`，gitignored）
4. **独立前端 `.env.local`**（`web-user/.env.local`、`web-admin/.env.local`）

幂等：二次运行会保留已有端口，避免外部书签/OAuth 回调失效。

## 文件结构

```
deploy/dev/
├── BUILD.bazel                  # sh_binary: :up :backend_only :clean
├── dev_runner.sh                # bazel run 入口（cd 回源码树 exec dev.sh）
├── dev.sh                       # 主流程（也支持直接 bash 调用）
├── docker-compose.yml           # postgres / redis / upstream-mock / adminer
├── .env                         # 自动生成（gitignored）
├── lib/
│   ├── log.sh                   # 彩色日志
│   ├── worktree.sh              # worktree 探测 + 哈希端口分配
│   ├── config_gen.sh            # 生成 .env / web .env.local
│   ├── compose.sh               # docker compose 生命周期
│   ├── host_services.sh         # bazel run 业务进程 + pid/端口双兜底杀
│   └── lifecycle.sh             # clean / show_result / run_seed
├── seed/README.md               # seed 由 //backend/cmd/e2e-seed 完成
└── runtime/                     # pid / log（gitignored）
```

## 与 e2e 的关系

`e2e/playwright.config.ts` 与 `e2e/helpers/env.ts` 共用同一套端口配置：

1. **先起 dev 再跑 e2e**：`bazel run //deploy/dev:up` → `bazel run //e2e:test` 直接连进去
2. **直接跑 e2e**：`global-setup.ts` 探测 backend 未就绪 → 自动 `bazel run //deploy/dev:backend_only` 拉起，再 `bazel run //web-user:next_dev` / `//web-admin:next_dev` 起前端

CI 走第二种模式即可。

## 种子数据

由 `bazel run //backend/cmd/e2e-seed` 完成（同时被 dev.sh 和 e2e/global-setup 复用，避免维护两套 seed）：

| 账号 | 密码 | 用途 |
|---|---|---|
| `admin@e2e.test` | `admin12345` | admin 登录 |
| `alice@e2e.test` | `user12345` | 普通用户 |

并插入 2 条 channel（指向 upstream-mock）、模型价格表、1 条兑换码 `E2E-REDEEM-CODE`、1 份订阅套餐 `e2e_monthly`。

## 进程停止机制（重要细节）

`bazel run //backend/cmd/server` 经 rules_go wrapper 会 fork+exec 出 binary，binary 可能脱离 launch 时的 process group（PPID=1）。所以 `stop_host_services` 双兜底：

1. **群组杀**：按 perl `setsid` 时的 pid 文件 `kill -- -<pid>`
2. **端口兜底**：用 `lsof -iTCP:<port> -sTCP:LISTEN -t` 找端口持有者，强制 kill

第 2 步保证即使 group 链断了也能确定性地停掉 binary。

## 故障排查

- **端口被占**：`lsof -nP -iTCP:20000 -sTCP:LISTEN`；另一个 worktree 没清干净就 `bazel run //deploy/dev:clean`
- **容器 healthcheck 失败**：`docker compose -p nexusapi-<worktree> logs <service>`
- **backend 起不来**：`tail -50 deploy/dev/runtime/backend/backend.log`
- **完全重来**：`bazel run //deploy/dev:clean` 后再 `bazel run //deploy/dev:up`
- **gazelle 报多 worktree 警告**：根 `BUILD.bazel` 已 `gazelle:exclude .claude`
