// 模型价格管理。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin 模型价格', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('展示 seed 价格', async ({ page }) => {
    await page.goto('/models');
    await expect(page.getByText(/gpt-4o-mini|claude-3-5-sonnet/).first()).toBeVisible();
  });

  test('页面无 uncaught', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/models');
    await page.waitForTimeout(500);
    expect(errors).toHaveLength(0);
  });
});
