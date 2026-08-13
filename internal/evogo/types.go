// Package evogo implementa o cliente HTTP do Evolution Go.
package evogo

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WebhookEnvelope é o payload base de eventos do Evolution Go.
type WebhookEnvelope struct {
	Event    string          `json:"event"`
	Instance string          `json:"instance"`
	Data     json.RawMessage `json:"data"`
}

// MessagesUpsertData representa dados de uma mensagem recebida.
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

// IncomingText extrai somente o contrato seguro de mensagem direta em texto.
// Evolution Go pode nomear o evento em maiúsculas ou minúsculas; o formato de
// dados é o mesmo, conforme o contrato whatsmeow/Fazer.ai homologado.
func (e WebhookEnvelope) IncomingText() (MessagesUpsertData, string, bool, error) {
	data, content, _, accepted, err := e.IncomingTextWithReason()
	return data, content, accepted, err
}

// IncomingTextWithReason devolve um motivo técnico fixo quando o webhook não
// deve ser encaminhado. O motivo nunca contém conteúdo, JID ou outro dado do
// payload e pode ser usado com segurança em logs e métricas.
func (e WebhookEnvelope) IncomingTextWithReason() (MessagesUpsertData, string, string, bool, error) {
	var zero MessagesUpsertData
	event := strings.ToUpper(strings.TrimSpace(e.Event))
	if event != "MESSAGES_UPSERT" && event != "MESSAGES.UPSERT" && event != "MESSAGE" {
		return zero, "", "unsupported_event", false, nil
	}
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return zero, "", "invalid_payload", false, fmt.Errorf("evogo: webhook data is required")
	}
	var data MessagesUpsertData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return zero, "", "invalid_payload", false, fmt.Errorf("evogo: decode message data: %w", err)
	}
	jid := strings.ToLower(strings.TrimSpace(data.Key.RemoteJID))
	if data.Key.FromMe {
		return data, "", "own_message", false, nil
	}
	if strings.HasSuffix(jid, "@g.us") || strings.HasSuffix(jid, "@broadcast") || strings.HasSuffix(jid, "@newsletter") {
		return data, "", "non_direct_message", false, nil
	}
	content, messageIsText := incomingMessageText(data.Message)
	messageType := strings.ToLower(strings.TrimSpace(data.MessageType))
	if !messageIsText || strings.TrimSpace(content) == "" || (messageType != "conversation" && messageType != "extendedtextmessage") {
		return data, "", "unsupported_message_type", false, nil
	}
	if strings.TrimSpace(data.Key.ID) == "" || strings.TrimSpace(data.Key.RemoteJID) == "" {
		return zero, "", "invalid_payload", false, fmt.Errorf("evogo: message key is required")
	}
	return data, content, "", true, nil
}

func incomingMessageText(message map[string]any) (string, bool) {
	if content, ok := message["conversation"].(string); ok {
		return content, true
	}
	extended, ok := message["extendedTextMessage"].(map[string]any)
	if !ok {
		return "", false
	}
	content, ok := extended["text"].(string)
	return content, ok
}

// ConnectionUpdateData representa uma mudança de estado da conexão.
type ConnectionUpdateData struct {
	State    string `json:"state"`
	Instance string `json:"instance"`
}

// SendTextRequest é o body de POST /send/text.
type SendTextRequest struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	ID     string `json:"id,omitempty"`
	Delay  int    `json:"delay,omitempty"`
}

// SendTextResponse é a resposta de envio de mensagem.
type SendTextResponse struct {
	Message string `json:"message"`
	Data    struct {
		Info struct {
			ID        string `json:"ID"`
			ServerID  int64  `json:"ServerID"`
			Timestamp string `json:"Timestamp"`
			Type      string `json:"Type"`
		} `json:"Info"`
	} `json:"data"`
}

// SendMediaRequest é o body de POST /send/media.
type SendMediaRequest struct {
	Number   string `json:"number"`
	URL      string `json:"url"`
	Type     string `json:"type"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	ID       string `json:"id,omitempty"`
	Delay    int    `json:"delay,omitempty"`
}

// ConnectRequest é o body de POST /instance/connect.
type ConnectRequest struct {
	WebhookURL      string   `json:"webhookUrl,omitempty"`
	Subscribe       []string `json:"subscribe,omitempty"`
	Immediate       bool     `json:"immediate,omitempty"`
	Phone           string   `json:"phone,omitempty"`
	RabbitMQEnable  string   `json:"rabbitmqEnable,omitempty"`
	WebsocketEnable string   `json:"websocketEnable,omitempty"`
	NATSEnable      string   `json:"natsEnable,omitempty"`
}

// ConnectResponse descreve o resultado do pareamento/conexão.
type ConnectResponse struct {
	Message string `json:"message"`
	Data    struct {
		JID         string `json:"jid"`
		WebhookURL  string `json:"webhookUrl"`
		EventString string `json:"eventString"`
	} `json:"data"`
}

// InstanceStatus descreve o estado da instância associada ao token.
type InstanceStatus struct {
	Message string `json:"message"`
	Data    struct {
		Connected bool   `json:"connected"`
		LoggedIn  bool   `json:"loggedIn"`
		Name      string `json:"name"`
		MyJID     string `json:"myJid"`
	} `json:"data"`
}
