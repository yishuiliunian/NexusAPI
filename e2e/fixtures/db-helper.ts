// db-helper.ts —— E2E 专用：直接查询 SQLite 文件。
//
// 用来获取后端生成但未通过 HTTP 暴露的数据（verify tokens、内部状态等）。
// 依赖系统 sqlite3 CLI（Ubuntu/macOS 预装）。
import { execSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..', '..');
export const DB_PATH = resolve(ROOT, 'e2e', '.tmp', 'nexus-e2e.db');

/** 执行 SQL 查询；返回行数组（每行为字符串数组）。 */
export function query(sql: string): string[][] {
  const out = execSync(`sqlite3 -separator $'\\t' ${DB_PATH} "${sql.replace(/"/g, '\\"')}"`, {
    encoding: 'utf8',
  });
  return out
    .split('\n')
    .filter(Boolean)
    .map((line) => line.split('\t'));
}

/** 获取某 user 最新的某 purpose 的 verify token ID。 */
export function latestVerifyToken(email: string, purpose: 'email_verify' | 'password_reset'): string | null {
  const rows = query(
    `SELECT v.id FROM verify_tokens v JOIN users u ON v.user_id = u.id WHERE u.email = '${email}' AND v.purpose = '${purpose}' ORDER BY v.created_at DESC LIMIT 1`
  );
  return rows[0]?.[0] ?? null;
}

/** 查某 user 的 EmailVerified 状态。 */
export function emailVerified(email: string): boolean {
  const rows = query(`SELECT email_verified FROM users WHERE email = '${email}'`);
  return rows[0]?.[0] === '1';
}
