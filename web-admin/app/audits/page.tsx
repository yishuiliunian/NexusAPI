'use client';

import { useEffect, useState } from 'react';
import { api, PageContent, PageHeader } from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

interface AuditLog {
  id: number;
  actor_id: number;
  action: string;
  target: string;
  ip: string;
  created_at: string;
}

export default function AuditsPage() {
  const [items, setItems] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  useEffect(() => {
    api.get<{ items: AuditLog[]; total: number }>('/api/admin/audits?size=200').then((r) => {
      setItems(r.items ?? []);
      setTotal(r.total ?? 0);
    });
  }, []);

  return (
    <AdminShell>
      <PageContent>
        <PageHeader dark title="审计日志" description={`共 ${total} 条 · 管理员操作追溯`} />

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">ID</th>
                <th className="px-5 py-3 text-left font-semibold">操作者</th>
                <th className="px-5 py-3 text-left font-semibold">动作</th>
                <th className="px-5 py-3 text-left font-semibold">目标</th>
                <th className="px-5 py-3 text-left font-semibold">IP</th>
                <th className="px-5 py-3 text-left font-semibold">时间</th>
              </tr>
            </thead>
            <tbody>
              {items.map((l) => (
                <tr key={l.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 text-slate-400">{l.id}</td>
                  <td className="px-5 py-3 text-slate-300">user#{l.actor_id}</td>
                  <td className="px-5 py-3 font-mono text-xs text-white">{l.action}</td>
                  <td className="px-5 py-3 font-mono text-xs text-slate-400">{l.target}</td>
                  <td className="px-5 py-3 font-mono text-xs text-slate-500">{l.ip}</td>
                  <td className="px-5 py-3 text-xs text-slate-500">
                    {new Date(l.created_at).toLocaleString('zh-CN')}
                  </td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-5 py-12 text-center text-sm text-slate-500">
                    暂无审计记录
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
