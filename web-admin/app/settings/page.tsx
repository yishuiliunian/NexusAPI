'use client';

// 系统设置 —— 当前仅展示运行参数。之后可拓展：
//   - SMTP 配置
//   - OAuth 客户端
//   - Stripe 网关开关
//   - 全局限流默认值
import { PageContent, PageHeader } from '@nexusapi/shared';
import { AdminShell } from '../../components/admin-shell';

export default function SettingsPage() {
  return (
    <AdminShell>
      <PageContent>
        <PageHeader dark title="系统设置" description="全局配置（大部分由 NEXUSAPI_* 环境变量驱动）" />

        <section className="rounded-lg border border-slate-700 bg-slate-800 p-6">
          <h2 className="text-sm font-semibold text-white">运行时配置</h2>
          <p className="mt-1 text-xs text-slate-500">
            以下配置由后端环境变量决定，重启 server 后生效：
          </p>

          <dl className="mt-5 divide-y divide-slate-700">
            <Row label="数据库驱动" value="NEXUSAPI_DATABASE_DRIVER" hint="postgres / sqlite" />
            <Row label="Redis 地址" value="NEXUSAPI_REDIS_ADDR" hint="空则退化内存存储" />
            <Row label="限流 RPM" value="NEXUSAPI_RATE_LIMIT_DEFAULT_RPM" hint="默认每 ApiKey 上限" />
            <Row label="限流 TPM" value="NEXUSAPI_RATE_LIMIT_DEFAULT_TPM" hint="token/分钟上限" />
            <Row label="故障转移次数" value="NEXUSAPI_RELAY_FAILOVER_ATTEMPTS" hint="中继失败重试" />
            <Row label="Stripe 启用" value="NEXUSAPI_PAYMENT_STRIPE_ENABLED" />
            <Row label="SMTP 主机" value="NEXUSAPI_MAIL_HOST" hint="留空禁用邮件" />
            <Row label="GitHub OAuth" value="NEXUSAPI_OAUTH_GITHUB_ENABLED" />
            <Row label="Google OAuth" value="NEXUSAPI_OAUTH_GOOGLE_ENABLED" />
          </dl>
        </section>
      </PageContent>
    </AdminShell>
  );
}

function Row({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="flex items-center justify-between gap-4 py-3 text-sm">
      <div>
        <div className="text-slate-300">{label}</div>
        {hint && <div className="text-xs text-slate-500">{hint}</div>}
      </div>
      <code className="font-mono text-xs text-brand-400">{value}</code>
    </div>
  );
}
