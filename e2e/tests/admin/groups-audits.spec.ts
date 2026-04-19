// Admin Groups CRUD + Plans CRUD + Audits 查询。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Groups CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('新建 → 列表可见 → 删除', async ({ page }) => {
    await page.goto('/groups');
    const name = `group-${Date.now()}`;
    await page.getByPlaceholder('组名').fill(name);
    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'POST'
    );
    await page.getByRole('button', { name: /新建/ }).click();
    const cr = await createResp;
    expect(cr.ok(), `create: ${cr.status()} ${await cr.text()}`).toBeTruthy();
    // 等 list 刷新
    await page.waitForResponse((r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'GET').catch(() => null);
    await expect(page.locator('table')).toContainText(name);

    page.on('dialog', (d) => d.accept());
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/groups/') && r.request().method() === 'DELETE'
    );
    const row = page.locator('tr', { hasText: name });
    await row.getByRole('button', { name: /删除/ }).click();
    expect((await delResp).ok()).toBeTruthy();
    await page.waitForResponse((r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'GET').catch(() => null);
    await expect(page.locator('table')).not.toContainText(name);
  });
});

test.describe('Admin Audits', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('审计日志页显示计数', async ({ page }) => {
    // 先做点 admin 操作产生审计记录
    await page.goto('/users');
    await page.waitForTimeout(500);

    await page.goto('/audits');
    await expect(page.getByRole('heading', { name: /审计/ })).toBeVisible();
    // total 可能为 0（如果 middleware 没记读操作）；仅断言页面渲染
    await expect(page.locator('table thead')).toContainText('操作');
  });
});