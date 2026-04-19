// OAuth 登录流程：github / google 两家都验证。
//
// 流程：
//   1. 客户端 GET /api/auth/oauth/github/authorize → 302 到 upstream mock
//   2. mock 302 回 /api/auth/oauth/github/callback?code=xxx&state=yyy
//   3. backend 调 mock /oauth/github/token → /user → 建用户 + 发 session → 302 到 post_login_url
//   4. 最终到达 /dashboard，/api/user/me 可读
import { expect, test } from '../../fixtures/auth';

test.describe('OAuth 登录', () => {
  for (const provider of ['github', 'google'] as const) {
    test(`${provider} OAuth 跳转回调能建立 session`, async ({ page }) => {
      // 清 cookie 确保无痕起步
      await page.context().clearCookies();

      // 跟随 302 链：/api/auth/oauth/github/authorize → mock → callback → /dashboard
      const resp = await page.goto(`/api/auth/oauth/${provider}/authorize`);
      // 最终落地页期望 /dashboard
      await expect(page).toHaveURL(/\/dashboard$|\/overview$/, { timeout: 15_000 });
      expect(resp?.status()).toBeLessThan(400);

      // /api/user/me 200
      const me = await page.request.get('/api/user/me');
      expect(me.ok(), `me status: ${me.status()}`).toBeTruthy();
      const body = (await me.json()) as { email: string; role: string };
      expect(body.email).toMatch(/@e2e\.test$/);
    });
  }

  test('OAuth state 被篡改时拒绝', async ({ page }) => {
    // 不经 authorize，直接伪造 callback
    const resp = await page.request.get('/api/auth/oauth/github/callback?code=fake&state=injected', {
      maxRedirects: 0,
      failOnStatusCode: false,
    });
    // 应拒绝（400/401/403），不应 302 到 dashboard
    expect(resp.status()).toBeGreaterThanOrEqual(400);
  });
});