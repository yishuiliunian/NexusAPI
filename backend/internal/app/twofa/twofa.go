// Package twofa 实现基于 TOTP 的 2FA。
//
// 不依赖外部库（pquerna/otp 等），自实现 RFC 6238 TOTP（HOTP+time）。
package twofa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

const (
	digits = 6
	period = 30
)

// GenerateSecret 生成随机 secret（20 字节 base32 编码）。
func GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// OtpauthURL 生成 otpauth:// URL（可用 QR 码扫描导入到 Google Authenticator 等）。
func OtpauthURL(secret, accountName, issuer string) string {
	label := url.PathEscape(issuer + ":" + accountName)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprintf("%d", digits))
	v.Set("period", fmt.Sprintf("%d", period))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, v.Encode())
}

// Generate 按当前时间生成 6 位 TOTP。
func Generate(secret string) (string, error) {
	return generateAt(secret, time.Now().Unix())
}

// Verify 校验用户输入的 6 位码（允许前后一个窗口的时间漂移）。
func Verify(secret, code string) bool {
	now := time.Now().Unix()
	for i := int64(-1); i <= 1; i++ {
		c, err := generateAt(secret, now+i*period)
		if err == nil && c == code {
			return true
		}
	}
	return false
}

func generateAt(secret string, ts int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("base32 decode: %w", err)
	}
	counter := uint64(ts / period)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	h := hmac.New(sha1.New, key)
	h.Write(buf)
	sum := h.Sum(nil)

	offset := sum[len(sum)-1] & 0x0F
	bin := (int(sum[offset])&0x7F)<<24 |
		(int(sum[offset+1])&0xFF)<<16 |
		(int(sum[offset+2])&0xFF)<<8 |
		(int(sum[offset+3]) & 0xFF)
	otp := bin % 1_000_000
	return fmt.Sprintf("%06d", otp), nil
}
