// Multi-provider relay 路径端到端：
//   1. /v1/chat/completions   → openai 协议 channel
//   2. /v1/messages           → claude 协议 channel
//   3. /v1/embeddings         → openai 协议 channel
//
// 卖点是「一个 ApiKey 覆盖所有 provider 路径」，这里集中验证三条都通。
import { expect, test } from '../../fixtures/auth';

test.describe('Multi-provider relay', () => {
  test.beforeEach(async ({ loginAsUser, grantQuota }) => {
    const { email } = await loginAsUser();
    await grantQuota(email, 10_000_000);
  });

  test('OpenAI /v1/chat/completions', async ({ page, createApiKey }) => {
    const { secret } = await createApiKey('multi-openai');
    const r = await page.request.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      data: { model: 'gpt-4o-mini', messages: [{ role: 'user', content: 'hi' }] },
    });
    expect(r.ok(), `status=${r.status()} body=${await r.text()}`).toBeTruthy();
    const body = (await r.json()) as {
      choices: Array<{ message: { content: string } }>;
      usage?: { total_tokens?: number };
    };
    expect(body.choices?.[0]?.message?.content).toMatch(/Hello/);
  });

  test('Anthropic /v1/messages', async ({ page, createApiKey }) => {
    const { secret } = await createApiKey('multi-claude');
    const r = await page.request.post('/v1/messages', {
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      data: { model: 'claude-3-5-sonnet', messages: [{ role: 'user', content: 'hi' }], max_tokens: 64 },
    });
    expect(r.ok(), `status=${r.status()} body=${await r.text()}`).toBeTruthy();
    const body = (await r.json()) as { type: string; content: Array<{ type: string; text: string }> };
    expect(body.type).toBe('message');
    expect(body.content?.[0]?.text).toMatch(/Hello/);
  });

  test('OpenAI /v1/embeddings', async ({ page, createApiKey }) => {
    const { secret } = await createApiKey('multi-embed');
    const r = await page.request.post('/v1/embeddings', {
      headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
      data: { model: 'text-embedding-3-small', input: 'hello world' },
    });
    expect(r.ok(), `status=${r.status()} body=${await r.text()}`).toBeTruthy();
    const body = (await r.json()) as { data: Array<{ embedding: number[] }> };
    expect(Array.isArray(body.data?.[0]?.embedding)).toBeTruthy();
  });
});
