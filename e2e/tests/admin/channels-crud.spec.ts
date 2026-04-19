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

  test('编辑渠道时 Provider 下拉可切换（回归 #165）', async ({ page }) => {
    // 背景：之前编辑模式下 Provider 被 disabled（isEdit），只能创建时选。
    // 用户反馈：运营时经常要把 openai 切到 openaicompat / claude 切到其他，
    // 禁用 Provider 不合理。修复后编辑时 Provider 下拉应当可操作。
    await page.goto('/channels');

    // 1. 先创建一个 openai 渠道
    await page.getByRole('button', { name: /新建/ }).click();
    const name = `e2e-prov-switch-${Date.now()}`;
    await page.getByPlaceholder('名称').fill(name);
    await page.getByTestId('channel-provider').selectOption('openai');
    await page.getByPlaceholder(/Base URL/).fill('http://127.0.0.1:18090');
    await page.getByPlaceholder('API Key').fill('sk-mock');
    await page.getByPlaceholder(/模型列表/).fill('gpt-4o-mini');
    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    expect((await createResp).ok()).toBeTruthy();
    await expect(page.locator('table')).toContainText(name);

    // 2. 点进编辑
    const row = page.locator('tr', { hasText: name });
    await row.getByRole('button', { name: /编辑/ }).click();

    // 3. 核心回归校验：Provider 下拉不被 disabled，可切换
    const providerSelect = page.getByTestId('channel-provider');
    await expect(providerSelect).toBeEnabled();
    await providerSelect.selectOption('claude');
    await expect(providerSelect).toHaveValue('claude');

    // 4. 保存，校验后端真的接受 provider 切换
    const updateResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: /保存/ }).click();
    const updated = await updateResp;
    expect(updated.ok()).toBeTruthy();
    const body = await updated.json();
    expect(body.provider).toBe('claude');

    // 5. 清理
    page.on('dialog', (d) => d.accept());
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'DELETE',
    );
    await page.locator('tr', { hasText: name }).getByRole('button', { name: /删除/ }).click();
    expect((await delResp).ok()).toBeTruthy();
  });
});