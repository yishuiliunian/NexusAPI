// UsersPage — admin /users 页 POM 占位。
// 当前 spec 大多直接用 page.request 走 API（更快），UI 操作覆盖不多。
// 此处先放骨架，随后补真实方法。
import type { Page, Locator } from '@playwright/test';
import { expect } from '@playwright/test';

export class UsersPage {
  constructor(public readonly page: Page) {}

  async goto(): Promise<void> {
    await this.page.goto('/users');
    await expect(this.page.getByRole('heading', { name: /用户管理|用户/ })).toBeVisible();
  }

  row(email: string): Locator {
    return this.page.locator('tr', { hasText: email });
  }
}
