'use client';

// Settings · 账户设置
//   - 账户信息
//   - 邮箱验证 / 密码重置（邮件）
//   - 2FA 开启 / 关闭

import { useEffect, useState, type ReactNode } from 'react';
import {
  ApiClientError,
  api,
  Badge,
  Button,
  Input,
  Me,
  PageContent,
  PageHeader,
  Section,
  userApi,
} from '@nexusapi/shared';
import { UserShell } from '../../components/user-shell';

const YUAN = (micro: number) => (micro / 1_000_000).toFixed(4);

export default function SettingsPage() {
  const [me, setMe] = useState<Me | null>(null);
  const [msg, setMsg] = useState<string>('');
  const [err, setErr] = useState<string>('');
  const [alertAt, setAlertAt] = useState(0);
  const [setup, setSetup] = useState<{ secret: string; url: string } | null>(null);
  const [otp, setOtp] = useState('');

  async function load() {
    const m = await userApi.me();
    setMe(m);
    setAlertAt(m.quota_alert_at ?? 0);
  }
  useEffect(() => {
    load();
  }, []);

  async function resend() {
    try {
      await api.post('/api/auth/resend');
      setMsg('验证邮件已发送（若 SMTP 已配置）');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'failed');
    }
  }

  async function forgot() {
    if (!me) return;
    try {
      await api.post('/api/auth/forgot', { email: me.email });
      setMsg('密码重置邮件已发送，请查收邮箱完成重置');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'failed');
    }
  }

  async function saveAlert() {
    try {
      await api.put('/api/user/quota-alert', { quota: alertAt });
      setMsg('余额告警阈值已保存');
      load();
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'failed');
    }
  }

  async function twoFASetup() {
    try {
      const r = await api.post<{ secret: string; url: string }>('/api/auth/2fa/setup');
      setSetup(r);
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'failed');
    }
  }

  async function twoFAEnable() {
    try {
      await api.post('/api/auth/2fa/enable', { code: otp });
      setMsg('2FA 已启用');
      setSetup(null);
      setOtp('');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'code invalid');
    }
  }

  async function twoFADisable() {
    if (!confirm('关闭 2FA 后账户安全性降低，确认继续？')) return;
    try {
      await api.post('/api/auth/2fa/disable');
      setMsg('2FA 已关闭');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'failed');
    }
  }

  async function logout() {
    await api.post('/api/auth/logout').catch(() => null);
    window.location.href = '/login';
  }

  if (!me) return <UserShell><div className="p-8 text-sm text-slate-500">加载中…</div></UserShell>;

  return (
    <UserShell>
      <PageContent>
        <PageHeader title="账户设置" description="账户资料 / 邮箱验证 / 密码 / 2FA" />

        {msg && (
          <div className="rounded-md border border-success bg-success-bg px-4 py-3 text-sm text-emerald-800">
            {msg}
          </div>
        )}
        {err && (
          <div className="rounded-md border border-danger bg-danger-bg px-4 py-3 text-sm text-danger">
            {err}
          </div>
        )}

        <Section title="账户信息">
          <div className="grid gap-4 p-5 md:grid-cols-2">
            <Field label="邮箱">
              <div className="flex items-center gap-2">
                <span className="text-sm text-slate-800">{me.email}</span>
                {me.email_verified ? (
                  <Badge variant="success" dot>已验证</Badge>
                ) : (
                  <>
                    <Badge variant="warning">未验证</Badge>
                    <Button variant="ghost" size="sm" onClick={resend}>
                      重发验证邮件
                    </Button>
                  </>
                )}
              </div>
            </Field>
            <Field label="角色">
              <Badge variant={me.role === 'admin' ? 'brand' : 'neutral'}>{me.role}</Badge>
            </Field>
            <Field label="当前余额">
              <span className="font-mono text-sm text-slate-800">¥{YUAN(me.quota)}</span>
            </Field>
            <Field label="累计消耗">
              <span className="font-mono text-sm text-slate-800">¥{YUAN(me.used_quota)}</span>
            </Field>
          </div>
        </Section>

        <Section title="安全" description="密码和两步验证">
          <div className="flex flex-col gap-4 p-5">
            <div className="flex items-center justify-between gap-3 rounded-md border border-slate-200 p-4">
              <div>
                <div className="text-sm font-medium text-slate-800">密码</div>
                <div className="text-xs text-slate-500">通过邮件链接重置当前密码</div>
              </div>
              <Button variant="secondary" onClick={forgot}>
                发送重置邮件
              </Button>
            </div>

            <div className="rounded-md border border-slate-200 p-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-medium text-slate-800">两步验证 (2FA)</div>
                  <div className="text-xs text-slate-500">
                    使用 Google Authenticator / 1Password 等 TOTP 应用
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button variant="secondary" onClick={twoFASetup}>
                    开启 2FA
                  </Button>
                  <Button variant="ghost" onClick={twoFADisable}>
                    关闭
                  </Button>
                </div>
              </div>

              {setup && (
                <div className="mt-4 rounded-md border border-slate-200 bg-slate-50 p-4">
                  <div className="text-xs font-medium text-slate-700">1. 扫描 / 手工添加到认证器</div>
                  <code className="mt-2 block break-all rounded bg-white p-2 font-mono text-xs text-slate-900">
                    {setup.url}
                  </code>
                  <div className="mt-3 text-xs font-medium text-slate-700">2. 输入认证器生成的 6 位码</div>
                  <div className="mt-2 flex gap-2">
                    <Input
                      value={otp}
                      onChange={(e) => setOtp(e.target.value)}
                      placeholder="123456"
                      maxLength={6}
                      className="w-32 font-mono tracking-widest"
                    />
                    <Button onClick={twoFAEnable} disabled={otp.length !== 6}>
                      启用
                    </Button>
                    <Button variant="ghost" onClick={() => setSetup(null)}>
                      取消
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </Section>

        <Section title="通知" description="余额低于设定值时触发告警邮件">
          <div className="flex items-center gap-3 p-5">
            <Input
              type="number"
              step="0.0001"
              min="0"
              value={(alertAt / 1_000_000).toString()}
              onChange={(e) => setAlertAt(Math.round(Number(e.target.value) * 1_000_000))}
              className="w-48 font-mono"
              placeholder="0 = 关闭"
            />
            <span className="text-xs text-slate-500">
              元 · 当前：{alertAt > 0 ? `¥${YUAN(alertAt)}` : '关闭'}
            </span>
            <div className="flex-1" />
            <Button variant="secondary" onClick={saveAlert}>
              保存
            </Button>
          </div>
        </Section>

        <Section title="危险区">
          <div className="flex items-center justify-between p-5">
            <div>
              <div className="text-sm font-medium text-slate-800">退出登录</div>
              <div className="text-xs text-slate-500">当前 session 会被立即销毁</div>
            </div>
            <Button variant="danger" onClick={logout}>
              退出
            </Button>
          </div>
        </Section>
      </PageContent>
    </UserShell>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className="mt-1">{children}</div>
    </div>
  );
}
