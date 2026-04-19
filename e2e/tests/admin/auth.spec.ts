// Admin 登录 + overview。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin 认证', () => {
  test('admin 登录成功后跳 overview', async ({ page }) => {
    await page.goto('/login');
    await page.fill('input[type="email"]', 'admin@e2e.test');
    await page.fill('input[type="password"]', 'admin12345');
    await page.getByRole('button', { name: /登录/ }).click();
    await expect(page).toHaveURL(/\/overview$/);
    await expect(page.getByText('admin@e2e.test')).toBeVisible();
  });

  test('普通 user 登录管理台被拦截', async ({ page, loginAsUser }) => {
    const { email, password } = await loginAsUser();
    // 清掉 cookie（user 端 session），避免 admin login 接口返回 user role
    await page.request.post('/api/auth/logout');

    await page.goto('/login');
    await page.fill('input[type="email"]', email);
    await page.fill('input[type="password"]', password);
    await page.getByRole('button', { name: /登录/ }).click();
    // 错误提示"仅管理员可访问"
    await expect(page.locator('.text-red-600, .bg-red-900, [class*="red"]').filter({ hasText: /管理员/ }).first()).toBeVisible();
  });

  test('未登录访问 overview 跳 login', async ({ page }) => {
    await page.goto('/overview');
    await expect(page).toHaveURL(/\/login$/);
  });

  test('overview 展示导航', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    await page.goto('/overview');
    for (const item of ['渠道', '模型', '用户', '日志']) {
      await expect(page.getByRole('link', { name: new RegExp(item) })).toBeVisible();
    }
  });
});
