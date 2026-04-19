'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { authApi, ApiClientError, Button, Input } from '@nexusapi/shared';

export default function RegisterPage() {
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
      await authApi.register(email, password);
      await authApi.login(email, password);
      router.push('/dashboard');
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'register failed');
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
          <h1 className="text-xl font-semibold text-slate-900">注册 NexusAPI</h1>
        </div>

        <div className="space-y-1">
          <label htmlFor="reg-email" className="text-xs font-medium text-slate-700">
            邮箱
          </label>
          <Input
            id="reg-email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
          />
        </div>

        <div className="space-y-1">
          <label htmlFor="reg-password" className="text-xs font-medium text-slate-700">
            密码（至少 8 位）
          </label>
          <Input
            id="reg-password"
            type="password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
          />
        </div>

        {err && <p className="text-sm text-danger">{err}</p>}

        <Button type="submit" disabled={loading} className="w-full">
          {loading ? '注册中…' : '注册'}
        </Button>

        <p className="text-center text-sm text-slate-500">
          已有账号？<Link href="/login" className="text-brand-600 underline">登录</Link>
        </p>
      </form>
    </main>
  );
}
