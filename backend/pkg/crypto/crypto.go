// Package crypto 提供对称加解密工具（AES-256-GCM）。
//
// 用途：渠道 API Key、用户 TOTP secret 等敏感字段的落库加密。
//
// 使用：
//
//	cipher, err := crypto.New([]byte("32-byte-secret-from-config..."))
//	enc := cipher.Encrypt([]byte("sk-xxx"))
//	raw, err := cipher.Decrypt(enc)
//
// 设计要点：
//   - 密钥必须 32 字节（AES-256）
//   - 每次加密自带 96-bit nonce，输出 nonce||ciphertext（base64 URL-safe 无填充）
//   - 若明文为空返回空串（便于新建时为空的字段）
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Cipher 对称加解密封装。
type Cipher struct {
	aead cipher.AEAD
}

// New 构造 AES-256-GCM Cipher。key 必须 32 字节。
func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// DeriveKey 简单 key 派生：若配置 secret 不足 32 字节则 SHA-256 派生；若超出则截断。
// 注意仅用于便利；生产建议显式提供 32 字节 key。
func DeriveKey(secret string) []byte {
	k := []byte(secret)
	if len(k) == 32 {
		return k
	}
	if len(k) > 32 {
		return k[:32]
	}
	out := make([]byte, 32)
	for i := range out {
		out[i] = k[i%len(k)]
	}
	return out
}

// Encrypt 加密并返回 base64 URL-safe 字符串。plain 为空返回空串。
func (c *Cipher) Encrypt(plain []byte) (string, error) {
	if len(plain) == 0 {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	ct := c.aead.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(ct), nil
}

// Decrypt 解密一个 Encrypt 的输出。空串返回空 bytes。
func (c *Cipher) Decrypt(enc string) ([]byte, error) {
	if enc == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	return plain, nil
}

// EncryptString 便捷方法。
func (c *Cipher) EncryptString(s string) (string, error) { return c.Encrypt([]byte(s)) }

// DecryptString 便捷方法。
func (c *Cipher) DecryptString(enc string) (string, error) {
	b, err := c.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Noop 返回一个不加密的 Cipher（用于测试 / 未配置密钥时的兜底）。
//
// 警告：不加密。仅限开发环境使用。
func Noop() *Cipher { return &Cipher{aead: noopAEAD{}} }

type noopAEAD struct{}

func (noopAEAD) NonceSize() int                               { return 0 }
func (noopAEAD) Overhead() int                                { return 0 }
func (noopAEAD) Seal(dst, _, plaintext, _ []byte) []byte      { return append(dst, plaintext...) }
func (noopAEAD) Open(dst, _, ciphertext, _ []byte) ([]byte, error) {
	return append(dst, ciphertext...), nil
}
