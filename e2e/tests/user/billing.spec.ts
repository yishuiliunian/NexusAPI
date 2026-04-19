// Billing 页 E2E
import { expect, test } from '../../fixtures/auth';

test.describe('Billing 按量计费', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('展示余额 Hero + 激活码 + 充值', async ({ page }) => {
    await page.goto('/billing');
    await expect(page.getByRole('heading', { name: /充值/ }).first()).toBeVisible();
    // 余额卡
    await expect(page.getByText('当前余额')).toBeVisible();
    // 激活码 Hero
    await expect(page.getByRole('heading', { name: /激活码兑换/ })).toBeVisible();
    await expect(page.getByPlaceholder(/NEXUS-/)).toBeVisible();
    // 充值区
    await expect(page.getByRole('heading', { name: /在线充值/ })).toBeVisible();
  });

  test('激活码兑换：无效码显示错误', async ({ page }) => {
    await page.goto('/billing');
    await page.getByPlaceholder(/NEXUS-/).fill('INVALID-CODE-123');
    await page.getByRole('button', { name: /激活/ }).click();
    await expect(page.getByText(/失败|not found|not_found/i).first()).toBeVisible({ timeout: 5000 });
  });

  test('充值金额选择切换', async ({ page }) => {
    await page.goto('/billing');
    // 选 ¥200 - buttons contain ¥50/¥100/¥200/¥500
    await page.getByRole('button', { name: /¥200/ }).first().click();
    // 不崩即通过（视觉验证在 visual regression 里）
  });
});
