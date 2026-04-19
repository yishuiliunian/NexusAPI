#!/usr/bin/env node
// dev-stack.mjs —— 本地一键启整栈。
//
// 启动方式（与项目 Bazel 策略一致，全部走 bazel run）：
//   backend   · bazel run //backend/cmd/server
//   seed      · bazel run //backend/cmd/e2e-seed
//   upstream  · node e2e/scripts/upstream-mock.mjs (独立 mock，不需要 Bazel)
//   web-user  · bazel run //web-user:next_dev
//   web-admin · bazel run //web-admin:next_dev
//
// 端口：
//   backend   :8080   upstream  :18090
//   web-user  :3000   web-admin :3001
//
// 种子账号：
//   admin@e2e.test / admin12345
//   alice@e2e.test / user12345
//   兑换码：E2E-REDEEM-CODE (1 元)
//
// 用法：
//   pnpm dev              # 一键起栈，Ctrl+C 停全部
//   pnpm dev --reset      # 清库再起
//   pnpm dev --no-seed    # 不做 seed（空库）
//   pnpm dev --no-admin   # 不启 admin 前端
//   pnpm dev --no-bazel   # 后端用 `go run`，前端用 `pnpm exec next dev`（冷启更快，调试用）
import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from 'node:http';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');

const PORTS = {
  backend: 8080,
  upstream: 18090,
  webUser: 3000,
  webAdmin: 3001,
};

const args = new Set(process.argv.slice(2));
const RESET = args.has('--reset');
const NO_SEED = args.has('--no-seed');
const NO_ADMIN = args.has('--no-admin');
const NO_BAZEL = args.has('--no-bazel');

const DB_DIR = resolve(ROOT, '.tmp');
const DB_PATH = resolve(DB_DIR, 'nexus-dev.db');

const COLOR = {
  upstream: '\x1b[36m',
  backend: '\x1b[32m',
  'web-user': '\x1b[35m',
  'web-admin': '\x1b[33m',
  seed: '\x1b[90m',
  info: '\x1b[1;37m',
};
const RESET_CLR = '\x1b[0m';
const prefix = (label) => `${COLOR[label] ?? ''}[${label}]${RESET_CLR} `;
const log = (label, msg) => process.stdout.write(prefix(label) + msg + (msg.endsWith('\n') ? '' : '\n'));

const kids = [];
function track(cp, label) {
  kids.push(cp);
  cp.stdout?.on('data', (b) => process.stdout.write(prefix(label) + b));
  cp.stderr?.on('data', (b) => process.stderr.write(prefix(label) + b));
  cp.on('exit', (code, sig) => {
    if (code !== 0 && sig !== 'SIGTERM' && sig !== 'SIGKILL') {
      log('info', `\x1b[31m[${label}] exited code=${code} sig=${sig}${RESET_CLR}`);
    }
  });
}

function start(label, cmd, cmdArgs, opts = {}) {
  const cp = spawn(cmd, cmdArgs, {
    cwd: opts.cwd ?? ROOT,
    env: { ...process.env, ...(opts.env ?? {}) },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  track(cp, label);
  return cp;
}

async function killAll() {
  log('info', '\n关闭所有子进程...\n');
  for (const cp of kids) {
    if (!cp.killed) {
      try { cp.kill('SIGTERM'); } catch {}
    }
  }
  await new Promise((r) => setTimeout(r, 500));
  for (const cp of kids) {
    if (!cp.killed) {
      try { cp.kill('SIGKILL'); } catch {}
    }
  }
}

process.on('SIGINT', async () => { await killAll(); process.exit(0); });
process.on('SIGTERM', async () => { await killAll(); process.exit(0); });

function waitFor(url, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolvePromise, reject) => {
    const poll = () => {
      const req = request(url, { method: 'GET', timeout: 1000 }, (res) => {
        res.resume();
        if (res.statusCode && res.statusCode < 500) return resolvePromise();
        if (Date.now() > deadline) return reject(new Error(`${url} status=${res.statusCode}`));
        setTimeout(poll, 300);
      });
      req.on('error', () => {
        if (Date.now() > deadline) return reject(new Error(`${url} timeout`));
        setTimeout(poll, 300);
      });
      req.end();
    };
    poll();
  });
}

function portBusy(port) {
  return new Promise((r) => {
    const req = request(`http://127.0.0.1:${port}`, { method: 'GET', timeout: 500 }, (res) => {
      res.resume();
      r(true);
    });
    req.on('error', () => r(false));
    req.on('timeout', () => { req.destroy(); r(false); });
    req.end();
  });
}

async function preflight() {
  const busy = [];
  for (const [name, port] of Object.entries(PORTS)) {
    if (name === 'webAdmin' && NO_ADMIN) continue;
    if (await portBusy(port)) busy.push(`${name}=${port}`);
  }
  if (busy.length) {
    log('info', `\x1b[31m端口被占用：${busy.join(', ')}${RESET_CLR}`);
    log('info', '请先 `pnpm stop` 或手动杀掉占用进程');
    process.exit(1);
  }
}

