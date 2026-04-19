// global-teardown.ts —— 停掉所有 global-setup 启的子进程。
export default async function globalTeardown(): Promise<void> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const fn = (globalThis as any).__nexus_teardown;
  if (typeof fn === 'function') {
    await fn();
  }
}
