// AI 任务提交 + 用户端列表查看。
//
// 注意：
//   - e2e 不起 worker（task poller），任务一直 pending。
//   - `/mj/*` 和 `/suno/*` 不在 web-user/next.config.ts 的 rewrites 列表里，
//     所以走 page.request 会 404；直接调 backend(API_BASE)。
//   - spec 仅验证「提交 → 任务记录入库 → /api/user/tasks 可见」，
//     不轮询任务完成（worker e2e 是另一档）。
import { fetch } from 'undici';
import { expect, test } from '../../fixtures/auth';
import { API_BASE } from '../../playwright.config';

test.describe('AI 任务：midjourney / suno', () => {
  test.beforeEach(async ({ loginAsUser, grantQuota }) => {
    const { email } = await loginAsUser();
    // 任务有 task_price，给点配额避免 402
    await grantQuota(email, 10_000_000);
  });

  test('mj/submit/imagine → 任务入库 → /api/user/tasks 可见', async ({ page, createApiKey }) => {
    const { secret } = await createApiKey('mj-key');
    const sub = await fetch(`${API_BASE}/mj/submit/imagine`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'a cute cat in space' }),
    });
    expect(sub.ok, `submit status=${sub.status}`).toBeTruthy();
    const body = (await sub.json()) as { code: number; result?: string; task_id?: string };
    expect(body.code).toBe(1);
    const taskID = body.result ?? body.task_id;
    expect(taskID).toBeTruthy();

    // user-side list（走 page.request 用 session）
    const list = await page.request.get('/api/user/tasks?page=1&size=10');
    expect(list.ok()).toBeTruthy();
    const j = (await list.json()) as { items: Array<{ id: string; provider: string }> };
    const found = j.items.find((t) => t.id === taskID);
    expect(found, 'task should appear in user list').toBeDefined();
    expect(found?.provider).toBe('midjourney');
  });

  test('suno/submit/lyrics → 任务入库', async ({ page, createApiKey }) => {
    const { secret } = await createApiKey('suno-key');
    const sub = await fetch(`${API_BASE}/suno/submit/lyrics`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'a song about coding' }),
    });
    expect(sub.ok, `suno submit status=${sub.status}`).toBeTruthy();
    const body = (await sub.json()) as { task_id: string };
    expect(body.task_id).toBeTruthy();

    const get = await page.request.get(`/api/user/tasks/${body.task_id}`);
    expect(get.ok()).toBeTruthy();
    const t = (await get.json()) as { id: string; provider: string };
    expect(t.id).toBe(body.task_id);
    expect(t.provider).toBe('suno');
  });

  test('admin /api/admin/tasks 看全站任务', async ({ page, createApiKey, loginAsAdmin }) => {
    // 用户先提一个 mj 任务
    const { secret } = await createApiKey('mj-for-admin');
    const sub = await fetch(`${API_BASE}/mj/submit/imagine`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'test' }),
    });
    expect(sub.ok, `submit status=${sub.status}`).toBeTruthy();

    // 切 admin 看全站任务
    await page.request.post('/api/auth/logout');
    await loginAsAdmin();
    const r = await page.request.get('/api/admin/tasks?page=1&size=20');
    expect(r.ok()).toBeTruthy();
    const j = (await r.json()) as { items: Array<{ provider: string }> };
    expect(j.items.length).toBeGreaterThan(0);
  });
});
