package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// VerifySignature valida o contrato de webhook do Chatwoot 4.16.2:
// sha256=HMAC-SHA256(secret, timestamp+"."+body). O timestamp também limita
// replay de mensagens antigas ou excessivamente adiantadas.
func VerifySignature(headerValue, timestamp, secret string, body []byte, now time.Time, replayWindow time.Duration) bool {
	if headerValue == "" || timestamp == "" || secret == "" || replayWindow <= 0 {
		return false
	}

	unixTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(unixTimestamp, 0))
	if delta < -replayWindow || delta > replayWindow {
		return false
	}

	digestHex, ok := strings.CutPrefix(headerValue, "sha256=")
	if !ok {
		return false
	}
	provided, err := hex.DecodeString(digestHex)
	if err != nil || len(provided) != sha256.Size {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}
