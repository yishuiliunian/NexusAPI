'use client';

// Input —— 文本输入框
import { cn } from '../lib/cn';
import { forwardRef, type InputHTMLAttributes } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, invalid = false, ...rest },
  ref
) {
  return (
    <input
      ref={ref}
      className={cn(
        'w-full rounded-md border px-3 py-2 text-sm placeholder-slate-400 transition-colors',
        'focus:outline-none focus:ring-2 focus:ring-brand-500 focus:border-brand-500',
        invalid
          ? 'border-danger bg-danger-bg/30'
          : 'border-slate-300 bg-white',
        className
      )}
      {...rest}
    />
  );
});
