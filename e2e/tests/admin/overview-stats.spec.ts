// Admin Overview 监控 E2E。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Overview 监控', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('四卡片 + 导航 + 图表占位可见', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByRole('heading', { name: /Admin Overview/ })).toBeVisible();
    await expect(page.getByText('总请求数')).toBeVisible();
    await expect(page.getByText('总收入 (元)')).toBeVisible();
    await expect(page.getByText('活跃用户')).toBeVisible();
    // "成功率" 在卡片 label + hint 里都出现；只验 StatCard label
    await expect(page.getByText(/^成功率$/).first()).toBeVisible();
    // 导航
    await expect(page.getByRole('link', { name: '渠道' })).toBeVisible();
    await expect(page.getByRole('link', { name: '模型' })).toBeVisible();
  });

  test('时间窗口切换', async ({ page }) => {
    await page.goto('/overview');
    const resp = page.waitForResponse((r) => r.url().includes('/api/admin/stats?days=30'));
    await page.locator('select').first().selectOption('30');
    const r = await resp;
    expect(r.ok()).toBeTruthy();
  });

  test('有流量：Top 用户表出现该用户', async ({ page, browser, loginAsAdmin, grantQuota }) => {
    // 独立 user context 产生流量（避免与 admin session 干扰）
    const userCtx = await browser.newContext({
      baseURL: `http://127.0.0.1:${(await import('../../playwright.config')).PORTS.webUser}`,
    });
    const userPage = await userCtx.newPage();
    const email = `top-${Date.now()}@e2e.test`;
    // 注册 + 登录
    await userPage.request.post('/api/auth/register', { data: { email, password: 'password123' } });
    await userPage.request.post('/api/auth/login', { data: { email, password: 'password123' } });
    await grantQuota(email, 10_000_000);
    // 拿 CSRF 建 key
    const cookies = await userCtx.cookies();
    const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;
    const keyResp = await userPage.request.post('/api/user/apikeys', {
      data: { name: 'top-test' },
      headers: { 'X-CSRF-Token': csrf! },
    });
    const { secret } = (await keyResp.json()) as { secret: string };
    for (let i = 0; i < 3; i++) {
      await userPage.request.post('/v1/chat/completions', {
        headers: { Authorization: `Bearer ${secret}` },
        data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
      });
    }
    await userCtx.close();

    // admin 看 overview
    await loginAsAdmin();
    await page.goto('/overview');
    await expect(page.getByText(email)).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('Admin stats API', () => {
  test('普通用户调 /api/admin/stats 被拒', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/admin/stats?days=7', { failOnStatusCode: false });
    expect([401, 403]).toContain(r.status());
  });

  test('admin 调 /api/admin/stats 返回合法结构', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const r = await page.request.get('/api/admin/stats?days=7');
    expect(r.ok()).toBeTruthy();
    const body = (await r.json()) as {
      summary: { days: number; total_requests: number; active_users: number };
      by_day: unknown[];
      by_model: unknown[];
      by_capability: unknown[];
      by_status: unknown[];
      top_users: unknown[];
    };
    expect(body.summary.days).toBe(7);
    expect(Array.isArray(body.top_users)).toBeTruthy();
    expect(Array.isArray(body.by_day)).toBeTruthy();
  });
});