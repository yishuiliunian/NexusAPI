// admin sync 操作 + 各类单点 API 冒烟覆盖。
//
// 把那些「只有 endpoint 没专项 spec」的接口集中验证：状态码合理、关键字段渲染。
import { expect, test } from '../../fixtures/auth';
import { fetch } from 'undici';
import { API_BASE } from '../../playwright.config';

async function adminCookie() {
  const r = await fetch(`${API_BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'admin@e2e.test', password: 'admin12345' }),
  });
  const setCookie = r.headers.get('set-cookie') ?? '';
  const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
  const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
  if (!sess || !csrf) throw new Error('admin no cookies');
  return { cookie: `nexus_session=${sess}; nexus_csrf=${csrf}`, csrf: csrf! };
}

test.describe('Admin sync 操作', () => {
  test('channels/:id/sync-models 触发上游同步', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const list = await page.request.get('/api/admin/channels');
    const items = ((await list.json()) as { items: Array<{ id: number; provider: string }> }).items;
    // 选 openai provider channel（upstream-mock 实现了 GET /v1/models）
    const target = items.find((c) => c.provider === 'openai');
    expect(target, 'need an openai channel from seed').toBeDefined();

    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const r = await page.request.post(`/api/admin/channels/${target!.id}/sync-models`, {
      headers: { 'X-CSRF-Token': csrf! },
      failOnStatusCode: false,
    });
    // 200 = 成功，400 = provider 不支持也算合理。
    expect([200, 400]).toContain(r.status());
    if (r.ok()) {
      const body = (await r.json()) as { models: string[]; count: number };
      expect(body.count).toBeGreaterThan(0);
    }
  });

  test.skip('models/sync-pricing 触发 LiteLLM 同步', async ({ page, loginAsAdmin }) => {
    // SKIP 原因：sync-pricing 真去调 LiteLLM 远端拉全量 model_prices，
    // 会把 seed 的 claude-3-5-sonnet / gpt-4o-mini 等替换成 LiteLLM 命名空间
    // 前缀的模型清单，污染后续 relay-providers 等 spec。
    // 等 backend 支持配置 NEXUSAPI_BILLING_LITELLM_URL 指向 upstream-mock 后 unskip。
    await loginAsAdmin();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const r = await page.request.post('/api/admin/models/sync-pricing', {
      headers: { 'X-CSRF-Token': csrf! },
      failOnStatusCode: false,
    });
    expect([200, 502, 503]).toContain(r.status());
  });
});

test.describe('单点 API 冒烟', () => {
  test('admin /api/admin/providers 列可用 providers', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const r = await page.request.get('/api/admin/providers');
    expect(r.ok()).toBeTruthy();
    const body = (await r.json()) as { providers: string[] };
    expect(body.providers).toEqual(expect.arrayContaining(['openai', 'claude']));
  });

  test('admin /api/admin/logs/usages 列全站 usage', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const r = await page.request.get('/api/admin/logs/usages?page=1&size=20');
    expect(r.ok()).toBeTruthy();
    const j = (await r.json()) as { items: unknown[] };
    expect(Array.isArray(j.items)).toBeTruthy();
  });

  test('admin /api/admin/redemptions 列激活码', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const r = await page.request.get('/api/admin/redemptions?page=1&size=20');
    expect(r.ok()).toBeTruthy();
    const j = (await r.json()) as { items: unknown[] };
    expect(Array.isArray(j.items)).toBeTruthy();
  });

  test('user /api/user/quota-alert 设阈值', async ({ page, loginAsUser, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 1_000_000);
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const r = await page.request.put('/api/user/quota-alert', {
      headers: { 'X-CSRF-Token': csrf! },
      data: { quota_alert_at: 100_000 },
    });
    expect(r.ok(), `quota-alert: ${r.status()} ${await r.text()}`).toBeTruthy();

    const me = await page.request.get('/api/user/me');
    const meBody = (await me.json()) as { quota_alert_at: number };
    expect(meBody.quota_alert_at).toBe(100_000);
  });

  test('user /api/user/ledgers 看账本流水', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/user/ledgers?page=1&size=20');
    expect(r.ok()).toBeTruthy();
    const j = (await r.json()) as { items: unknown[] };
    expect(Array.isArray(j.items)).toBeTruthy();
  });

  test('user /api/billing/gateways 列支付网关', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/billing/gateways');
    expect(r.ok()).toBeTruthy();
    const j = (await r.json()) as { gateways: string[] };
    expect(Array.isArray(j.gateways)).toBeTruthy();
  });
});
