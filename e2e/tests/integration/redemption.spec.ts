// 兑换码流程：seed 预置 E2E-REDEEM-CODE / 1_000_000 micro。
//   - 首次兑换成功 + 余额到账
//   - 再次兑换同码报错（幂等）
//   - 不存在的码报错
import { expect, test } from '../../fixtures/auth';

const SEED_CODE = 'E2E-REDEEM-CODE';
const SEED_AMOUNT = 1_000_000;

async function getMe(page: import('@playwright/test').Page): Promise<{ quota: number }> {
  const r = await page.request.get('/api/user/me');
  expect(r.ok()).toBeTruthy();
  return (await r.json()) as { quota: number };
}

test.describe('兑换码', () => {
  test('合法码一次兑换成功 + 幂等拒绝', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    expect(csrf).toBeDefined();
    const headers = { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf! };

    const before = (await getMe(page)).quota;

    const r = await page.request.post('/api/billing/redeem', {
      headers,
      data: { code: SEED_CODE },
    });
    expect(r.ok(), `redeem: ${r.status()} ${await r.text()}`).toBeTruthy();
    const { amount } = (await r.json()) as { amount: number };
    expect(amount).toBe(SEED_AMOUNT);

    const after = (await getMe(page)).quota;
    expect(after - before).toBe(SEED_AMOUNT);

    // 再次兑换同码 → 应失败
    const again = await page.request.post('/api/billing/redeem', {
      headers,
      data: { code: SEED_CODE },
      failOnStatusCode: false,
    });
    expect(again.ok()).toBeFalsy();
  });

  test('不存在的码报错', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const r = await page.request.post('/api/billing/redeem', {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf! },
      data: { code: 'NOT-EXIST-CODE-XXX' },
      failOnStatusCode: false,
    });
    expect(r.ok()).toBeFalsy();
  });

  test('空码被拒', async ({ page, loginAsUser }) => {
    await loginAsUser();
    const csrf = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    const r = await page.request.post('/api/billing/redeem', {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf! },
      data: { code: '' },
      failOnStatusCode: false,
    });
    expect(r.ok()).toBeFalsy();
  });
});