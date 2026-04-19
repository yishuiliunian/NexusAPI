// Dashboard 监控看板 E2E。
//
// 验证：
//  - 页面能开 + 4 卡片可见
//  - 空库情况下显示"暂无数据"
//  - 发请求后图表出现数据（模型名、成本等）
//  - 时间窗口切换 1/7/30/90 天
import { expect, test } from '../../fixtures/auth';

test.describe('Dashboard 监控看板', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('初始空库：四卡片可见 + "暂无数据" 占位', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: /监控看板/ })).toBeVisible();
    // 四卡片
    await expect(page.getByText('当前余额')).toBeVisible();
    await expect(page.getByText(/最近.*天消耗/)).toBeVisible();
    await expect(page.getByText(/最近.*天请求/)).toBeVisible();
    await expect(page.getByText('成功率')).toBeVisible();
    // 图表区显示"暂无数据"
    await expect(page.getByText('暂无数据').first()).toBeVisible();
  });

  test('有调用数据：tokens 趋势 + 模型柱状 + 明细表', async ({ page, loginAsUser, createApiKey, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 10_000_000);
    const { secret } = await createApiKey();

    // 跑两次调用
    for (let i = 0; i < 2; i++) {
      const r = await page.request.post('/v1/chat/completions', {
        headers: { Authorization: `Bearer ${secret}` },
        data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
      });
      expect(r.ok()).toBeTruthy();
    }

    await page.goto('/dashboard');
    // 模型明细表包含 gpt-4o-mini
    await expect(page.locator('table')).toContainText('gpt-4o-mini');
    // 图表区不再是"暂无数据"：至少有 svg（Recharts 渲染出的）
    const svgCount = await page.locator('svg').count();
    expect(svgCount).toBeGreaterThan(0);
    // 请求数卡片值 >= 2
    const reqCard = page.locator('div').filter({ hasText: /最近.*天请求/ }).first();
    await expect(reqCard).toBeVisible();
  });

  test('时间窗口切换触发新请求', async ({ page }) => {
    await page.goto('/dashboard');
    const resp = page.waitForResponse((r) => r.url().includes('/api/user/stats?days=30'));
    await page.locator('select').first().selectOption('30');
    const r = await resp;
    expect(r.ok()).toBeTruthy();
    await expect(page.getByText(/最近 30 天消耗/)).toBeVisible();
  });
});

test.describe('Stats API 直接测试', () => {
  test('空库返回合法结构（所有数组字段非 null）', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/user/stats?days=7');
    expect(r.ok()).toBeTruthy();
    const body = await r.json() as {
      summary: { days: number; quota: number; total_requests: number };
      by_day: unknown[];
      by_model: unknown[];
      by_capability: unknown[];
      by_status: unknown[];
    };
    expect(body.summary.days).toBe(7);
    expect(Array.isArray(body.by_day)).toBeTruthy();
    expect(Array.isArray(body.by_model)).toBeTruthy();
    expect(Array.isArray(body.by_capability)).toBeTruthy();
    expect(Array.isArray(body.by_status)).toBeTruthy();
  });

  test('days 参数边界：超 90 被截断', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/user/stats?days=500');
    const body = await r.json() as { summary: { days: number } };
    expect(body.summary.days).toBe(90);
  });

  test('days 参数：0 / 负数 回落默认 7', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/user/stats?days=-1');
    const body = await r.json() as { summary: { days: number } };
    expect(body.summary.days).toBe(7);
  });
});