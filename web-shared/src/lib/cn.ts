import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// cn 合并 className：解决 Tailwind 冲突样式的覆盖问题。
// 示例：cn('p-2', condition && 'p-4') → 条件真时结果为 'p-4'
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
