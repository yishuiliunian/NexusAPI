// API Keys CRUD。
import { expect, test } from '../../fixtures/auth';

test.describe('ApiKey 管理', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('创建 + 展示 secret + 列表刷新', async ({ page }) => {
    await page.goto('/keys');
    await expect(page.getByRole('heading', { name: /API Keys/ })).toBeVisible();
    await page.getByTestId('apikey-name').fill('dev-key');
    await page.getByRole('button', { name: /\+ 新建|^创建$/ }).click();

    // 一次性 secret 显示（用 testid 精准定位弹窗内的 code，避免与 curl 示例冲突）
    const secret = page.getByTestId('apikey-secret');
    await expect(secret).toBeVisible();
    await expect(secret).toContainText(/sk-nexus-/);

    // 列表出现该 key
    await expect(page.locator('table')).toContainText('dev-key');
  });

  test('空名称不提交（无 apikey 生成）', async ({ page }) => {
    await page.goto('/keys');
    // 名称为空时按钮 disabled，不可创建
    await expect(page.getByRole('button', { name: /\+ 新建|^创建$/ })).toBeDisabled();
    // 没创建过 key → 弹窗里的 secret 元素不存在
    await expect(page.getByTestId('apikey-secret')).toHaveCount(0);
  });

  test('删除 key（autoaccept confirm）', async ({ page }) => {
    await page.goto('/keys');
    await page.getByTestId('apikey-name').fill('to-delete');
    await page.getByRole('button', { name: /\+ 新建|^创建$/ }).click();
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

    // 删完最后一个：列表空态文案
    await expect(page.getByText('还没有密钥')).toBeVisible();
  });
});
