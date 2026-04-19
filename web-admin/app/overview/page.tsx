'use client';

// Admin Overview · 全局监控
// 深色主题，4 KPI + 趋势 + Top 模型 + Top 用户

import { useEffect, useState, type ReactNode } from 'react';
import {
  adminApi,
  AdminStats,
  ApiClientError,
  Badge,
  PageContent,
  PageHeader,
  SimpleBarChart,
  StatCard,
  TrendChart,
} from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

const USD = (micro: number) => (micro / 1_000_000).toFixed(4);
const FMT = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : n.toLocaleString());

export default function OverviewPage() {
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [days, setDays] = useState(7);
  const [err, setErr] = useState('');

  useEffect(() => {
    adminApi
      .stats(days)
      .then(setStats)
      .catch((e) => {
        if (e instanceof ApiClientError) setErr(e.body.message);
      });
  }, [days]);

  return (
    <AdminShell>
      <PageContent>
        <PageHeader
          dark
          title="全局概览"
          description={
            stats ? `最近 ${stats.summary.days} 天 · ${new Date().toLocaleDateString('zh-CN')}` : '加载统计…'
          }
          actions={
            <>
              <div className="flex items-center gap-1.5 rounded-full bg-success/20 px-2.5 py-1 text-xs font-medium text-green-400">
                <span className="h-1.5 w-1.5 rounded-full bg-success" />
                系统正常
              </div>
              <TimeTabs days={days} onChange={setDays} />
            </>
          }
        />

        {err && (
          <div className="rounded-md border border-danger bg-danger/20 px-4 py-3 text-sm text-red-200">
            {err}
          </div>
        )}

        {stats && (
          <>
            <StatGrid stats={stats} />
            <div className="grid gap-4 lg:grid-cols-2">
              <DarkSection title="全局请求趋势" subtitle="每日调用总量">
                {(stats.by_day ?? []).length === 0 ? (
                  <Empty />
                ) : (
                  <TrendChart
                    data={(stats.by_day ?? []).map((d) => ({
                      date: d.date.slice(5),
                      requests: d.requests,
                    }))}
                    series={[{ key: 'requests', label: '请求数' }]}
                  />
                )}
              </DarkSection>
              <DarkSection title="收入趋势" subtitle="每日营收（USD）">
                {(stats.by_day ?? []).length === 0 ? (
                  <Empty />
                ) : (
                  <TrendChart
                    data={(stats.by_day ?? []).map((d) => ({ date: d.date.slice(5), cost: d.cost }))}
                    series={[{ key: 'cost', label: '收入', color: '#10B981' }]}
                    yFormat={(v) => (v / 1_000_000).toFixed(4)}
                  />
                )}
              </DarkSection>
            </div>

            <div className="grid gap-4 lg:grid-cols-2">
              <DarkSection title="Top 模型" subtitle="按收入（USD）">
                {(stats.by_model ?? []).length === 0 ? (
                  <Empty />
                ) : (
                  <SimpleBarChart
                    data={(stats.by_model ?? []).map((m) => ({ name: m.model, value: m.cost }))}
                    yFormat={(v) => (v / 1_000_000).toFixed(4)}
                    color="#10B981"
                  />
                )}
              </DarkSection>

              <DarkSection title="Top 10 用户" subtitle="按 7 天收入">
                {(stats.top_users ?? []).length === 0 ? (
                  <Empty />
                ) : (
                  <table className="w-full text-xs">
                    <thead className="text-slate-500">
                      <tr className="border-b border-slate-700">
                        <th className="py-2 text-left font-medium">#</th>
                        <th className="py-2 text-left font-medium">Email</th>
                        <th className="py-2 text-right font-medium">请求</th>
                        <th className="py-2 text-right font-medium">收入</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(stats.top_users ?? []).map((u, i) => (
                        <tr key={u.user_id} className="border-b border-slate-700">
                          <td className="py-2 text-slate-400">{i + 1}</td>
                          <td className="py-2 text-white">{u.email || `<user#${u.user_id}>`}</td>
                          <td className="py-2 text-right text-slate-300">{FMT(u.requests)}</td>
                          <td className="py-2 text-right font-mono text-success">${USD(u.cost)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </DarkSection>
            </div>
          </>
        )}
      </PageContent>
    </AdminShell>
  );
}

function TimeTabs({ days, onChange }: { days: number; onChange: (d: number) => void }) {
  const opts = [1, 7, 30, 90];
  return (
    <div className="flex rounded-md border border-slate-700 bg-slate-800 p-0.5">
      {opts.map((v) => (
        <button
          key={v}
          onClick={() => onChange(v)}
          className={
            'px-3 py-1.5 text-xs font-medium rounded transition-colors ' +
            (days === v ? 'bg-brand-500/30 text-white' : 'text-slate-400 hover:text-white')
          }
        >
          {v}D
        </button>
      ))}
    </div>
  );
}

function StatGrid({ stats }: { stats: AdminStats }) {
  const s = stats.summary;
  const successAccent =
    s.success_rate >= 0.95 ? 'success' : s.success_rate >= 0.8 ? 'warning' : 'danger';
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
      <DarkStatCard label="总请求数" value={FMT(s.total_requests)} hint={`最近 ${s.days} 天`} />
      <DarkStatCard label="总收入 (USD)" value={`$${USD(s.total_cost)}`} hint="累计消耗" accent="success" />
      <DarkStatCard label="活跃用户" value={s.active_users.toString()} hint="有调用的用户数" accent="brand" />
      <DarkStatCard
        label="成功率"
        value={`${(s.success_rate * 100).toFixed(1)}%`}
        hint="调用成功率"
        accent={successAccent}
      />
    </div>
  );
}

function DarkStatCard({
  label,
  value,
  hint,
  accent = 'brand',
}: {
  label: string;
  value: string;
  hint?: string;
  accent?: 'brand' | 'success' | 'warning' | 'danger';
}) {
  const dotColor = {
    brand: 'bg-brand-500',
    success: 'bg-success',
    warning: 'bg-warning',
    danger: 'bg-danger',
  }[accent];
  return (
    <div className="rounded-lg border border-slate-700 bg-slate-800 p-5">
      <div className="flex items-center gap-2">
        <span className={'h-1.5 w-1.5 rounded-full ' + dotColor} />
        <span className="text-xs font-medium uppercase tracking-wide text-slate-400">{label}</span>
      </div>
      <div className="mt-2 text-3xl font-bold text-white">{value}</div>
      {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
    </div>
  );
}

function DarkSection({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-lg border border-slate-700 bg-slate-800">
      <header className="border-b border-slate-700 px-5 py-4">
        <h2 className="text-sm font-semibold text-white">{title}</h2>
        {subtitle && <p className="mt-0.5 text-xs text-slate-400">{subtitle}</p>}
      </header>
      <div className="p-4">{children}</div>
    </section>
  );
}

function Empty() {
  return <div className="grid h-48 place-items-center text-sm text-slate-500">暂无数据</div>;
}

// 抑制未使用警告
void StatCard;
void Badge;
