'use client';

import { useEffect, useState, type FormEvent } from 'react';
import {
  adminApi,
  AdminUser,
  ApiClientError,
  Badge,
  Button,
  Input,
  PageContent,
  PageHeader,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

const YUAN = (micro: number) => (micro / 1_000_000).toFixed(4);

// 生成 16 字符强密码（URL-safe base64 截断）
function randomPassword(len = 16) {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  const b64 = btoa(String.fromCharCode(...bytes));
  return b64.replace(/[+/=]/g, '').slice(0, len);
}

export default function AdminUsersPage() {
  const [items, setItems] = useState<AdminUser[]>([]);
  const [q, setQ] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState({
    email: '',
    password: randomPassword(),
    role: 'user' as 'user' | 'admin',
    quotaYuan: 0,
  });
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');

  async function load() {
    const r = await adminApi.listUsers();
    setItems(r.items);
  }
  useEffect(() => {
    load();
  }, []);

  function openCreate() {
    setCreateForm({ email: '', password: randomPassword(), role: 'user', quotaYuan: 0 });
    setCreateError('');
    setShowCreate(true);
  }

  async function submitCreate(e: FormEvent) {
    e.preventDefault();
    setCreating(true);
    setCreateError('');
    try {
      await adminApi.createUser({
        email: createForm.email.trim(),
        password: createForm.password,
        role: createForm.role,
        quota: Math.round(createForm.quotaYuan * 1_000_000),
      });
      alert(
        `创建成功\n邮箱: ${createForm.email}\n密码: ${createForm.password}\n\n请复制保存！关闭后无法再查看。`,
      );
      setShowCreate(false);
      load();
    } catch (err) {
      setCreateError(err instanceof ApiClientError ? err.body.message : '创建失败');
    } finally {
      setCreating(false);
    }
  }

  async function setQuota(u: AdminUser) {
    const v = prompt(`${u.email} 当前余额 ¥${YUAN(u.quota)}，输入新余额（micro）`);
    if (!v) return;
    await adminApi.updateUserQuota(u.id, Number(v));
    load();
  }

  async function setRpm(u: AdminUser) {
    const current = u.rpm_limit ?? 0;
    const v = prompt(
      `${u.email} 当前 RPM 上限: ${current === 0 ? '不限' : current}\n输入新 RPM（0 表示不限）`,
      String(current),
    );
    if (v === null) return;
    const n = Number(v);
    if (!Number.isFinite(n) || n < 0) {
      alert('RPM 必须是 >= 0 的整数');
      return;
    }
    await adminApi.updateUserRpm(u.id, Math.floor(n));
    load();
  }

  async function toggleStatus(u: AdminUser) {
    const next = u.status === 'active' ? 'banned' : 'active';
    await adminApi.updateUserStatus(u.id, next);
    load();
  }

  const filtered = q
    ? items.filter((u) => u.email.toLowerCase().includes(q.toLowerCase()))
    : items;

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="用户管理"
          description={`共 ${items.length} 个用户 · 创建 / 调整余额 / RPM / 封禁`}
          actions={
            <div className="flex items-center gap-2">
              <Input
                placeholder="按邮箱搜索..."
                value={q}
                onChange={(e) => setQ(e.target.value)}
                className="w-56 bg-slate-800 text-white border-slate-700"
              />
              <Button onClick={openCreate} data-testid="btn-new-user">
                + 新建用户
              </Button>
            </div>
          }
        />

        {showCreate && (
          <section className="rounded-lg border border-slate-700 bg-slate-800 p-5">
            <h2 className="text-sm font-semibold text-white">新建用户</h2>
            <form onSubmit={submitCreate} className="mt-4 grid gap-3 md:grid-cols-2">
              <div>
                <label className="text-xs font-medium text-slate-300">邮箱</label>
                <Input
                  data-testid="new-user-email"
                  type="email"
                  required
                  value={createForm.email}
                  onChange={(e) => setCreateForm({ ...createForm, email: e.target.value })}
                  placeholder="user@example.com"
                  className="mt-1 bg-slate-900 text-white border-slate-700"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-slate-300">初始密码（至少 8 位）</label>
                <div className="mt-1 flex gap-2">
                  <Input
                    data-testid="new-user-password"
                    type="text"
                    required
                    minLength={8}
                    value={createForm.password}
                    onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
                    className="flex-1 bg-slate-900 text-white border-slate-700 font-mono"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setCreateForm({ ...createForm, password: randomPassword() })}
                  >
                    ↻ 重生成
                  </Button>
                </div>
              </div>
              <div>
                <label className="text-xs font-medium text-slate-300">角色</label>
                <select
                  data-testid="new-user-role"
                  value={createForm.role}
                  onChange={(e) =>
                    setCreateForm({ ...createForm, role: e.target.value as 'user' | 'admin' })
                  }
                  className="mt-1 w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white"
                >
                  <option value="user">user</option>
                  <option value="admin">admin</option>
                </select>
              </div>
              <div>
                <label className="text-xs font-medium text-slate-300">初始余额（元）</label>
                <Input
                  data-testid="new-user-quota"
                  type="number"
                  step="0.01"
                  min="0"
                  value={createForm.quotaYuan}
                  onChange={(e) =>
                    setCreateForm({ ...createForm, quotaYuan: Number(e.target.value) })
                  }
                  className="mt-1 bg-slate-900 text-white border-slate-700 font-mono"
                />
              </div>
              {createError && (
                <div className="md:col-span-2 text-sm text-red-400">{createError}</div>
              )}
              <div className="md:col-span-2 flex justify-end gap-2">
                <Button type="button" variant="ghost" onClick={() => setShowCreate(false)}>
                  取消
                </Button>
                <Button type="submit" disabled={creating}>
                  {creating ? '创建中…' : '创建'}
                </Button>
              </div>
            </form>
          </section>
        )}

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">ID</th>
                <th className="px-5 py-3 text-left font-semibold">Email</th>
                <th className="px-5 py-3 text-left font-semibold">角色</th>
                <th className="px-5 py-3 text-right font-semibold">余额</th>
                <th className="px-5 py-3 text-right font-semibold">已消耗</th>
                <th className="px-5 py-3 text-right font-semibold">RPM 上限</th>
                <th className="px-5 py-3 text-left font-semibold">状态</th>
                <th className="px-5 py-3 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((u) => (
                <tr key={u.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 text-slate-400">{u.id}</td>
                  <td className="px-5 py-3 text-white">{u.email}</td>
                  <td className="px-5 py-3">
                    <Badge variant={u.role === 'admin' ? 'brand' : 'neutral'}>{u.role}</Badge>
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">
                    ¥{YUAN(u.quota)}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-400">
                    ¥{YUAN(u.used_quota)}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">
                    {u.rpm_limit && u.rpm_limit > 0 ? u.rpm_limit : <span className="text-slate-500">不限</span>}
                  </td>
                  <td className="px-5 py-3">
                    <Badge variant={u.status === 'active' ? 'success' : 'danger'} dot>
                      {u.status}
                    </Badge>
                  </td>
                  <td className="px-5 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => setQuota(u)}>
                        改余额
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => setRpm(u)}>
                        改 RPM
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => toggleStatus(u)}>
                        {u.status === 'active' ? '封禁' : '解封'}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-5 py-12 text-center text-sm text-slate-500">
                    无匹配
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </section>
      </PageContent>
    </AdminShell>
  );
}
