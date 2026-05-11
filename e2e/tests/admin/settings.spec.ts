// Admin /settings 页：当前为只读运行参数清单。
// 测试维度：标题、章节、所有 NEXUSAPI_* 行均渲染。
// 若未来加入可编辑控件，应在此 spec 增加修改路径用例。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin /settings 只读渲染', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('运行时配置 9 行全部呈现', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByRole('heading', { name: /系统设置/ })).toBeVisible();
    await expect(page.getByText(/运行时配置/)).toBeVisible();

    // 与 web-admin/app/settings/page.tsx 中的 Row 一一对照
    const expected = [
      'NEXUSAPI_DATABASE_DRIVER',
      'NEXUSAPI_REDIS_ADDR',
      'NEXUSAPI_RATE_LIMIT_DEFAULT_RPM',
      'NEXUSAPI_RATE_LIMIT_DEFAULT_TPM',
      'NEXUSAPI_RELAY_FAILOVER_ATTEMPTS',
      'NEXUSAPI_PAYMENT_STRIPE_ENABLED',
      'NEXUSAPI_MAIL_HOST',
      'NEXUSAPI_OAUTH_GITHUB_ENABLED',
      'NEXUSAPI_OAUTH_GOOGLE_ENABLED',
    ];
    for (const key of expected) {
      await expect(page.locator('code', { hasText: key })).toBeVisible();
    }
  });
});
