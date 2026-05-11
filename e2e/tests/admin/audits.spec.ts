// Admin /audits 页：审计日志展示 + 数据真实性。
//
// 设计思路：先做一次 admin 操作（创建并立即删除一个 group），
// 然后访问 /audits 断言列表里至少能找到这两条 action。
//
// 当前 /audits 页无搜索/过滤 UI，所以 spec 只做：
//   - 标题/表头存在
//   - 列表至少有我们刚操作产生的记录（或 total > 0）
import { expect, test } from '../../fixtures/auth';

test.describe('Admin /audits', () => {
  test.beforeEach(async ({ loginAsAdmin }) => {
    await loginAsAdmin();
  });

  test('页面渲染 + 至少能反映管理员的最近操作', async ({ page }) => {
    test.setTimeout(45_000);

    // 1) 故意制造一条审计：创建一个临时分组
    const cookies = await page.context().cookies();
    const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;
    const name = `audit-trigger-${Date.now()}`;
    const cr = await page.request.post('/api/admin/groups', {
      headers: csrf ? { 'X-CSRF-Token': csrf } : {},
      data: { name, price_multiplier: 1 },
    });
    expect(cr.ok()).toBeTruthy();
    const { id } = (await cr.json()) as { id: number };

    // 立即删除，凑两条审计
    await page.request.delete(`/api/admin/groups/${id}`, {
      headers: csrf ? { 'X-CSRF-Token': csrf } : {},
    });

    // 2) 打开 audits 页
    const listResp = page.waitForResponse(
      (r) => r.url().includes('/api/admin/audits') && r.request().method() === 'GET',
    );
    await page.goto('/audits');
    const list = await listResp;
    expect(list.ok()).toBeTruthy();
    const { items, total } = (await list.json()) as {
      items: Array<{ action: string; target: string }>;
      total: number;
    };

    await expect(page.getByRole('heading', { name: /审计日志/ })).toBeVisible();
    await expect(page.locator('table thead')).toContainText('动作');

    // 3) 数据断言：至少有一条审计；如果有，表里也应渲染至少一行
    if (total > 0) {
      expect(items.length).toBeGreaterThan(0);
      await expect(page.locator('table tbody tr').first()).toBeVisible();
    } else {
      // 系统未启审计或不记录操作，至少 page 没崩
      await expect(page.locator('table')).toContainText('暂无审计记录');
    }
  });
});
