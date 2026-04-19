'use client';

// UserShell —— 用户端壳
// 封装导航 / 登录校验 / Next Link 注入
import Link from 'next/link';
import { useRouter, usePathname } from 'next/navigation';
import { useEffect, useState, type ReactNode } from 'react';
import { ApiClientError, AppShell, Me, userApi, type NavItem } from '@nexusapi/shared';

const NAV: NavItem[] = [
  { label: '监控看板', href: '/dashboard', group: '主要' },
  { label: 'API Keys', href: '/keys', group: '主要' },
  { label: '充值', href: '/billing', group: '账户' },
  { label: '账户设置', href: '/settings', group: '账户' },
];

export function UserShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    userApi
      .me()
      .then((m) => {
        setMe(m);
        setLoading(false);
      })
      .catch((e) => {
        if (e instanceof ApiClientError && e.status === 401) {
          router.replace('/login');
          return;
        }
        setLoading(false);
      });
  }, [router]);

  if (loading) {
    return <div className="grid min-h-screen place-items-center text-sm text-slate-500">加载中…</div>;
  }
  if (!me) return null;

  return (
    <AppShell
      theme="light"
      brand="NexusAPI"
      nav={NAV}
      activeHref={pathname}
      user={{ email: me.email, role: me.role === 'admin' ? '管理员' : '普通用户' }}
      Link={Link}
    >
      {children}
    </AppShell>
  );
}
