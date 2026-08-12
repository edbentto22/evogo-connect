package chatwoot

import (
	"encoding/json"
	"testing"
)

// TestWebhookEnvelopeDecode garante que o payload típico do Chatwoot é
// parseado corretamente (defesa contra mudanças upstream).
func TestWebhookEnvelopeDecode(t *testing.T) {
	raw := `{
		"event": "message_created",
		"message_type": "outgoing",
		"id": 12345,
		"content": "Olá, como podemos ajudar?",
		"content_type": "text",
		"private": false,
		"created_at": 1698765432,
		"inbox_id": 7,
		"account_id": 1,
		"conversation": {
			"id": 678,
			"inbox_id": 7,
			"account_id": 1,
			"status": "open",
			"contact_inbox": {
				"source_id": "5511999999999@s.whatsapp.net",
				"inbox_id": 7,
				"contact_id": 42
			}
		},
		"sender": {
			"type": "User",
			"id": 1,
			"name": "Agente"
		},
		"attachments": []
	}`
	var env WebhookEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Event != "message_created" {
		t.Errorf("event=%q", env.Event)
	}
	if env.MessageType != "outgoing" {
		t.Errorf("message_type=%q", env.MessageType)
	}
	if env.ID != 12345 {
		t.Errorf("id=%d", env.ID)
	}
	if env.Conversation.ContactInbox.SourceID != "5511999999999@s.whatsapp.net" {
		t.Errorf("source_id=%q", env.Conversation.ContactInbox.SourceID)
	}
	if env.Conversation.ContactInbox.InboxID != 7 {
		t.Errorf("contact_inbox.inbox_id=%d", env.Conversation.ContactInbox.InboxID)
	}
	if env.InboxID != 7 {
		t.Errorf("top-level inbox_id=%d", env.InboxID)
	}
}

// TestWebhookEnvelopeWithAttachment cobre mídia.
func TestWebhookEnvelopeWithAttachment(t *testing.T) {
	raw := `{
		"event": "message_created",
		"message_type": "outgoing",
		"id": 1,
		"content": "segue o doc",
		"conversation": {
			"id": 1,
			"contact_inbox": {"source_id":"5511@s.whatsapp.net","inbox_id":1,"contact_id":1}
		},
		"attachments": [
			{"id":99,"file_type":"file","file_url":"https://example.com/doc.pdf","file_name":"doc.pdf"}
		]
	}`
	var env WebhookEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Attachments) != 1 {
		t.Fatalf("got %d attachments", len(env.Attachments))
	}
	if env.Attachments[0].FileType != "file" {
		t.Errorf("file_type=%q", env.Attachments[0].FileType)
	}
}
