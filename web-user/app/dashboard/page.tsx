'use client';

// Dashboard · 监控看板
// 4 KPI 卡 + Tokens 趋势 + 请求量/消耗双栏 + 按模型柱状 + 近期调用表

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  Badge,
  Button,
  EmptyState,
  PageContent,
  PageHeader,
  Section,
  SimpleBarChart,
  StatCard,
  Stats,
  TrendChart,
  Usage,
  userApi,
} from '@nexusapi/shared';
import { UserShell } from '../../components/user-shell';

const USD = (micro: number) => (micro / 1_000_000).toFixed(4);
const FMT = (n: number) => n.toLocaleString();

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [usages, setUsages] = useState<Usage[]>([]);
  const [days, setDays] = useState(7);
  const [err, setErr] = useState('');

  useEffect(() => {
    userApi
      .stats(days)
      .then(setStats)
      .catch((e) => setErr(e instanceof Error ? e.message : 'load failed'));
    userApi.usages(1, 10).then((p) => setUsages(p.items ?? []));
  }, [days]);

  return (
    <UserShell>
      <PageContent>
        <PageHeader
          title="监控看板"
          description={stats ? `最近 ${stats.summary.days} 天的调用与消耗趋势` : '加载统计…'}
          actions={
            <>
              <TimeTabs days={days} onChange={setDays} />
              <Link href="/keys">
                <Button>+ 新建 Key</Button>
              </Link>
            </>
          }
        />

        {err && (
          <div className="rounded-md border border-danger bg-danger-bg px-4 py-3 text-sm text-danger">
            {err}
          </div>
        )}

        {stats && (
          <>
            <StatGrid stats={stats} />
            <TokensTrendCard stats={stats} />
            <div className="grid gap-4 lg:grid-cols-2">
              <RequestsCard stats={stats} />
              <CostCard stats={stats} />
            </div>
            <div className="grid gap-4 lg:grid-cols-2">
              <ByModelCard stats={stats} />
              <ByCapabilityCard stats={stats} />
            </div>
            <RecentCallsCard usages={usages} />
          </>
        )}
      </PageContent>
    </UserShell>
  );
}

