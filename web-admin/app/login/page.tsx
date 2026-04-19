'use client';

// 登录页：admin 专用
import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { authApi, ApiClientError, Button, Input } from '@nexusapi/shared';

export default function AdminLoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr('');
    setLoading(true);
    try {
      const me = await authApi.login(email, password);
      if (me.role !== 'admin') {
        setErr('仅管理员可访问');
        setLoading(false);
        return;
      }
      router.push('/overview');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'login failed');
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-slate-900 p-4">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm space-y-5 rounded-lg border border-slate-700 bg-slate-800 p-8 shadow-lifted"
      >
        <div className="flex flex-col items-center gap-2">
          <div className="grid h-12 w-12 place-items-center rounded-xl bg-brand-500 text-xl font-bold text-white">
            N
          </div>
          <div className="flex flex-col items-center gap-0.5">
            <h1 className="text-xl font-semibold text-white">NexusAPI</h1>
            <span className="text-[10px] font-medium uppercase tracking-wider text-slate-500">
              管理后台
            </span>
          </div>
        </div>

        <div className="space-y-1">
          <label htmlFor="admin-email" className="text-xs font-medium text-slate-300">
            邮箱
          </label>
          <Input
            id="admin-email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="bg-slate-900 text-white border-slate-700"
            autoComplete="email"
          />
        </div>

        <div className="space-y-1">
          <label htmlFor="admin-password" className="text-xs font-medium text-slate-300">
            密码
          </label>
          <Input
            id="admin-password"
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="bg-slate-900 text-white border-slate-700"
            autoComplete="current-password"
          />
        </div>

        {err && <p className="text-sm text-red-400">{err}</p>}

        <Button type="submit" disabled={loading} className="w-full">
          {loading ? '登录中…' : '登录'}
        </Button>
      </form>
    </main>
  );
}
