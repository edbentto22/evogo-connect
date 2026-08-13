// Package chatwoot implementa o cliente HTTP do Chatwoot e os tipos do
// webhook payload da API Channel inbox.
package chatwoot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WebhookTimestamp aceita os dois contratos observados em webhooks do
// Chatwoot: Unix timestamp numérico e timestamp serializado como string.
// Strings RFC3339 são normalizadas para Unix seconds, mantendo a semântica do
// contrato numérico original.
type WebhookTimestamp int64

func (t *WebhookTimestamp) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return fmt.Errorf("decode webhook timestamp: value is required")
	}

	if strings.HasPrefix(raw, `"`) {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return fmt.Errorf("decode webhook timestamp string: %w", err)
		}
		if seconds, err := strconv.ParseInt(text, 10, 64); err == nil {
			*t = WebhookTimestamp(seconds)
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return fmt.Errorf("decode webhook timestamp string: %w", err)
		}
		*t = WebhookTimestamp(parsed.Unix())
		return nil
	}

	var seconds int64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return fmt.Errorf("decode webhook timestamp: %w", err)
	}
	*t = WebhookTimestamp(seconds)
	return nil
}

// ─── Webhook payload (Chatwoot API Channel) ──────────────────────────────

// WebhookEnvelope é o payload que o Chatwoot POSTa no webhook_url da inbox.
type WebhookEnvelope struct {
	Event       string           `json:"event"`        // "message_created", "message_updated", "conversation_*"
	MessageType string           `json:"message_type"` // "incoming" | "outgoing"
	ID          int64            `json:"id"`           // ID da mensagem
	Content     string           `json:"content"`
	ContentType string           `json:"content_type"` // "text", "input_email", etc.
	Private     bool             `json:"private"`
	CreatedAt   WebhookTimestamp `json:"created_at"`

	Conversation ConversationRef `json:"conversation"`
	Sender       SenderRef       `json:"sender"`
	Attachments  []Attachment    `json:"attachments"`

	InboxID   int `json:"inbox_id"`   // redundante com conversation.contact_inbox.inbox_id
	AccountID int `json:"account_id"` // redundante
}

// ConversationRef é a referência à conversa.
type ConversationRef struct {
	ID           int64            `json:"id"`
	ContactInbox ContactInbox     `json:"contact_inbox"`
	Meta         ConversationMeta `json:"meta"`
	AccountID    int              `json:"account_id"`
	InboxID      int              `json:"inbox_id"`
	Status       string           `json:"status"`
	CreatedAt    WebhookTimestamp `json:"created_at"`
}

// ConversationMeta contém a representação do contato incluída pelo webhook
// do Chatwoot/Fazer.ai. Identifier é um fallback para forks que omitem
// contact_inbox.source_id na serialização de mensagens outgoing.
type ConversationMeta struct {
	Sender ConversationContact `json:"sender"`
}

// ConversationContact identifica o contato associado à conversa.
type ConversationContact struct {
	Identifier string `json:"identifier"`
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
	ID          int64  `json:"id"`
	FileType    string `json:"file_type"` // "image" | "audio" | "video" | "file"
	DataURL     string `json:"data_url"`
	ThumbURL    string `json:"thumb_url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Extension   string `json:"extension,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
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
	ID              int    `json:"id"`
	Name            string `json:"name"`
	ChannelID       int    `json:"channel_id"`
	ChannelType     string `json:"channel_type"`
	AccountID       int    `json:"account_id"`
	HMACToken       string `json:"hmac_token,omitempty"`
	Secret          string `json:"secret,omitempty"`
	WebhookURL      string `json:"webhook_url"`
	InboxIdentifier string `json:"inbox_identifier"`
}

// ─── Create contact (POST /api/v1/accounts/{account_id}/contacts) ────────

// ContactCreatePayload é o body para criar contato.
type ContactCreatePayload struct {
	Name             string         `json:"name"`
	Identifier       string         `json:"identifier"`
	CustomAttributes map[string]any `json:"custom_attributes,omitempty"`
}

// ContactResponse é a resposta de criar contato.
type ContactResponse struct {
	ID             int                    `json:"id"`
	Name           string                 `json:"name"`
	Identifier     string                 `json:"identifier"`
	ContactInboxes []ContactInboxResponse `json:"contact_inboxes"`
}

// ContactListResponse é o envelope retornado pelos endpoints de contatos.
type ContactListResponse struct {
	Payload []ContactResponse `json:"payload"`
	ID      int               `json:"id,omitempty"`
}

// ContactCreateResponse é o envelope retornado ao criar um contato.
type ContactCreateResponse struct {
	Payload struct {
		Contact      ContactResponse       `json:"contact"`
		ContactInbox *ContactInboxResponse `json:"contact_inbox,omitempty"`
	} `json:"payload"`
}

// ContactInboxResponse representa o vínculo entre contato e inbox.
type ContactInboxResponse struct {
	ID       int    `json:"id"`
	SourceID string `json:"source_id"`
	Inbox    struct {
		ID int `json:"id"`
	} `json:"inbox"`
}

// ContactInboxCreatePayload cria o source_id estável do canal API.
type ContactInboxCreatePayload struct {
	InboxID  int    `json:"inbox_id"`
	SourceID string `json:"source_id"`
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
}

// ConversationListResponse é o envelope paginado da listagem de conversas.
type ConversationListResponse struct {
	Data []ConversationResponse `json:"data"`
}

// UnmarshalJSON aceita as duas respostas usadas pelo Chatwoot e pelo fork
// Fazer.ai: `data: [...]` e `data: { payload: [...] }`.
func (r *ConversationListResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode conversations response: %w", err)
	}
	if err := json.Unmarshal(raw.Data, &r.Data); err == nil {
		return nil
	}
	var wrapped struct {
		Payload []ConversationResponse `json:"payload"`
	}
	if err := json.Unmarshal(raw.Data, &wrapped); err != nil {
		return fmt.Errorf("decode conversations response data: %w", err)
	}
	r.Data = wrapped.Payload
	return nil
}

// ─── Create message (POST /api/v1/accounts/{account_id}/conversations/{cid}/messages) ───

// MessageCreatePayload é o body para criar mensagem (incoming de cliente).
type MessageCreatePayload struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"` // "incoming" | "outgoing"
	Private     bool   `json:"private"`
	ContentType string `json:"content_type,omitempty"`
}
