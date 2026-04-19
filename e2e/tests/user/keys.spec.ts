// API Keys CRUD。
import { expect, test } from '../../fixtures/auth';

test.describe('ApiKey 管理', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('创建 + 展示 secret + 列表刷新', async ({ page }) => {
    await page.goto('/keys');
    await expect(page.getByRole('heading', { name: /API Keys/ })).toBeVisible();
    await page.fill('input[placeholder*="名称"]', 'dev-key');
    await page.getByRole('button', { name: '创建' }).click();

    // 一次性 secret 显示
    const secret = page.locator('code').first();
    await expect(secret).toBeVisible();
    await expect(secret).toContainText(/sk-nexus-/);

    // 列表出现该 key
    await expect(page.locator('table')).toContainText('dev-key');
  });

  test('空名称不提交（无 apikey 生成）', async ({ page }) => {
    await page.goto('/keys');
    await page.getByRole('button', { name: '创建' }).click();
    // 依然在页面，code.sk-nexus 不应出现
    await expect(page.locator('code').filter({ hasText: /sk-nexus-/ })).toHaveCount(0);
  });

  test('删除 key（autoaccept confirm）', async ({ page }) => {
    await page.goto('/keys');
    await page.fill('input[placeholder*="名称"]', 'to-delete');
    await page.getByRole('button', { name: '创建' }).click();
    await expect(page.locator('table')).toContainText('to-delete');

    // 先关闭 secret 弹层，避免 "关闭" / "复制" 按钮混淆
    const closeBtn = page.getByRole('button', { name: '关闭' });
    if (await closeBtn.count()) await closeBtn.click();

    page.on('dialog', (d) => d.accept());

    // 等待 DELETE 请求完成
    const delResp = page.waitForResponse(
      (r) => r.url().includes('/api/user/apikeys/') && r.request().method() === 'DELETE'
    );
    await page
      .locator('table tr', { hasText: 'to-delete' })
      .getByRole('button', { name: '删除' })
      .click();
    const r = await delResp;
    expect(r.ok(), `delete resp: ${r.status()}`).toBeTruthy();

    // 删完最后一个：table 消失、显示 "暂无密钥"
    await expect(page.getByText('暂无密钥')).toBeVisible();
  });
});
