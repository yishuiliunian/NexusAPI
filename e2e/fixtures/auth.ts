// fixtures/auth.ts —— Playwright 扩展 test，注入登录/注册/apikey helpers。
//
// 使用：
//   import { test, expect } from '../../fixtures/auth';
//   test('foo', async ({ page, loginAsUser, createApiKey }) => { ... });
//
// 策略：每个测试独立随机邮箱。session cookie 由 Playwright context 自动维护。
import { test as base, type Page, expect } from '@playwright/test';
import { fetch as undiciFetch } from 'undici';
import { API_BASE, PORTS } from '../playwright.config';

export type AuthFixtures = {
  // loginAsUser 注册（如不存在）+ 登录；返回邮箱。
  loginAsUser: (overrides?: { email?: string; password?: string }) => Promise<{ email: string; password: string }>;
  // loginAsAdmin 直接用 seed 出的 admin 登录。
  loginAsAdmin: () => Promise<{ email: string; password: string }>;
  // createApiKey 当前页面已登录，通过 /api/user/apikeys 建一个。
  createApiKey: (name?: string) => Promise<{ id: number; secret: string }>;
  // grantQuota 通过 admin 接口给当前登录 user 加配额（micro 单位）。
  // 独立 admin session，不影响当前 page 的 user 会话。
  grantQuota: (email: string, quota: number) => Promise<void>;
  // resetDb 调用 seed --reset 清库。少用，只在 integration 需要时。
  resetDb: () => Promise<void>;
};

export const SEED_ADMIN = { email: 'admin@e2e.test', password: 'admin12345' };
export const SEED_USER = { email: 'alice@e2e.test', password: 'user12345' };

function randEmail() {
  const s = Math.random().toString(36).slice(2, 10);
  return `u-${s}@e2e.test`;
}

export const test = base.extend<AuthFixtures>({
  loginAsUser: async ({ page }, use) => {
    async function login(overrides?: { email?: string; password?: string }) {
      const email = overrides?.email ?? randEmail();
      const password = overrides?.password ?? 'password123';
      // 注册接口：幂等失败忽略（已注册时）
      const reg = await page.request.post('/api/auth/register', {
        data: { email, password },
        failOnStatusCode: false,
      });
      if (!reg.ok() && reg.status() !== 409 && reg.status() !== 400) {
        throw new Error(`register failed: ${reg.status()} ${await reg.text()}`);
      }
      const loginRes = await page.request.post('/api/auth/login', {
        data: { email, password },
      });
      expect(loginRes.ok(), 'login should succeed').toBeTruthy();
      return { email, password };
    }
    await use(login);
  },

  loginAsAdmin: async ({ page }, use) => {
    async function login() {
      const res = await page.request.post('/api/auth/login', { data: SEED_ADMIN });
      expect(res.ok(), `admin login: ${res.status()} ${await res.text()}`).toBeTruthy();
      return SEED_ADMIN;
    }
    await use(login);
  },

  createApiKey: async ({ page }, use) => {
    async function create(name = 'e2e-key') {
      // page.request 继承 browser cookies，但不自动镜像 CSRF header。
      // 已登录态下 /api/* CSRF middleware 会校验，所以手动读 cookie 注入。
      const cookies = await page.context().cookies();
      const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;
      const res = await page.request.post('/api/user/apikeys', {
        data: { name },
        headers: csrf ? { 'X-CSRF-Token': csrf } : {},
      });
      expect(res.ok(), `create apikey: ${res.status()} ${await res.text()}`).toBeTruthy();
      const body = (await res.json()) as { id: number; secret: string };
      return body;
    }
    await use(create);
  },

  grantQuota: async ({}, use) => {
    async function grant(email: string, quota: number) {
      // 独立 admin 登录（不污染 page 的 session）。
      const loginResp = await undiciFetch(`${API_BASE}/api/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(SEED_ADMIN),
      });
      if (!loginResp.ok) throw new Error(`admin login: ${loginResp.status}`);
      const setCookie = loginResp.headers.get('set-cookie') ?? '';
      const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
      const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
      if (!sess || !csrf) throw new Error('admin login missing cookies');
      const cookieHeader = `nexus_session=${sess}; nexus_csrf=${csrf}`;

      // 查 user id
      const listResp = await undiciFetch(`${API_BASE}/api/admin/users?size=200`, {
        headers: { Cookie: cookieHeader },
      });
      if (!listResp.ok) throw new Error(`list users: ${listResp.status}`);
      const listBody = (await listResp.json()) as { items: Array<{ id: number; email: string }> };
      const target = listBody.items.find((u) => u.email === email);
      if (!target) throw new Error(`user ${email} not found`);

      // 设置 quota（绝对值）
      const upd = await undiciFetch(`${API_BASE}/api/admin/users/${target.id}/quota`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Cookie: cookieHeader,
          'X-CSRF-Token': csrf,
        },
        body: JSON.stringify({ quota }),
      });
      if (!upd.ok) throw new Error(`grant quota: ${upd.status} ${await upd.text()}`);
    }
    await use(grant);
  },

  resetDb: async ({}, use) => {
    async function reset() {
      // 通过 spawn go run e2e-seed 重置，而不是走 HTTP（没有 admin 接口）。
      const { spawnSync } = await import('node:child_process');
      const { resolve, dirname } = await import('node:path');
      const { fileURLToPath } = await import('node:url');
      const here = dirname(fileURLToPath(import.meta.url));
      const root = resolve(here, '..', '..');
      const dbPath = resolve(root, 'e2e', '.tmp', 'nexus-e2e.db');
      const r = spawnSync(
        'go',
        [
          'run',
          './cmd/e2e-seed',
          '--sqlite',
          dbPath,
          '--upstream-url',
          `http://127.0.0.1:${PORTS.upstream}`,
          '--reset',
        ],
        { cwd: resolve(root, 'backend'), stdio: 'inherit' }
      );
      if (r.status !== 0) throw new Error(`seed reset failed: ${r.status}`);
    }
    await use(reset);
  },
});

export { expect };

// 便捷：把 Page 和 request 封装的 helper。
export async function httpLogout(page: Page): Promise<void> {
  await page.request.post('/api/auth/logout');
}
