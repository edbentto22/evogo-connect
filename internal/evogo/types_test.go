package evogo

import (
	"encoding/json"
	"testing"
)

func TestSendTextRequestJSON(t *testing.T) {
	r := SendTextRequest{Number: "5511999999999", Text: "oi"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// delay=0 é omitido por `omitempty`
	want := `{"number":"5511999999999","text":"oi"}`
	if string(b) != want {
		t.Errorf("got %s, want %s", string(b), want)
	}
}

func TestWebhookSetRequestJSON(t *testing.T) {
	r := WebhookSetRequest{
		URL:    "https://example.com/wh",
		Events: []string{"MESSAGES_UPSERT", "CONNECTION_UPDATE"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if got != `{"url":"https://example.com/wh","webhook_by_events":false,"webhook_base64":false,"events":["MESSAGES_UPSERT","CONNECTION_UPDATE"]}` {
		t.Errorf("got %s", got)
	}
}

func TestWebhookEnvelopeDecode(t *testing.T) {
	raw := `{"event":"MESSAGES_UPSERT","instance":"demo","data":{"key":{"remoteJid":"5511@s.whatsapp.net","fromMe":false,"id":"ABC"},"messageType":"conversation","pushName":"João"}}`
	var env WebhookEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Event != "MESSAGES_UPSERT" {
		t.Errorf("event=%q", env.Event)
	}
	if env.Instance != "demo" {
		t.Errorf("instance=%q", env.Instance)
	}
}
