// Dashboard 基础冒烟（旧版兼容保留）。
// 深度监控测试见 dashboard-stats.spec.ts。
import { expect, test } from '../../fixtures/auth';

test.describe('Dashboard 冒烟', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('展示四卡片监控', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: /监控看板/ })).toBeVisible();
    await expect(page.getByText('当前余额')).toBeVisible();
    await expect(page.getByText(/最近.*天消耗/)).toBeVisible();
    await expect(page.getByText(/最近.*天请求/)).toBeVisible();
  });

  test('快捷链接跳 /keys 成功', async ({ page }) => {
    await page.goto('/dashboard');
    await page.getByRole('link', { name: /API Keys/ }).first().click();
    await expect(page).toHaveURL(/\/keys$/);
  });

  test('快捷链接跳 /logs', async ({ page }) => {
    await page.goto('/dashboard');
    await page.getByRole('link', { name: /调用日志|日志/ }).first().click();
    await expect(page).toHaveURL(/\/logs$/);
  });
});