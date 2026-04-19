// Admin Channels 深度 CRUD。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Channels CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('创建渠道 → 列表可见 → 删除', async ({ page }) => {
    await page.goto('/channels');

    // 展开新建表单
    await page.getByRole('button', { name: /新建/ }).click();
    const name = `e2e-test-${Date.now()}`;
    // 按 placeholder 定位
    await page.getByPlaceholder('名称').fill(name);
    // provider 下拉
    const select = page.locator('select').first();
    await select.selectOption('openai');
    await page.getByPlaceholder(/Base URL/).fill('http://127.0.0.1:18090');
    await page.getByPlaceholder('API Key').fill('sk-mock');
    await page.getByPlaceholder(/模型列表/).fill('gpt-4o-mini');

    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: '创建' }).click();
    expect((await createResp).ok()).toBeTruthy();

    // 列表显示新 channel
    await expect(page.locator('table')).toContainText(name);

    // 删除
    page.on('dialog', (d) => d.accept());
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'DELETE'
    );
    const row = page.locator('tr', { hasText: name });
    await row.getByRole('button', { name: /删除/ }).click();
    expect((await delResp).ok()).toBeTruthy();
    await expect(page.locator('table')).not.toContainText(name);
  });
});