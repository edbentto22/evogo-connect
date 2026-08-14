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
		FromMe    *bool  `json:"fromMe"`
		ID        string `json:"id"`
	} `json:"key"`
	PushName         string         `json:"pushName"`
	Message          map[string]any `json:"message"`
	MessageType      string         `json:"messageType"`
	MessageTimestamp int64          `json:"messageTimestamp"`
	// Info é o formato emitido por versões recentes do Evolution Go, baseado
	// diretamente em whatsmeow. Ele é mantido ao lado de Key para preservar a
	// compatibilidade com os dois contratos de webhook.
	Info struct {
		ID       string          `json:"id"`
		Chat     json.RawMessage `json:"chat"`
		Sender   json.RawMessage `json:"sender"`
		IsFromMe *bool           `json:"isFromMe"`
		PushName string          `json:"pushName"`
	} `json:"info"`
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
	data, content, own, reason, accepted, err := e.DirectTextWithReason()
	if err != nil || !accepted {
		return data, content, reason, accepted, err
	}
	if own {
		return data, "", "own_message", false, nil
	}
	return data, content, "", true, nil
}

// DirectTextWithReason extrai um texto direto, independentemente de ele ter
// sido enviado pelo número conectado ou recebido por ele. O booleano own só é
// verdadeiro quando o payload identifica explicitamente uma mensagem própria.
// Chamadores que só aceitam mensagens de clientes devem usar IncomingText.
func (e WebhookEnvelope) DirectTextWithReason() (MessagesUpsertData, string, bool, string, bool, error) {
	var zero MessagesUpsertData
	event := strings.ToUpper(strings.TrimSpace(e.Event))
	if event != "MESSAGES_UPSERT" && event != "MESSAGES.UPSERT" && event != "MESSAGE" {
		return zero, "", false, "unsupported_event", false, nil
	}
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return zero, "", false, "invalid_payload", false, fmt.Errorf("evogo: webhook data is required")
	}
	var data MessagesUpsertData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return zero, "", false, "invalid_payload", false, fmt.Errorf("evogo: decode message data: %w", err)
	}
	normalizeMessageInfo(&data)
	jid := strings.ToLower(strings.TrimSpace(data.Key.RemoteJID))
	if strings.HasSuffix(jid, "@g.us") || strings.HasSuffix(jid, "@broadcast") || strings.HasSuffix(jid, "@newsletter") {
		return data, "", false, "non_direct_message", false, nil
	}
	// A estrutura da mensagem é o contrato confiável: alguns builds da
	// Evolution Go variam o rótulo messageType, mas preservam conversation ou
	// extendedTextMessage.text para textos diretos.
	content, messageIsText := incomingMessageText(data.Message)
	if !messageIsText || strings.TrimSpace(content) == "" {
		return data, "", false, "unsupported_message_structure", false, nil
	}
	if strings.TrimSpace(data.Key.ID) == "" || strings.TrimSpace(data.Key.RemoteJID) == "" {
		return zero, "", false, "invalid_payload", false, fmt.Errorf("evogo: message key is required (%s)", webhookDataShape(e.Data))
	}
	return data, content, data.IsFromMe(), "", true, nil
}

// normalizeMessageInfo adapta o payload nativo do Evolution Go (info) para o
// contrato interno usado pelo bridge. A chave legada tem precedência quando
// estiver completa.
func normalizeMessageInfo(data *MessagesUpsertData) {
	if strings.TrimSpace(data.Key.ID) == "" {
		data.Key.ID = data.Info.ID
	}
	if strings.TrimSpace(data.Key.RemoteJID) == "" {
		data.Key.RemoteJID = selectInfoJID(data.Info.Chat, data.Info.Sender)
	}
	if strings.TrimSpace(data.PushName) == "" {
		data.PushName = data.Info.PushName
	}
}

// IsFromMe é deliberadamente conservador: se qualquer contrato disponível
// identificar a mensagem como própria, ela jamais é encaminhada ao Chatwoot.
func (data MessagesUpsertData) IsFromMe() bool {
	return data.Key.FromMe != nil && *data.Key.FromMe || data.Info.IsFromMe != nil && *data.Info.IsFromMe
}

func selectInfoJID(chatRaw, senderRaw json.RawMessage) string {
	chat := webhookJID(chatRaw)
	if isNonDirectJID(chat) || isDirectJID(chat) {
		return chat
	}
	sender := webhookJID(senderRaw)
	if isDirectJID(sender) {
		return sender
	}
	if chat != "" {
		return chat
	}
	return sender
}

func isDirectJID(jid string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(jid)), "@s.whatsapp.net")
}

func isNonDirectJID(jid string) bool {
	jid = strings.ToLower(strings.TrimSpace(jid))
	return strings.HasSuffix(jid, "@g.us") || strings.HasSuffix(jid, "@broadcast") || strings.HasSuffix(jid, "@newsletter")
}

// webhookJID converte o JID serializado pela Evolution Go, aceitando tanto o
// objeto documentado {user, server} quanto uma representação textual.
func webhookJID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	var jid struct {
		User   string `json:"user"`
		Server string `json:"server"`
	}
	if err := json.Unmarshal(raw, &jid); err != nil {
		return ""
	}
	user := strings.TrimSpace(jid.User)
	server := strings.TrimSpace(jid.Server)
	if user == "" || server == "" {
		return ""
	}
	return user + "@" + server
}

// webhookDataShape descreve somente a presença de campos estruturais esperados.
// Nunca inclui valores recebidos, preservando conteúdo, JID e nome do contato.
func webhookDataShape(raw json.RawMessage) string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "data_not_object"
	}
	_, hasKey := object["key"]
	_, hasMessage := object["message"]
	_, hasData := object["data"]
	_, hasInfo := object["info"]
	return fmt.Sprintf("has_key=%t has_message=%t has_data=%t has_info=%t", hasKey, hasMessage, hasData, hasInfo)
}

func incomingMessageText(message map[string]any) (string, bool) {
	if content, ok := message["conversation"].(string); ok {
		if strings.TrimSpace(content) != "" {
			return content, true
		}
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
