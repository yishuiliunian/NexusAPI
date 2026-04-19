// cn.test.ts —— 验证 Tailwind 类名冲突合并。
import { describe, expect, it } from 'vitest';
import { cn } from './cn';

describe('cn', () => {
  it('合并基础类名', () => {
    expect(cn('p-2', 'text-sm')).toBe('p-2 text-sm');
  });

  it('Tailwind 冲突时后者胜出', () => {
    // 同维度 p-2 和 p-4 → 保留后者
    expect(cn('p-2', 'p-4')).toBe('p-4');
  });

  it('条件类：falsy 忽略', () => {
    expect(cn('p-2', false && 'p-4', undefined, null)).toBe('p-2');
  });

  it('数组参数', () => {
    expect(cn(['p-2', 'text-sm'], 'font-bold')).toBe('p-2 text-sm font-bold');
  });

  it('对象参数根据布尔值启用类', () => {
    expect(cn({ 'p-2': true, 'p-4': false })).toBe('p-2');
  });
});
