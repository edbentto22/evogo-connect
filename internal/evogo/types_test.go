package evogo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendTextRequestJSON(t *testing.T) {
	b, err := json.Marshal(SendTextRequest{Number: "5511999999999", Text: "oi"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"number":"5511999999999","text":"oi"}`, string(b))
}

func TestSendMediaRequestJSON(t *testing.T) {
	b, err := json.Marshal(SendMediaRequest{
		Number:   "5511999999999",
		URL:      "https://example.com/doc.pdf",
		Type:     "document",
		Filename: "doc.pdf",
		Caption:  "arquivo",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"number":"5511999999999",
		"url":"https://example.com/doc.pdf",
		"type":"document",
		"filename":"doc.pdf",
		"caption":"arquivo"
	}`, string(b))
}

func TestConnectRequestJSON(t *testing.T) {
	b, err := json.Marshal(ConnectRequest{
		WebhookURL: "https://connector.example.com/webhook/evo/demo",
		Subscribe:  []string{"MESSAGE", "CONNECTION"},
		Immediate:  true,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"webhookUrl":"https://connector.example.com/webhook/evo/demo",
		"subscribe":["MESSAGE","CONNECTION"],
		"immediate":true
	}`, string(b))
}

func TestWebhookEnvelopeDecode(t *testing.T) {
	raw := `{"event":"MESSAGES_UPSERT","instance":"demo","data":{"key":{"remoteJid":"5511@s.whatsapp.net","fromMe":false,"id":"ABC"},"messageType":"conversation","pushName":"João"}}`
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	assert.Equal(t, "MESSAGES_UPSERT", env.Event)
	assert.Equal(t, "demo", env.Instance)
}
