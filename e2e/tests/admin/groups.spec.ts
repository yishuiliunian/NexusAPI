// Admin /groups CRUD（拆自原 groups-audits.spec.ts，并补强）。
//
// 覆盖：
//   1. UI 新建分组：填名称 + 倍率 → 创建成功 → 列表显示
//   2. UI 删除：confirm dialog 接受 → 行消失
//   3. API 维度：创建后通过 /api/admin/groups 反查倍率一致
//
// 注意：当前 UI 没有编辑入口，无法测 multiplier 改动后的用户计费链路；
// 那部分留给后续 API + integration 联动 spec。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin /groups CRUD', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('UI 新建 → 列表可见 → 删除', async ({ page }) => {
    await page.goto('/groups');
    const name = `group-${Date.now()}`;
    await page.getByPlaceholder('组名（例：VIP / 内部）').fill(name);
    await page.getByPlaceholder('倍率').fill('1.5');

    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: /新建/ }).click();
    const cr = await createResp;
    expect(cr.ok(), `create: ${cr.status()} ${await cr.text()}`).toBeTruthy();

    // 等列表刷新
    await page
      .waitForResponse(
        (r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'GET',
      )
      .catch(() => null);
    await expect(page.locator('table')).toContainText(name);
    await expect(page.locator('tr', { hasText: name })).toContainText('1.5');

    // 删除
    page.on('dialog', (d) => d.accept());
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/groups/') && r.request().method() === 'DELETE',
    );
    await page
      .locator('tr', { hasText: name })
      .getByRole('button', { name: /删除/ })
      .click();
    expect((await delResp).ok()).toBeTruthy();
    await page
      .waitForResponse(
        (r) => r.url().endsWith('/api/admin/groups') && r.request().method() === 'GET',
      )
      .catch(() => null);
    await expect(page.locator('table')).not.toContainText(name);
  });

  test('API 创建后 GET 列表反查倍率正确', async ({ page }) => {
    const name = `api-group-${Date.now()}`;
    const cookies = await page.context().cookies();
    const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;

    const cr = await page.request.post('/api/admin/groups', {
      headers: csrf ? { 'X-CSRF-Token': csrf } : {},
      data: { name, price_multiplier: 0.8 },
    });
    expect(cr.ok()).toBeTruthy();
    const created = (await cr.json()) as { id: number; price_multiplier: number };
    expect(created.price_multiplier).toBe(0.8);

    const list = await page.request.get('/api/admin/groups');
    expect(list.ok()).toBeTruthy();
    const { items } = (await list.json()) as {
      items: Array<{ id: number; name: string; price_multiplier: number }>;
    };
    const found = items.find((g) => g.id === created.id);
    expect(found?.price_multiplier).toBe(0.8);

    // 清理
    await page.request.delete(`/api/admin/groups/${created.id}`, {
      headers: csrf ? { 'X-CSRF-Token': csrf } : {},
    });
  });
});
