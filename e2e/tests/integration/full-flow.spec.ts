// 端到端 happy path：user 注册 → 建 key → 调 /v1 → logs 可见 → 余额扣除。
import { expect, test } from '../../fixtures/auth';

test.describe('端到端：注册 → 建 key → 调用 → 计费落库', () => {
  test('完整链路可走通', async ({ page, loginAsUser, grantQuota }) => {
    // 1. 注册登录
    const { email } = await loginAsUser();
    // 1.5 给用户加配额（否则预占会 402）
    await grantQuota(email, 10_000_000);

    // 2. 建 ApiKey（UI 路径）
    await page.goto('/keys');
    await page.getByTestId('apikey-name').fill('e2e-key');
    await page.getByRole('button', { name: /\+ 新建|^创建$/ }).click();
    const secretCode = page.getByTestId('apikey-secret');
    await expect(secretCode).toBeVisible();
    const secret = (await secretCode.textContent()) ?? '';
    expect(secret).toMatch(/^sk-nexus-/);

    // 3. 用 secret 调 /v1/chat/completions → mock 上游返回
    const resp = await page.request.post('/v1/chat/completions', {
      headers: {
        Authorization: `Bearer ${secret}`,
        'Content-Type': 'application/json',
      },
      data: {
        model: 'gpt-4o-mini',
        messages: [{ role: 'user', content: 'hi' }],
      },
    });
    expect(resp.ok(), `chat status=${resp.status()}: ${await resp.text()}`).toBeTruthy();
    const body = (await resp.json()) as { choices?: Array<{ message?: { content?: string } }> };
    expect(body.choices?.[0]?.message?.content).toMatch(/Hello/);

    // 4. /api/user/usages 应多一条
    const usages = await page.request.get('/api/user/usages?page=1&size=10');
    expect(usages.ok()).toBeTruthy();
    const up = (await usages.json()) as { items: Array<{ model: string }> };
    expect(up.items.length).toBeGreaterThan(0);
    expect(up.items[0].model).toBe('gpt-4o-mini');

    // 5. dashboard 能打开并显示 "监控看板" 标题，包含本次调用的模型
    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: /监控看板/ })).toBeVisible();
    // 等待最近调用异步加载
    await page.waitForTimeout(500);
    await expect(page.locator('body')).toContainText('gpt-4o-mini');

    // 6. /api/user/me 包含正确 email
    const meResp = await page.request.get('/api/user/me');
    const me = (await meResp.json()) as { email: string };
    expect(me.email).toBe(email);
  });
});
