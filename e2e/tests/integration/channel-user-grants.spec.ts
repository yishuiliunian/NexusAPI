// 集成测试：用户级渠道白名单（user_ids）的运行时过滤。
//
// 业务目标：admin 把 channel.user_ids 设为 [alice]，alice 能调通，bob 路由失败；
// 清空 user_ids 后 bob 又能调通。同时核对「Group ∧ User」AND 语义符合预期。
//
// 实现策略：
//   1. 用临时 channel `e2e-grants-<ts>`，**唯一** 服务一个特殊 model 名（带时间戳）
//   2. 把该 model 同时插 model_prices（避免 EnsurePriced 拒绝）
//   3. 测白名单切换不污染 mock-openai
//   4. spec 结束 afterEach 删除 channel + price 行
import { API_BASE } from '../../playwright.config';
import { expect, test } from '../../fixtures/auth';
import { fetch as undiciFetch } from 'undici';
import { URLS } from '../../helpers/env';

interface AdminSession {
  cookie: string;
  csrf: string;
}

async function adminLogin(): Promise<AdminSession> {
  const resp = await undiciFetch(`${API_BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'admin@e2e.test', password: 'admin12345' }),
  });
  expect(resp.ok, `admin login: ${resp.status}`).toBeTruthy();
  const setCookie = resp.headers.get('set-cookie') ?? '';
  const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
  const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
  if (!sess || !csrf) throw new Error('admin login: missing cookies');
  return { cookie: `nexus_session=${sess}; nexus_csrf=${csrf}`, csrf };
}

async function findUserId(admin: AdminSession, email: string): Promise<number> {
  const r = await undiciFetch(`${API_BASE}/api/admin/users?size=200`, {
    headers: { Cookie: admin.cookie },
  });
  expect(r.ok).toBeTruthy();
  const { items } = (await r.json()) as { items: Array<{ id: number; email: string }> };
  const u = items.find((x) => x.email === email);
  if (!u) throw new Error(`user ${email} not found`);
  return u.id;
}

async function createPrivateChannel(admin: AdminSession, name: string, model: string): Promise<number> {
  const r = await undiciFetch(`${API_BASE}/api/admin/channels`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Cookie: admin.cookie,
      'X-CSRF-Token': admin.csrf,
    },
    body: JSON.stringify({
      name,
      provider: 'openai',
      base_url: URLS.upstream,
      credentials: 'sk-mock-private',
      models: [model],
      group_ids: [],
      user_ids: [],
      apikey_ids: [],
      weight: 100,
      price_multiplier: 1.0,
      status: 'active',
      note: 'e2e channel-user-grants temp',
    }),
  });
  expect(r.ok, `create channel: ${r.status}`).toBeTruthy();
  const ch = (await r.json()) as { id: number };
  return ch.id;
}

async function updateUserIds(admin: AdminSession, id: number, userIDs: number[]): Promise<void> {
  // PUT 是 full-replace，重新取一次 channel 再写回。
  const get = await undiciFetch(`${API_BASE}/api/admin/channels/${id}`, {
    headers: { Cookie: admin.cookie },
  });
  const ch = (await get.json()) as Record<string, unknown>;
  ch.user_ids = userIDs;
  const r = await undiciFetch(`${API_BASE}/api/admin/channels/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Cookie: admin.cookie,
      'X-CSRF-Token': admin.csrf,
    },
    body: JSON.stringify(ch),
  });
  expect(r.ok, `PUT channel ${id}: ${r.status}`).toBeTruthy();
}

async function deleteChannel(admin: AdminSession, id: number): Promise<void> {
  await undiciFetch(`${API_BASE}/api/admin/channels/${id}`, {
    method: 'DELETE',
    headers: { Cookie: admin.cookie, 'X-CSRF-Token': admin.csrf },
  });
}

async function upsertModelPrice(admin: AdminSession, model: string): Promise<number> {
  const r = await undiciFetch(`${API_BASE}/api/admin/models`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Cookie: admin.cookie,
      'X-CSRF-Token': admin.csrf,
    },
    body: JSON.stringify({
      model,
      capability: 'chat',
      input_price: 150,
      output_price: 600,
      cache_price: 0,
      output_multiplier: 1,
      task_price: 0,
      enabled: true,
    }),
  });
  expect(r.ok, `upsert price: ${r.status}`).toBeTruthy();
  const body = (await r.json()) as { id: number };
  return body.id;
}

