'use client';

// Billing · 按量计费充值
//   1. 余额大卡
//   2. 激活码（Hero）
//   3. Stripe 充值（金额挑选）
//   4. 历史记录

import { useEffect, useState } from 'react';
import {
  ApiClientError,
  api,
  Badge,
  Button,
  Card,
  EmptyState,
  Input,
  Me,
  PageContent,
  PageHeader,
  Section,
  userApi,
} from '@nexusapi/shared';
import { UserShell } from '../../components/user-shell';

const YUAN = (micro: number) => (micro / 1_000_000).toFixed(4);

const TOPUP_PRESETS = [
  { cents: 700, yuan: 50, label: '轻度使用' },
  { cents: 1400, yuan: 100, label: '推荐金额', recommended: true },
  { cents: 2800, yuan: 200, label: '团队常用' },
  { cents: 7000, yuan: 500, label: '大额优惠' },
];

interface Order {
  id: string;
  amount: number;
  amount_cents: number;
  currency: string;
  gateway: string;
  status: string;
  created_at: string;
  paid_at: string | null;
}

export default function BillingPage() {
  const [me, setMe] = useState<Me | null>(null);
  const [code, setCode] = useState('');
  const [cents, setCents] = useState(1400);
  const [redeeming, setRedeeming] = useState(false);
  const [charging, setCharging] = useState(false);
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [orders, setOrders] = useState<Order[]>([]);
  const [gateways, setGateways] = useState<string[]>([]);

  async function refresh() {
    try {
      const [m, o, g] = await Promise.all([
        userApi.me(),
        api.get<{ items: Order[] }>('/api/billing/orders'),
        api.get<{ gateways: string[] }>('/api/billing/gateways').catch(() => ({ gateways: [] })),
      ]);
      setMe(m);
      setOrders(o.items ?? []);
      setGateways(g.gateways ?? []);
    } catch {}
  }

  useEffect(() => {
    refresh();
  }, []);

  async function redeem() {
    if (!code.trim()) return;
    setRedeeming(true);
    setMsg(null);
    try {
      const r = await api.post<{ amount: number }>('/api/billing/redeem', { code: code.trim() });
      setMsg({ type: 'success', text: `已到账 ¥${YUAN(r.amount)}` });
      setCode('');
      refresh();
    } catch (e) {
      setMsg({
        type: 'error',
        text: e instanceof ApiClientError ? e.body.message : '激活失败',
      });
    } finally {
      setRedeeming(false);
    }
  }

  async function topup(gateway: string) {
    setCharging(true);
    setMsg(null);
    try {
      const r = await api.post<{ checkout_url: string }>('/api/billing/topup', {
        amount_cents: cents,
        currency: 'USD',
        gateway,
      });
      if (r.checkout_url) {
        window.location.href = r.checkout_url;
      } else {
        setMsg({ type: 'error', text: '未返回 checkout URL' });
      }
    } catch (e) {
      setMsg({
        type: 'error',
        text: e instanceof ApiClientError ? e.body.message : '下单失败',
      });
    } finally {
      setCharging(false);
    }
  }

  if (!me) return <UserShell><div className="p-8 text-sm text-slate-500">加载中…</div></UserShell>;

  return (
    <UserShell>
      <PageContent>
        <PageHeader
          title="充值"
          description="按量计费 · 使用激活码或在线充值补充余额"
        />

        {msg && (
          <div
            className={
              'rounded-md border px-4 py-3 text-sm ' +
              (msg.type === 'success'
                ? 'border-success bg-success-bg text-emerald-800'
                : 'border-danger bg-danger-bg text-danger')
            }
          >
            {msg.text}
          </div>
        )}

        {/* 余额卡 + 用量卡 */}
        <div className="grid gap-4 lg:grid-cols-3">
          <div className="lg:col-span-2 overflow-hidden rounded-lg bg-gradient-to-br from-brand-600 to-brand-400 p-6 text-white">
            <div className="text-xs font-medium uppercase tracking-wider text-white/80">
              当前余额
            </div>
            <div className="mt-3 flex items-baseline gap-2">
              <span className="text-xl font-medium">¥</span>
              <span className="text-5xl font-bold">{YUAN(me.quota)}</span>
            </div>
            <div className="mt-5 flex flex-wrap gap-6 text-xs">
              <Stat label="累计消耗" value={`¥${YUAN(me.used_quota)}`} />
            </div>
          </div>

          <Card padding="md">
            <div className="text-xs font-medium uppercase tracking-wider text-slate-500">
              账户
            </div>
            <div className="mt-2 text-sm font-medium text-slate-800">{me.email}</div>
            <div className="mt-1 text-xs text-slate-500">
              {me.email_verified ? (
                <span className="text-success">✓ 邮箱已验证</span>
              ) : (
                <span className="text-warning">⚠ 邮箱未验证</span>
              )}
            </div>
          </Card>
        </div>

        {/* 激活码（Hero） */}
        <Card padding="lg" className="shadow-subtle">
          <div className="mb-5 flex items-center gap-4">
            <div className="grid h-14 w-14 place-items-center rounded-md bg-brand-50 text-2xl">🎟</div>
            <div>
              <h2 className="text-xl font-semibold text-slate-900">激活码兑换</h2>
              <p className="text-sm text-slate-500">
                粘贴从运营活动 / 合作渠道 / 社区获得的激活码，配额即时到账
              </p>
            </div>
          </div>

          <div className="flex flex-col gap-3 md:flex-row md:items-end">
            <div className="flex-1">
              <label className="text-xs font-medium text-slate-700">激活码</label>
              <div className="mt-2">
                <Input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="NEXUS-DY-XXXXXXXXXX"
                  className="h-12 font-mono tracking-wider text-base"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') redeem();
                  }}
                />
              </div>
            </div>
            <Button size="lg" onClick={redeem} disabled={!code.trim() || redeeming}>
              {redeeming ? '激活中…' : '激活 →'}
            </Button>
          </div>

          <div className="mt-4 rounded-md bg-info-bg px-3 py-2.5 text-xs text-cyan-800">
            💡 每张激活码只能使用一次 · 到账后永久有效 · 不可转让
          </div>
        </Card>

        {/* Stripe 充值 */}
        <Section title="在线充值" description="Stripe · 支持 Visa / MasterCard / 支付宝 · 到账永久有效">
          <div className="p-5">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              {TOPUP_PRESETS.map((p) => (
                <button
                  key={p.cents}
                  onClick={() => setCents(p.cents)}
                  className={
                    'relative rounded-md border p-4 text-left transition-colors ' +
                    (cents === p.cents
                      ? 'border-brand-500 bg-brand-50'
                      : 'border-slate-300 bg-white hover:border-slate-400')
                  }
                >
                  {p.recommended && (
                    <span className="absolute right-2 top-2 rounded-full bg-brand-600 px-1.5 py-0.5 text-[10px] font-bold text-white">
                      推荐
                    </span>
                  )}
                  <div className={'text-2xl font-bold ' + (cents === p.cents ? 'text-brand-600' : 'text-slate-900')}>
                    ¥{p.yuan}
                  </div>
                  <div className="mt-0.5 text-xs text-slate-500">${p.cents / 100} USD</div>
                  <div className="mt-1 text-[11px] text-slate-400">{p.label}</div>
                </button>
              ))}
            </div>

            <div className="mt-5 flex flex-col items-start justify-between gap-3 md:flex-row md:items-center">
              <div className="text-xs text-slate-500">
                <span className="text-success">✓</span> 永久有效 · 无过期 · 发票可开
              </div>
              <div className="flex gap-2">
                {gateways.length === 0 ? (
                  <span className="text-xs text-slate-400">未启用支付网关</span>
                ) : (
                  gateways.map((g) => (
                    <Button key={g} onClick={() => topup(g)} disabled={charging}>
                      {charging ? '下单中…' : `前往 ${g} 支付 →`}
                    </Button>
                  ))
                )}
              </div>
            </div>
          </div>
        </Section>

        {/* 历史记录 */}
        <Section title="最近订单" description="充值 / 激活码使用历史">
          {orders.length === 0 ? (
            <EmptyState title="暂无订单记录" />
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-5 py-3 text-left font-medium">订单号</th>
                  <th className="px-5 py-3 text-left font-medium">网关</th>
                  <th className="px-5 py-3 text-right font-medium">金额</th>
                  <th className="px-5 py-3 text-left font-medium">状态</th>
                  <th className="px-5 py-3 text-left font-medium">创建时间</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((o) => (
                  <tr key={o.id} className="border-t border-slate-100">
                    <td className="px-5 py-3 font-mono text-xs text-slate-700">
                      {o.id.slice(0, 8)}…
                    </td>
                    <td className="px-5 py-3 text-slate-700">{o.gateway}</td>
                    <td className="px-5 py-3 text-right font-semibold text-slate-900">
                      ¥{(o.amount_cents / 100).toFixed(2)}
                    </td>
                    <td className="px-5 py-3">
                      <Badge variant={o.status === 'paid' ? 'success' : 'neutral'} dot>
                        {o.status}
                      </Badge>
                    </td>
                    <td className="px-5 py-3 text-xs text-slate-500">
                      {new Date(o.created_at).toLocaleString('zh-CN')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Section>
      </PageContent>
    </UserShell>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[11px] font-medium uppercase tracking-wider text-white/70">{label}</div>
      <div className="mt-1 text-sm font-semibold">{value}</div>
    </div>
  );
}
