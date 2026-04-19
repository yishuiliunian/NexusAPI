'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { authApi, ApiClientError, Button, Input } from '@nexusapi/shared';

export default function LoginPage() {
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
      await authApi.login(email, password);
      router.push('/dashboard');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'login failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-gradient-to-br from-brand-50 to-white p-4">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm space-y-5 rounded-lg border border-slate-200 bg-white p-8 shadow-card"
      >
        <div className="flex flex-col items-center gap-2">
          <div className="grid h-12 w-12 place-items-center rounded-xl bg-brand-600 text-xl font-bold text-white">
            N
          </div>
          <h1 className="text-xl font-semibold text-slate-900">登录 NexusAPI</h1>
        </div>

        <div className="space-y-1">
          <label htmlFor="login-email" className="text-xs font-medium text-slate-700">
            邮箱
          </label>
          <Input
            id="login-email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
          />
        </div>

        <div className="space-y-1">
          <label htmlFor="login-password" className="text-xs font-medium text-slate-700">
            密码
          </label>
          <Input
            id="login-password"
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </div>

        {err && <p className="text-sm text-danger">{err}</p>}

        <Button type="submit" disabled={loading} className="w-full">
          {loading ? '登录中…' : '登录'}
        </Button>

        <p className="text-center text-sm text-slate-500">
          还没有账号？<Link href="/register" className="text-brand-600 underline">注册</Link>
        </p>
      </form>
    </main>
  );
}
