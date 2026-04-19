// Admin 激活码批量生成 E2E
import { expect, test } from '../../fixtures/auth';

test.describe('Admin 激活码批量', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('页面渲染 + 表单字段', async ({ page }) => {
    await page.goto('/redemption');
    await expect(page.getByRole('heading', { name: /激活码生成/ })).toBeVisible();
    await expect(page.getByPlaceholder(/2026|推广|抖音/)).toBeVisible();
  });

  test('批量生成 10 张激活码', async ({ page }) => {
    await page.goto('/redemption');

    await page.getByPlaceholder(/2026|推广|抖音/).fill(`e2e-batch-${Date.now()}`);

    // count
    const inputs = page.locator('input[type="number"]');
    await inputs.nth(0).fill('10');

    // 提交（选 $5 预设）
    await page.getByRole('button', { name: /^$5$/ }).click();

    const resp = page.waitForResponse((r) => r.url().endsWith('/api/admin/redemption-batches') && r.request().method() === 'POST');
    await page.getByRole('button', { name: /生成 10 张/ }).click();
    const r = await resp;
    expect(r.ok()).toBeTruthy();

    // 成功提示
    await expect(page.getByText(/已生成 10/)).toBeVisible({ timeout: 5000 });
  });
});
