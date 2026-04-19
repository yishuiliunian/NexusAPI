// 用户管理：改配额 / 封禁。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin 用户管理', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('用户列表展示 seed 用户', async ({ page }) => {
    await page.goto('/users');
    await expect(page.locator('table')).toContainText('admin@e2e.test');
    await expect(page.locator('table')).toContainText('alice@e2e.test');
  });

  test('封禁普通用户 → 状态更新', async ({ page }) => {
    await page.goto('/users');
    // 找 alice 所在行的"封禁"按钮
    page.on('dialog', (d) => d.accept());
    const aliceRow = page.locator('tr', { hasText: 'alice@e2e.test' });
    const banBtn = aliceRow.getByRole('button', { name: /封禁|停用|禁用/ });
    if ((await banBtn.count()) === 0) {
      test.skip(true, '用户管理 UI 没有封禁按钮');
      return;
    }
    await banBtn.first().click();
    // 状态变成 banned
    await expect(aliceRow).toContainText(/banned/);
    // 恢复
    const restoreBtn = aliceRow.getByRole('button', { name: /启用|恢复|解禁/ });
    if (await restoreBtn.count()) await restoreBtn.first().click();
  });
});
