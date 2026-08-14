package chatwoot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBody = 1 << 20

// ErrNotFound é retornado quando o recurso não existe.
var ErrNotFound = errors.New("chatwoot: not found")

// HTTPError representa uma resposta não-2xx sem expor o corpo do upstream.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("chatwoot: %s %s returned status %d", e.Method, e.Path, e.StatusCode)
}

// TransportError preserva a causa para errors.Is/As, mas evita que a URL
// (que pode conter um identifier/JID na query) apareça em logs.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "chatwoot: upstream transport failed" }
func (e *TransportError) Unwrap() error { return e.Err }

// Client é o cliente HTTP administrativo do Chatwoot.
type Client struct {
	baseURL    string
	accountID  int
	token      string
	httpClient *http.Client
}

// NewClient cria um client autenticado por api_access_token.
func NewClient(baseURL string, accountID int, token string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		accountID: accountID,
		token:     token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// CreateAPIInbox cria uma inbox API Channel apontando para o conector.
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
	path := fmt.Sprintf("/api/v1/accounts/%d/inboxes", c.accountID)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create inbox: %w", err)
	}
	return &out, nil
}

// GetInbox busca dados de uma inbox.
func (c *Client) GetInbox(ctx context.Context, inboxID int) (*InboxResponse, error) {
	var out InboxResponse
	path := fmt.Sprintf("/api/v1/accounts/%d/inboxes/%d", c.accountID, inboxID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: get inbox: %w", err)
	}
	return &out, nil
}

// CreateContact cria um contato identificado de forma estável pelo JID.
// O vínculo com a inbox é criado separadamente por EnsureContactInbox.
func (c *Client) CreateContact(ctx context.Context, name, identifier string) (*ContactResponse, error) {
	payload := ContactCreatePayload{Name: name, Identifier: identifier}
	var out ContactCreateResponse
	path := fmt.Sprintf("/api/v1/accounts/%d/contacts", c.accountID)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create contact: %w", err)
	}
	if out.Payload.Contact.ID == 0 {
		return nil, errors.New("chatwoot: create contact returned no contact")
	}
	return &out.Payload.Contact, nil
}

// FindContactByIdentifier busca um contato pelo identifier e valida a
// correspondência exata para não aceitar resultados parciais da busca.
func (c *Client) FindContactByIdentifier(ctx context.Context, identifier string) (*ContactResponse, error) {
	var out ContactListResponse
	query := url.Values{"q": []string{identifier}}.Encode()
	path := fmt.Sprintf("/api/v1/accounts/%d/contacts/search?%s", c.accountID, query)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: search contact: %w", err)
	}
	for i := range out.Payload {
		if out.Payload[i].Identifier == identifier {
			return &out.Payload[i], nil
		}
	}
	return nil, ErrNotFound
}

