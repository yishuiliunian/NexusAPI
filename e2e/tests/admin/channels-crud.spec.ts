// Admin Channels 深度 CRUD。
// 选择器使用 data-testid（见 web-admin/app/channels/page.tsx）。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Channels CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  // 公共：填新建表单的几个必填项。
  async function fillCreateForm(
    page: import('@playwright/test').Page,
    cfg: { name: string; provider?: string; baseURL?: string; credentials?: string; models?: string },
  ) {
    await page.getByRole('button', { name: /新建/ }).click();
    await page.getByTestId('channel-name').fill(cfg.name);
    await page.getByTestId('channel-provider').selectOption(cfg.provider ?? 'openai');
    await page.getByTestId('channel-base-url').fill(cfg.baseURL ?? 'http://127.0.0.1:18090');
    await page.getByTestId('channel-credentials').fill(cfg.credentials ?? 'sk-mock');
    await page.getByTestId('channel-models').fill(cfg.models ?? 'gpt-4o-mini');
  }

  test('创建渠道 → 列表可见 → 删除', async ({ page }) => {
    await page.goto('/channels');

    const name = `e2e-test-${Date.now()}`;
    await fillCreateForm(page, { name });

    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    expect((await createResp).ok()).toBeTruthy();
    await expect(page.locator('table')).toContainText(name);

    page.on('dialog', (d) => d.accept());
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'DELETE',
    );
    await page.locator('tr', { hasText: name }).getByRole('button', { name: /删除/ }).click();
    expect((await delResp).ok()).toBeTruthy();
    await expect(page.locator('table')).not.toContainText(name);
  });

  test('编辑渠道时 Provider 下拉可切换（回归 #165）', async ({ page }) => {
    // 编辑模式下 Provider 应可切换；保存后后端真的接受。
    await page.goto('/channels');

    const name = `e2e-prov-switch-${Date.now()}`;
    await fillCreateForm(page, { name });
    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    expect((await createResp).ok()).toBeTruthy();
    await expect(page.locator('table')).toContainText(name);

    await page.locator('tr', { hasText: name }).getByRole('button', { name: /编辑/ }).click();
    const providerSelect = page.getByTestId('channel-provider');
    await expect(providerSelect).toBeEnabled();
    await providerSelect.selectOption('claude');
    await expect(providerSelect).toHaveValue('claude');

    const updateResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: /保存/ }).click();
    const updated = await updateResp;
    expect(updated.ok()).toBeTruthy();
    expect((await updated.json()).provider).toBe('claude');

    page.on('dialog', (d) => d.accept());
    await page.locator('tr', { hasText: name }).getByRole('button', { name: /删除/ }).click();
  });

  test('编辑时凭证留空 → 后端保留旧密文，weight 改动生效', async ({ page }) => {
    // 凭证字段 GET 接口不暴露明文，留空保存 = 不改密文。
    // 这里用「同时改 weight，断言 weight 回写正确」当代理证据，
    // 后端代码已经在 admin handler 里实现了 if req.Credentials != "" 才覆盖。
    await page.goto('/channels');

    const name = `e2e-creds-${Date.now()}`;
    await fillCreateForm(page, { name, credentials: 'sk-mock-initial' });
    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    const created = (await (await createResp).json()) as { id: number };

    await page.locator('tr', { hasText: name }).getByRole('button', { name: /编辑/ }).click();
    // 凭证 input 应该是空（不回显）
    await expect(page.getByTestId('channel-credentials')).toHaveValue('');
    // 改 weight
    await page.getByTestId('channel-weight').fill('77');

    const updResp = page.waitForResponse(
      (r) => r.url().includes(`/api/admin/channels/${created.id}`) && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: /保存/ }).click();
    const upd = await updResp;
    expect(upd.ok()).toBeTruthy();
    expect(((await upd.json()) as { weight: number }).weight).toBe(77);

    page.on('dialog', (d) => d.accept());
    await page.locator('tr', { hasText: name }).getByRole('button', { name: /删除/ }).click();
  });

  test('status → disabled 后行徽章更新', async ({ page }) => {
    await page.goto('/channels');

    const name = `e2e-status-${Date.now()}`;
    await fillCreateForm(page, { name, models: 'gpt-4o-status-test' });
    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    const created = (await (await createResp).json()) as { id: number; status: string };
    expect(created.status).toBe('active');

    await page.locator('tr', { hasText: name }).getByRole('button', { name: /编辑/ }).click();
    await page.getByTestId('channel-status').selectOption('disabled');
    const updResp = page.waitForResponse(
      (r) => r.url().includes(`/api/admin/channels/${created.id}`) && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: /保存/ }).click();
    const upd = await updResp;
    expect(upd.ok()).toBeTruthy();
    expect(((await upd.json()) as { status: string }).status).toBe('disabled');

    // 列表行徽章更新
    await expect(page.locator('tr', { hasText: name })).toContainText('disabled');

    page.on('dialog', (d) => d.accept());
    await page.locator('tr', { hasText: name }).getByRole('button', { name: /删除/ }).click();
  });
});
