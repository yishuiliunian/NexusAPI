// global-teardown.ts —— 停掉所有 global-setup 启的子进程 + 兜底杀前端端口。
//
// 为什么需要端口兜底：`bazel run //web-user:next_dev` 这种启动方式，
// 子进程 spawn 出 bazel client → next-server，next-server 是 grandchild，
// cp.kill() 只杀直接 child（bazel client）。next-server 会变孤儿继续 listen。
// 所以额外用 lsof 端口杀确保前端真的退出。
import { spawnSync } from 'node:child_process';
import { PORTS } from '../helpers/env';

function killPort(port: number): void {
  spawnSync(
    'bash',
    [
      '-c',
      `lsof -nP -iTCP:${port} -sTCP:LISTEN -t 2>/dev/null | xargs -r kill -9 2>/dev/null || true`,
    ],
    { stdio: 'ignore' },
  );
}

export default async function globalTeardown(): Promise<void> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const fn = (globalThis as any).__nexus_teardown;
  if (typeof fn === 'function') {
    await fn();
  }
  // 端口兜底：保证下一轮 e2e/dev 能干净启动。backend 由 //deploy/dev:clean 管理，
  // 这里只清前端（e2e 自己启的进程）。
  killPort(PORTS.webUser);
  killPort(PORTS.webAdmin);
}
