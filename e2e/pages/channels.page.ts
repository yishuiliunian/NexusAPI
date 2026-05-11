// ChannelsPage — admin /channels 页 POM。
// 关注三组操作：创建表单、编辑表单、行操作（编辑/删除）。
import type { Page, Locator } from '@playwright/test';
import { expect } from '@playwright/test';

export interface ChannelFormState {
  name: string;
  provider?: string;
  baseURL?: string;
  credentials?: string;
  models?: string;
  weight?: string;
  status?: 'active' | 'disabled' | 'testing';
}

export class ChannelsPage {
  constructor(public readonly page: Page) {}

  async goto(): Promise<void> {
    await this.page.goto('/channels');
    await expect(this.page.getByRole('heading', { name: /渠道管理/ })).toBeVisible();
  }

  /** 打开新建面板。 */
  async openCreate(): Promise<void> {
    await this.page.getByRole('button', { name: /新建/ }).click();
  }

  /** 打开指定行的编辑面板。 */
  async openEdit(name: string): Promise<void> {
    await this.row(name).getByRole('button', { name: /编辑/ }).click();
  }

  /** 行定位器（用名字匹配第一列）。 */
  row(name: string): Locator {
    return this.page.locator('tr', { hasText: name });
  }

  /** 填表（任意字段，按提供的项填）。 */
  async fillForm(state: ChannelFormState): Promise<void> {
    if (state.name) await this.page.getByTestId('channel-name').fill(state.name);
    if (state.provider) await this.page.getByTestId('channel-provider').selectOption(state.provider);
    if (state.baseURL) await this.page.getByTestId('channel-base-url').fill(state.baseURL);
    if (state.credentials) await this.page.getByTestId('channel-credentials').fill(state.credentials);
    if (state.models) await this.page.getByTestId('channel-models').fill(state.models);
    if (state.weight) await this.page.getByTestId('channel-weight').fill(state.weight);
    if (state.status) await this.page.getByTestId('channel-status').selectOption(state.status);
  }

  /** 切换 user_ids 多选某个 email 的勾选状态。 */
  async toggleUserGrant(email: string): Promise<void> {
    await this.page
      .locator('label', { hasText: email })
      .locator('input[type="checkbox"]')
      .click();
  }

  /** 提交创建表单并等待 POST 响应。 */
  async submitCreate(): Promise<{ id: number; user_ids: number[] | null; status: string }> {
    const resp = this.page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/channels') && r.request().method() === 'POST',
    );
    await this.page.getByRole('button', { name: '创建' }).click();
    const r = await resp;
    if (!r.ok()) {
      throw new Error(`create channel: ${r.status()} ${await r.text()}`);
    }
    return r.json();
  }

  /** 提交编辑表单并等待 PUT 响应。 */
  async submitEdit(id: number): Promise<{ id: number; user_ids: number[] | null; status: string; weight: number; provider: string }> {
    const resp = this.page.waitForResponse(
      (r) => r.url().includes(`/api/admin/channels/${id}`) && r.request().method() === 'PUT',
    );
    await this.page.getByRole('button', { name: /保存/ }).click();
    const r = await resp;
    if (!r.ok()) {
      throw new Error(`update channel: ${r.status()} ${await r.text()}`);
    }
    return r.json();
  }

  /** 删除指定行（自动接受 confirm dialog）。 */
  async delete(name: string): Promise<void> {
    this.page.once('dialog', (d) => d.accept());
    const resp = this.page.waitForResponse(
      (r) => r.url().includes('/api/admin/channels/') && r.request().method() === 'DELETE',
    );
    await this.row(name).getByRole('button', { name: /删除/ }).click();
    const r = await resp;
    if (!r.ok()) {
      throw new Error(`delete channel: ${r.status()}`);
    }
  }
}
