// global-setup.ts —— Playwright 全局启动钩子。
//
// 流程：
//  1. 清理旧 SQLite 文件
//  2. 启动 upstream-mock（:18090）
//  3. 启动 backend server（:18080，SQLite，关 OAuth/Stripe）
//  4. 等 backend /healthz
//  5. 运行 e2e-seed 插 admin/user/channel/prices
//  6. 启动 web-user dev（:13000）
//  7. 启动 web-admin dev（:13001）
//  8. 等 user/admin 前端 ready
//
// 进程表放 globalThis，全局 teardown 用。
import { mkdirSync, rmSync, existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { FullConfig } from '@playwright/test';
import { killAll, start, waitFor } from './process-manager';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..', '..');

// 为 teardown 暴露。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).__nexus_teardown = killAll;

export default async function globalSetup(_config: FullConfig): Promise<void> {
  const BACKEND_PORT = 18080;
  const WEB_USER_PORT = 13000;
  const WEB_ADMIN_PORT = 13001;
  const UPSTREAM_PORT = 18090;
  const DB_DIR = resolve(ROOT, 'e2e', '.tmp');
  const DB_PATH = resolve(DB_DIR, 'nexus-e2e.db');
  mkdirSync(DB_DIR, { recursive: true });

  // 清旧 DB
  for (const suffix of ['', '-journal', '-wal', '-shm']) {
    const p = DB_PATH + suffix;
    if (existsSync(p)) rmSync(p);
  }

  // 1) upstream mock
  start({
    cwd: ROOT,
    cmd: 'node',
    args: ['e2e/scripts/upstream-mock.mjs'],
    env: { UPSTREAM_PORT: String(UPSTREAM_PORT) },
    label: 'upstream',
  });
  await waitFor(`http://127.0.0.1:${UPSTREAM_PORT}/healthz`, 10_000);

  // 2) backend server
  // 通过环境变量禁用 OAuth/Stripe；SQLite 用上面的文件路径。
  start({
    cwd: resolve(ROOT, 'backend'),
    cmd: 'go',
    args: ['run', './cmd/server'],
    env: {
      NEXUSAPI_APP_ENV: 'development',
      NEXUSAPI_SERVER_HOST: '127.0.0.1',
      NEXUSAPI_SERVER_PORT: String(BACKEND_PORT),
      NEXUSAPI_DATABASE_DRIVER: 'sqlite',
      NEXUSAPI_DATABASE_DSN: DB_PATH,
      NEXUSAPI_LOG_LEVEL: 'info',
      NEXUSAPI_LOG_FORMAT: 'console',
      // 不启 redis；Server 支持 REDIS_ADDR 为空时走内存降级
      NEXUSAPI_REDIS_ADDR: '',
      NEXUSAPI_SECURITY_ENCRYPTION_KEY: '', // 使用 Noop cipher
      NEXUSAPI_SITE_BASE_URL: `http://127.0.0.1:${WEB_USER_PORT}`,
      NEXUSAPI_AUTH_SESSION_TTL_HOURS: '24',
      NEXUSAPI_RELAY_FAILOVER_ATTEMPTS: '1',
      // OAuth 启用 github + google，URL 全部重定向到 upstream-mock
      NEXUSAPI_OAUTH_POST_LOGIN_URL: `http://127.0.0.1:${WEB_USER_PORT}/dashboard`,
      NEXUSAPI_OAUTH_GITHUB_ENABLED: 'true',
      NEXUSAPI_OAUTH_GITHUB_CLIENT_ID: 'mock-gh-id',
      NEXUSAPI_OAUTH_GITHUB_CLIENT_SECRET: 'mock-gh-secret',
      NEXUSAPI_OAUTH_GITHUB_AUTHORIZE_URL: `http://127.0.0.1:${UPSTREAM_PORT}/oauth/github/authorize`,
      NEXUSAPI_OAUTH_GITHUB_TOKEN_URL: `http://127.0.0.1:${UPSTREAM_PORT}/oauth/github/token`,
      NEXUSAPI_OAUTH_GITHUB_API_BASE: `http://127.0.0.1:${UPSTREAM_PORT}`,
      NEXUSAPI_OAUTH_GOOGLE_ENABLED: 'true',
      NEXUSAPI_OAUTH_GOOGLE_CLIENT_ID: 'mock-gg-id',
      NEXUSAPI_OAUTH_GOOGLE_CLIENT_SECRET: 'mock-gg-secret',
      NEXUSAPI_OAUTH_GOOGLE_AUTHORIZE_URL: `http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/authorize`,
      NEXUSAPI_OAUTH_GOOGLE_TOKEN_URL: `http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/token`,
      NEXUSAPI_OAUTH_GOOGLE_API_BASE: `http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/userinfo`,
      // 限流默认设低一点，好触发 429 测试
      NEXUSAPI_RATE_LIMIT_DEFAULT_RPM: '1000',
      NEXUSAPI_RATE_LIMIT_DEFAULT_TPM: '0',
      // Stripe 启用（webhook secret 固定，签名可本地复现）
      NEXUSAPI_PAYMENT_STRIPE_ENABLED: 'true',
      NEXUSAPI_PAYMENT_STRIPE_SECRET_KEY: 'sk_test_e2e_fake',
      NEXUSAPI_PAYMENT_STRIPE_WEBHOOK_SECRET: 'whsec_e2e_fixed',
      NEXUSAPI_PAYMENT_STRIPE_SUCCESS_URL: `http://127.0.0.1:${WEB_USER_PORT}/billing?paid=1`,
      NEXUSAPI_PAYMENT_STRIPE_CANCEL_URL: `http://127.0.0.1:${WEB_USER_PORT}/billing?canceled=1`,
      NEXUSAPI_PAYMENT_STRIPE_PRODUCT_NAME: 'NexusAPI E2E Credits',
      NEXUSAPI_PAYMENT_STRIPE_API_BASE: `http://127.0.0.1:${UPSTREAM_PORT}`,
      NEXUSAPI_PAYMENT_MICRO_PER_CENT: '10000',
    },
    label: 'backend',
  });
  await waitFor(`http://127.0.0.1:${BACKEND_PORT}/healthz`, 60_000);

  // 3) e2e-seed
  const seed = start({
    cwd: resolve(ROOT, 'backend'),
    cmd: 'go',
    args: [
      'run',
      './cmd/e2e-seed',
      '--sqlite',
      DB_PATH,
      '--upstream-url',
      `http://127.0.0.1:${UPSTREAM_PORT}`,
      '--reset',
    ],
    label: 'seed',
  });
  await new Promise<void>((resolvePromise, reject) => {
    seed.on('exit', (code) => {
      if (code === 0) resolvePromise();
      else reject(new Error(`seed exited ${code}`));
    });
  });

  // 4) web-user / web-admin
  start({
    cwd: resolve(ROOT, 'web-user'),
    cmd: 'pnpm',
    args: ['exec', 'next', 'dev', '-p', String(WEB_USER_PORT)],
    env: { NEXUSAPI_BACKEND_URL: `http://127.0.0.1:${BACKEND_PORT}` },
    label: 'web-user',
  });
  start({
    cwd: resolve(ROOT, 'web-admin'),
    cmd: 'pnpm',
    args: ['exec', 'next', 'dev', '-p', String(WEB_ADMIN_PORT)],
    env: { NEXUSAPI_BACKEND_URL: `http://127.0.0.1:${BACKEND_PORT}` },
    label: 'web-admin',
  });

  await Promise.all([
    waitFor(`http://127.0.0.1:${WEB_USER_PORT}/login`, 120_000),
    waitFor(`http://127.0.0.1:${WEB_ADMIN_PORT}/login`, 120_000),
  ]);
}
