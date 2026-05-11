// 视觉回归：关键页面首屏截图 diff。
//
// 首次跑会生成 baseline（在 .playwright-snapshots/）。
// 后续跑若像素差 > threshold 即失败。
//
// 策略：
//   - 动态数据（邮箱、时间戳）mask 掉避免假阳性
//   - 使用 fullPage: false 只截首屏
//
// 首次启用：删除任何遗留 baseline 后跑 `bazel run //e2e:test -- --update-snapshots tests/visual`。
import { expect, test } from '../../fixtures/auth';
import { URLS } from '../../helpers/env';

// 像素容差：5% 以内视为相同（CSS 渲染抗锯齿 / 字体 hinting 等会有微差）
const TOLERANCE = { maxDiffPixelRatio: 0.05 };

test.describe.configure({ mode: 'serial' });

test.describe('User 视觉回归', () => {
  test('login 页', async ({ page }) => {
    await page.goto(`${URLS.user}/login`);
    await expect(page).toHaveScreenshot('user-login.png', TOLERANCE);
  });

  test('register 页', async ({ page }) => {
    await page.goto(`${URLS.user}/register`);
    await expect(page).toHaveScreenshot('user-register.png', TOLERANCE);
  });

  test('dashboard 页（登录后）', async ({ page, loginAsUser }) => {
    await loginAsUser({ email: 'visual-user@e2e.test', password: 'password123' });
    await page.goto(`${URLS.user}/dashboard`);
    await page.waitForLoadState('networkidle').catch(() => null);
    // mask 邮箱 / 余额 / 已消耗（动态数据）
    await expect(page).toHaveScreenshot('user-dashboard.png', {
      ...TOLERANCE,
      mask: [page.locator('.rounded.border.p-4')],
    });
  });
});

test.describe('Admin 视觉回归', () => {
  test('login 页', async ({ page }) => {
    await page.goto(`${URLS.admin}/login`);
    await expect(page).toHaveScreenshot('admin-login.png', TOLERANCE);
  });

  test('overview 页', async ({ page, loginAsAdmin }) => {
    await loginAsAdmin();
    await page.goto(`${URLS.admin}/overview`);
    await page.waitForLoadState('networkidle').catch(() => null);
    // mask 当前管理员 email
    await expect(page).toHaveScreenshot('admin-overview.png', {
      ...TOLERANCE,
      mask: [page.locator('section p').last()],
    });
  });
});