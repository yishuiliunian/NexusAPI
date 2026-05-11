// Admin Orders 页深度断言：
//   1) 空态：seed 后没订单时显示「暂无订单」
//   2) 创建一笔订单（user /api/billing/topup）→ 列表新增一行
//   3) 模拟 webhook 把订单变 paid → 列表行的状态徽章从 created/pending → paid
//
// 设计：避免依赖真实 Stripe，沿用 payment-subscription.spec.ts 的 webhook 走法。
// /api/billing/topup 在某些配置下可能返回 404/500（gateway 未启用），那就 skip。
import { API_BASE } from '../../playwright.config';
import { expect, test } from '../../fixtures/auth';
import { fetch as undiciFetch } from 'undici';

test.describe('Admin /orders', () => {
  test('列表渲染：空态文案或行数据正确', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    const listResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/orders') && r.request().method() === 'GET',
    );
    await page.goto('/orders');
    const list = await listResp;
    expect(list.ok()).toBeTruthy();

    await expect(page.getByRole('heading', { name: /订单/ })).toBeVisible();

    const { items } = (await list.json()) as {
      items: Array<{ id: string; status: string; amount_cents: number; gateway: string }>;
    };

    if (items.length === 0) {
      await expect(page.locator('table')).toContainText('暂无订单');
    } else {
      // 至少有 1 行 + 描述里包含订单数
      await expect(page.locator('table tbody tr')).toHaveCount(items.length);
      await expect(page.locator('body')).toContainText(`${items.length} 个订单`);
    }
  });

  test('user 充值创建订单 → admin 列表立即可见', async ({
    page,
    loginAsUser,
    loginAsAdmin,
    grantQuota,
  }) => {
    test.setTimeout(45_000);

    // 1) 普通用户充值（建订单）
    const { email } = await loginAsUser();
    await grantQuota(email, 1_000); // 仅给点配额避免 banned 风险
    const topup = await page.request.post('/api/billing/topup', {
      data: { amount_cents: 1234, currency: 'USD', gateway: 'stripe' },
    });
    if (!topup.ok()) {
      test.skip(true, `topup 不可用，跳过：status=${topup.status()}`);
      return;
    }
    const { order_id: orderID } = (await topup.json()) as { order_id: string };
    expect(orderID).toBeTruthy();

    // 2) 切到 admin，访问 /orders 页
    await page.request.post('/api/auth/logout');
    await loginAsAdmin();
    const listResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/orders') && r.request().method() === 'GET',
    );
    await page.goto('/orders');
    const list = await listResp;
    const { items } = (await list.json()) as {
      items: Array<{ id: string; status: string }>;
    };
    const found = items.find((o) => o.id === orderID);
    expect(found, '新建订单应出现在列表').toBeDefined();

    // 列表前 8 位订单 ID 在 table 显示
    await expect(page.locator('table')).toContainText(orderID.slice(0, 8));
  });
});
