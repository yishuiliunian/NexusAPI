// Web-user 根 / Landing 页：未登录显示登录/注册入口。
import { expect, test } from '../../fixtures/auth';

test.describe('Web-user 根页面', () => {
  test('未登录访问 / → Landing 渲染 + 入口可达', async ({ page }) => {
    const resp = await page.goto('/');
    expect(resp?.status()).toBeLessThan(400);

    await expect(page.getByRole('heading', { name: 'NexusAPI' })).toBeVisible();
    await expect(page.getByRole('link', { name: /登录/ })).toHaveAttribute('href', '/login');
    await expect(page.getByRole('link', { name: /注册/ })).toHaveAttribute('href', '/register');
  });

  test('点击「登录」跳到 /login 页面', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: /登录/ }).first().click();
    await expect(page).toHaveURL(/\/login$/);
  });
});
