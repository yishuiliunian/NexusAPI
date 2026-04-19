'use client';

// 渠道管理：
//   - 顶部：+ 新建渠道
//   - 列表：编辑 / 删除 两个行操作
//   - 编辑面板：支持创建和编辑两种模式，模型字段旁提供「从上游同步」（仅编辑模式）
//
// KISS：一个 editor 面板覆盖 create/edit；"同步模型"调用后端后回写到 form.models，
// 用户可编辑后「保存」再持久化（保留 admin 手动微调能力）。

import { useEffect, useState, type ReactNode } from 'react';
import {
  adminApi,
  ApiClientError,
  Badge,
  Button,
  Channel,
  Input,
  PageContent,
  PageHeader,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

type Mode = 'closed' | 'create' | { kind: 'edit'; id: number };

interface FormState {
  name: string;
  provider: string;
  base_url: string;
  credentials: string;
  models: string;
  weight: number;
  price_multiplier: number;
  status: string;
  note: string;
}

const EMPTY_FORM: FormState = {
  name: '',
  provider: 'openai',
  base_url: '',
  credentials: '',
  models: '',
  weight: 100,
  price_multiplier: 1.0,
  status: 'active',
  note: '',
};

export default function ChannelsPage() {
  const [items, setItems] = useState<Channel[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [mode, setMode] = useState<Mode>('closed');
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [syncing, setSyncing] = useState(false);
  const [saving, setSaving] = useState(false);

  async function load() {
    const r = await adminApi.listChannels();
    setItems(r.items);
    const p = await adminApi.providers();
    setProviders(p.providers);
  }

  useEffect(() => {
    load();
  }, []);

  function openCreate() {
    setForm(EMPTY_FORM);
    setMode('create');
  }

  function openEdit(c: Channel) {
    setForm({
      name: c.name,
      provider: c.provider,
      base_url: c.base_url ?? '',
      credentials: '', // 不回显凭证；留空表示保持不变
      models: (c.models ?? []).join(', '),
      weight: c.weight,
      price_multiplier: c.price_multiplier,
      status: c.status ?? 'active',
      note: c.note ?? '',
    });
    setMode({ kind: 'edit', id: c.id });
  }

  function close() {
    setMode('closed');
    setForm(EMPTY_FORM);
  }

  async function submit() {
    setSaving(true);
    try {
      const payload = {
        ...form,
        models: form.models
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
      };
      if (mode === 'create') {
        await adminApi.createChannel(payload);
      } else if (typeof mode === 'object') {
        await adminApi.updateChannel(mode.id, payload);
      }
      await load();
      close();
    } catch (e) {
      alert(e instanceof ApiClientError ? e.body.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }

  async function del(id: number) {
    if (!confirm('删除该渠道？')) return;
    await adminApi.deleteChannel(id);
    load();
  }

  async function syncModels() {
    if (typeof mode !== 'object') return;
    setSyncing(true);
    try {
      const r = await adminApi.syncChannelModels(mode.id);
      setForm((f) => ({ ...f, models: r.models.join(', ') }));
      alert(`已拉到 ${r.count} 个模型，请检查并保存`);
      // 后端已经持久化 models，这里回写到 form 方便 admin 审阅/微调再覆盖保存。
      await load();
    } catch (e) {
      alert(e instanceof ApiClientError ? e.body.message : '同步失败');
    } finally {
      setSyncing(false);
    }
  }

  const editorOpen = mode !== 'closed';
  const isEdit = typeof mode === 'object';
  const title = isEdit ? `编辑渠道 · ${form.name}` : '新建渠道';

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="渠道管理"
          description={`${items.length} 个上游渠道`}
          actions={
            editorOpen ? (
              <Button variant="ghost" onClick={close}>
                取消
              </Button>
            ) : (
              <Button onClick={openCreate}>+ 新建渠道</Button>
            )
          }
        />

        {editorOpen && (
          <section className="rounded-lg border border-slate-700 bg-slate-800 p-5">
            <h2 className="text-sm font-semibold text-white">{title}</h2>
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              <DarkLabel label="名称">
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  className="bg-slate-900 text-white border-slate-700"
                />
              </DarkLabel>
              <DarkLabel label="Provider">
                <select
                  data-testid="channel-provider"
                  className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white"
                  value={form.provider}
                  onChange={(e) => setForm({ ...form, provider: e.target.value })}
                >
                  {providers.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </DarkLabel>
              <DarkLabel label="Base URL（留空用默认）">
                <Input
                  value={form.base_url}
                  onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                  placeholder="https://api.openai.com/v1"
                  className="bg-slate-900 text-white border-slate-700 font-mono"
                />
              </DarkLabel>
              <DarkLabel label={isEdit ? '凭证（留空表示不变）' : '凭证（API Key）'}>
                <Input
                  value={form.credentials}
                  onChange={(e) => setForm({ ...form, credentials: e.target.value })}
                  placeholder="sk-..."
                  className="bg-slate-900 text-white border-slate-700 font-mono"
                />
              </DarkLabel>
              <div className="md:col-span-2">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-medium text-slate-300">模型（逗号分隔）</label>
                  {isEdit && (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={syncing}
                      onClick={syncModels}
                    >
                      {syncing ? '同步中…' : '↻ 从上游同步'}
                    </Button>
                  )}
                </div>
                <div className="mt-1">
                  <Input
                    value={form.models}
                    onChange={(e) => setForm({ ...form, models: e.target.value })}
                    placeholder="gpt-4o,gpt-4o-mini,claude-3-5-sonnet"
                    className="bg-slate-900 text-white border-slate-700 font-mono"
                  />
                </div>
              </div>
              <DarkLabel label="权重">
                <Input
                  type="number"
                  value={form.weight}
                  onChange={(e) => setForm({ ...form, weight: Number(e.target.value) })}
                  className="bg-slate-900 text-white border-slate-700 font-mono"
                />
              </DarkLabel>
              <DarkLabel label="价格倍率">
                <Input
                  type="number"
                  step="0.01"
                  value={form.price_multiplier}
                  onChange={(e) => setForm({ ...form, price_multiplier: Number(e.target.value) })}
                  className="bg-slate-900 text-white border-slate-700 font-mono"
                />
              </DarkLabel>
              <DarkLabel label="状态">
                <select
                  className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white"
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value })}
                >
                  <option value="active">active</option>
                  <option value="disabled">disabled</option>
                  <option value="testing">testing</option>
                </select>
              </DarkLabel>
              <DarkLabel label="备注">
                <Input
                  value={form.note}
                  onChange={(e) => setForm({ ...form, note: e.target.value })}
                  className="bg-slate-900 text-white border-slate-700"
                />
              </DarkLabel>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={close}>
                取消
              </Button>
              <Button onClick={submit} disabled={saving}>
                {saving ? '保存中…' : isEdit ? '保存' : '创建'}
              </Button>
            </div>
          </section>
        )}

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">ID</th>
                <th className="px-5 py-3 text-left font-semibold">名称</th>
                <th className="px-5 py-3 text-left font-semibold">Provider</th>
                <th className="px-5 py-3 text-left font-semibold">模型</th>
                <th className="px-5 py-3 text-right font-semibold">权重</th>
                <th className="px-5 py-3 text-right font-semibold">倍率</th>
                <th className="px-5 py-3 text-left font-semibold">状态</th>
                <th className="px-5 py-3 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((c) => (
                <tr key={c.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 text-slate-400">{c.id}</td>
                  <td className="px-5 py-3 font-medium text-white">{c.name}</td>
                  <td className="px-5 py-3">
                    <Badge variant="neutral">{c.provider}</Badge>
                  </td>
                  <td className="px-5 py-3 max-w-sm truncate font-mono text-xs text-slate-400">
                    {(c.models ?? []).join(', ')}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">{c.weight}</td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">{c.price_multiplier}</td>
                  <td className="px-5 py-3">
                    <Badge variant={c.status === 'active' ? 'success' : 'danger'} dot>
                      {c.status}
                    </Badge>
                  </td>
                  <td className="px-5 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(c)}>
                        编辑
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => del(c.id)}>
                        删除
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-5 py-12 text-center text-sm text-slate-500">
                    还没有渠道
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

function DarkLabel({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <label className="text-xs font-medium text-slate-300">{label}</label>
      <div className="mt-1">{children}</div>
    </div>
  );
}
