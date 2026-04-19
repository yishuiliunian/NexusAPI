'use client';

// AdminShell —— 管理员端壳
import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useEffect, useState, type ReactNode } from 'react';
import { ApiClientError, AppShell, Me, userApi, type NavItem } from '@nexusapi/shared';

const NAV: NavItem[] = [
  { label: '全局概览', href: '/overview', group: '运营' },
  { label: '渠道管理', href: '/channels', group: '运营' },
  { label: '模型价格', href: '/models', group: '运营' },
  { label: '用户', href: '/users', group: '运营' },
  { label: '分组', href: '/groups', group: '运营' },
  { label: '订单', href: '/orders', group: '财务' },
  { label: '激活码生成', href: '/redemption', group: '财务' },
  { label: '审计日志', href: '/audits', group: '系统' },
  { label: '系统设置', href: '/settings', group: '系统' },
];

export function AdminShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    userApi
      .me()
      .then((m) => {
        if (m.role !== 'admin') {
          router.replace('/login');
          return;
        }
        setMe(m);
        setLoading(false);
      })
      .catch((e) => {
        if (e instanceof ApiClientError) router.replace('/login');
      });
  }, [router]);

  if (loading) {
    return <div className="grid min-h-screen place-items-center bg-slate-900 text-sm text-slate-400">加载中…</div>;
  }
  if (!me) return null;

  return (
    <AppShell
      theme="dark"
      brand="NexusAPI"
      brandSubtitle="管理后台"
      nav={NAV}
      activeHref={pathname}
      user={{ email: me.email, role: '管理员' }}
      Link={Link}
    >
      {children}
    </AppShell>
  );
}
