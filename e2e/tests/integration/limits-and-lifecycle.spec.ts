// 配额耗尽 / 限流 / session 失效场景。
import { fetch } from 'undici';
import { expect, test } from '../../fixtures/auth';
import { API_BASE } from '../../playwright.config';
import { URLS } from '../../helpers/env';

test.describe('Quota 耗尽', () => {
  test('quota=0 调 /v1 应 402', async ({ page, loginAsUser, createApiKey }) => {
    await loginAsUser();
    const { secret } = await createApiKey();

    const r = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}` },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(r.status()).toBe(402);
    const body = (await r.json()) as { code: string };
    expect(body.code).toBe('insufficient_quota');
  });
});

test.describe('Session 失效', () => {
  test('删掉 session cookie 后 /api/user/me 401', async ({ page, loginAsUser }) => {
    await loginAsUser();
    // 验证 me 正常
    expect((await page.request.get('/api/user/me')).ok()).toBeTruthy();
    // 清 cookie
    await page.context().clearCookies();
    const r = await page.request.get('/api/user/me');
    expect(r.status()).toBe(401);
  });

  test('伪造 session cookie 401', async ({ page }) => {
    await page.context().clearCookies();
    await page.context().addCookies([
      {
        name: 'nexus_session',
        value: 'fake-session-does-not-exist',
        url: URLS.user,
      },
    ]);
    const r = await page.request.get('/api/user/me');
    expect(r.status()).toBe(401);
  });
});

test.describe('Ban user', () => {
  test('admin ban → user ApiKey 被拒 403', async ({ page, loginAsUser, createApiKey, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 10_000_000);
    const { secret } = await createApiKey();

    // warm call
    const ok = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}` },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(ok.ok()).toBeTruthy();

    // admin ban
    const loginResp = await fetch(`${API_BASE}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: 'admin@e2e.test', password: 'admin12345' }),
    });
    const setCookie = loginResp.headers.get('set-cookie') ?? '';
    const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
    const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
    expect(sess && csrf).toBeTruthy();
    const cookieHeader = `nexus_session=${sess}; nexus_csrf=${csrf}`;

    // find user id
    const list = await fetch(`${API_BASE}/api/admin/users?size=200`, {
      headers: { Cookie: cookieHeader },
    });
    const { items } = (await list.json()) as { items: Array<{ id: number; email: string }> };
    const target = items.find((u) => u.email === email);
    expect(target).toBeDefined();

    // ban
    const ban = await fetch(`${API_BASE}/api/admin/users/${target!.id}/status`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader, 'X-CSRF-Token': csrf! },
      body: JSON.stringify({ status: 'banned' }),
    });
    expect(ban.ok).toBeTruthy();

    // user call should fail
    const fail = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}` },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(fail.ok()).toBeFalsy();
    expect([401, 403]).toContain(fail.status());

    // restore
    await fetch(`${API_BASE}/api/admin/users/${target!.id}/status`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader, 'X-CSRF-Token': csrf! },
      body: JSON.stringify({ status: 'active' }),
    });
  });
});
