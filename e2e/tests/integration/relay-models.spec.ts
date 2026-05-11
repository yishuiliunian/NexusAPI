// /v1/models —— relay 模型清单端点。聚合所有 active channel 的 Models。
import { expect, test } from '../../fixtures/auth';

test.describe('Relay /v1/models', () => {
  test('已认证用户拉到模型清单（含 seed channel 模型）', async ({ page, loginAsUser, createApiKey, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 1_000);
    const { secret } = await createApiKey('models-list');

    const r = await page.request.get('/v1/models', {
      headers: { Authorization: `Bearer ${secret}` },
    });
    expect(r.ok(), `status=${r.status()} body=${await r.text()}`).toBeTruthy();

    const body = (await r.json()) as { data: Array<{ id: string; object: string; provider?: string }> };
    expect(Array.isArray(body.data)).toBeTruthy();
    const ids = body.data.map((m) => m.id);
    // seed 出来的模型应包含
    expect(ids).toEqual(expect.arrayContaining(['gpt-4o-mini', 'claude-3-5-sonnet']));
    // 同名模型去重：不应有重复
    expect(new Set(ids).size).toBe(ids.length);
  });

  test('未带 ApiKey 调用应 401', async ({ page }) => {
    const r = await page.request.get('/v1/models', { failOnStatusCode: false });
    expect(r.status()).toBe(401);
  });
});
