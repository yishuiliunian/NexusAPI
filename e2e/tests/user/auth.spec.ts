// 登录 / 注册 / 登出三条核心认证路径。
import { expect, test } from '../../fixtures/auth';

test.describe('认证流程', () => {
  test('登录页可见且必填校验生效', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: /登录/ })).toBeVisible();
    await page.locator('button[type="submit"]').click();
    // HTML5 required 拦截：submit 不会发请求，URL 不变。
    await expect(page).toHaveURL(/\/login$/);
  });

  test('注册 → 自动登录 → 跳 dashboard', async ({ page }) => {
    await page.goto('/register');
    const email = `reg-${Date.now()}@e2e.test`;
    await page.fill('input[type="email"]', email);
    await page.fill('input[type="password"]', 'password123');
    await page.getByRole('button', { name: /注册/ }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
    // 新 Dashboard 展示"监控看板"标题
    await expect(page.getByRole('heading', { name: /监控看板/ })).toBeVisible();
  });

  test('密码错误显示错误文案', async ({ page, loginAsUser }) => {
    const { email } = await loginAsUser();
    await page.request.post('/api/auth/logout');
    await page.goto('/login');
    await page.fill('input[type="email"]', email);
    await page.fill('input[type="password"]', 'wrong-password');
    await page.getByRole('button', { name: /登录/ }).click();
    await expect(page.locator('.text-red-600')).toContainText(/邮箱|密码/);
  });

  test('登录后通过 UI 跳转 dashboard', async ({ page, loginAsUser }) => {
    const { email, password } = await loginAsUser();
    await page.request.post('/api/auth/logout');
    await page.goto('/login');
    await page.fill('input[type="email"]', email);
    await page.fill('input[type="password"]', password);
    await page.getByRole('button', { name: /登录/ }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
  });

  test('未登录访问 dashboard 跳 /login', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login$/);
  });
});
