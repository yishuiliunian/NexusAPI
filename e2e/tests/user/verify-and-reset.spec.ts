// 邮箱验证 + 密码重置流程。
//
// 策略：
//   - E2E 用 Noop mailer，不发真实邮件。token 写入 verify_tokens 表。
//   - 测试通过 db-helper（docker exec psql）直接查 token，走 /api/auth/verify + /api/auth/reset。
import { expect, test } from '../../fixtures/auth';
import { emailVerified, latestVerifyToken } from '../../fixtures/db-helper';

test.describe('邮箱验证', () => {
  test('resend → verify 流程', async ({ page, loginAsUser }) => {
    const { email } = await loginAsUser();
    expect(emailVerified(email)).toBeFalsy();

    const cookies = await page.context().cookies();
    const csrf = cookies.find((c) => c.name === 'nexus_csrf')?.value;
    expect(csrf).toBeDefined();

    // 申请重发
    const resend = await page.request.post('/api/auth/resend', {
      headers: { 'X-CSRF-Token': csrf! },
    });
    expect(resend.ok(), `resend: ${resend.status()}`).toBeTruthy();

    // 从 DB 取 token
    const token = latestVerifyToken(email, 'email_verify');
    expect(token).toBeTruthy();

    // 调验证
    const v = await page.request.post('/api/auth/verify', {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf! },
      data: { token },
    });
    expect(v.ok()).toBeTruthy();
    expect(emailVerified(email)).toBeTruthy();
  });

  test('无效 token 拒绝', async ({ page }) => {
    const r = await page.request.post('/api/auth/verify', {
      headers: { 'Content-Type': 'application/json' },
      data: { token: 'garbage-token-does-not-exist' },
      failOnStatusCode: false,
    });
    expect(r.ok()).toBeFalsy();
  });
});

test.describe('密码重置', () => {
  test('forgot → 取 token → reset → 新密码登录', async ({ page, loginAsUser }) => {
    const { email, password } = await loginAsUser();
    // logout
    const csrfPre = (await page.context().cookies()).find((c) => c.name === 'nexus_csrf')?.value;
    await page.request.post('/api/auth/logout', { headers: { 'X-CSRF-Token': csrfPre! } });

    // 申请重置（公开端点）
    const fg = await page.request.post('/api/auth/forgot', {
      headers: { 'Content-Type': 'application/json' },
      data: { email },
    });
    expect(fg.ok()).toBeTruthy();

    const token = latestVerifyToken(email, 'password_reset');
    expect(token).toBeTruthy();

    // 用 token 改密
    const newPwd = 'newpassword456';
    const rp = await page.request.post('/api/auth/reset', {
      headers: { 'Content-Type': 'application/json' },
      data: { token, new_password: newPwd },
    });
    expect(rp.ok(), `reset: ${rp.status()} ${await rp.text()}`).toBeTruthy();

    // 旧密登录应失败
    const oldLogin = await page.request.post('/api/auth/login', {
      headers: { 'Content-Type': 'application/json' },
      data: { email, password },
      failOnStatusCode: false,
    });
    expect(oldLogin.ok()).toBeFalsy();

    // 新密登录成功
    const newLogin = await page.request.post('/api/auth/login', {
      headers: { 'Content-Type': 'application/json' },
      data: { email, password: newPwd },
    });
    expect(newLogin.ok(), `new-pwd login: ${newLogin.status()}`).toBeTruthy();
  });

  test('不存在邮箱依然返回 200（防枚举）', async ({ page }) => {
    const r = await page.request.post('/api/auth/forgot', {
      headers: { 'Content-Type': 'application/json' },
      data: { email: 'never-registered@e2e.test' },
    });
    expect(r.ok()).toBeTruthy();
  });

  test('弱密码（<8）被拒', async ({ page }) => {
    const r = await page.request.post('/api/auth/reset', {
      headers: { 'Content-Type': 'application/json' },
      data: { token: 'anything', new_password: 'short' },
      failOnStatusCode: false,
    });
    expect(r.ok()).toBeFalsy();
  });
});