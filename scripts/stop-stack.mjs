#!/usr/bin/env node
// stop-stack.mjs —— 关闭 pnpm dev 可能残留的进程。
//
// 按端口 kill：8080 / 3000 / 3001 / 18090
// 按进程名 kill：go run cmd/server / next dev / upstream-mock
import { execSync } from 'node:child_process';

const ports = [8080, 3000, 3001, 18090];

function sh(cmd) {
  try {
    return execSync(cmd, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  } catch {
    return '';
  }
}

// 1. 按端口 kill
for (const p of ports) {
  const pids = sh(`lsof -ti tcp:${p}`).split('\n').filter(Boolean);
  for (const pid of pids) {
    try {
      process.kill(Number(pid), 'SIGKILL');
      console.log(`killed ${pid} on :${p}`);
    } catch {
      // process may have already exited
    }
  }
}

// 2. 按名字 pkill（macOS/Linux 通用）
const patterns = ['go run.*cmd/server', 'next dev', 'upstream-mock', 'dev-stack'];
for (const pat of patterns) {
  sh(`pkill -f ${JSON.stringify(pat)}`);
}

console.log('stopped');
