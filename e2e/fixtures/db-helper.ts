// db-helper.ts —— E2E 专用 DB 查询（Postgres）。
//
// 实现：spawnSync('docker', ['exec', '<container>', 'psql', '-U', ..., '-At', '-c', sql])
// 数组形式不经过 shell，单引号原样传给 psql，避免 shell 吃掉。
// `-At` = 不打印 header / aligned，行用 \n 分隔，列用 | 分隔。
import { spawnSync } from 'node:child_process';
import { composeProjectName, getEnv } from '../helpers/env';

function psqlExec(sql: string): string {
  const project = composeProjectName();
  const user = getEnv('POSTGRES_USER', 'nexusapi');
  const db = getEnv('POSTGRES_DB', 'nexusapi');
  const r = spawnSync(
    'docker',
    ['exec', `${project}-postgres-1`, 'psql', '-U', user, '-d', db, '-At', '-c', sql],
    { encoding: 'utf8' },
  );
  if (r.status !== 0) {
    throw new Error(`psql failed (code=${r.status}): ${r.stderr || r.stdout}`);
  }
  return r.stdout.trim();
}

/** 执行 SQL 查询；返回行数组（每行为列值数组，| 分隔）。 */
export function query(sql: string): string[][] {
  const out = psqlExec(sql);
  return out
    .split('\n')
    .filter(Boolean)
    .map((line) => line.split('|'));
}

/** 获取某 user 最新的某 purpose 的 verify token ID。 */
export function latestVerifyToken(
  email: string,
  purpose: 'email_verify' | 'password_reset',
): string | null {
  const rows = query(
    `SELECT v.id FROM verify_tokens v JOIN users u ON v.user_id = u.id WHERE u.email = '${email}' AND v.purpose = '${purpose}' ORDER BY v.created_at DESC LIMIT 1`,
  );
  return rows[0]?.[0] ?? null;
}

/** 查某 user 的 email_verified 状态。 */
export function emailVerified(email: string): boolean {
  const rows = query(`SELECT email_verified FROM users WHERE email = '${email}'`);
  return rows[0]?.[0] === 't' || rows[0]?.[0] === 'true';
}

// DB_PATH 保留导出以兼容旧 import，但已无意义（Postgres 没有文件路径）。
export const DB_PATH = '';
