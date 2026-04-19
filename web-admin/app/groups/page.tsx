'use client';

import { useEffect, useState } from 'react';
import { api, Button, Input, PageContent, PageHeader } from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

interface Group {
  id: number;
  name: string;
  price_multiplier: number;
}

export default function GroupsPage() {
  const [items, setItems] = useState<Group[]>([]);
  const [name, setName] = useState('');
  const [mult, setMult] = useState(1);

  async function load() {
    const r = await api.get<{ items: Group[] }>('/api/admin/groups');
    setItems(r.items ?? []);
  }
  useEffect(() => {
    load();
  }, []);

  async function create() {
    if (!name.trim()) return;
    await api.post('/api/admin/groups', { name: name.trim(), price_multiplier: mult });
    setName('');
    setMult(1);
    load();
  }

  async function del(id: number) {
    if (!confirm('删除？该组下用户的 group_id 会被置 0。')) return;
    await api.del(`/api/admin/groups/${id}`);
    load();
  }

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="用户分组"
          description="按分组设置价格倍率（例如 VIP 享受 0.8 倍价）"
        />

        <section className="rounded-lg border border-slate-700 bg-slate-800 p-5">
          <h2 className="text-sm font-semibold text-white">新建分组</h2>
          <div className="mt-3 flex flex-wrap gap-2">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="组名（例：VIP / 内部）"
              className="w-60 bg-slate-900 text-white border-slate-700"
            />
            <Input
              type="number"
              step="0.1"
              value={mult}
              onChange={(e) => setMult(Number(e.target.value))}
              placeholder="倍率"
              className="w-32 bg-slate-900 text-white border-slate-700 font-mono"
            />
            <Button onClick={create}>新建</Button>
          </div>
        </section>

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">ID</th>
                <th className="px-5 py-3 text-left font-semibold">名称</th>
                <th className="px-5 py-3 text-right font-semibold">价格倍率</th>
                <th className="px-5 py-3 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((g) => (
                <tr key={g.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 text-slate-400">{g.id}</td>
                  <td className="px-5 py-3 text-white">{g.name}</td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">{g.price_multiplier}</td>
                  <td className="px-5 py-3 text-right">
                    <Button variant="ghost" size="sm" onClick={() => del(g.id)}>
                      删除
                    </Button>
                  </td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-5 py-12 text-center text-sm text-slate-500">
                    暂无分组
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
