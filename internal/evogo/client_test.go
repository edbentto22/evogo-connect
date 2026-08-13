package evogo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSendTextUsesInstanceTokenAndVersionedContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/send/text", r.URL.Path)
		assert.Equal(t, "instance-token", r.Header.Get("apikey"))

		var payload SendTextRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "5511999999999", payload.Number)
		assert.Equal(t, "olá", payload.Text)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"success","data":{"Info":{"ID":"msg-1","Type":"ExtendedTextMessage"}}}`))
	}))
	defer server.Close()

	response, err := NewClient(server.URL+"/", "instance-token").SendText(context.Background(), "5511999999999", "olá")
	require.NoError(t, err)
	assert.Equal(t, "msg-1", response.Data.Info.ID)
}

func TestClientSendMediaUsesVersionedContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/send/media", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "https://example.com/image.jpg", payload["url"])
		assert.Equal(t, "image", payload["type"])
		assert.NotContains(t, payload, "media")
		assert.NotContains(t, payload, "mediatype")
		_, _ = w.Write([]byte(`{"status":"sent"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "instance-token").SendMedia(context.Background(), SendMediaRequest{
		Number: "5511999999999",
		URL:    "https://example.com/image.jpg",
		Type:   "image",
	})
	require.NoError(t, err)
}

func TestClientConnectAndStatusContracts(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "instance-token", r.Header.Get("apikey"))
		switch r.URL.Path {
		case "/instance/connect":
			assert.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"message":"success","data":{"jid":"5511@s.whatsapp.net","webhookUrl":"https://connector.example/webhook/evo/demo/secret","eventString":"MESSAGES_UPSERT"}}`))
		case "/instance/status":
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"message":"success","data":{"name":"demo","connected":true,"loggedIn":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "instance-token")
	_, err := client.Connect(context.Background(), ConnectRequest{Immediate: true})
	require.NoError(t, err)
	status, err := client.GetStatus(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "demo", status.Data.Name)
	assert.True(t, status.Data.Connected)
	assert.Equal(t, 2, requests)
}

func TestClientHTTPErrorDoesNotExposeUpstreamBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"token":"must-not-leak","content":"private"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "instance-token").SendText(context.Background(), "5511", "private")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "must-not-leak")
	assert.NotContains(t, err.Error(), "private")

	var httpErr *HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadGateway, httpErr.StatusCode)
}

func TestClientDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	var leaked string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("apikey")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := NewClient(redirector.URL, "instance-token").SendText(context.Background(), "5511", "teste")
	require.Error(t, err)
	assert.Empty(t, leaked)
}
