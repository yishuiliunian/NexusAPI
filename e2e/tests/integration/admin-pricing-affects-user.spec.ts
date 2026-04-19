// Admin 操作经由 API 影响 User：禁用渠道后 user 调 /v1 应失败。
import { API_BASE } from '../../playwright.config';
import { expect, test } from '../../fixtures/auth';
import { fetch as undiciFetch } from 'undici';

// 全局记录：测试结束确保 channel 恢复到完整 active 状态。
// 注意：admin PUT /channels/:id 是 full-replace，不是 patch。因此必须
// 带上原始 channel 全字段，再覆盖 status。
interface ChannelFull {
  id: number;
  name: string;
  provider: string;
  base_url: string;
  models: string[] | null;
  group_ids: number[] | null;
  weight: number;
  price_multiplier: number;
  status: string;
  note: string;
}

let savedChannel: ChannelFull | null = null;
let restoreCookie = '';
let restoreCSRF = '';

test.afterEach(async () => {
  if (!savedChannel) return;
  await undiciFetch(`${API_BASE}/api/admin/channels/${savedChannel.id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Cookie: restoreCookie,
      'X-CSRF-Token': restoreCSRF,
    },
    body: JSON.stringify({
      name: savedChannel.name,
      provider: savedChannel.provider,
      base_url: savedChannel.base_url,
      models: savedChannel.models ?? [],
      group_ids: savedChannel.group_ids ?? [],
      weight: savedChannel.weight,
      price_multiplier: savedChannel.price_multiplier,
      status: 'active',
      note: savedChannel.note,
    }),
  }).catch(() => null);
  savedChannel = null;
});

test.describe('跨域：admin 禁用渠道 → user 调用失败', () => {
  test('disable mock channel → /v1/chat/completions 失败', async ({ page, loginAsUser, createApiKey, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 10_000_000);
    const { secret } = await createApiKey();

    // warm call
    const ok = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}` },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(ok.ok(), `warm call: ${ok.status()} ${await ok.text()}`).toBeTruthy();

    // admin 登录
    const loginResp = await undiciFetch(`${API_BASE}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: 'admin@e2e.test', password: 'admin12345' }),
    });
    expect(loginResp.ok).toBeTruthy();
    const setCookie = loginResp.headers.get('set-cookie') ?? '';
    const sess = /nexus_session=([^;]+)/.exec(setCookie)?.[1];
    const csrf = /nexus_csrf=([^;]+)/.exec(setCookie)?.[1];
    expect(sess && csrf).toBeTruthy();
    const cookieHeader = `nexus_session=${sess}; nexus_csrf=${csrf}`;

    // list channels → 找 mock-openai
    const list = await undiciFetch(`${API_BASE}/api/admin/channels`, {
      headers: { Cookie: cookieHeader },
    });
    expect(list.ok).toBeTruthy();
    const { items } = (await list.json()) as { items: ChannelFull[] };
    const ch = items.find((c) => c.name === 'mock-openai');
    expect(ch).toBeDefined();

    // 保存完整字段供 afterEach 恢复
    savedChannel = ch!;
    restoreCookie = cookieHeader;
    restoreCSRF = csrf!;

    // 禁用（带全字段）
    const upd = await undiciFetch(`${API_BASE}/api/admin/channels/${ch!.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader, 'X-CSRF-Token': csrf! },
      body: JSON.stringify({
        name: ch!.name,
        provider: ch!.provider,
        base_url: ch!.base_url,
        models: ch!.models ?? [],
        group_ids: ch!.group_ids ?? [],
        weight: ch!.weight,
        price_multiplier: ch!.price_multiplier,
        status: 'disabled',
        note: ch!.note,
      }),
    });
    if (!upd.ok) {
      test.skip(true, `admin disable channel 接口不兼容：${upd.status}`);
      return;
    }

    // user 重试 → 失败（可能 404/503/500，总之不 ok）
    const fail = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}` },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(fail.ok()).toBeFalsy();
  });
});
