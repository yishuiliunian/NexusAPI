'use client';

// AppShell / Sidebar / Topbar —— 两种主题（user 浅 / admin 深）
//
// Link 组件由消费方注入（next/link 或普通 a），避免 shared 对 Next 产生 peer 依赖。
import { cn } from '../lib/cn';
import { type ComponentType, type ReactNode } from 'react';

export type NavItem = {
  label: string;
  href: string;
  group?: string;
  badge?: ReactNode;
};

export type LinkComponent = ComponentType<{
  href: string;
  className?: string;
  children: ReactNode;
}>;

const DefaultLink: LinkComponent = ({ href, className, children }) => (
  <a href={href} className={className}>
    {children}
  </a>
);

export function AppShell({
  theme = 'light',
  brand,
  brandSubtitle,
  user,
  nav,
  activeHref,
  Link = DefaultLink,
  children,
}: {
  theme?: 'light' | 'dark';
  brand: ReactNode;
  brandSubtitle?: ReactNode;
  user?: { email: string; role?: string };
  nav: NavItem[];
  activeHref: string;
  Link?: LinkComponent;
  children: ReactNode;
}) {
  const dark = theme === 'dark';

  return (
    <div className={cn('flex min-h-screen', dark ? 'bg-slate-900' : 'bg-slate-50')}>
      <aside
        className={cn(
          'w-60 shrink-0 border-r',
          dark
            ? 'border-slate-700 bg-slate-800 text-slate-300'
            : 'border-slate-200 bg-white text-slate-700'
        )}
      >
        <div className="flex flex-col gap-2 p-4">
          {/* Brand */}
          <div className="mb-2 flex items-center gap-2.5 px-2 py-2">
            <div
              className={cn(
                'grid h-7 w-7 place-items-center rounded-md font-bold text-white',
                dark ? 'bg-brand-500' : 'bg-brand-600'
              )}
            >
              N
            </div>
            <div className="flex flex-col">
              <span className={cn('text-base font-bold', dark ? 'text-white' : 'text-slate-900')}>
                {brand}
              </span>
              {brandSubtitle && (
                <span className="text-[10px] font-medium uppercase tracking-wider text-slate-500">
                  {brandSubtitle}
                </span>
              )}
            </div>
          </div>

          {/* Nav */}
          <NavList items={nav} activeHref={activeHref} dark={dark} Link={Link} />
        </div>

        {user && (
          <div className="mx-3 mb-4">
            <div
              className={cn(
                'flex items-center gap-2 rounded-md border p-2.5',
                dark ? 'border-slate-700 bg-slate-900' : 'border-slate-200 bg-slate-50'
              )}
            >
              <div className="grid h-8 w-8 place-items-center rounded-full bg-brand-100 text-xs font-bold text-brand-600">
                {user.email[0]?.toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <div
                  className={cn('truncate text-xs font-medium', dark ? 'text-white' : 'text-slate-800')}
                >
                  {user.email}
                </div>
                <div className="text-[10px] text-slate-500">{user.role ?? '用户'}</div>
              </div>
            </div>
          </div>
        )}
      </aside>

      <main className="flex-1 min-w-0">{children}</main>
    </div>
  );
}

function NavList({
  items,
  activeHref,
  dark,
  Link,
}: {
  items: NavItem[];
  activeHref: string;
  dark: boolean;
  Link: LinkComponent;
}) {
  // group by item.group
  const groups: Record<string, NavItem[]> = {};
  for (const it of items) {
    const g = it.group ?? '';
    if (!groups[g]) groups[g] = [];
    groups[g].push(it);
  }

  return (
    <nav className="flex flex-col gap-0.5">
      {Object.entries(groups).map(([group, list]) => (
        <div key={group} className="flex flex-col gap-0.5">
          {group && (
            <div
              className={cn(
                'mt-3 px-2 text-[10px] font-semibold uppercase tracking-wider',
                dark ? 'text-slate-500' : 'text-slate-400'
              )}
            >
              {group}
            </div>
          )}
          {list.map((it) => {
            const active = it.href === activeHref;
            return (
              <Link
                key={it.href}
                href={it.href}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                  active
                    ? dark
                      ? 'bg-brand-500/20 font-semibold text-white'
                      : 'bg-brand-50 font-semibold text-brand-600'
                    : dark
                    ? 'text-slate-300 hover:bg-slate-700/50'
                    : 'text-slate-600 hover:bg-slate-100'
                )}
              >
                {active && (
                  <span
                    className={cn('h-5 w-1 rounded-full', dark ? 'bg-brand-500' : 'bg-brand-600')}
                  />
                )}
                <span className="flex-1">{it.label}</span>
                {it.badge}
              </Link>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

export function PageHeader({
  title,
  description,
  actions,
  className,
  dark = false,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
  dark?: boolean;
}) {
  return (
    <header
      className={cn('mb-6 flex flex-wrap items-start justify-between gap-4', className)}
    >
      <div className="flex flex-col gap-1">
        <h1 className={cn('text-3xl font-bold', dark ? 'text-white' : 'text-slate-900')}>
          {title}
        </h1>
        {description && (
          <p className={cn('text-sm', dark ? 'text-slate-400' : 'text-slate-500')}>
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  );
}

export function PageContent({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={cn('space-y-6 p-8', className)}>{children}</div>;
}
