// 订阅套餐 CRUD + admin grant 给用户。
//
// 链路：
//   1. admin PUT /api/admin/plans 创建/更新 plan
//   2. admin POST /api/admin/subscriptions/grant 给 user 开通本地订阅
//   3. user GET /api/billing/subscription 看到自己的订阅
//   4. admin DELETE /api/admin/plans/:id 删除 plan
import { expect, test } from '../../fixtures/auth';
import { fetch } from 'undici';
import { API_BASE } from '../../playwright.config';

async function adminCookies() {
  const resp = await fetch(`${API_BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'admin@e2e.test', password: 'admin12345' }),
  });
  const setCookie = resp.headers.get('set-cookie') ?? '';
  const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
  const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
  if (!sess || !csrf) throw new Error('admin login no cookies');
  return { cookie: `nexus_session=${sess}; nexus_csrf=${csrf}`, csrf: csrf! };
}

test.describe('Subscription：plan CRUD + admin grant', () => {
  test('UpsertPlan → GET plans → grant 给 user → user 看见 → DELETE', async ({
    page,
    loginAsUser,
  }) => {
    test.setTimeout(60_000);

    // 1) 注册 user
    const u = await loginAsUser();

    // 2) admin login
    const admin = await adminCookies();

    // 3) admin upsert plan
    const planCode = `e2e_plan_${Date.now()}`;
    const upsert = await fetch(`${API_BASE}/api/admin/plans`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Cookie: admin.cookie,
        'X-CSRF-Token': admin.csrf,
      },
      body: JSON.stringify({
        code: planCode,
        name: 'E2E Test Plan',
        price_cents: 999,
        currency: 'USD',
        period_days: 30,
        included_quota: 5_000_000,
        enabled: true,
      }),
    });
    expect(upsert.ok, `upsert plan status=${upsert.status}`).toBeTruthy();
    const upsertBody = (await upsert.json()) as { plan: { id: number; code: string } };
    const planID = upsertBody.plan.id;
    expect(upsertBody.plan.code).toBe(planCode);

    // 4) admin GET plans → 包含 planCode
    const list = await fetch(`${API_BASE}/api/admin/plans`, { headers: { Cookie: admin.cookie } });
    const lb = (await list.json()) as { items: Array<{ code: string }> };
    expect(lb.items.some((p) => p.code === planCode)).toBeTruthy();

    // 5) 找 user id
    const ulist = await fetch(`${API_BASE}/api/admin/users?size=200`, {
      headers: { Cookie: admin.cookie },
    });
    const uitems = ((await ulist.json()) as { items: Array<{ id: number; email: string }> }).items;
    const target = uitems.find((x) => x.email === u.email);
    expect(target).toBeDefined();

    // 6) admin grant
    const grant = await fetch(`${API_BASE}/api/admin/subscriptions/grant`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Cookie: admin.cookie,
        'X-CSRF-Token': admin.csrf,
      },
      body: JSON.stringify({ user_id: target!.id, plan_code: planCode }),
    });
    expect(grant.ok, `grant status=${grant.status} ${await grant.text()}`).toBeTruthy();

    // 7) user 端 GET /api/billing/subscription 应能看见
    const sub = await page.request.get('/api/billing/subscription');
    expect(sub.ok()).toBeTruthy();
    const sj = (await sub.json()) as { subscription: { plan_code?: string } | null };
    expect(sj.subscription?.plan_code).toBe(planCode);

    // 8) 清理：admin DELETE plan（subscription 表行会孤儿，但本测试不强清）
    const del = await fetch(`${API_BASE}/api/admin/plans/${planID}`, {
      method: 'DELETE',
      headers: { Cookie: admin.cookie, 'X-CSRF-Token': admin.csrf },
    });
    expect([200, 404, 409]).toContain(del.status);
  });
});
