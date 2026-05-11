// helpers/console-errors.ts — 主动捕获 console.error 与 pageerror。
//
// 设计思路（取自 AgentsMesh 同名 helper）：
//   - watchPageErrors(page) 在测试开始时绑定 console/pageerror 事件
//   - 返回 collector，测试任意点都可读已积累的错误
//   - 默认 ignore 一组已知噪声（Next.js dev 的 dev-related 警告、HMR 通知等），
//     未匹配的 console.error 任一即视为失败
//   - assertNoConsoleErrors(collector) 在测试末尾或 fixture teardown 中调用

import type { ConsoleMessage, Page } from '@playwright/test';

export interface ConsoleErrorCollector {
  consoleErrors: string[];
  pageErrors: string[];
  dispose: () => void;
}

// 已知噪声白名单：这些字符串出现在 console.error 文案里就忽略。
// 如果某条 spec 想要更严格（比如新功能页面），可以在 spec 内显式断言而非依赖此 helper。
const IGNORE_PATTERNS: Array<string | RegExp> = [
  // Next.js dev / HMR 内部告警
  /\[Fast Refresh\]/,
  /Download the React DevTools/,
  /Module not found: Can't resolve.*\.map$/,
  // 浏览器对 favicon / source-map 的 404
  /Failed to load resource:.*\/favicon\.ico/,
  /Failed to load resource:.*\.map/,
];

function shouldIgnore(text: string): boolean {
  for (const p of IGNORE_PATTERNS) {
    if (typeof p === 'string' ? text.includes(p) : p.test(text)) return true;
  }
  return false;
}

/** 给 page 绑定监听，返回 collector。在 fixture/test 开始时调用。 */
export function watchPageErrors(page: Page): ConsoleErrorCollector {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];

  const onConsole = (msg: ConsoleMessage) => {
    if (msg.type() !== 'error') return;
    const text = msg.text();
    if (shouldIgnore(text)) return;
    consoleErrors.push(text);
  };
  const onPageError = (err: Error) => {
    pageErrors.push(err.message || String(err));
  };

  page.on('console', onConsole);
  page.on('pageerror', onPageError);

  return {
    consoleErrors,
    pageErrors,
    dispose() {
      page.off('console', onConsole);
      page.off('pageerror', onPageError);
    },
  };
}

/** 断言到测试结束前没有任何未忽略的 console.error / pageerror。 */
export function assertNoConsoleErrors(c: ConsoleErrorCollector): void {
  const total = c.consoleErrors.length + c.pageErrors.length;
  if (total === 0) return;
  const lines: string[] = [];
  if (c.consoleErrors.length > 0) {
    lines.push('--- console.error ---');
    lines.push(...c.consoleErrors);
  }
  if (c.pageErrors.length > 0) {
    lines.push('--- pageerror ---');
    lines.push(...c.pageErrors);
  }
  throw new Error(`检测到 ${total} 条前端错误：\n${lines.join('\n')}`);
}
