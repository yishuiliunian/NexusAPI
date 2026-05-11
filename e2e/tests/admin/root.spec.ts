// Web-admin 根 / Landing 页：未登录显示管理员登录入口。
import { expect, test } from '../../fixtures/auth';

test.describe('Web-admin 根页面', () => {
  test('未登录访问 / → Landing 渲染 + 登录入口可达', async ({ page }) => {
    const resp = await page.goto('/');
    expect(resp?.status()).toBeLessThan(400);

    await expect(page.getByRole('heading', { name: /NexusAPI Admin/ })).toBeVisible();
    await expect(page.getByRole('link', { name: /管理员登录/ })).toHaveAttribute('href', '/login');
  });

  test('已登录后访问 / → 仍展示 Landing（管理员可手动进入 /overview）', async ({
    page,
    loginAsAdmin,
  }) => {
    // 项目当前未在根做重定向；若未来改为自动 redirect，这条用例需更新。
    await loginAsAdmin();
    const resp = await page.goto('/');
    expect(resp?.status()).toBeLessThan(400);
    await expect(page.getByRole('heading', { name: /NexusAPI Admin/ })).toBeVisible();
  });
});
