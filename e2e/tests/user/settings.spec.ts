// Settings 页面：资料 / 2FA 设置入口 / 密码修改入口。
import { expect, test } from '../../fixtures/auth';

test.describe('Settings', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('展示邮箱和余额', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByText(/邮箱：/)).toBeVisible();
    await expect(page.getByText(/^余额：/)).toBeVisible();
  });

  test('页面无未捕获异常', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/settings');
    await page.waitForTimeout(500);
    expect(errors, errors.join('\n')).toHaveLength(0);
  });

  test('2FA 设置按钮存在时可点', async ({ page }) => {
    await page.goto('/settings');
    const btn = page.getByRole('button', { name: /2FA|两步|动态密码/ });
    if ((await btn.count()) === 0) {
      test.skip(true, '无 2FA 按钮');
      return;
    }
    // 自动吃掉弹窗（2FA setup 返回 otpauth URL，alert 展示）
    page.on('dialog', (d) => d.accept());
    await btn.first().click();
  });
});