function bazelPrebuild() {
  const targets = [
    '//backend/cmd/server',
    '//backend/cmd/e2e-seed',
  ];
  if (!NO_BAZEL) {
    targets.push('//web-user:next_dev');
    if (!NO_ADMIN) targets.push('//web-admin:next_dev');
  }
  log('info', `bazel build ${targets.join(' ')} ...`);
  const r = spawnSync('bazel', ['build', ...targets], { cwd: ROOT, stdio: 'inherit' });
  if (r.status !== 0) throw new Error('bazel prebuild failed');
}

function backendEnv() {
  return {
    NEXUSAPI_APP_ENV: 'development',
    NEXUSAPI_SERVER_HOST: '127.0.0.1',
    NEXUSAPI_SERVER_PORT: String(PORTS.backend),
    NEXUSAPI_DATABASE_DRIVER: 'sqlite',
    NEXUSAPI_DATABASE_DSN: DB_PATH,
    NEXUSAPI_LOG_LEVEL: 'info',
    NEXUSAPI_LOG_FORMAT: 'console',
    NEXUSAPI_REDIS_ADDR: '',
    NEXUSAPI_SECURITY_ENCRYPTION_KEY: '',
    NEXUSAPI_SITE_BASE_URL: `http://127.0.0.1:${PORTS.webUser}`,
    NEXUSAPI_AUTH_SESSION_TTL_HOURS: '24',
    NEXUSAPI_OAUTH_POST_LOGIN_URL: `http://127.0.0.1:${PORTS.webUser}/dashboard`,
    NEXUSAPI_OAUTH_GITHUB_ENABLED: 'true',
    NEXUSAPI_OAUTH_GITHUB_CLIENT_ID: 'dev-gh-id',
    NEXUSAPI_OAUTH_GITHUB_CLIENT_SECRET: 'dev-gh-secret',
    NEXUSAPI_OAUTH_GITHUB_AUTHORIZE_URL: `http://127.0.0.1:${PORTS.upstream}/oauth/github/authorize`,
    NEXUSAPI_OAUTH_GITHUB_TOKEN_URL: `http://127.0.0.1:${PORTS.upstream}/oauth/github/token`,
    NEXUSAPI_OAUTH_GITHUB_API_BASE: `http://127.0.0.1:${PORTS.upstream}`,
    NEXUSAPI_OAUTH_GOOGLE_ENABLED: 'true',
    NEXUSAPI_OAUTH_GOOGLE_CLIENT_ID: 'dev-gg-id',
    NEXUSAPI_OAUTH_GOOGLE_CLIENT_SECRET: 'dev-gg-secret',
    NEXUSAPI_OAUTH_GOOGLE_AUTHORIZE_URL: `http://127.0.0.1:${PORTS.upstream}/oauth/google/authorize`,
    NEXUSAPI_OAUTH_GOOGLE_TOKEN_URL: `http://127.0.0.1:${PORTS.upstream}/oauth/google/token`,
    NEXUSAPI_OAUTH_GOOGLE_API_BASE: `http://127.0.0.1:${PORTS.upstream}/oauth/google/userinfo`,
    NEXUSAPI_PAYMENT_STRIPE_ENABLED: 'true',
    NEXUSAPI_PAYMENT_STRIPE_SECRET_KEY: 'sk_dev_fake',
    NEXUSAPI_PAYMENT_STRIPE_WEBHOOK_SECRET: 'whsec_dev_fixed',
    NEXUSAPI_PAYMENT_STRIPE_SUCCESS_URL: `http://127.0.0.1:${PORTS.webUser}/billing?paid=1`,
    NEXUSAPI_PAYMENT_STRIPE_CANCEL_URL: `http://127.0.0.1:${PORTS.webUser}/billing?canceled=1`,
    NEXUSAPI_PAYMENT_STRIPE_PRODUCT_NAME: 'NexusAPI Dev Credits',
    NEXUSAPI_PAYMENT_STRIPE_API_BASE: `http://127.0.0.1:${PORTS.upstream}`,
    NEXUSAPI_PAYMENT_MICRO_PER_CENT: '10000',
  };
}

function frontendEnv() {
  return { NEXUSAPI_BACKEND_URL: `http://127.0.0.1:${PORTS.backend}` };
}

function backendCmd() {
  if (NO_BAZEL) {
    return { cmd: 'go', args: ['run', './cmd/server'], cwd: resolve(ROOT, 'backend') };
  }
  return { cmd: 'bazel', args: ['run', '//backend/cmd/server'], cwd: ROOT };
}

function seedCmd() {
  const flags = [
    '--sqlite', DB_PATH,
    '--upstream-url', `http://127.0.0.1:${PORTS.upstream}`,
  ];
  if (RESET) flags.push('--reset');
  if (NO_BAZEL) {
    return { cmd: 'go', args: ['run', './cmd/e2e-seed', ...flags], cwd: resolve(ROOT, 'backend') };
  }
  return { cmd: 'bazel', args: ['run', '//backend/cmd/e2e-seed', '--', ...flags], cwd: ROOT };
}

