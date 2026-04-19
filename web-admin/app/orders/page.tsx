'use client';

import { useEffect, useState } from 'react';
import { api, Badge, PageContent, PageHeader } from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

interface Order {
  id: string;
  user_id: number;
  amount_cents: number;
  currency: string;
  gateway: string;
  status: string;
  created_at: string;
  paid_at: string | null;
}

export default function OrdersPage() {
  const [items, setItems] = useState<Order[]>([]);
  useEffect(() => {
    api
      .get<{ items: Order[] }>('/api/admin/orders')
      .then((r) => setItems(r.items ?? []))
      .catch(() => setItems([]));
  }, []);

  const total = items.filter((o) => o.status === 'paid').reduce((s, o) => s + o.amount_cents, 0);

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="订单"
          description={`${items.length} 个订单 · 已收 ¥${(total / 100).toFixed(2)}`}
        />

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">订单 ID</th>
                <th className="px-5 py-3 text-left font-semibold">用户</th>
                <th className="px-5 py-3 text-right font-semibold">金额</th>
                <th className="px-5 py-3 text-left font-semibold">网关</th>
                <th className="px-5 py-3 text-left font-semibold">状态</th>
                <th className="px-5 py-3 text-left font-semibold">创建</th>
                <th className="px-5 py-3 text-left font-semibold">付款</th>
              </tr>
            </thead>
            <tbody>
              {items.map((o) => (
                <tr key={o.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 font-mono text-xs text-white">{o.id.slice(0, 8)}…</td>
                  <td className="px-5 py-3 text-slate-300">user#{o.user_id}</td>
                  <td className="px-5 py-3 text-right font-mono text-slate-200">
                    {(o.amount_cents / 100).toFixed(2)} {o.currency}
                  </td>
                  <td className="px-5 py-3 text-slate-400">{o.gateway}</td>
                  <td className="px-5 py-3">
                    <Badge variant={o.status === 'paid' ? 'success' : 'neutral'} dot>
                      {o.status}
                    </Badge>
                  </td>
                  <td className="px-5 py-3 text-xs text-slate-500">
                    {new Date(o.created_at).toLocaleString('zh-CN')}
                  </td>
                  <td className="px-5 py-3 text-xs text-slate-500">
                    {o.paid_at ? new Date(o.paid_at).toLocaleString('zh-CN') : '—'}
                  </td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-5 py-12 text-center text-sm text-slate-500">
                    暂无订单
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
