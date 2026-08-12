package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignaturePlain(t *testing.T) {
	token := "secret-token-123"
	body := `{"event":"message_created","id":1}`
	if !VerifySignature(token, token, body, "plain") {
		t.Error("plain mode: expected match")
	}
	if VerifySignature("wrong", token, body, "plain") {
		t.Error("plain mode: should not match wrong signature")
	}
	if VerifySignature("", token, body, "plain") {
		t.Error("plain mode: empty header should not match")
	}
	if VerifySignature(token, "", body, "plain") {
		t.Error("plain mode: empty token should not match")
	}
}

func TestVerifySignatureHMAC(t *testing.T) {
	token := "secret-token-123"
	body := `{"event":"message_created","id":1}`

	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(body))
	good := hex.EncodeToString(mac.Sum(nil))

	if !VerifySignature(good, token, body, "hmac") {
		t.Error("hmac mode: expected match for valid digest")
	}
	if VerifySignature("deadbeef", token, body, "hmac") {
		t.Error("hmac mode: should not match bad digest")
	}
	// Body alterado não deve bater
	if VerifySignature(good, token, body+` `, "hmac") {
		t.Error("hmac mode: should not match altered body")
	}
}

func TestVerifySignatureDefaultsToPlain(t *testing.T) {
	token := "abc"
	body := `{}`
	if !VerifySignature("abc", token, body, "") {
		t.Error("default mode: should behave like plain")
	}
	if !VerifySignature("abc", token, body, "unknown-mode") {
		t.Error("unknown mode: should fall back to plain")
	}
}