// EnsureContactInbox garante que o contato está vinculado à inbox e que o
// contact_inbox.source_id é exatamente o JID informado.
func (c *Client) EnsureContactInbox(ctx context.Context, contact *ContactResponse, inboxID int, sourceID string) (*ContactInboxResponse, error) {
	for i := range contact.ContactInboxes {
		ci := &contact.ContactInboxes[i]
		if ci.Inbox.ID == inboxID && ci.SourceID == sourceID {
			return ci, nil
		}
	}

	payload := ContactInboxCreatePayload{InboxID: inboxID, SourceID: sourceID}
	var out ContactInboxResponse
	path := fmt.Sprintf("/api/v1/accounts/%d/contacts/%d/contact_inboxes", c.accountID, contact.ID)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		createErr := err
		if contact.Identifier != "" {
			refreshed, findErr := c.FindContactByIdentifier(ctx, contact.Identifier)
			if findErr == nil {
				for i := range refreshed.ContactInboxes {
					ci := &refreshed.ContactInboxes[i]
					if ci.Inbox.ID == inboxID && ci.SourceID == sourceID {
						return ci, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("chatwoot: create contact inbox: %w", createErr)
	}
	if out.SourceID != sourceID || out.Inbox.ID != inboxID {
		return nil, errors.New("chatwoot: contact inbox returned unexpected binding")
	}
	return &out, nil
}

// CreateConversation cria uma conversa para um contato.
func (c *Client) CreateConversation(ctx context.Context, sourceID string, contactID, inboxID int) (*ConversationResponse, error) {
	payload := ConversationCreatePayload{SourceID: sourceID, ContactID: contactID, InboxID: inboxID, Status: "open"}
	var out ConversationResponse
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations", c.accountID)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		return nil, fmt.Errorf("chatwoot: create conversation: %w", err)
	}
	if out.ID == 0 || out.AccountID != c.accountID || out.InboxID != inboxID || out.Status != "open" {
		return nil, errors.New("chatwoot: conversation returned unexpected binding")
	}
	return &out, nil
}

// FindOpenConversation procura uma conversa aberta do contato na inbox. A API
// retorna resultados paginados; a primeira página contém as conversas recentes,
// incluindo a conversa aberta criada pelo connector.
func (c *Client) FindOpenConversation(ctx context.Context, contactID, inboxID int) (*ConversationResponse, error) {
	for page := 1; page <= 10; page++ {
		query := url.Values{"contact_id": []string{fmt.Sprintf("%d", contactID)}, "inbox_id": []string{fmt.Sprintf("%d", inboxID)}, "page": []string{fmt.Sprintf("%d", page)}}.Encode()
		var out ConversationListResponse
		path := fmt.Sprintf("/api/v1/accounts/%d/conversations?%s", c.accountID, query)
		if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("chatwoot: list contact conversations: %w", err)
		}
		for i := range out.Data {
			conversation := &out.Data[i]
			if conversation.InboxID == inboxID && conversation.Status == "open" {
				return conversation, nil
			}
		}
		if len(out.Data) == 0 {
			break
		}
	}
	return nil, ErrNotFound
}

// CreateIncomingMessage posta uma mensagem incoming na conversa.
func (c *Client) CreateIncomingMessage(ctx context.Context, conversationID int64, content, contentType string) error {
	payload := MessageCreatePayload{Content: content, MessageType: "incoming", Private: false, ContentType: contentType}
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.accountID, conversationID)
	if err := c.do(ctx, http.MethodPost, path, payload, nil); err != nil {
		return fmt.Errorf("chatwoot: create incoming message: %w", err)
	}
	return nil
}

// CreateOutgoingMessage posta uma mensagem pública outgoing e devolve seu ID
// estável no Chatwoot. O bridge usa esse ID para suprimir o webhook de retorno
// que, de outro modo, reenviaria uma mensagem manual ao WhatsApp.
func (c *Client) CreateOutgoingMessage(ctx context.Context, conversationID int64, content, contentType string) (int64, error) {
	payload := MessageCreatePayload{Content: content, MessageType: "outgoing", Private: false, ContentType: contentType}
	var out MessageResponse
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.accountID, conversationID)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		return 0, fmt.Errorf("chatwoot: create outgoing message: %w", err)
	}
	if out.ID <= 0 {
		return 0, errors.New("chatwoot: create outgoing message returned no id")
	}
	return out.ID, nil
}

// EnsureContactConversation cria ou reutiliza o contato, o vínculo da inbox e
// a conversa de API. O identifier é sempre o JID normalizado pelo bridge.
func (c *Client) EnsureContactConversation(ctx context.Context, name, jid string, inboxID int) (*ContactResponse, *ConversationResponse, error) {
	contact, err := c.FindContactByIdentifier(ctx, jid)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, nil, fmt.Errorf("chatwoot: find incoming contact: %w", err)
		}
		contact, err = c.CreateContact(ctx, name, jid)
		if err != nil {
			return nil, nil, fmt.Errorf("chatwoot: create incoming contact: %w", err)
		}
	}
	if _, err := c.EnsureContactInbox(ctx, contact, inboxID, jid); err != nil {
		return nil, nil, fmt.Errorf("chatwoot: ensure incoming contact inbox: %w", err)
	}
	conversation, err := c.FindOpenConversation(ctx, contact.ID, inboxID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, nil, fmt.Errorf("chatwoot: find incoming conversation: %w", err)
	}
	if errors.Is(err, ErrNotFound) {
		conversation, err = c.CreateConversation(ctx, jid, contact.ID, inboxID)
		if err != nil {
			return nil, nil, fmt.Errorf("chatwoot: create incoming conversation: %w", err)
		}
	}
	return contact, conversation, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("chatwoot: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("chatwoot: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("api_access_token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("chatwoot: read response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{Method: method, Path: req.URL.Path, StatusCode: resp.StatusCode}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("chatwoot: decode response: %w", err)
		}
	}
	return nil
}
