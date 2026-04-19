// 剩余管理页：groups / audits / orders / redemption / settings。
// 每个页仅验证：登录后可打开、无 pageerror、存在标题。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin 其他页面冒烟', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  const pages = [
    { path: '/groups', keyword: /分组|Group/ },
    { path: '/audits', keyword: /审计|Audit/ },
    { path: '/orders', keyword: /订单|Order/ },
    { path: '/redemption', keyword: /激活码|Redemption/ },
    { path: '/settings', keyword: /设置|Setting/ },
  ];

  for (const p of pages) {
    test(`${p.path} 打开不崩`, async ({ page }) => {
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      const resp = await page.goto(p.path);
      expect(resp?.status()).toBeLessThan(500);
      await page.waitForTimeout(500);
      expect(errors, errors.join('\n')).toHaveLength(0);
      await expect(page.locator('body')).toContainText(p.keyword);
    });
  }
});
