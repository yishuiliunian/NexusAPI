package twofa

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestGenerateSecret_ValidBase32(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s); err != nil {
		t.Errorf("生成的 secret 不是合法 base32: %v", err)
	}
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	s1, _ := GenerateSecret()
	s2, _ := GenerateSecret()
	if s1 == s2 {
		t.Error("两次生成的 secret 相同")
	}
}

func TestOtpauthURL(t *testing.T) {
	u := OtpauthURL("JBSWY3DPEHPK3PXP", "alice@x", "NexusAPI")
	if u == "" {
		t.Error("空 url")
	}
	// 必含关键参数
	for _, want := range []string{"secret=JBSWY3DPEHPK3PXP", "issuer=NexusAPI", "otpauth://totp/"} {
		if !contains(u, want) {
			t.Errorf("url 缺 %q: %s", want, u)
		}
	}
}

func TestGenerate_Is6Digits(t *testing.T) {
	s, _ := GenerateSecret()
	code, err := Generate(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Errorf("code 长度: %q", code)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Errorf("非数字: %q", code)
		}
	}
}

func TestVerify_AcceptsCurrentCode(t *testing.T) {
	s, _ := GenerateSecret()
	code, _ := Generate(s)
	if !Verify(s, code) {
		t.Error("当前 code 应通过")
	}
}

func TestVerify_RejectsWrongCode(t *testing.T) {
	s, _ := GenerateSecret()
	if Verify(s, "000000") {
		// 这是随机概率事件（~1/10^6），在 deterministic 测试中几乎不可能
		t.Error("随机错误 code 不应通过")
	}
}

func TestVerify_AcceptsPreviousAndNextWindow(t *testing.T) {
	s, _ := GenerateSecret()
	now := time.Now().Unix()
	prev, _ := generateAt(s, now-30)
	next, _ := generateAt(s, now+30)
	if !Verify(s, prev) {
		t.Error("前一 30s 窗口的 code 应通过")
	}
	if !Verify(s, next) {
		t.Error("后一 30s 窗口的 code 应通过")
	}
}

func TestVerify_RejectsTooOldWindow(t *testing.T) {
	s, _ := GenerateSecret()
	old, _ := generateAt(s, time.Now().Unix()-120)
	if Verify(s, old) {
		t.Error("2 分钟前的 code 不应通过（只允许 ±1 窗口）")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
