package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/edbentto22/evogo-connect/internal/evogo"
)

func TestSetupCredentialPrefersFlag(t *testing.T) {
	t.Setenv("CHATWOOT_TOKEN", "from-env")

	value, err := setupCredential("from-flag", "CHATWOOT_TOKEN")

	assert.NoError(t, err)
	assert.Equal(t, "from-flag", value)
}

func TestSetupCredentialUsesEnvironment(t *testing.T) {
	t.Setenv("EVO_INSTANCE_TOKEN", "from-env")

	value, err := setupCredential("", "EVO_INSTANCE_TOKEN")

	assert.NoError(t, err)
	assert.Equal(t, "from-env", value)
}

func TestSetupCredentialRequiresValue(t *testing.T) {
	t.Setenv("CHATWOOT_TOKEN", "")

	_, err := setupCredential("", "CHATWOOT_TOKEN")

	assert.EqualError(t, err, "credential missing: use the corresponding flag or env CHATWOOT_TOKEN")
}

func TestValidEvoInstanceName(t *testing.T) {
	assert.True(t, validEvoInstanceName("demo_01-A"))
	for _, invalid := range []string{"", " space", "demo/path", "demo?x", "démö"} {
		assert.False(t, validEvoInstanceName(invalid))
	}
}

func TestHasMessageSubscription(t *testing.T) {
	assert.True(t, hasMessageSubscription("MESSAGES_UPSERT, CONNECTION_UPDATE"))
	assert.True(t, hasMessageSubscription("messages.upsert,connection.update"))
	assert.True(t, hasMessageSubscription("MESSAGE"))
	assert.False(t, hasMessageSubscription("MESSAGES_UPDATE"))
}

func TestConfigureEvogoWebhookUsesMessageCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/instance/connect", r.URL.Path)
		var request evogo.ConnectRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, []string{"MESSAGE"}, request.Subscribe)
		assert.True(t, request.Immediate)
		assert.Equal(t, "https://connector.example/webhook/evo/demo/secret", request.WebhookURL)
		_, _ = w.Write([]byte(`{"message":"success","data":{"webhookUrl":"https://connector.example/webhook/evo/demo/secret","eventString":"messages.upsert"}}`))
	}))
	defer server.Close()

	err := configureEvogoWebhook(context.Background(), evogo.NewClient(server.URL, "token"), "https://connector.example", "demo", "secret")
	require.NoError(t, err)
}
