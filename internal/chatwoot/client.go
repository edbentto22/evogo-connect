package chatwoot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client é o cliente HTTP do Chatwoot (server-side, não client-facing).
type Client struct {
	baseURL    string
	accountID  int
	token      string
	httpClient *http.Client
}

// NewClient cria um client. token = api_access_token (Profile → Access Token
// ou Platform App).
func NewClient(baseURL string, accountID int, token string) *Client {
	return &Client{
		baseURL:   baseURL,
		accountID: accountID,
		token:     token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ─── Inboxes ─────────────────────────────────────────────────────────────

// CreateAPIInbox cria uma inbox API Channel apontando pro nosso webhook.
func (c *Client) CreateAPIInbox(ctx context.Context, name, webhookURL string) (*InboxResponse, error) {
	payload := InboxCreatePayload{
		Name:                 name,
		GreetingEnabled:      false,
		EnableAutoAssignment: true,
		Channel: InboxChannel{
			Type:          "api",
			WebhookURL:    webhookURL,
			HMACMandatory: true,
		},
	}
	var out InboxResponse
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/accounts/%d/inboxes", c.accountID), payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create inbox: %w", err)
	}
	return &out, nil
}

// GetInbox busca dados de uma inbox.
func (c *Client) GetInbox(ctx context.Context, inboxID int) (*InboxResponse, error) {
	var out InboxResponse
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/inboxes/%d", c.accountID, inboxID), nil, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: get inbox: %w", err)
	}
	return &out, nil
}

// ─── Contacts ────────────────────────────────────────────────────────────

// CreateContact cria um contato numa inbox específica, com source_id = JID.
// O Chatwoot deduz automaticamente o contact_inbox; source_id é a chave
// estável que sobrevive a renomeação do contato.
func (c *Client) CreateContact(ctx context.Context, inboxID int, name, sourceID string) (*ContactResponse, error) {
	payload := ContactCreatePayload{
		Name:     name,
		InboxID:  inboxID,
		SourceID: sourceID,
	}
	var out ContactResponse
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/accounts/%d/contacts", c.accountID), payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create contact: %w", err)
	}
	return &out, nil
}

// FindContactBySourceID busca um contato pelo source_id (JID) numa inbox.
// Retorna ErrNotFound se não existe.
func (c *Client) FindContactBySourceID(ctx context.Context, inboxID int, sourceID string) (*ContactResponse, error) {
	var out struct {
		Payload []ContactResponse `json:"payload"`
	}
	path := fmt.Sprintf("/api/v1/accounts/%d/contacts/search?q=%s", c.accountID, sourceID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: search contact: %w", err)
	}
	for _, ct := range out.Payload {
		if ct.SourceID == sourceID {
			return &ct, nil
		}
	}
	return nil, ErrNotFound
}

// ErrNotFound é retornado quando o recurso não existe.
var ErrNotFound = fmt.Errorf("chatwoot: not found")

// ─── Conversations ───────────────────────────────────────────────────────

// CreateConversation cria uma conversa para um contato.
func (c *Client) CreateConversation(ctx context.Context, sourceID string, contactID, inboxID int) (*ConversationResponse, error) {
	payload := ConversationCreatePayload{
		SourceID:  sourceID,
		ContactID: contactID,
		InboxID:   inboxID,
		Status:    "open",
	}
	var out ConversationResponse
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/accounts/%d/conversations", c.accountID), payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create conversation: %w", err)
	}
	return &out, nil
}

// ─── Messages ────────────────────────────────────────────────────────────

// CreateIncomingMessage posta uma mensagem incoming (do cliente) na conversa.
func (c *Client) CreateIncomingMessage(ctx context.Context, conversationID int64, content string, contentType string) error {
	payload := MessageCreatePayload{
		Content:     content,
		MessageType: "incoming",
		Private:     false,
		ContentType: contentType,
	}
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.accountID, conversationID)
	if err := c.do(ctx, http.MethodPost, path, payload, nil); err != nil {
		return fmt.Errorf("chatwoot: create incoming message: %w", err)
	}
	return nil
}

// ─── HTTP transport ──────────────────────────────────────────────────────

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("chatwoot: marshal: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("chatwoot: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("api_access_token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chatwoot: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("chatwoot: %s %s → %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("chatwoot: unmarshal: %w (body: %s)", err, string(respBody))
		}
	}
	return nil
}
