package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func signForTest(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	secret := "webhook-secret"
	body := []byte(`{"event":"message_created","id":1}`)
	valid := signForTest(secret, timestamp, body)

	tests := []struct {
		name      string
		signature string
		timestamp string
		secret    string
		body      []byte
		now       time.Time
		want      bool
	}{
		{name: "valid", signature: valid, timestamp: timestamp, secret: secret, body: body, now: now, want: true},
		{name: "altered body", signature: valid, timestamp: timestamp, secret: secret, body: append(body, ' '), now: now},
		{name: "wrong secret", signature: valid, timestamp: timestamp, secret: "wrong", body: body, now: now},
		{name: "missing prefix", signature: valid[len("sha256="):], timestamp: timestamp, secret: secret, body: body, now: now},
		{name: "invalid hex", signature: "sha256=invalid", timestamp: timestamp, secret: secret, body: body, now: now},
		{name: "expired", signature: valid, timestamp: timestamp, secret: secret, body: body, now: now.Add(6 * time.Minute)},
		{name: "too far in future", signature: valid, timestamp: timestamp, secret: secret, body: body, now: now.Add(-6 * time.Minute)},
		{name: "invalid timestamp", signature: valid, timestamp: "not-unix", secret: secret, body: body, now: now},
		{name: "empty secret", signature: valid, timestamp: timestamp, body: body, now: now},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, VerifySignature(tt.signature, tt.timestamp, tt.secret, tt.body, tt.now, 5*time.Minute))
		})
	}
}
