// totp.ts —— RFC 6238 最小实现；仅用于 E2E 测试。
//
// 后端 (twofa/twofa.go) 使用 base32 无 padding 的 20 字节 secret。
// 这里用 Node 内置 crypto 计算 HOTP/TOTP，不依赖外部库。
import { createHmac } from 'node:crypto';

/** 解 RFC 4648 base32（无 padding 可接受）。 */
function base32Decode(str: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const clean = str.replace(/=+$/g, '').toUpperCase();
  const out: number[] = [];
  let bits = 0;
  let value = 0;
  for (const c of clean) {
    const i = alphabet.indexOf(c);
    if (i < 0) continue;
    value = (value << 5) | i;
    bits += 5;
    if (bits >= 8) {
      out.push((value >> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Buffer.from(out);
}

/** 按 RFC 6238，30s 步长、SHA1、6 位。 */
export function totp(secret: string, atSeconds = Math.floor(Date.now() / 1000)): string {
  const key = base32Decode(secret);
  const counter = Math.floor(atSeconds / 30);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const hmac = createHmac('sha1', key).update(buf).digest();
  const offset = hmac[hmac.length - 1] & 0xf;
  const code =
    ((hmac[offset] & 0x7f) << 24) |
    ((hmac[offset + 1] & 0xff) << 16) |
    ((hmac[offset + 2] & 0xff) << 8) |
    (hmac[offset + 3] & 0xff);
  return (code % 1_000_000).toString().padStart(6, '0');
}
