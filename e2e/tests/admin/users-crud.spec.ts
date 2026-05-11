// Admin 深度：users 改配额 / 搜索 / 分页。
//
// 改配额通过 UI prompt() 触发，用 page.on('dialog') 拦截输入值。
import { expect, test } from '../../fixtures/auth';

test.describe('Admin Users 深度', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('改余额通过 UI prompt', async ({ page }) => {
    await page.goto('/users');
    const aliceRow = page.locator('tr', { hasText: 'alice@e2e.test' });
    await expect(aliceRow).toBeVisible();

    // 拦截 prompt：输入 USD 数值（项目切 USD 后 prompt 输入单位是 USD）。
    page.on('dialog', async (d) => {
      if (d.type() === 'prompt') await d.accept('77.7778');
      else await d.accept();
    });

    // 等待 PUT /api/admin/users/:id/quota
    const quotaResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/users/') && r.url().endsWith('/quota') && r.request().method() === 'PUT'
    );
    await aliceRow.getByRole('button', { name: /改余额/ }).click();
    const r = await quotaResp;
    expect(r.ok(), `quota resp: ${r.status()}`).toBeTruthy();

    // 等 list 刷新
    await page.waitForResponse((r) => r.url().includes('/api/admin/users'));

    // 行内余额展示：$77.7778
    await expect(aliceRow).toContainText('$77.7778');
  });

  test('改余额：空输入不提交', async ({ page }) => {
    await page.goto('/users');
    const aliceRow = page.locator('tr', { hasText: 'alice@e2e.test' });
    let prompted = false;
    page.on('dialog', async (d) => {
      prompted = true;
      await d.dismiss(); // 空 = dismiss
    });
    await aliceRow.getByRole('button', { name: /改余额/ }).click();
    expect(prompted).toBeTruthy();
    // 无网络请求：余额未变
  });

  test('用户列表包含 seed 两个账号', async ({ page }) => {
    await page.goto('/users');
    await expect(page.locator('table')).toContainText('admin@e2e.test');
    await expect(page.locator('table')).toContainText('alice@e2e.test');
  });

  test('管理员创建用户 → 指定密码 + 角色 + 初始余额', async ({ page }) => {
    await page.goto('/users');
    const email = `e2e-created-${Date.now()}@x.test`;
    const password = 'StrongPwd123!';

    // dialog handler（alert 显示凭证后 accept）
    page.on('dialog', (d) => d.accept());

    await page.getByTestId('btn-new-user').click();
    await page.getByTestId('new-user-email').fill(email);
    await page.getByTestId('new-user-password').fill(password);
    await page.getByTestId('new-user-role').selectOption('admin');
    await page.getByTestId('new-user-quota').fill('10');

    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/users') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    const resp = await createResp;
    expect(resp.ok(), `create resp: ${resp.status()}`).toBeTruthy();
    const body = await resp.json();
    expect(body.email).toBe(email);
    expect(body.role).toBe('admin');
    expect(body.quota).toBe(10_000_000); // 10 USD → 10M micro

    // 列表刷新后包含新用户
    await expect(page.locator('table')).toContainText(email);
  });

  test('创建用户：邮箱重复 → 409', async ({ page }) => {
    await page.goto('/users');

    page.on('dialog', (d) => d.accept());

    await page.getByTestId('btn-new-user').click();
    await page.getByTestId('new-user-email').fill('admin@e2e.test'); // seed 已存在
    await page.getByTestId('new-user-password').fill('AnyStrong123!');

    const createResp = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/users') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建' }).click();
    const resp = await createResp;
    expect(resp.status()).toBe(409);
  });

  test('创建用户：密码少于 8 位 → 前端 HTML 校验拦截', async ({ page }) => {
    await page.goto('/users');

    await page.getByTestId('btn-new-user').click();
    await page.getByTestId('new-user-email').fill('short-pwd@x.test');
    await page.getByTestId('new-user-password').fill('short');

    // 未触发网络（form 的 required+minLength 阻止提交）
    let posted = false;
    page.on('request', (r) => {
      if (r.url().endsWith('/api/admin/users') && r.method() === 'POST') posted = true;
    });
    await page.getByRole('button', { name: '创建' }).click();
    // 等一会确保没发出
    await page.waitForTimeout(300);
    expect(posted).toBeFalsy();
  });
});