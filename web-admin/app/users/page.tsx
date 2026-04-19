'use client';

import { useEffect, useState } from 'react';
import {
  adminApi,
  AdminUser,
  Badge,
  Button,
  Input,
  PageContent,
  PageHeader,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

const YUAN = (micro: number) => (micro / 1_000_000).toFixed(4);

export default function AdminUsersPage() {
  const [items, setItems] = useState<AdminUser[]>([]);
  const [q, setQ] = useState('');

  async function load() {
    const r = await adminApi.listUsers();
    setItems(r.items);
  }
  useEffect(() => {
    load();
  }, []);

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
          description={`共 ${items.length} 个用户 · 调整余额 / RPM / 封禁`}
          actions={
            <Input
              placeholder="按邮箱搜索..."
              value={q}
              onChange={(e) => setQ(e.target.value)}
              className="w-56 bg-slate-800 text-white border-slate-700"
            />
          }
        />

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
