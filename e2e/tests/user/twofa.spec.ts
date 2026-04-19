// 2FA 完整流程：setup → enable → disable。
//
// 后端当前只做"开启/关闭"标记，登录时未挂 2FA 校验；因此测试只断言：
//   - setup 返回合法 base32 secret + otpauth URL
//   - enable 接收正确 TOTP 码通过，错误码拒绝
//   - disable 清空 secret
import { expect, test } from '../../fixtures/auth';
import { totp } from '../../fixtures/totp';

test.describe('2FA TOTP 流程', () => {
  test.beforeEach(async ({ loginAsUser }) => {
    await loginAsUser();
  });

  test('setup → 错码拒绝 → 对码通过 → disable', async ({ page }) => {
    const cookies = await page.context().cookies();
    const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;
    expect(csrf).toBeDefined();
    const headers = { 'X-CSRF-Token': csrf! };

    // setup
    const setup = await page.request.post('/api/auth/2fa/setup', { headers });
    expect(setup.ok(), `setup: ${setup.status()}`).toBeTruthy();
    const { secret, url } = (await setup.json()) as { secret: string; url: string };
    expect(secret).toMatch(/^[A-Z2-7]{10,}$/);
    expect(url).toMatch(/^otpauth:\/\/totp\//);

    // 错误码拒绝
    const bad = await page.request.post('/api/auth/2fa/enable', {
      headers: { 'Content-Type': 'application/json', ...headers },
      data: { code: '000000' },
      failOnStatusCode: false,
    });
    expect(bad.status()).toBe(401);

    // 正确码通过
    const code = totp(secret);
    const good = await page.request.post('/api/auth/2fa/enable', {
      headers: { 'Content-Type': 'application/json', ...headers },
      data: { code },
    });
    expect(good.ok(), `enable: ${good.status()} ${await good.text()}`).toBeTruthy();

    // disable
    const off = await page.request.post('/api/auth/2fa/disable', { headers });
    expect(off.ok()).toBeTruthy();

    // disable 后再 enable 应当需要重新 setup（TwoFASecret 已清空）
    const retry = await page.request.post('/api/auth/2fa/enable', {
      headers: { 'Content-Type': 'application/json', ...headers },
      data: { code: totp(secret) },
      failOnStatusCode: false,
    });
    expect(retry.ok()).toBeFalsy();
  });
});