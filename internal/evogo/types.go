// Package evogo implementa o cliente HTTP do evolution-go (REST API e webhook events).
package evogo

// ─── Webhook payload ─────────────────────────────────────────────────────

// WebhookEnvelope é o payload base de qualquer evento evolution-go.
type WebhookEnvelope struct {
	Event    string `json:"event"` // "MESSAGES_UPSERT", "MESSAGES_UPDATE", etc.
	Instance string `json:"instance"`
	Data     any    `json:"data"`
}

// MessagesUpsertData é o data de MESSAGES_UPSERT.
type MessagesUpsertData struct {
	Key struct {
		RemoteJID string `json:"remoteJid"`
		FromMe    bool   `json:"fromMe"`
		ID        string `json:"id"`
	} `json:"key"`
	PushName         string         `json:"pushName"`
	Message          map[string]any `json:"message"`
	MessageType      string         `json:"messageType"`
	MessageTimestamp int64          `json:"messageTimestamp"`
}

// ConnectionUpdateData é o data de CONNECTION_UPDATE.
type ConnectionUpdateData struct {
	State    string `json:"state"` // "open" | "close" | "connecting"
	Instance string `json:"instance"`
}

// ─── Send text (POST /message/sendText/{instance}) ───────────────────────

// SendTextRequest é o body de sendText.
type SendTextRequest struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	Delay  int    `json:"delay,omitempty"` // ms
}

// SendTextResponse é a resposta genérica (depende da versão).
type SendTextResponse struct {
	Key struct {
		RemoteJID string `json:"remoteJid"`
		FromMe    bool   `json:"fromMe"`
		ID        string `json:"id"`
	} `json:"key"`
	Message          any    `json:"message"`
	MessageTimestamp int64  `json:"messageTimestamp"`
	Status           string `json:"status"`
}

// ─── Send media (POST /message/sendMedia/{instance}) ─────────────────────

// SendMediaRequest é o body de sendMedia.
// Media pode ser URL (http) ou base64.
type SendMediaRequest struct {
	Number    string `json:"number"`
	MediaType string `json:"mediatype"` // "image" | "audio" | "video" | "document" | "sticker"
	Media     string `json:"media"`     // URL ou base64
	FileName  string `json:"fileName,omitempty"`
	Caption   string `json:"caption,omitempty"`
	MimeType  string `json:"mimetype,omitempty"`
	Delay     int    `json:"delay,omitempty"`
}

// ─── Webhook set (POST /webhook/set/{instance}) ──────────────────────────

// WebhookSetRequest configura o webhook global da instância.
type WebhookSetRequest struct {
	URL             string   `json:"url"`
	WebhookByEvents bool     `json:"webhook_by_events"`
	WebhookBase64   bool     `json:"webhook_base64"`
	Events          []string `json:"events"`
}

// ─── Connect (POST /instance/connect) ────────────────────────────────────

// ConnectRequest inicia a conexão (e pareamento QR) da instância.
type ConnectRequest struct {
	Number string `json:"number,omitempty"` // pode ser vazio
}

// ConnectResponse inclui o QR code (base64) e status.
type ConnectResponse struct {
	Code         string `json:"code,omitempty"`
	Base64       string `json:"base64,omitempty"`
	Count        int    `json:"count,omitempty"`
	InstanceName string `json:"instanceName"`
	State        string `json:"state,omitempty"` // "open" | "close" | "connecting"
}

// ─── Instance status (GET /instance/status/{instance}) ───────────────────

// InstanceStatus é a resposta de status.
type InstanceStatus struct {
	Instance struct {
		Name      string `json:"name"`
		State     string `json:"state"`
		ServerURL string `json:"serverUrl"`
		APKey     string `json:"apikey"` // echo
		OwnerJID  string `json:"ownerJid,omitempty"`
	} `json:"instance"`
}
