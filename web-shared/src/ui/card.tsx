'use client';

// Card / StatCard / Section —— 通用容器
import { cn } from '../lib/cn';
import { type ReactNode } from 'react';

export function Card({
  className,
  children,
  padding = 'md',
}: {
  className?: string;
  children: ReactNode;
  padding?: 'none' | 'sm' | 'md' | 'lg';
}) {
  const pad = { none: '', sm: 'p-4', md: 'p-5', lg: 'p-6' }[padding];
  return (
    <div className={cn('rounded-lg border border-slate-200 bg-white', pad, className)}>
      {children}
    </div>
  );
}

type AccentColor = 'brand' | 'success' | 'warning' | 'danger' | 'info' | 'purple';

const accentDot: Record<AccentColor, string> = {
  brand: 'bg-brand-500',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
  purple: 'bg-chart-4',
};

export function StatCard({
  label,
  value,
  hint,
  accent = 'brand',
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  accent?: AccentColor;
}) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-5">
      <div className="flex items-center gap-2">
        <span className={cn('h-1.5 w-1.5 rounded-full', accentDot[accent])} />
        <span className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</span>
      </div>
      <div className="mt-2 text-3xl font-bold text-slate-900">{value}</div>
      {hint && <div className="mt-1 text-xs text-slate-400">{hint}</div>}
    </div>
  );
}

export function Section({
  title,
  description,
  action,
  children,
  className,
}: {
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('rounded-lg border border-slate-200 bg-white', className)}>
      {(title || action) && (
        <header className="flex items-center justify-between border-b border-slate-100 px-5 py-4">
          <div className="flex flex-col gap-0.5">
            {title && <h2 className="text-base font-semibold text-slate-900">{title}</h2>}
            {description && <p className="text-xs text-slate-500">{description}</p>}
          </div>
          {action}
        </header>
      )}
      <div>{children}</div>
    </section>
  );
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
      {icon && <div className="text-3xl text-slate-300">{icon}</div>}
      <div className="text-sm font-medium text-slate-700">{title}</div>
      {description && <div className="max-w-sm text-xs text-slate-500">{description}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
