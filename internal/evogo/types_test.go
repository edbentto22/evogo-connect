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

func TestWebhookEnvelopeIncomingTextContract(t *testing.T) {
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"event":"MESSAGES_UPSERT","instance":"demo","data":{"key":{"remoteJid":"5511999999999@s.whatsapp.net","fromMe":false,"id":"ABC"},"message":{"conversation":"olá"},"messageType":"conversation"}}`), &env))
	message, content, accepted, err := env.IncomingText()
	require.NoError(t, err)
	assert.True(t, accepted)
	assert.Equal(t, "ABC", message.Key.ID)
	assert.Equal(t, "olá", content)
}

func TestWebhookEnvelopeIncomingTextAcceptsEvolutionDotEvent(t *testing.T) {
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"event":"messages.upsert","instance":"demo","data":{"key":{"remoteJid":"5511999999999@s.whatsapp.net","fromMe":false,"id":"ABC-dot"},"message":{"conversation":"olá"},"messageType":"conversation"}}`), &env))
	message, content, accepted, err := env.IncomingText()
	require.NoError(t, err)
	assert.True(t, accepted)
	assert.Equal(t, "ABC-dot", message.Key.ID)
	assert.Equal(t, "olá", content)
}

func TestWebhookEnvelopeIncomingTextRejectsUnsupportedEventPunctuation(t *testing.T) {
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"event":"messages---upsert","data":{"key":{"remoteJid":"5511999999999@s.whatsapp.net","fromMe":false,"id":"ABC"},"message":{"conversation":"olá"},"messageType":"conversation"}}`), &env))
	_, _, accepted, err := env.IncomingText()
	require.NoError(t, err)
	assert.False(t, accepted)
}

func TestWebhookEnvelopeIncomingTextReasonNeverContainsContent(t *testing.T) {
	const secretContent = "conteudo-que-nao-pode-ir-ao-log"
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"event":"MESSAGE","data":{"key":{"remoteJid":"5511999999999@s.whatsapp.net","fromMe":false,"id":"ABC"},"message":{"conversation":"`+secretContent+`"},"messageType":"imageMessage"}}`), &env))
	_, content, reason, accepted, err := env.IncomingTextWithReason()
	require.NoError(t, err)
	assert.False(t, accepted)
	assert.Empty(t, content)
	assert.Equal(t, "unsupported_message_type", reason)
	assert.NotContains(t, reason, secretContent)
}

func TestWebhookEnvelopeIncomingTextSupportsExtendedTextAndSkipsBroadcasts(t *testing.T) {
	var env WebhookEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"event":"MESSAGE","data":{"key":{"remoteJid":"5511@s.whatsapp.net","id":"x"},"message":{"extendedTextMessage":{"text":"oi"}},"messageType":"extendedTextMessage"}}`), &env))
	_, content, accepted, err := env.IncomingText()
	require.NoError(t, err)
	assert.True(t, accepted)
	assert.Equal(t, "oi", content)
	env.Data = json.RawMessage(`{"key":{"remoteJid":"status@broadcast","id":"x"},"message":{"conversation":"oi"},"messageType":"conversation"}`)
	_, _, accepted, err = env.IncomingText()
	require.NoError(t, err)
	assert.False(t, accepted)
}
