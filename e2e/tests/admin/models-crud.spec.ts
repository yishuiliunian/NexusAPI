// Admin Models CRUD（价格 upsert + 删除）。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Models CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('upsert 新价格 → 列表可见', async ({ page }) => {
    await page.goto('/models');
    const model = `e2e-model-${Date.now()}`;

    await page.getByTestId('model-name').fill(model);
    await page.getByTestId('model-capability').selectOption('chat');
    // input_price/output_price 在 UI 是 USD per 1M tokens；填 0.0002 / 0.0008（即 200/800 micro-per-token）
    await page.getByTestId('model-input-price').fill('0.0002');
    await page.getByTestId('model-output-price').fill('0.0008');

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