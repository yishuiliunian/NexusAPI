// LoginPage — 用户/管理员登录页通用 POM。
// 用法：const lp = new LoginPage(page); await lp.goto(); await lp.login(email, password);
import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';

export class LoginPage {
  constructor(public readonly page: Page) {}

  async goto(): Promise<void> {
    await this.page.goto('/login');
    await expect(this.page).toHaveURL(/\/login/);
  }

  /** 走 UI 路径登录（多数 spec 应直接用 fixtures/auth 的 loginAsUser/Admin 走 API，更快）。 */
  async login(email: string, password: string): Promise<void> {
    await this.page.getByPlaceholder(/邮箱|email/i).fill(email);
    await this.page.getByPlaceholder(/密码|password/i).fill(password);
    await this.page.getByRole('button', { name: /登录/ }).click();
  }
}
