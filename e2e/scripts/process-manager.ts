// process-manager.ts —— 统一管理 e2e 启动的子进程。
//
// 由 global-setup.ts 调用。global-teardown.ts 不需要单独处理，
// 因为 Playwright 进程退出时，我们 register 的 exit 钩子会 SIGTERM 所有子进程。
import { spawn, type ChildProcess, type SpawnOptions } from 'node:child_process';
import { fetch as undiciFetch } from 'undici';

// 全局子进程列表，退出时一起干掉
const kids: ChildProcess[] = [];

export function trackChild(cp: ChildProcess) {
  kids.push(cp);
}

export async function killAll(): Promise<void> {
  for (const cp of kids) {
    if (!cp.killed) {
      try {
        cp.kill('SIGTERM');
      } catch {
        // ignore
      }
    }
  }
  // 给它们 3s 优雅退出
  await new Promise((r) => setTimeout(r, 500));
  for (const cp of kids) {
    if (!cp.killed) {
      try {
        cp.kill('SIGKILL');
      } catch {
        // ignore
      }
    }
  }
}

process.on('exit', () => {
  for (const cp of kids) {
    if (!cp.killed) {
      try {
        cp.kill('SIGKILL');
      } catch {
        // ignore
      }
    }
  }
});

type StartOpts = {
  cwd: string;
  cmd: string;
  args: string[];
  env?: NodeJS.ProcessEnv;
  label: string;
};

export function start({ cwd, cmd, args, env, label }: StartOpts): ChildProcess {
  const opts: SpawnOptions = {
    cwd,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  };
  const cp = spawn(cmd, args, opts);
  trackChild(cp);
  // 聚合到 Playwright 的输出，便于 CI 日志排查。
  cp.stdout?.on('data', (b) => process.stderr.write(`[${label}] ${b}`));
  cp.stderr?.on('data', (b) => process.stderr.write(`[${label}!] ${b}`));
  cp.on('exit', (code, sig) => {
    if (code !== 0 && sig !== 'SIGTERM' && sig !== 'SIGKILL') {
      process.stderr.write(`[${label}] exited code=${code} sig=${sig}\n`);
    }
  });
  return cp;
}

// waitFor 轮询 URL 直到 200 或超时。
export async function waitFor(url: string, timeoutMs = 60_000, intervalMs = 500): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      const res = await undiciFetch(url);
      if (res.ok || res.status === 404) return;
      lastErr = `status=${res.status}`;
    } catch (e) {
      lastErr = e;
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  throw new Error(`waitFor ${url} timed out: ${String(lastErr)}`);
}
