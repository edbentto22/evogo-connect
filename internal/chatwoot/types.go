// Package chatwoot implementa o cliente HTTP do Chatwoot e os tipos do
// webhook payload da API Channel inbox.
package chatwoot

import (
	"time"
)

// ─── Webhook payload (Chatwoot API Channel) ──────────────────────────────

// WebhookEnvelope é o payload que o Chatwoot POSTa no webhook_url da inbox.
type WebhookEnvelope struct {
	Event       string `json:"event"`        // "message_created", "message_updated", "conversation_*"
	MessageType string `json:"message_type"` // "incoming" | "outgoing"
	ID          int64  `json:"id"`           // ID da mensagem
	Content     string `json:"content"`
	ContentType string `json:"content_type"` // "text", "input_email", etc.
	Private     bool   `json:"private"`
	CreatedAt   int64  `json:"created_at"` // unix seconds

	Conversation ConversationRef `json:"conversation"`
	Sender       SenderRef       `json:"sender"`
	Attachments  []Attachment    `json:"attachments"`

	InboxID   int `json:"inbox_id"`   // redundante com conversation.contact_inbox.inbox_id
	AccountID int `json:"account_id"` // redundante
}

// ConversationRef é a referência à conversa.
type ConversationRef struct {
	ID           int64        `json:"id"`
	ContactInbox ContactInbox `json:"contact_inbox"`
	AccountID    int          `json:"account_id"`
	InboxID      int          `json:"inbox_id"`
	Status       string       `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`
}

// ContactInbox referencia a session do contato na inbox.
type ContactInbox struct {
	SourceID  string `json:"source_id"` // aqui guardamos o JID do WhatsApp
	InboxID   int    `json:"inbox_id"`
	ContactID int    `json:"contact_id"`
}

// SenderRef identifica o remetente.
type SenderRef struct {
	Type      string `json:"type"` // "User" | "Contact" | "AgentBot" | "Captain::Assistant"
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"thumbnail,omitempty"`
}

// Attachment é um anexo da mensagem.
type Attachment struct {
	ID       int64  `json:"id"`
	FileType string `json:"file_type"` // "image" | "audio" | "video" | "file"
	FileURL  string `json:"file_url"`
	FileName string `json:"file_name"`
	MimeType string `json:"file_type_content_type,omitempty"`
}

// ─── Create inbox (POST /api/v1/accounts/{account_id}/inboxes) ────────────

// InboxCreatePayload é o body para criar inbox API.
type InboxCreatePayload struct {
	Name                 string       `json:"name"`
	GreetingEnabled      bool         `json:"greeting_enabled"`
	GreetingMessage      string       `json:"greeting_message,omitempty"`
	EnableAutoAssignment bool         `json:"enable_auto_assignment"`
	Channel              InboxChannel `json:"channel"`
}

// InboxChannel é o canal API.
type InboxChannel struct {
	Type          string `json:"type"` // "api"
	WebhookURL    string `json:"webhook_url"`
	HMACMandatory bool   `json:"hmac_mandatory"`
}

// InboxResponse é a resposta de criar inbox.
type InboxResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ChannelID int    `json:"channel_id"`
	AccountID int    `json:"account_id"`
	Channel   struct {
		ID         int    `json:"id"`
		WebWidget  string `json:"web_widget"`
		HMACToken  string `json:"hmac_token,omitempty"`
		WebhookURL string `json:"webhook_url"`
	} `json:"channel"`
}

// ─── Create contact (POST /api/v1/accounts/{account_id}/contacts) ────────

// ContactCreatePayload é o body para criar contato.
type ContactCreatePayload struct {
	Name             string         `json:"name"`
	InboxID          int            `json:"inbox_id"`
	SourceID         string         `json:"source_id"` // JID do WhatsApp
	CustomAttributes map[string]any `json:"custom_attributes,omitempty"`
}

// ContactResponse é a resposta de criar contato.
type ContactResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	SourceID string `json:"source_id"`
	InboxID  int    `json:"inbox_id"`
}

// ─── Create conversation (POST /api/v1/accounts/{account_id}/conversations) ───

// ConversationCreatePayload cria uma conversa para um contato (em inbox API).
type ConversationCreatePayload struct {
	SourceID  string `json:"source_id"`
	ContactID int    `json:"contact_id,omitempty"`
	InboxID   int    `json:"inbox_id"`
	Status    string `json:"status,omitempty"` // "open" | "resolved" | "pending" | "snoozed"
}

// ConversationResponse é a resposta de criar conversa.
type ConversationResponse struct {
	ID        int    `json:"id"`
	AccountID int    `json:"account_id"`
	InboxID   int    `json:"inbox_id"`
	Status    string `json:"status"`
	ContactID int    `json:"contact_id"`
}

// ─── Create message (POST /api/v1/accounts/{account_id}/conversations/{cid}/messages) ───

// MessageCreatePayload é o body para criar mensagem (incoming de cliente).
type MessageCreatePayload struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"` // "incoming" | "outgoing"
	Private     bool   `json:"private"`
	ContentType string `json:"content_type,omitempty"`
}
