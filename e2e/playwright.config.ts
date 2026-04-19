// Playwright E2E 配置：真栈端到端。
//
// 使用 global-setup.ts 启动 backend + upstream mock + web-user + web-admin，
// global-teardown.ts 负责清理。
//
// 端口（避免撞开发端口）：
//   backend   18080
//   upstream  18090
//   web-user  13000
//   web-admin 13001
import { defineConfig, devices } from '@playwright/test';

// 对外常量，方便测试文件 import。
export const PORTS = {
  backend: 18080,
  upstream: 18090,
  webUser: 13000,
  webAdmin: 13001,
} as const;

export const USER_BASE = `http://127.0.0.1:${PORTS.webUser}`;
export const ADMIN_BASE = `http://127.0.0.1:${PORTS.webAdmin}`;
export const API_BASE = `http://127.0.0.1:${PORTS.backend}`;

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
