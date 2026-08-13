package chatwoot

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

func TestCreateAPIInboxParsesFlatResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/accounts/1/inboxes", r.URL.Path)
		assert.Equal(t, "chatwoot-token", r.Header.Get("api_access_token"))
		var payload InboxCreatePayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "api", payload.Channel.Type)
		assert.True(t, payload.Channel.HMACMandatory)
		_, _ = w.Write([]byte(`{
			"id":7,"channel_id":9,"channel_type":"Channel::Api",
			"secret":"webhook-secret","webhook_url":"https://connector.test/webhook/chatwoot",
			"inbox_identifier":"inbox-key"
		}`))
	}))
	defer server.Close()

	inbox, err := NewClient(server.URL+"/", 1, "chatwoot-token").CreateAPIInbox(
		context.Background(), "WhatsApp", "https://connector.test/webhook/chatwoot",
	)
	require.NoError(t, err)
	assert.Equal(t, 7, inbox.ID)
	assert.Equal(t, "webhook-secret", inbox.Secret)
}

func TestContactLifecycleUsesIdentifierAndExactSourceID(t *testing.T) {
	const jid = "5511999999999@s.whatsapp.net"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/accounts/1/contacts":
			var payload ContactCreatePayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, jid, payload.Identifier)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":42,"name":"João","identifier":"` + jid + `","contact_inboxes":[]},"contact_inbox":null}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/accounts/1/contacts/42/contact_inboxes":
			var payload ContactInboxCreatePayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, 7, payload.InboxID)
			assert.Equal(t, jid, payload.SourceID)
			_, _ = w.Write([]byte(`{"id":11,"source_id":"` + jid + `","inbox":{"id":7}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 1, "token")
	contact, err := client.CreateContact(context.Background(), "João", jid)
	require.NoError(t, err)
	contactInbox, err := client.EnsureContactInbox(context.Background(), contact, 7, jid)
	require.NoError(t, err)
	assert.Equal(t, jid, contactInbox.SourceID)
	assert.Equal(t, 2, requests)
}

func TestFindContactByIdentifierEscapesAndMatchesExactly(t *testing.T) {
	const identifier = "5511+test@s.whatsapp.net"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, identifier, r.URL.Query().Get("q"))
		_, _ = w.Write([]byte(`{"payload":[
			{"id":1,"identifier":"partial"},
			{"id":2,"identifier":"` + identifier + `","contact_inboxes":[]}
		]}`))
	}))
	defer server.Close()

	contact, err := NewClient(server.URL, 1, "token").FindContactByIdentifier(context.Background(), identifier)
	require.NoError(t, err)
	assert.Equal(t, 2, contact.ID)
}

func TestEnsureContactInboxReusesExactExistingLink(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	contact := &ContactResponse{ID: 42}
	contact.ContactInboxes = append(contact.ContactInboxes, ContactInboxResponse{ID: 11, SourceID: "5511@s.whatsapp.net"})
	contact.ContactInboxes[0].Inbox.ID = 7

	ci, err := NewClient(server.URL, 1, "token").EnsureContactInbox(context.Background(), contact, 7, "5511@s.whatsapp.net")
	require.NoError(t, err)
	assert.Equal(t, 11, ci.ID)
	assert.False(t, called)
}

func TestEnsureContactInboxRecoversConcurrentConflict(t *testing.T) {
	const jid = "5511@s.whatsapp.net"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusUnprocessableEntity)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/accounts/1/contacts/search":
			_, _ = w.Write([]byte(`{"payload":[{"id":42,"identifier":"` + jid + `","contact_inboxes":[{"id":11,"source_id":"` + jid + `","inbox":{"id":7}}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	contact := &ContactResponse{ID: 42, Identifier: jid}
	ci, err := NewClient(server.URL, 1, "token").EnsureContactInbox(context.Background(), contact, 7, jid)
	require.NoError(t, err)
	assert.Equal(t, 11, ci.ID)
}

func TestCreateConversationUsesMappedContactAndInbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/accounts/1/conversations", r.URL.Path)
		assert.Equal(t, "token", r.Header.Get("api_access_token"))
		var payload ConversationCreatePayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "5511999999999@s.whatsapp.net", payload.SourceID)
		assert.Equal(t, 42, payload.ContactID)
		assert.Equal(t, 7, payload.InboxID)
		assert.Equal(t, "open", payload.Status)
		// Este é o formato plano retornado pela action create do Chatwoot/Fazer.ai.
		_, _ = w.Write([]byte(`{"id":99,"account_id":1,"inbox_id":7,"status":"open"}`))
	}))
	defer server.Close()

	conversation, err := NewClient(server.URL, 1, "token").CreateConversation(context.Background(), "5511999999999@s.whatsapp.net", 42, 7)
	require.NoError(t, err)
	assert.Equal(t, 99, conversation.ID)
}

func TestCreateConversationRejectsUnexpectedBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":99,"account_id":1,"inbox_id":8,"status":"open"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, 1, "token").CreateConversation(context.Background(), "5511999999999@s.whatsapp.net", 42, 7)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unexpected binding")
}

func TestClientHTTPErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"token":"must-not-leak"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, 1, "token").CreateContact(context.Background(), "João", "5511@s.whatsapp.net")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "must-not-leak")
	var httpErr *HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
}

func TestSearchHTTPErrorDoesNotExposeIdentifierInPath(t *testing.T) {
	const identifier = "5511999999999@s.whatsapp.net"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, 1, "token").FindContactByIdentifier(context.Background(), identifier)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), identifier)
	assert.NotContains(t, err.Error(), "q=")
}

func TestClientDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	var leaked string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("api_access_token")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := NewClient(redirector.URL, 1, "chatwoot-token").GetInbox(context.Background(), 7)
	require.Error(t, err)
	assert.Empty(t, leaked)
}

func TestFindOpenConversationAcceptsFazerAIPayloadAndPaginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/accounts/1/conversations", r.URL.Path)
		assert.Equal(t, "44", r.URL.Query().Get("contact_id"))
		assert.Equal(t, "7", r.URL.Query().Get("inbox_id"))
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"data":{"payload":[{"id":2,"inbox_id":7,"status":"resolved"}]}}`))
		case "2":
			_, _ = w.Write([]byte(`{"data":[{"id":3,"inbox_id":7,"status":"open"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	conversation, err := NewClient(server.URL, 1, "token").FindOpenConversation(context.Background(), 44, 7)
	require.NoError(t, err)
	assert.Equal(t, 3, conversation.ID)
}
