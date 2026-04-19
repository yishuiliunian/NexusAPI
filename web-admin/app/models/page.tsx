'use client';

import { useEffect, useState, type ReactNode } from 'react';
import {
  adminApi,
  ApiClientError,
  Badge,
  Button,
  Input,
  ModelPrice,
  PageContent,
  PageHeader,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

const CAPABILITIES = ['chat', 'responses', 'embedding', 'rerank', 'image', 'tts', 'stt', 'task'];

export default function ModelsPage() {
  const [items, setItems] = useState<ModelPrice[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [form, setForm] = useState<Partial<ModelPrice>>({
    model: '',
    capability: 'chat',
    input_price: 0,
    output_price: 0,
    output_multiplier: 1,
    enabled: true,
  });

  async function load() {
    const r = await adminApi.listModels();
    setItems(r.items);
  }
  useEffect(() => {
    load();
  }, []);

  async function submit() {
    if (!form.model) {
      alert('请填写模型名');
      return;
    }
    await adminApi.upsertModel(form);
    setForm({
      model: '',
      capability: 'chat',
      input_price: 0,
      output_price: 0,
      output_multiplier: 1,
      enabled: true,
    });
    load();
  }

  async function del(id: number) {
    if (!confirm('删除价格？')) return;
    await adminApi.deleteModel(id);
    load();
  }

  async function syncFromLiteLLM() {
    if (!confirm('将从 LiteLLM 全量覆盖除 task 类外的所有模型价格。继续？')) return;
    setSyncing(true);
    try {
      const r = await adminApi.syncPricing();
      alert(`同步完成：新增 ${r.inserted}、删除 ${r.deleted}、跳过 ${r.skipped}`);
      load();
    } catch (e) {
      alert(e instanceof ApiClientError ? e.body.message : '同步失败');
    } finally {
      setSyncing(false);
    }
  }

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="模型价格"
          description="按 model + capability upsert · 价格单位：元 / 1M tokens"
          actions={
            <Button onClick={syncFromLiteLLM} disabled={syncing}>
              {syncing ? '同步中…' : '从 LiteLLM 同步'}
            </Button>
          }
        />

        <section className="rounded-lg border border-slate-700 bg-slate-800 p-5">
          <h2 className="text-sm font-semibold text-white">新增 / 更新</h2>
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            <Field label="model">
              <Input
                value={form.model ?? ''}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                placeholder="gpt-4o-mini"
                className="bg-slate-900 text-white border-slate-700 font-mono"
              />
            </Field>
            <Field label="capability">
              <select
                value={form.capability}
                onChange={(e) => setForm({ ...form, capability: e.target.value })}
                className="w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white"
              >
                {CAPABILITIES.map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </select>
            </Field>
            <Field label="input 价（元 / 1M tokens）">
              <Input
                type="number"
                step="0.0001"
                value={(form.input_price ?? 0) / 1_000_000}
                onChange={(e) =>
                  setForm({ ...form, input_price: Math.round(Number(e.target.value) * 1_000_000) })
                }
                className="bg-slate-900 text-white border-slate-700 font-mono"
              />
            </Field>
            <Field label="output 价（元 / 1M tokens）">
              <Input
                type="number"
                step="0.0001"
                value={(form.output_price ?? 0) / 1_000_000}
                onChange={(e) =>
                  setForm({ ...form, output_price: Math.round(Number(e.target.value) * 1_000_000) })
                }
                className="bg-slate-900 text-white border-slate-700 font-mono"
              />
            </Field>
            <Field label="cache 读价（元 / 1M tokens）">
              <Input
                type="number"
                step="0.0001"
                value={(form.cache_price ?? 0) / 1_000_000}
                onChange={(e) =>
                  setForm({ ...form, cache_price: Math.round(Number(e.target.value) * 1_000_000) })
                }
                className="bg-slate-900 text-white border-slate-700 font-mono"
              />
            </Field>
            <Field label="task 按次价（元 / 次）">
              <Input
                type="number"
                step="0.0001"
                value={(form.task_price ?? 0) / 1_000_000}
                onChange={(e) =>
                  setForm({ ...form, task_price: Math.round(Number(e.target.value) * 1_000_000) })
                }
                className="bg-slate-900 text-white border-slate-700 font-mono"
              />
            </Field>
          </div>
          <div className="mt-4 flex items-center justify-between">
            <label className="flex items-center gap-2 text-xs text-slate-300">
              <input
                type="checkbox"
                checked={!!form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              启用
            </label>
            <Button onClick={submit}>保存</Button>
          </div>
        </section>

        <section className="overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-900 text-xs uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3 text-left font-semibold">Model</th>
                <th className="px-5 py-3 text-left font-semibold">Capability</th>
                <th className="px-5 py-3 text-right font-semibold">Input</th>
                <th className="px-5 py-3 text-right font-semibold">Output</th>
                <th className="px-5 py-3 text-right font-semibold">Cache</th>
                <th className="px-5 py-3 text-right font-semibold">Task</th>
                <th className="px-5 py-3 text-left font-semibold">Enabled</th>
                <th className="px-5 py-3 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.id} className="border-t border-slate-700">
                  <td className="px-5 py-3 font-mono text-xs text-white">{p.model}</td>
                  <td className="px-5 py-3">
                    <Badge variant="neutral">{p.capability}</Badge>
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">
                    ¥{(p.input_price / 1_000_000).toFixed(4)}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-300">
                    ¥{(p.output_price / 1_000_000).toFixed(4)}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-400">
                    ¥{(p.cache_price / 1_000_000).toFixed(4)}
                  </td>
                  <td className="px-5 py-3 text-right font-mono text-slate-400">
                    ¥{(p.task_price / 1_000_000).toFixed(4)}
                  </td>
                  <td className="px-5 py-3">
                    <Badge variant={p.enabled ? 'success' : 'neutral'}>
                      {p.enabled ? 'on' : 'off'}
                    </Badge>
                  </td>
                  <td className="px-5 py-3 text-right">
                    <Button variant="ghost" size="sm" onClick={() => del(p.id)}>
                      删除
                    </Button>
                  </td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-5 py-12 text-center text-sm text-slate-500">
                    暂无价格配置
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

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <label className="text-xs font-medium text-slate-300">{label}</label>
      <div className="mt-1">{children}</div>
    </div>
  );
}
