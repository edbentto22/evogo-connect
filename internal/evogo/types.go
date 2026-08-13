// Package evogo implementa o cliente HTTP do Evolution Go.
package evogo

// WebhookEnvelope é o payload base de eventos do Evolution Go.
type WebhookEnvelope struct {
	Event    string `json:"event"`
	Instance string `json:"instance"`
	Data     any    `json:"data"`
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
