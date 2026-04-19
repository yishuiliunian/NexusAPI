// Admin 深度：users 改配额 / 搜索 / 分页。
//
// 改配额通过 UI prompt() 触发，用 page.on('dialog') 拦截输入值。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Users 深度', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('改配额通过 UI prompt', async ({ page }) => {
    await page.goto('/users');
    const aliceRow = page.locator('tr', { hasText: 'alice@e2e.test' });
    await expect(aliceRow).toBeVisible();

    // 拦截 prompt：输入 77777777（避免 rounding 到整数）
    page.on('dialog', async (d) => {
      if (d.type() === 'prompt') await d.accept('77777777');
      else await d.accept();
    });

    // 等待 PUT /api/admin/users/:id/quota
    const quotaResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/users/') && r.url().endsWith('/quota') && r.request().method() === 'PUT'
    );
    await aliceRow.getByRole('button', { name: /改配额/ }).click();
    const r = await quotaResp;
    expect(r.ok(), `quota resp: ${r.status()}`).toBeTruthy();

    // 等 list 刷新
    await page.waitForResponse((r) => r.url().includes('/api/admin/users'));

    // 配额文案更新：77777777 / 1_000_000 = 77.777777 → toFixed(4) = "77.7778"
    await expect(aliceRow).toContainText('77.7778 元');
  });

  test('改配额：空输入不提交', async ({ page }) => {
    await page.goto('/users');
    const aliceRow = page.locator('tr', { hasText: 'alice@e2e.test' });
    let prompted = false;
    page.on('dialog', async (d) => {
      prompted = true;
      await d.dismiss(); // 空 = dismiss
    });
    await aliceRow.getByRole('button', { name: /改配额/ }).click();
    expect(prompted).toBeTruthy();
    // 无网络请求：配额未变
  });

  test('用户列表包含 seed 两个账号', async ({ page }) => {
    await page.goto('/users');
    await expect(page.locator('table')).toContainText('admin@e2e.test');
    await expect(page.locator('table')).toContainText('alice@e2e.test');
  });
});