// global-setup.ts —— Playwright 全局启动钩子。
//
// 工作流：
//   1. 检测 backend /healthz 是否已就绪
//      - 已就绪（dev.sh 在跑）→ 直接复用，仅 reseed
//      - 未就绪 → 调 deploy/dev/dev.sh --backend-only 拉起 docker infra + backend
//   2. 启动 web-user / web-admin 两个 Next.js dev（如果还没在跑）
//   3. 等所有前端 ready
//
// 进程表放 globalThis，全局 teardown 用。
import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import type { FullConfig } from '@playwright/test';
import { fetch as undiciFetch } from 'undici';
import { PORTS, postgresDSN, URLS } from '../helpers/env';
import { killAll, start, waitFor } from './process-manager';

// 当 e2e 通过 `bazel run //e2e:test` 启动时，cwd 在 bazel runfiles 副本，
// 但 spawn bazel 必须从源码树。bazel run 会设置 BUILD_WORKSPACE_DIRECTORY 指向源码根。
// 直接调（pnpm/node）时无此变量，fallback 到 cwd（开发者通常在仓库根跑）。
const ROOT = process.env.BUILD_WORKSPACE_DIRECTORY ?? process.cwd();
// 为 teardown 暴露。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).__nexus_teardown = killAll;

async function isHealthy(url: string): Promise<boolean> {
  try {
    const res = await undiciFetch(url, { method: 'GET' });
    return res.ok;
  } catch {
    return false;
  }
}

// reseed 直接调 `bazel run //backend/cmd/e2e-seed`，--reset 清空业务表再灌种子。
function reseed(): void {
  const r = spawnSync(
    'bazel',
    [
      'run',
      '//backend/cmd/e2e-seed',
      '--',
      '--postgres-dsn',
      postgresDSN(),
      '--upstream-url',
      URLS.upstream,
      '--reset',
    ],
    { cwd: ROOT, stdio: 'inherit' },
  );
  if (r.status !== 0) {
    throw new Error(`bazel run //backend/cmd/e2e-seed failed with status=${r.status}`);
  }
}

// killPortListeners 清掉占某端口的进程。用于回收残留前端：
// 之前 e2e 跑过留下的 next dev 可能 listen 着但不响应（HMR/编译卡死），
// 探活失败但 spawn 新前端会 EADDRINUSE，所以先杀掉端口持有者。
function killPortListeners(port: number): void {
  spawnSync('bash', ['-c', `lsof -nP -iTCP:${port} -sTCP:LISTEN -t 2>/dev/null | xargs -r kill -9 2>/dev/null || true`], {
    stdio: 'ignore',
  });
}

async function ensureFrontendDown(port: number, healthUrl: string): Promise<void> {
  if (await isHealthy(healthUrl)) return; // 复用健康进程
  killPortListeners(port);
}

export default async function globalSetup(_config: FullConfig): Promise<void> {
  const backendUrl = `http://127.0.0.1:${PORTS.backend}/healthz`;
  const userUrl = `http://127.0.0.1:${PORTS.webUser}/login`;
  const adminUrl = `http://127.0.0.1:${PORTS.webAdmin}/login`;

  // 1) backend（含 docker infra）
  if (await isHealthy(backendUrl)) {
    process.stderr.write(`[e2e] backend 已就绪（dev 在跑），跳过启动，仅 reseed\n`);
    reseed();
  } else {
    process.stderr.write(
      `[e2e] backend 未就绪，调 bazel run //deploy/dev:backend_only 拉起 infra+backend\n`,
    );
    const r = spawnSync('bazel', ['run', '//deploy/dev:backend_only'], {
      cwd: ROOT,
      stdio: 'inherit',
    });
    if (r.status !== 0) {
      throw new Error(`bazel run //deploy/dev:backend_only failed with status=${r.status}`);
    }
    // 该 target 已经跑过 seed，无需再 reseed
  }

  // 2) web-user / web-admin —— 探活，没起就 spawn（走 bazel run，与 dev.sh 一致）
  //
  // 外层 `bazel run //e2e:test` 设了：
  //   - BUILD_WORKING_DIRECTORY=<e2e/>
  //   - JS_BINARY__CHDIR=e2e
  // 这两个变量会被子 bazel 调用的 launcher 继承。其中 JS_BINARY__CHDIR 必须
  // 改为目标包的相对路径（否则 next 启动在仓库根，读不到 web-user/next.config.ts，
  // /api 转发 rewrites 失效），BUILD_WORKING_DIRECTORY 改为源码根方便 launcher 内 cd。
  const userEnv = {
    NEXUSAPI_BACKEND_URL: `http://127.0.0.1:${PORTS.backend}`,
    BUILD_WORKING_DIRECTORY: ROOT,
    JS_BINARY__CHDIR: 'web-user',
  };
  const adminEnv = {
    NEXUSAPI_BACKEND_URL: `http://127.0.0.1:${PORTS.backend}`,
    BUILD_WORKING_DIRECTORY: ROOT,
    JS_BINARY__CHDIR: 'web-admin',
  };

  // 上一轮残留可能 listen 着但不响应（HMR 卡死）：探活不通 → 强杀回收端口。
  await ensureFrontendDown(PORTS.webUser, userUrl);
  await ensureFrontendDown(PORTS.webAdmin, adminUrl);

  if (!(await isHealthy(userUrl))) {
    start({
      cwd: ROOT,
      cmd: 'bazel',
      args: ['run', '//web-user:next_dev', '--', '--port', String(PORTS.webUser)],
      env: userEnv,
      label: 'web-user',
    });
  }
  if (!(await isHealthy(adminUrl))) {
    start({
      cwd: ROOT,
      cmd: 'bazel',
      args: ['run', '//web-admin:next_dev', '--', '--port', String(PORTS.webAdmin)],
      env: adminEnv,
      label: 'web-admin',
    });
  }

  await Promise.all([waitFor(userUrl, 120_000), waitFor(adminUrl, 120_000)]);
}
