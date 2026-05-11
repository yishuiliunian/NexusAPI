// e2e/helpers/env.ts — 端口/凭证从 deploy/dev/.env 读取，避免 e2e 与 dev 双轨。
//
// 优先级：
//   1. process.env（CI / 临时 override）
//   2. deploy/dev/.env（dev.sh 自动生成）
//   3. fallback 默认值（main worktree 的 20000 系列）
//
// 这样允许两种使用方式：
//   - 已经 `./deploy/dev/dev.sh` → e2e 直接连进已有服务
//   - 没起 dev → e2e 自己用同一套端口启动
//
// 文件用 CJS 友好写法（不使用 `import.meta.url`），让 playwright 1.x 的 CJS
// transform.js 在加载 playwright.config.ts → helpers/env 这条 require 链上不出错。

import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// findRepoRoot 在 BUILD_WORKSPACE_DIRECTORY 缺失时（pnpm/直跑场景）向上找
// 标志文件 deploy/dev/.env。bazel run //e2e:test 路径下，BUILD_WORKSPACE_DIRECTORY
// 已设为源码根，直接用最准。
function findRepoRoot(): string {
  const fromBazel = process.env.BUILD_WORKSPACE_DIRECTORY;
  if (fromBazel) return fromBazel;
  let cur = process.cwd();
  for (let i = 0; i < 6; i++) {
    if (existsSync(resolve(cur, 'deploy/dev/.env'))) return cur;
    const parent = resolve(cur, '..');
    if (parent === cur) break;
    cur = parent;
  }
  return process.cwd();
}

const DOT_ENV = resolve(findRepoRoot(), 'deploy', 'dev', '.env');

let cached: Record<string, string> | null = null;

function parseEnvFile(): Record<string, string> {
  if (cached) return cached;
  if (!existsSync(DOT_ENV)) {
    cached = {};
    return cached;
  }
  const out: Record<string, string> = {};
  for (const line of readFileSync(DOT_ENV, 'utf8').split('\n')) {
    const t = line.trim();
    if (!t || t.startsWith('#')) continue;
    const eq = t.indexOf('=');
    if (eq < 0) continue;
    const key = t.slice(0, eq).trim();
    const val = t.slice(eq + 1).trim().replace(/^["']|["']$/g, '');
    out[key] = val;
  }
  cached = out;
  return out;
}

/** 取一个端口/字符串配置：env > .env file > fallback。 */
export function getEnv(key: string, fallback: string): string {
  return process.env[key] ?? parseEnvFile()[key] ?? fallback;
}

/** 取整型端口。 */
export function getPort(key: string, fallback: number): number {
  return Number(getEnv(key, String(fallback)));
}

// 端口与 base URL —— 与 deploy/dev/lib/config_gen.sh 的 slot 完全对齐。
export const PORTS = {
  backend: getPort('BACKEND_PORT', 20000),
  upstream: getPort('UPSTREAM_PORT', 20001),
  webUser: getPort('WEB_USER_PORT', 20002),
  webAdmin: getPort('WEB_ADMIN_PORT', 20003),
  postgres: getPort('POSTGRES_PORT', 20004),
  redis: getPort('REDIS_PORT', 20005),
} as const;

export const URLS = {
  api: `http://127.0.0.1:${PORTS.backend}`,
  user: `http://127.0.0.1:${PORTS.webUser}`,
  admin: `http://127.0.0.1:${PORTS.webAdmin}`,
  upstream: `http://127.0.0.1:${PORTS.upstream}`,
} as const;

/** Postgres DSN（给 backend / seed 用）。 */
export function postgresDSN(): string {
  const user = getEnv('POSTGRES_USER', 'nexusapi');
  const pass = getEnv('POSTGRES_PASSWORD', 'nexusapi_dev');
  const db = getEnv('POSTGRES_DB', 'nexusapi');
  return `postgres://${user}:${pass}@127.0.0.1:${PORTS.postgres}/${db}?sslmode=disable`;
}

/** worktree 名字（用于诊断输出）。 */
export function worktreeName(): string {
  return getEnv('WORKTREE_NAME', 'unknown');
}

/** Compose project 名（docker compose -p 用）。 */
export function composeProjectName(): string {
  return getEnv('COMPOSE_PROJECT_NAME', 'nexusapi-main');
}

/** 种子数据账号（与 backend/cmd/e2e-seed 默认值一致）。 */
export const SEED_ADMIN = { email: 'admin@e2e.test', password: 'admin12345' } as const;
export const SEED_USER = { email: 'alice@e2e.test', password: 'user12345' } as const;
