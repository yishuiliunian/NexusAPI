// Vitest 配置：jsdom 环境用于模拟浏览器 document/fetch。
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: false,
    include: ['src/**/*.test.ts'],
  },
});
