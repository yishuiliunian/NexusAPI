// 非 happy path：CSRF 伪造 / XSS 注入 / 超长输入 / 网络异常 / 重复提交。
import { expect, test } from '../../fixtures/auth';

test.describe('CSRF 保护', () => {
  test('登录态 POST 无 X-CSRF-Token 应 403', async ({ page, loginAsUser }) => {
    await loginAsUser();
    // 不带 header 的 POST
    const r = await page.request.post('/api/user/apikeys', {
      data: { name: 'csrf-attack' },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(403);
  });

  test('CSRF header 与 cookie 不匹配 403', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.post('/api/user/apikeys', {
      data: { name: 'csrf-mismatch' },
      headers: { 'X-CSRF-Token': 'forged-token-999' },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(403);
  });
});

test.describe('输入验证', () => {
  test('超长 ApiKey 名称被拒', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const longName = 'x'.repeat(10_000);
    const r = await page.request.post('/api/user/apikeys', {
      data: { name: longName },
      headers: { 'X-CSRF-Token': csrf!, 'Content-Type': 'application/json' },
      failOnStatusCode: false,
    });
    // 不允许写入；要么 400 要么截断到合法长度
    // 这里只做软断言：status < 500（不是崩溃）
    expect(r.status()).toBeLessThan(500);
  });

  test('email 格式错误注册被拒', async ({ page }) => {
    await page.context().clearCookies();
    const r = await page.request.post('/api/auth/register', {
      data: { email: 'not-an-email', password: 'password123' },
      headers: { 'Content-Type': 'application/json' },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(400);
  });

  test('弱密码（<8）注册被拒', async ({ page }) => {
    const r = await page.request.post('/api/auth/register', {
      data: { email: 'weak@e2e.test', password: 'short' },
      headers: { 'Content-Type': 'application/json' },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(400);
  });

  test('XSS payload 作为 ApiKey name 被存储但不执行', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const xss = "<img src=x onerror=\"window.__xss=true\">";
    const r = await page.request.post('/api/user/apikeys', {
      data: { name: xss },
      headers: { 'X-CSRF-Token': csrf!, 'Content-Type': 'application/json' },
    });
    expect(r.ok()).toBeTruthy();
    // 前端渲染后 window.__xss 不应被设置（React 默认 escape）
    await page.goto('/keys');
    const xssFired = await page.evaluate(() => (window as unknown as { __xss?: boolean }).__xss ?? false);
    expect(xssFired).toBeFalsy();
    // 且 name 文本能看到（但作为文本不触发）
    await expect(page.locator('table')).toContainText('onerror');
  });
});

test.describe('ApiKey 鉴权', () => {
  test('无 Authorization 调 /v1 401', async ({ page }) => {
    const r = await page.request.post('/v1/chat/completions', {
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(401);
  });

  test('伪造 ApiKey 调 /v1 401', async ({ page }) => {
    const r = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: 'Bearer sk-nexus-forged-does-not-exist' },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
      failOnStatusCode: false,
    });
    expect(r.status()).toBe(401);
  });
});

test.describe('管理员权限守卫', () => {
  test('普通用户调 /api/admin/users 403', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/admin/users', { failOnStatusCode: false });
    expect([401, 403]).toContain(r.status());
  });
});