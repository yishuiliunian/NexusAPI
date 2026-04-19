'use client';

// Badge —— 状态标签
import { cn } from '../lib/cn';
import { type ReactNode } from 'react';

type Variant = 'success' | 'warning' | 'danger' | 'info' | 'neutral' | 'brand';

const variants: Record<Variant, string> = {
  success: 'bg-success-bg text-success',
  warning: 'bg-warning-bg text-amber-700',
  danger: 'bg-danger-bg text-danger',
  info: 'bg-info-bg text-cyan-700',
  neutral: 'bg-slate-100 text-slate-600',
  brand: 'bg-brand-50 text-brand-600',
};

export function Badge({
  variant = 'neutral',
  className,
  children,
  dot = false,
}: {
  variant?: Variant;
  className?: string;
  children: ReactNode;
  dot?: boolean;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium',
        variants[variant],
        className
      )}
    >
      {dot && <span className="h-1.5 w-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}
