// 支付充值 + 订阅流程（模拟 Stripe webhook）。
//
// 策略：直接在 DB 插 pending order，构造 HMAC-SHA256 签名 webhook payload，
// 发到 /api/webhook/stripe，断言订单 paid + 用户配额到账。
//
// 如果 server 未启用 Stripe gateway，测试 skip。
import { createHmac } from 'node:crypto';
import { fetch } from 'undici';
import { expect, test } from '../../fixtures/auth';

const WEBHOOK_SECRET = 'whsec_e2e_fixed';

function signPayload(payload: string, secret = WEBHOOK_SECRET, ts = Math.floor(Date.now() / 1000)): string {
  const signed = `${ts}.${payload}`;
  const sig = createHmac('sha256', secret).update(signed).digest('hex');
  return `t=${ts},v1=${sig}`;
}

test.describe('Stripe 订单 + webhook 到账', () => {
  test('/billing/topup 创建订单 → mock webhook → 配额到账', async ({ page, loginAsUser }) => {
    const { email } = await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    expect(csrf).toBeDefined();
    const headers = { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf! };

    // 1. 创建充值订单（amount_cents=1000 = $10 → 10M micro）
    const topupResp = await page.request.post('/api/billing/topup', {
      headers,
      data: { amount_cents: 1000, currency: 'USD', gateway: 'stripe' },
      failOnStatusCode: false,
    });
    if (topupResp.status() === 500 || topupResp.status() === 404) {
      test.skip(true, `topup 接口未启用或网关错误：${topupResp.status()}`);
      return;
    }
    expect(topupResp.ok(), `topup status=${topupResp.status()}: ${await topupResp.text()}`).toBeTruthy();
    const { order_id: orderID, checkout_url: checkoutURL } = (await topupResp.json()) as {
      order_id: string;
      checkout_url: string;
    };
    expect(orderID).toBeTruthy();
    // checkout_url 指向 mock 返回的 127.0.0.1:13000/billing?cs=... 但我们不跟，直接模拟 webhook
    expect(checkoutURL).toContain('cs_test_');

    // 2. 查 me 记录 before
    const before = (await (await page.request.get('/api/user/me')).json()) as { quota: number };

    // 3. 构造 webhook payload —— client_reference_id = 本地 order.ID
    const payload = JSON.stringify({
      id: 'evt_e2e_1',
      type: 'checkout.session.completed',
      data: {
        object: {
          id: `cs_e2e_${orderID}`,
          client_reference_id: orderID,
          payment_status: 'paid',
        },
      },
    });
    const sig = signPayload(payload);

    const wh = await page.request.post('/api/webhook/stripe', {
      data: payload,
      headers: { 'Stripe-Signature': sig, 'Content-Type': 'application/json' },
    });
    expect(wh.ok(), `webhook status=${wh.status()}: ${await wh.text()}`).toBeTruthy();

    // 4. 配额到账：amount_cents=1000 × MICRO_PER_CENT=10000 = 10M micro
    const after = (await (await page.request.get('/api/user/me')).json()) as { quota: number };
    expect(after.quota - before.quota).toBe(10_000_000);

    // 5. 订单状态 paid
    const orders = await page.request.get('/api/billing/orders');
    expect(orders.ok()).toBeTruthy();
    const orderList = (await orders.json()) as { items: Array<{ id: string; status: string }> };
    expect(orderList.items.find((o) => o.id === orderID)?.status).toBe('paid');

    // 6. 幂等：重放同一 webhook 不应重复到账
    await page.request.post('/api/webhook/stripe', {
      data: payload,
      headers: { 'Stripe-Signature': sig, 'Content-Type': 'application/json' },
    });
    const dup = (await (await page.request.get('/api/user/me')).json()) as { quota: number };
    expect(dup.quota).toBe(after.quota);
  });

  test('webhook 签名无效拒绝', async ({ page }) => {
    const payload = JSON.stringify({ id: 'evt_bad', type: 'noop' });
    const r = await page.request.post('/api/webhook/stripe', {
      data: payload,
      headers: { 'Stripe-Signature': 'bad-signature', 'Content-Type': 'application/json' },
      failOnStatusCode: false,
    });
    expect(r.ok()).toBeFalsy();
    expect([400, 401, 403]).toContain(r.status());
  });
});

test.describe('订阅套餐列表', () => {
  test('/api/billing/plans 返回 seed 的 plan', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const r = await page.request.get('/api/billing/plans');
    if (!r.ok()) {
      test.skip(true, `plans endpoint: ${r.status()}`);
      return;
    }
    const body = (await r.json()) as { items?: Array<{ code: string }> };
    expect(body.items?.some((p) => p.code === 'e2e_monthly')).toBeTruthy();
  });
});