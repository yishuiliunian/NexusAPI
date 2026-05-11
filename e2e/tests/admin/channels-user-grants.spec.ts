// Admin channels 编辑页：允许的用户（user_ids）多选 UI 闭环。
// 使用 POM：pages/channels.page.ts。
//
// 验证：
//   1. 创建一个新渠道（默认 user_ids 空）
//   2. 编辑模式打开 → 看到「允许的用户」分组（含 alice/admin 复选框）
//   3. 勾选 alice → 保存 → API 反查 user_ids 落库
//   4. 重新打开编辑 → 复选框回显勾选
//   5. 清空 + 保存 → user_ids 变回 []
import { expect, test } from '../../fixtures/auth';
import { ChannelsPage } from '../../pages';

test.describe('Admin Channels：user_ids 多选 UI', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('勾选 → 保存 → 回显 → 清空全链路', async ({ page }) => {
    test.setTimeout(45_000);
    const ch = new ChannelsPage(page);
    await ch.goto();

    // 1) 创建临时渠道
    await ch.openCreate();
    const name = `e2e-user-grants-${Date.now()}`;
    await ch.fillForm({
      name,
      provider: 'openai',
      baseURL: 'http://127.0.0.1:18090',
      credentials: 'sk-mock',
      models: 'gpt-4o-mini',
    });
    const created = await ch.submitCreate();
    expect(created.user_ids ?? []).toEqual([]);

    // 2) 进编辑：「允许的用户」分组渲染
    await ch.openEdit(name);
    await expect(page.locator('text=/允许的用户/')).toBeVisible();
    const aliceCheckbox = page
      .locator('label', { hasText: 'alice@e2e.test' })
      .locator('input[type="checkbox"]');
    await expect(aliceCheckbox).not.toBeChecked();

    // 3) 勾选 alice → 保存 → 接口含 user_ids
    await ch.toggleUserGrant('alice@e2e.test');
    await expect(aliceCheckbox).toBeChecked();
    const upd1 = await ch.submitEdit(created.id);
    expect((upd1.user_ids ?? []).length).toBe(1);

    // 4) 重开编辑回显
    await ch.openEdit(name);
    await expect(
      page.locator('label', { hasText: 'alice@e2e.test' }).locator('input[type="checkbox"]'),
    ).toBeChecked();
    // 「清空」按钮显示数量
    await expect(page.getByRole('button', { name: /清空（1 已选）/ })).toBeVisible();

    // 5) 清空 + 保存 → user_ids 变回 []
    await page.getByRole('button', { name: /清空（1 已选）/ }).click();
    const upd2 = await ch.submitEdit(created.id);
    expect(upd2.user_ids ?? []).toEqual([]);

    // 清理
    await ch.delete(name);
  });
});
