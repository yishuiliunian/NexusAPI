package crypto

import (
	"bytes"
	"testing"
)

func TestNew_KeyLengthValidation(t *testing.T) {
	if _, err := New(make([]byte, 31)); err == nil {
		t.Error("31-byte key should error")
	}
	if _, err := New(make([]byte, 33)); err == nil {
		t.Error("33-byte key should error")
	}
	if _, err := New(make([]byte, 32)); err != nil {
		t.Errorf("32-byte key should work, got %v", err)
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	c, err := New(make([]byte, 32))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	plain := []byte("sk-test-1234567890")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == string(plain) {
		t.Error("ciphertext should differ from plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(plain, dec) {
		t.Errorf("got %q, want %q", dec, plain)
	}
}

func TestEncrypt_EmptyReturnsEmpty(t *testing.T) {
	c, _ := New(make([]byte, 32))
	enc, err := c.Encrypt(nil)
	if err != nil || enc != "" {
		t.Errorf("empty plain → empty, got (%q, %v)", enc, err)
	}
	dec, err := c.Decrypt("")
	if err != nil || dec != nil {
		t.Errorf("empty enc → nil, got (%v, %v)", dec, err)
	}
}

func TestEncrypt_UniqueNonce(t *testing.T) {
	c, _ := New(make([]byte, 32))
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if a == b {
		t.Error("identical plaintext should produce different ciphertext (unique nonce)")
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	c, _ := New(make([]byte, 32))
	enc, _ := c.Encrypt([]byte("secret"))
	// Flip a character; GCM auth should reject.
	broken := []byte(enc)
	broken[len(broken)-1] ^= 0x01
	if _, err := c.Decrypt(string(broken)); err == nil {
		t.Error("tampered ciphertext should fail")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	c, _ := New(make([]byte, 32))
	if _, err := c.Decrypt("!!not-base64!!"); err == nil {
		t.Error("invalid base64 should fail")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	c, _ := New(make([]byte, 32))
	// Legal base64 but shorter than nonce size.
	if _, err := c.Decrypt("AA"); err == nil {
		t.Error("too-short ciphertext should fail")
	}
}

func TestDeriveKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"exact 32", "12345678901234567890123456789012", 32},
		{"over 32 truncated", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 32},
		{"short padded", "abc", 32},
		{"single char", "x", 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveKey(tt.in); len(got) != tt.want {
				t.Errorf("len=%d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	if !bytes.Equal(DeriveKey("abc"), DeriveKey("abc")) {
		t.Error("DeriveKey should be deterministic")
	}
}

func TestNoop_PassThrough(t *testing.T) {
	c := Noop()
	enc, err := c.Encrypt([]byte("plain"))
	if err != nil {
		t.Fatalf("noop encrypt: %v", err)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("noop decrypt: %v", err)
	}
	if string(dec) != "plain" {
		t.Errorf("noop should roundtrip, got %q", dec)
	}
}

func TestStringHelpers(t *testing.T) {
	c, _ := New(make([]byte, 32))
	enc, err := c.EncryptString("hello")
	if err != nil {
		t.Fatalf("encrypt string: %v", err)
	}
	got, err := c.DecryptString(enc)
	if err != nil {
		t.Fatalf("decrypt string: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}