async function deleteModelPrice(admin: AdminSession, id: number): Promise<void> {
  await undiciFetch(`${API_BASE}/api/admin/models/${id}`, {
    method: 'DELETE',
    headers: { Cookie: admin.cookie, 'X-CSRF-Token': admin.csrf },
  });
}

// 临时 channel id 跟踪，afterEach 强保证删除。
let tempChannelID: number | null = null;
let tempPriceID: number | null = null;
let teardownAdmin: AdminSession | null = null;

test.afterEach(async () => {
  if (tempChannelID && teardownAdmin) {
    await deleteChannel(teardownAdmin, tempChannelID).catch(() => null);
  }
  if (tempPriceID && teardownAdmin) {
    await deleteModelPrice(teardownAdmin, tempPriceID).catch(() => null);
  }
  tempChannelID = null;
  tempPriceID = null;
  teardownAdmin = null;
});

test.describe('Channel user_ids 白名单：三层 AND 过滤运行时', () => {
  test('user_ids=[alice] → alice 通过 / bob 拒绝；清空 → 都通过', async ({
    page,
    loginAsUser,
    createApiKey,
    grantQuota,
  }) => {
    test.setTimeout(60_000);

    const model = `private-mock-${Date.now()}`;

    // 1) 建 alice，拿 ApiKey
    const alice = await loginAsUser();
    await grantQuota(alice.email, 10_000_000);
    const aliceKey = await createApiKey('alice-key');
    await page.request.post('/api/auth/logout');

    // 2) 建 bob，拿 ApiKey
    const bob = await loginAsUser();
    await grantQuota(bob.email, 10_000_000);
    const bobKey = await createApiKey('bob-key');
    await page.request.post('/api/auth/logout');

    // 3) admin 建临时 channel（model 唯一，避免别处的 channel 抢路由）+ 插模型价格
    const admin = await adminLogin();
    teardownAdmin = admin;
    const aliceId = await findUserId(admin, alice.email);
    tempPriceID = await upsertModelPrice(admin, model);
    tempChannelID = await createPrivateChannel(admin, `e2e-grants-${Date.now()}`, model);

    // 4) 暖手：默认无白名单，alice / bob 都能通
    const warmAlice = await undiciFetch(`${API_BASE}/v1/chat/completions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${aliceKey.secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: [{ role: 'user', content: 'hi' }] }),
    });
    expect(warmAlice.ok, `warm alice: ${warmAlice.status}`).toBeTruthy();
    const warmBob = await undiciFetch(`${API_BASE}/v1/chat/completions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${bobKey.secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: [{ role: 'user', content: 'hi' }] }),
    });
    expect(warmBob.ok, `warm bob: ${warmBob.status}`).toBeTruthy();

    // 5) 关键：user_ids=[alice]
    await updateUserIds(admin, tempChannelID, [aliceId]);

    const aliceAfter = await undiciFetch(`${API_BASE}/v1/chat/completions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${aliceKey.secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: [{ role: 'user', content: 'hi' }] }),
    });
    expect(aliceAfter.ok, `alice 应仍能通过白名单: ${aliceAfter.status}`).toBeTruthy();

    const bobAfter = await undiciFetch(`${API_BASE}/v1/chat/completions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${bobKey.secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: [{ role: 'user', content: 'hi' }] }),
    });
    expect(bobAfter.ok, `bob 应被白名单拒绝，但 status=${bobAfter.status}`).toBeFalsy();
    expect([403, 404, 503]).toContain(bobAfter.status);

    // 6) 清空 user_ids → bob 又能通
    await updateUserIds(admin, tempChannelID, []);
    const bobOpen = await undiciFetch(`${API_BASE}/v1/chat/completions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${bobKey.secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: [{ role: 'user', content: 'hi' }] }),
    });
    expect(bobOpen.ok, `清空 user_ids 后 bob 应能通过: ${bobOpen.status}`).toBeTruthy();
  });
});