function webUserCmd() {
  if (NO_BAZEL) {
    return {
      cmd: 'pnpm',
      args: ['exec', 'next', 'dev', '-p', String(PORTS.webUser)],
      cwd: resolve(ROOT, 'web-user'),
    };
  }
  return {
    cmd: 'bazel',
    args: ['run', '//web-user:next_dev', '--', '-p', String(PORTS.webUser)],
    cwd: ROOT,
  };
}

function webAdminCmd() {
  if (NO_BAZEL) {
    return {
      cmd: 'pnpm',
      args: ['exec', 'next', 'dev', '-p', String(PORTS.webAdmin)],
      cwd: resolve(ROOT, 'web-admin'),
    };
  }
  return {
    cmd: 'bazel',
    args: ['run', '//web-admin:next_dev', '--', '-p', String(PORTS.webAdmin)],
    cwd: ROOT,
  };
}

async function main() {
  log('info', `NexusAPI 开发栈启动（bazel=${!NO_BAZEL}, reset=${RESET}, seed=${!NO_SEED}, admin=${!NO_ADMIN}）`);

  await preflight();

  mkdirSync(DB_DIR, { recursive: true });
  if (RESET) {
    for (const suffix of ['', '-journal', '-wal', '-shm']) {
      const p = DB_PATH + suffix;
      if (existsSync(p)) rmSync(p);
    }
    log('info', '已清空 SQLite');
  }

  if (!NO_BAZEL) bazelPrebuild();

  start('upstream', 'node', ['e2e/scripts/upstream-mock.mjs'], {
    env: { UPSTREAM_PORT: String(PORTS.upstream) },
  });
  await waitFor(`http://127.0.0.1:${PORTS.upstream}/healthz`, 10_000);
  log('info', `upstream mock 就绪 http://127.0.0.1:${PORTS.upstream}`);

  const bc = backendCmd();
  start('backend', bc.cmd, bc.args, { cwd: bc.cwd, env: backendEnv() });
  await waitFor(`http://127.0.0.1:${PORTS.backend}/healthz`, 120_000);
  log('info', `backend 就绪 http://127.0.0.1:${PORTS.backend}`);

  if (!NO_SEED) {
    const sc = seedCmd();
    await new Promise((resolvePromise, reject) => {
      const cp = spawn(sc.cmd, sc.args, { cwd: sc.cwd, stdio: ['ignore', 'pipe', 'pipe'] });
      cp.stdout.on('data', (b) => process.stdout.write(prefix('seed') + b));
      cp.stderr.on('data', (b) => process.stderr.write(prefix('seed') + b));
      cp.on('exit', (code) => (code === 0 ? resolvePromise() : reject(new Error(`seed exit ${code}`))));
    });
    log('info', 'seed 完成');
  }

  const wu = webUserCmd();
  start('web-user', wu.cmd, wu.args, { cwd: wu.cwd, env: frontendEnv() });

  if (!NO_ADMIN) {
    const wa = webAdminCmd();
    start('web-admin', wa.cmd, wa.args, { cwd: wa.cwd, env: frontendEnv() });
  }

  const waits = [waitFor(`http://127.0.0.1:${PORTS.webUser}/login`, 180_000)];
  if (!NO_ADMIN) waits.push(waitFor(`http://127.0.0.1:${PORTS.webAdmin}/login`, 180_000));
  await Promise.all(waits);

  const runner = NO_BAZEL ? 'go/pnpm' : 'bazel run';
  log('info', '');
  log('info', '\x1b[1;32m✓ 全栈已就绪\x1b[0m');
  log('info', `  backend    http://127.0.0.1:${PORTS.backend}   (${runner})`);
  log('info', `  upstream   http://127.0.0.1:${PORTS.upstream}  (node mock)`);
  log('info', `  web-user   http://127.0.0.1:${PORTS.webUser}   (${runner})`);
  if (!NO_ADMIN) log('info', `  web-admin  http://127.0.0.1:${PORTS.webAdmin}  (${runner})`);
  log('info', '');
  if (!NO_SEED) {
    log('info', '种子账号：');
    log('info', '  admin@e2e.test / admin12345');
    log('info', '  alice@e2e.test / user12345');
    log('info', '  兑换码：E2E-REDEEM-CODE (1 元)');
  }
  log('info', '按 Ctrl+C 停全部\n');

  await new Promise((_r, rej) => {
    for (const cp of kids) {
      cp.on('exit', (code, sig) => {
        if (code !== 0 && sig !== 'SIGTERM' && sig !== 'SIGKILL') {
          rej(new Error(`child exited unexpectedly: code=${code}`));
        }
      });
    }
  });
}

main().catch(async (e) => {
  log('info', `\x1b[31mfatal: ${e.message}${RESET_CLR}`);
  await killAll();
  process.exit(1);
});
