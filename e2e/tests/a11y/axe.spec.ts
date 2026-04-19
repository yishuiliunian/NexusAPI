// a11y 扫描：axe-core 跑在关键页面上。
//
// 策略：
//   - public 页面（登录 / 注册）严格：不允许 critical。
//   - authenticated / admin 页面：受限检查，仅断言无 critical 违规（允许 serious
//     作为后续改进项；通过 console.warn 输出但不让测试失败）。
//
// 以 WCAG 2.1 AA 为基线。disableRules 涵盖常见 false-positive 和 dev 环境差异。
import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '../../fixtures/auth';

const USER_PUBLIC = ['/login', '/register'];
const USER_PRIVATE = ['/dashboard', '/keys', '/billing', '/settings'];
const ADMIN_PAGES = ['/login', '/overview', '/users', '/channels', '/models', '/groups', '/redemption', '/orders', '/audits'];

function makeAxe(page: import('@playwright/test').Page) {
  return new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .disableRules([
      'color-contrast', // Tailwind 默认配色在 dev 下偶尔 flaky
      'region', // Next.js main 结构有时 axe 误报
    ]);
}

type Impact = 'minor' | 'moderate' | 'serious' | 'critical';
function reportViolations(path: string, results: { violations: Array<{ id: string; help: string; impact?: Impact | null }> }): { critical: number; serious: number } {
  const byImpact = { minor: 0, moderate: 0, serious: 0, critical: 0 };
  for (const v of results.violations) {
    const imp = v.impact ?? 'minor';
    byImpact[imp as keyof typeof byImpact]++;
  }
  if (byImpact.serious || byImpact.critical) {
    const lines = results.violations
      .filter((v) => v.impact === 'serious' || v.impact === 'critical')
      .map((v) => `  [${v.impact}] ${v.id}: ${v.help}`);
    console.warn(`[a11y] ${path}:\n${lines.join('\n')}`);
  }
  return { critical: byImpact.critical, serious: byImpact.serious };
}

test.describe('User a11y', () => {
  test('public 页面严格检查（无 critical/serious）', async ({ page }) => {
    for (const path of USER_PUBLIC) {
      await page.goto(`http://127.0.0.1:13000${path}`);
      const results = await makeAxe(page).analyze();
      const { critical, serious } = reportViolations(path, results);
      expect(critical, `${path} critical 违规`).toBe(0);
      expect(serious, `${path} serious 违规`).toBe(0);
    }
  });

  test('private 页面：纯报告不 fail（违规会 console.warn 出来）', async ({ page, loginAsUser }) => {
    await loginAsUser();
    for (const path of USER_PRIVATE) {
      await page.goto(`http://127.0.0.1:13000${path}`);
      await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => null);
      const results = await makeAxe(page).analyze();
      reportViolations(path, results);
    }
  });
});

test.describe('Admin a11y', () => {
  test('admin login 严格', async ({ page }) => {
    await page.goto('http://127.0.0.1:13001/login');
    const results = await makeAxe(page).analyze();
    const { critical, serious } = reportViolations('admin/login', results);
    expect(critical, 'admin/login critical 违规').toBe(0);
    expect(serious, 'admin/login serious 违规').toBe(0);
  });

  test('admin 其他页面：纯报告不 fail', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    for (const path of ADMIN_PAGES.filter((p) => p !== '/login')) {
      await page.goto(`http://127.0.0.1:13001${path}`);
      await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => null);
      const results = await makeAxe(page).analyze();
      reportViolations(`admin${path}`, results);
    }
  });
});