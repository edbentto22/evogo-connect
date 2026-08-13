package evogo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResponseBody = 1 << 20

// HTTPError representa uma resposta não-2xx sem expor o corpo do upstream.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("evogo: %s %s returned status %d", e.Method, e.Path, e.StatusCode)
}

// TransportError preserva a causa sem expor a URL configurada em logs.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "evogo: upstream transport failed" }
func (e *TransportError) Unwrap() error { return e.Err }

// Client é o cliente HTTP do Evolution Go 0.7.2.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient cria um client autenticado pelo token individual da instância.
func NewClient(baseURL, instanceToken string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   instanceToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// SendText envia uma mensagem de texto.
func (c *Client) SendText(ctx context.Context, number, message string) (*SendTextResponse, error) {
	return c.SendTextWithID(ctx, number, message, "")
}

// SendTextWithID envia texto com um ID determinístico, permitindo que o
// upstream reconheça uma repetição após falha entre envio e persistência.
func (c *Client) SendTextWithID(ctx context.Context, number, message, id string) (*SendTextResponse, error) {
	payload := SendTextRequest{Number: number, Text: message, ID: id}
	var out SendTextResponse
	if err := c.do(ctx, http.MethodPost, "/send/text", payload, &out); err != nil {
		return nil, fmt.Errorf("evogo: send text: %w", err)
	}
	return &out, nil
}

// SendMedia envia uma mídia por URL.
func (c *Client) SendMedia(ctx context.Context, req SendMediaRequest) (*SendTextResponse, error) {
	var out SendTextResponse
	if err := c.do(ctx, http.MethodPost, "/send/media", req, &out); err != nil {
		return nil, fmt.Errorf("evogo: send media: %w", err)
	}
	return &out, nil
}

// Connect configura a conexão e as assinaturas de eventos da instância.
func (c *Client) Connect(ctx context.Context, req ConnectRequest) (*ConnectResponse, error) {
	var out ConnectResponse
	if err := c.do(ctx, http.MethodPost, "/instance/connect", req, &out); err != nil {
		return nil, fmt.Errorf("evogo: connect: %w", err)
	}
	return &out, nil
}

// GetStatus busca o status da instância associada ao token.
func (c *Client) GetStatus(ctx context.Context) (*InstanceStatus, error) {
	var out InstanceStatus
	if err := c.do(ctx, http.MethodGet, "/instance/status", nil, &out); err != nil {
		return nil, fmt.Errorf("evogo: status: %w", err)
	}
	return &out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("evogo: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("evogo: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("evogo: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("evogo: decode response: %w", err)
		}
	}
	return nil
}
