'use client';

// Redemption · 激活码批量生成
//   - 左：新建批次表单（名称 / 数量 / 前缀 / 面额 / 有效期）
//   - 右：本月统计 + 最近批次
//   - 底：当前批次激活码表格

import { useEffect, useState, type ReactNode } from 'react';
import {
  api,
  ApiClientError,
  Badge,
  Button,
  Input,
  PageContent,
  PageHeader,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

const PRESETS = [1_000_000, 5_000_000, 10_000_000, 50_000_000]; // 1/5/10/50 USD

interface Batch {
  id: number;
  name: string;
  prefix: string;
  amount: number;
  count: number;
  redeemed: number;
  expires_at: string | null;
  created_at: string;
}

interface RedemptionRow {
  id: number;
  code: string;
  amount: number;
  expires_at: string | null;
  used_by: number | null;
  used_at: string | null;
}

export default function RedemptionPage() {
  const [name, setName] = useState('');
  const [prefix, setPrefix] = useState('NEXUS-');
  const [count, setCount] = useState(100);
  const [amount, setAmount] = useState<number>(5_000_000); // 5 USD
  const [expires, setExpires] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [msg, setMsg] = useState('');
  const [err, setErr] = useState('');

  const [batches, setBatches] = useState<Batch[]>([]);
  const [currentBatch, setCurrentBatch] = useState<Batch | null>(null);
  const [codes, setCodes] = useState<RedemptionRow[]>([]);

  async function loadBatches() {
    try {
      const r = await api.get<{ items: Batch[] }>('/api/admin/redemption-batches');
      setBatches(r.items ?? []);
    } catch {}
  }

  async function loadCodes(batchId: number) {
    try {
      const r = await api.get<{ items: RedemptionRow[] }>(
        `/api/admin/redemptions?batch_id=${batchId}`
      );
      setCodes(r.items ?? []);
    } catch {
      setCodes([]);
    }
  }

  useEffect(() => {
    loadBatches();
  }, []);

  async function submit() {
    if (!name.trim() || count <= 0 || amount <= 0) {
      setErr('请填写批次名称、数量、面额');
      return;
    }
    setSubmitting(true);
    setErr('');
    setMsg('');
    try {
      const r = await api.post<{ id: number; name: string }>(
        '/api/admin/redemption-batches',
        {
          name: name.trim(),
          prefix: prefix.trim(),
          count,
          amount,
          expires_at: expires || null,
        }
      );
      setMsg(`已生成 ${count} 张激活码（批次：${r.name}）`);
      setName('');
      loadBatches();
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : '生成失败');
    } finally {
      setSubmitting(false);
    }
  }

  function pickBatch(b: Batch) {
    setCurrentBatch(b);
    loadCodes(b.id);
  }

  async function exportCsv() {
    if (!currentBatch) return;
    const lines = ['code,amount_yuan,expires_at,used_by,used_at'];
    codes.forEach((c) => {
      lines.push(
        [
          c.code,
          (c.amount / 1_000_000).toFixed(4),
          c.expires_at ?? '',
          c.used_by ?? '',
          c.used_at ?? '',
        ].join(',')
      );
    });
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `batch-${currentBatch.id}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const totalYuan = batches.reduce((s, b) => s + (b.amount * b.count) / 1_000_000, 0);
  const redeemedCount = batches.reduce((s, b) => s + b.redeemed, 0);
  const totalCount = batches.reduce((s, b) => s + b.count, 0);

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="激活码生成"
          description="批量签发激活码 · 运营活动 / 合作伙伴分发 / 补偿发放"
        />

        {err && (
          <div className="rounded-md border border-danger bg-danger/20 px-4 py-3 text-sm text-red-200">
            {err}
          </div>
        )}
        {msg && (
          <div className="rounded-md border border-success bg-success/20 px-4 py-3 text-sm text-green-300">
            {msg}
          </div>
        )}

        <div className="grid gap-5 lg:grid-cols-2">
          {/* 新建批次表单 */}
          <section className="rounded-lg border border-slate-700 bg-slate-800 p-6">
            <h2 className="text-base font-semibold text-white">新建批次</h2>
            <p className="mt-0.5 text-xs text-slate-400">数量、面额、前缀、有效期</p>

            <div className="mt-5 space-y-4">
              <DarkField label="批次名称">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例：2026 春季推广 · 抖音合作"
                  className="bg-slate-900 text-white border-slate-700"
                />
              </DarkField>

              <div className="grid gap-4 sm:grid-cols-2">
                <DarkField label="生成数量">
                  <Input
                    type="number"
                    value={count}
                    onChange={(e) => setCount(Number(e.target.value))}
                    className="bg-slate-900 text-white border-slate-700 font-mono"
                  />
                  <p className="mt-1 text-[11px] text-slate-500">建议 ≤ 10,000 / 批次</p>
                </DarkField>
                <DarkField label="前缀">
                  <Input
                    value={prefix}
                    onChange={(e) => setPrefix(e.target.value)}
                    className="bg-slate-900 text-white border-slate-700 font-mono"
                  />
                </DarkField>
              </div>

              <DarkField label="单张面额">
                <div className="grid grid-cols-5 rounded-md border border-slate-700 bg-slate-900">
                  {PRESETS.map((v) => (
                    <button
                      key={v}
                      onClick={() => setAmount(v)}
                      className={
                        'py-2.5 text-sm font-medium transition-colors ' +
                        (amount === v
                          ? 'bg-brand-500 text-white rounded-md'
                          : 'text-slate-400 hover:text-white')
                      }
                    >
                      ${(v / 1_000_000).toFixed(0)}
                    </button>
                  ))}
                  <input
                    type="number"
                    placeholder="自定义"
                    onChange={(e) => setAmount(Number(e.target.value) * 1_000_000)}
                    className="bg-slate-900 px-2 py-2 text-xs text-slate-300 placeholder-slate-500 outline-none rounded-md"
                  />
                </div>
                <p className="mt-1 text-[11px] text-slate-500">单位：USD</p>
              </DarkField>

              <DarkField label="有效期（可选）">
                <Input
                  type="date"
                  value={expires}
                  onChange={(e) => setExpires(e.target.value)}
                  className="bg-slate-900 text-white border-slate-700 font-mono"
                />
              </DarkField>

              <div className="rounded-md border border-slate-700 bg-slate-900 p-3">
                <div className="text-[10px] font-medium uppercase tracking-wider text-slate-500">
                  预计总金额
                </div>
                <div className="mt-1 flex items-baseline gap-2">
                  <span className="text-2xl font-bold text-brand-400">
                    ${((amount * count) / 1_000_000).toFixed(2)}
                  </span>
                  <span className="text-xs text-slate-500">
                    {count} × ${(amount / 1_000_000).toFixed(2)}
                  </span>
                </div>
              </div>

              <Button className="w-full" onClick={submit} disabled={submitting}>
                {submitting ? '生成中…' : `生成 ${count} 张`}
              </Button>
            </div>
          </section>

          {/* 统计 + 批次列表 */}
          <div className="flex flex-col gap-5">
            <section className="rounded-lg border border-slate-700 bg-slate-800 p-6">
              <h2 className="text-sm font-semibold text-white">本月运营数据</h2>
              <div className="mt-4 grid grid-cols-3 gap-6">
                <Metric label="已生成" value={totalCount.toLocaleString()} hint="张激活码" />
                <Metric
                  label="已兑换"
                  value={redeemedCount.toLocaleString()}
                  hint={totalCount > 0 ? `${((redeemedCount / totalCount) * 100).toFixed(1)}%` : '0%'}
                  color="text-success"
                />
                <Metric label="累计放款" value={`$${totalYuan.toFixed(0)}`} color="text-brand-400" />
              </div>
            </section>

            <section className="rounded-lg border border-slate-700 bg-slate-800">
              <header className="border-b border-slate-700 px-5 py-4">
                <h2 className="text-sm font-semibold text-white">最近批次</h2>
                <p className="mt-0.5 text-xs text-slate-400">点击查看详情 / 导出 CSV</p>
              </header>
              {batches.length === 0 ? (
                <div className="p-8 text-center text-sm text-slate-500">暂无批次</div>
              ) : (
                <div className="divide-y divide-slate-700">
                  {batches.slice(0, 5).map((b) => (
                    <button
                      key={b.id}
                      onClick={() => pickBatch(b)}
                      className="flex w-full items-center gap-4 px-5 py-3 text-left transition-colors hover:bg-slate-700/30"
                    >
                      <div className="flex-1">
                        <div className="text-sm font-semibold text-white">{b.name}</div>
                        <div className="mt-0.5 font-mono text-[11px] text-slate-500">
                          {b.prefix} · {b.count} 张 · ${(b.amount / 1_000_000).toFixed(0)} / 张
                        </div>
                      </div>
                      <div className="text-right">
                        <div className="font-mono text-sm text-brand-400">
                          ${((b.amount * b.count) / 1_000_000).toFixed(0)}
                        </div>
                        <Badge variant={b.redeemed === b.count ? 'success' : 'neutral'}>
                          {b.count === 0 ? '空' : `${Math.round((b.redeemed / b.count) * 100)}%`}
                        </Badge>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </section>
          </div>
        </div>

        {/* 当前批次激活码明细 */}
        {currentBatch && (
          <section className="rounded-lg border border-slate-700 bg-slate-800">
            <header className="flex items-center justify-between border-b border-slate-700 px-5 py-4">
              <div>
                <h2 className="text-sm font-semibold text-white">批次：{currentBatch.name}</h2>
                <p className="mt-0.5 font-mono text-[11px] text-slate-400">
                  {currentBatch.prefix}* · {currentBatch.count} 张 · 已兑换 {currentBatch.redeemed}
                </p>
              </div>
              <Button variant="secondary" onClick={exportCsv}>
                导出 CSV
              </Button>
            </header>

            <table className="w-full text-xs">
              <thead className="bg-slate-900 text-[10px] uppercase tracking-wider text-slate-500">
                <tr>
                  <th className="px-5 py-2.5 text-left font-semibold">激活码</th>
                  <th className="px-5 py-2.5 text-right font-semibold">面额</th>
                  <th className="px-5 py-2.5 text-left font-semibold">有效期</th>
                  <th className="px-5 py-2.5 text-left font-semibold">状态</th>
                  <th className="px-5 py-2.5 text-left font-semibold">兑换用户</th>
                </tr>
              </thead>
              <tbody>
                {codes.slice(0, 20).map((c) => (
                  <tr key={c.id} className="border-t border-slate-700">
                    <td className="px-5 py-2.5 font-mono tracking-wider text-white">{c.code}</td>
                    <td className="px-5 py-2.5 text-right font-mono text-brand-400">
                      ${(c.amount / 1_000_000).toFixed(2)}
                    </td>
                    <td className="px-5 py-2.5 font-mono text-slate-400">
                      {c.expires_at ? c.expires_at.slice(0, 10) : '—'}
                    </td>
                    <td className="px-5 py-2.5">
                      {c.used_by ? (
                        <Badge variant="success" dot>
                          已兑换
                        </Badge>
                      ) : (
                        <Badge variant="neutral">未兑换</Badge>
                      )}
                    </td>
                    <td className="px-5 py-2.5 text-slate-300">
                      {c.used_by
                        ? `user#${c.used_by} · ${new Date(c.used_at ?? '').toLocaleString('zh-CN')}`
                        : '—'}
                    </td>
                  </tr>
                ))}
                {codes.length > 20 && (
                  <tr>
                    <td colSpan={5} className="px-5 py-3 text-center text-xs text-slate-500">
                      共 {codes.length} 张，仅显示前 20 条 · 导出 CSV 查看完整列表
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>
        )}
      </PageContent>
    </AdminShell>
  );
}

function DarkField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <label className="text-xs font-medium text-slate-300">{label}</label>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}

function Metric({
  label,
  value,
  hint,
  color = 'text-white',
}: {
  label: string;
  value: string;
  hint?: string;
  color?: string;
}) {
  return (
    <div>
      <div className="text-[10px] font-medium uppercase tracking-wider text-slate-500">{label}</div>
      <div className={`mt-1 text-2xl font-bold ${color}`}>{value}</div>
      {hint && <div className="text-[11px] text-slate-500">{hint}</div>}
    </div>
  );
}