function TimeTabs({ days, onChange }: { days: number; onChange: (d: number) => void }) {
  const opts = [
    { v: 1, label: '1D' },
    { v: 7, label: '7D' },
    { v: 30, label: '30D' },
    { v: 90, label: '90D' },
  ];
  return (
    <div className="flex rounded-md border border-slate-300 bg-white p-0.5">
      {opts.map((o) => (
        <button
          key={o.v}
          onClick={() => onChange(o.v)}
          className={
            'px-3 py-1.5 text-xs font-medium rounded transition-colors ' +
            (days === o.v
              ? 'bg-brand-50 text-brand-600'
              : 'text-slate-500 hover:text-slate-700')
          }
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

function StatGrid({ stats }: { stats: Stats }) {
  const s = stats.summary;
  const successAccent = s.success_rate >= 0.95 ? 'success' : s.success_rate >= 0.8 ? 'warning' : 'danger';
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
      <StatCard
        label="当前余额"
        value={`$${USD(s.quota)}`}
        hint={`累计消耗 $${USD(s.used_quota)}`}
        accent="brand"
      />
      <StatCard
        label={`${s.days} 天消耗`}
        value={`$${USD(s.total_cost)}`}
        hint={`日均 $${USD(Math.floor(s.total_cost / Math.max(s.days, 1)))}`}
        accent="warning"
      />
      <StatCard
        label={`${s.days} 天请求`}
        value={FMT(s.total_requests)}
        hint={`日均 ${Math.floor(s.total_requests / Math.max(s.days, 1))} 次`}
        accent="purple"
      />
      <StatCard
        label="成功率"
        value={`${(s.success_rate * 100).toFixed(1)}%`}
        hint={s.total_requests > 0 ? '基于最近调用' : '暂无调用数据'}
        accent={successAccent}
      />
    </div>
  );
}

function TokensTrendCard({ stats }: { stats: Stats }) {
  const data = (stats.by_day ?? []).map((d) => ({
    date: d.date.slice(5),
    prompt: d.prompt_tokens,
    completion: d.completion_tokens,
  }));
  return (
    <Section title="Tokens 消耗趋势" description="按日聚合 prompt + completion">
      <div className="p-4">
        {data.length === 0 ? (
          <EmptyState title="暂无数据" description="在 API Keys 建密钥后，发起第一次调用试试" />
        ) : (
          <TrendChart
            data={data}
            series={[
              { key: 'prompt', label: 'Prompt' },
              { key: 'completion', label: 'Completion' },
            ]}
          />
        )}
      </div>
    </Section>
  );
}

function RequestsCard({ stats }: { stats: Stats }) {
  const data = (stats.by_day ?? []).map((d) => ({ date: d.date.slice(5), requests: d.requests }));
  return (
    <Section title="请求量趋势">
      <div className="p-4">
        {data.length === 0 ? (
          <EmptyState title="暂无数据" />
        ) : (
          <TrendChart data={data} series={[{ key: 'requests', label: '请求数' }]} type="line" />
        )}
      </div>
    </Section>
  );
}

function CostCard({ stats }: { stats: Stats }) {
  const data = (stats.by_day ?? []).map((d) => ({ date: d.date.slice(5), cost: d.cost }));
  return (
    <Section title="每日消耗（USD）">
      <div className="p-4">
        {data.length === 0 ? (
          <EmptyState title="暂无数据" />
        ) : (
          <TrendChart
            data={data}
            series={[{ key: 'cost', label: '消耗' }]}
            yFormat={(v) => (v / 1_000_000).toFixed(4)}
          />
        )}
      </div>
    </Section>
  );
}

function ByModelCard({ stats }: { stats: Stats }) {
  const data = (stats.by_model ?? []).map((m) => ({ name: m.model, value: m.cost }));
  return (
    <Section title="按模型消耗 Top" description="按收入排序（USD）">
      <div className="p-4">
        {data.length === 0 ? (
          <EmptyState title="暂无数据" />
        ) : (
          <SimpleBarChart data={data} yFormat={(v) => (v / 1_000_000).toFixed(4)} />
        )}
      </div>
    </Section>
  );
}

function ByCapabilityCard({ stats }: { stats: Stats }) {
  const list = stats.by_model ?? [];
  if (list.length === 0) {
    return (
      <Section title="模型明细">
        <EmptyState title="暂无数据" />
      </Section>
    );
  }
  return (
    <Section title="模型明细" description="请求数 / 消耗 / 单价">
      <table className="w-full text-sm">
        <thead className="bg-slate-50 text-xs uppercase text-slate-500">
          <tr>
            <th className="px-4 py-2 text-left font-medium">模型</th>
            <th className="px-4 py-2 text-right font-medium">请求</th>
            <th className="px-4 py-2 text-right font-medium">消耗（USD）</th>
          </tr>
        </thead>
        <tbody>
          {list.slice(0, 6).map((m) => (
            <tr key={m.model} className="border-t border-slate-100">
              <td className="px-4 py-2 font-mono text-xs text-slate-800">{m.model}</td>
              <td className="px-4 py-2 text-right">{FMT(m.requests)}</td>
              <td className="px-4 py-2 text-right text-slate-700">{USD(m.cost)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Section>
  );
}

function RecentCallsCard({ usages }: { usages: Usage[] }) {
  return (
    <Section
      title="最近调用"
      description="实时追踪 · 点击查看详情"
      action={<Link href="/keys" className="text-xs text-brand-600">查看全部 →</Link>}
    >
      {usages.length === 0 ? (
        <EmptyState title="还没有调用记录" description="用 API Key 调 /v1/chat/completions 后将显示在这里" />
      ) : (
        <table className="w-full text-sm">
          <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-2.5 text-left font-medium">时间</th>
              <th className="px-4 py-2.5 text-left font-medium">模型</th>
              <th className="px-4 py-2.5 text-left font-medium">能力</th>
              <th className="px-4 py-2.5 text-right font-medium">Prompt</th>
              <th className="px-4 py-2.5 text-right font-medium">Output</th>
              <th className="px-4 py-2.5 text-right font-medium">费用</th>
              <th className="px-4 py-2.5 text-left font-medium">状态</th>
            </tr>
          </thead>
          <tbody>
            {usages.map((u) => (
              <tr key={u.id} className="border-t border-slate-100">
                <td className="px-4 py-2.5 text-xs text-slate-500">
                  {new Date(u.created_at).toLocaleTimeString('zh-CN', { hour12: false })}
                </td>
                <td className="px-4 py-2.5 font-mono text-xs text-slate-800">{u.model}</td>
                <td className="px-4 py-2.5">
                  <Badge variant="neutral">{u.capability}</Badge>
                </td>
                <td className="px-4 py-2.5 text-right text-slate-700">{FMT(u.prompt_tokens)}</td>
                <td className="px-4 py-2.5 text-right text-slate-700">{FMT(u.completion_tokens)}</td>
                <td className="px-4 py-2.5 text-right font-semibold text-slate-900">
                  ${USD(u.cost)}
                </td>
                <td className="px-4 py-2.5">
                  <Badge variant={u.status === 'success' ? 'success' : 'danger'} dot>
                    {u.status}
                  </Badge>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Section>
  );
}
