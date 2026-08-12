package evogo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client é o cliente HTTP do evolution-go.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient cria um client. apiKey = GLOBAL_API_KEY do evolution-go.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ─── Send ────────────────────────────────────────────────────────────────

// SendText envia uma mensagem de texto.
func (c *Client) SendText(ctx context.Context, instance, number, text string) (*SendTextResponse, error) {
	payload := SendTextRequest{
		Number: number,
		Text:   text,
	}
	var out SendTextResponse
	path := fmt.Sprintf("/message/sendText/%s", instance)
	if err := c.do(ctx, http.MethodPost, path, payload, &out); err != nil {
		return nil, fmt.Errorf("evogo: send text: %w", err)
	}
	return &out, nil
}

// SendMedia envia mídia (URL ou base64).
func (c *Client) SendMedia(ctx context.Context, instance string, req SendMediaRequest) (*SendTextResponse, error) {
	var out SendTextResponse
	path := fmt.Sprintf("/message/sendMedia/%s", instance)
	if err := c.do(ctx, http.MethodPost, path, req, &out); err != nil {
		return nil, fmt.Errorf("evogo: send media: %w", err)
	}
	return &out, nil
}

// ─── Webhook ─────────────────────────────────────────────────────────────

// SetWebhook configura o webhook global de uma instância.
// Aponta TODOS os eventos para uma URL. events: lista de eventos a entregar.
func (c *Client) SetWebhook(ctx context.Context, instance, url string, events []string, base64 bool) error {
	payload := WebhookSetRequest{
		URL:             url,
		WebhookByEvents: false,
		WebhookBase64:   base64,
		Events:          events,
	}
	path := fmt.Sprintf("/webhook/set/%s", instance)
	return c.do(ctx, http.MethodPost, path, payload, nil)
}

// ─── Instance lifecycle ──────────────────────────────────────────────────

// Connect inicia pareamento (gera QR se necessário).
func (c *Client) Connect(ctx context.Context, instance string) (*ConnectResponse, error) {
	var out ConnectResponse
	path := fmt.Sprintf("/instance/connect/%s", instance)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("evogo: connect: %w", err)
	}
	return &out, nil
}

// GetStatus busca o status da instância.
func (c *Client) GetStatus(ctx context.Context, instance string) (*InstanceStatus, error) {
	var out InstanceStatus
	path := fmt.Sprintf("/instance/status/%s", instance)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("evogo: status: %w", err)
	}
	return &out, nil
}

// CreateInstance cria uma nova instância (caso ainda não exista).
func (c *Client) CreateInstance(ctx context.Context, instanceName, number string) error {
	payload := map[string]any{
		"instanceName": instanceName,
		"number":       number,
		"integration":  "WHATSAPP-BAILEYS",
	}
	return c.do(ctx, http.MethodPost, "/instance/create", payload, nil)
}

// ─── HTTP transport ──────────────────────────────────────────────────────

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("evogo: marshal: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("evogo: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("evogo: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("evogo: %s %s → %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("evogo: unmarshal: %w (body: %s)", err, string(respBody))
		}
	}
	return nil
}
