// Admin Models CRUD（价格 upsert + 删除）。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Models CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('upsert 新价格 → 列表可见', async ({ page }) => {
    await page.goto('/models');
    const model = `e2e-model-${Date.now()}`;

    await page.getByPlaceholder(/model \(gpt-4o-mini\)/).fill(model);
    await page.locator('select').first().selectOption('chat');
    await page.getByPlaceholder(/input_price/).fill('200');
    await page.getByPlaceholder(/output_price/).fill('800');

    const resp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/models') && r.request().method() === 'PUT'
    );
    await page.getByRole('button', { name: /保存/ }).click();
    expect((await resp).ok()).toBeTruthy();

    // 列表可见
    await expect(page.locator('table')).toContainText(model);

    // 删除
    page.on('dialog', (d) => d.accept());
    const row = page.locator('tr', { hasText: model });
    await row.getByRole('button', { name: /删除/ }).click();
    // 等删除 API
    await page.waitForTimeout(200);
    await expect(page.locator('table')).not.toContainText(model);
  });
});