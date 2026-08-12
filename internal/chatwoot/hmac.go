package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifySignature verifica a assinatura do webhook do Chatwoot.
//
// O Chatwoot, para API Channel inboxes, envia o token HMAC diretamente no
// header `X-Chatwoot-Signature` (não um HMAC do body). Para máxima
// compatibilidade, suportamos DOIS modos:
//
//   - Mode "plain": o header é comparado diretamente ao token (compara
//     constant-time).
//   - Mode "hmac":  o header é `hex(hmac_sha256(body, token))` (modo avançado,
//     se o operador configurou manualmente no Chatwoot).
//
// `mode` pode ser "plain" (default) ou "hmac".
func VerifySignature(headerValue, token, body, mode string) bool {
	if headerValue == "" || token == "" {
		return false
	}
	switch mode {
	case "hmac":
		mac := hmac.New(sha256.New, []byte(token))
		mac.Write([]byte(body))
		expected := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(headerValue))
	default: // "plain"
		return hmac.Equal([]byte(headerValue), []byte(token))
	}
}
