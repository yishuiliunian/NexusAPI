// Playwright E2E 配置：真栈端到端。
//
// 端口与 base URL 全部从 helpers/env.ts 取（优先 process.env，其次 deploy/dev/.env，
// 最后 fallback 到 20000 系列）。这样 e2e 与 dev 共用同一套端口分配，且支持 worktree 隔离。
//
// 启动策略：
//   - 如果 backend /healthz 已就绪（用户在跑 ./deploy/dev/dev.sh）→ global-setup 仅 reset DB
//   - 否则 global-setup 会调 deploy/dev/dev.sh --backend-only 拉起 infra+backend，
//     再自己 spawn web-user / web-admin
import { defineConfig, devices } from '@playwright/test';
import { PORTS, URLS } from './helpers/env';

// 重新导出以兼容既有 spec：`import { PORTS, API_BASE } from '../../playwright.config'`
export { PORTS } from './helpers/env';

export const USER_BASE = URLS.user;
export const ADMIN_BASE = URLS.admin;
export const API_BASE = URLS.api;

export default defineConfig({
  testDir: './tests',
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: false, // 共享 DB，串行保证测试间独立
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1, // 单 worker：避免 DB 写冲突
  reporter: process.env.CI ? [['github'], ['list'], ['html', { open: 'never' }]] : 'list',
  globalSetup: './scripts/global-setup.ts',
  globalTeardown: './scripts/global-teardown.ts',
  use: {
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
  },
  projects: [
    {
      name: 'user',
      testDir: './tests/user',
      use: { ...devices['Desktop Chrome'], baseURL: USER_BASE },
    },
    {
      name: 'admin',
      testDir: './tests/admin',
      use: { ...devices['Desktop Chrome'], baseURL: ADMIN_BASE },
    },
    {
      name: 'integration',
      testDir: './tests/integration',
      use: { ...devices['Desktop Chrome'], baseURL: USER_BASE },
    },
    {
      name: 'a11y',
      testDir: './tests/a11y',
      use: { ...devices['Desktop Chrome'], baseURL: USER_BASE },
    },
    {
      name: 'visual',
      testDir: './tests/visual',
      use: { ...devices['Desktop Chrome'], baseURL: USER_BASE },
    },
    // 跨浏览器覆盖：仅跑 user 项目的冒烟 spec。Firefox/WebKit 默认跳过（CI 有 MULTI_BROWSER=1 才开）。
    ...(process.env.MULTI_BROWSER
      ? [
          {
            name: 'user-firefox',
            testDir: './tests/user',
            testMatch: /auth\.spec\.ts|dashboard\.spec\.ts/,
            use: { ...devices['Desktop Firefox'], baseURL: USER_BASE },
          },
          {
            name: 'user-webkit',
            testDir: './tests/user',
            testMatch: /auth\.spec\.ts|dashboard\.spec\.ts/,
            use: { ...devices['Desktop Safari'], baseURL: USER_BASE },
          },
          {
            name: 'user-mobile',
            testDir: './tests/user',
            testMatch: /auth\.spec\.ts|dashboard\.spec\.ts/,
            use: { ...devices['iPhone 13'], baseURL: USER_BASE },
          },
        ]
      : []),
  ],
});
// 静态使用一下 PORTS，防止 tree-shaking 把 helpers/env.ts 误删；同时辅助调试。
void PORTS;
