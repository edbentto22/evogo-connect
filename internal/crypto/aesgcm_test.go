package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// helper: gera 32 bytes aleatórios em base64
func newTestKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, KeySize)
	for i := range b {
		b[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := newTestKey(t)
	c, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []string{
		"",
		"hello",
		"5511999999999@s.whatsapp.net",
		"eyJhbGciOiJIUzI1NiJ9.payload.signature",
		"uma string com acentos: ção, não, então, ã, é, í, ó, ú",
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			ct, err := c.EncryptString(s)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			// ciphertext deve ser nonce (12) + ct + tag (16)
			if len(ct) < NonceSize+16 {
				t.Fatalf("ciphertext too short: %d bytes", len(ct))
			}
			pt, err := c.DecryptString(ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if pt != s {
				t.Errorf("got %q, want %q", pt, s)
			}
		})
	}
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	// Mesma plaintext deve produzir ciphertexts diferentes (nonce aleatório)
	key := newTestKey(t)
	c, _ := New(key)
	ct1, _ := c.EncryptString("same")
	ct2, _ := c.EncryptString("same")
	if bytes.Equal(ct1, ct2) {
		t.Error("expected different ciphertexts (nonce reuse?)")
	}
}

func TestNewRejectsShortKey(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := New(short); err == nil {
		t.Error("expected error for short key")
	}
}

func TestNewRejectsBadBase64(t *testing.T) {
	if _, err := New("!!!not base64!!!"); err == nil {
		t.Error("expected error for bad base64")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := newTestKey(t)
	c, _ := New(key)
	if _, err := c.Decrypt([]byte("abc")); err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	key := newTestKey(t)
	c, _ := New(key)
	ct, _ := c.EncryptString("secret")
	// Tamper no meio
	ct[len(ct)-1] ^= 0xFF
	if _, err := c.Decrypt(ct); err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}
