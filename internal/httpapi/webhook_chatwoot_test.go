package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/edbentto22/evogo-connect/internal/bridge"
	"github.com/edbentto22/evogo-connect/internal/store"
)

type webhookTestStore struct {
	tenant *store.Tenant
}

func (s *webhookTestStore) Ping(context.Context) error { return nil }

func (s *webhookTestStore) GetTenantByChatwootInbox(_ context.Context, inboxID int) (*store.Tenant, error) {
	if inboxID != s.tenant.ChatwootInboxID {
		return nil, store.ErrNotFound
	}
	return s.tenant, nil
}

func (s *webhookTestStore) SetPaused(context.Context, bool, string) error { return nil }
func (s *webhookTestStore) IsPaused(context.Context) (bool, error)        { return false, nil }
func (s *webhookTestStore) ListTenants(context.Context) ([]store.Tenant, error) {
	return []store.Tenant{*s.tenant}, nil
}
func (s *webhookTestStore) ClaimIdempotency(context.Context, string, string, uuid.UUID, time.Duration, string) (store.IdempotencyClaim, error) {
	return store.ClaimAcquired, nil
}
func (s *webhookTestStore) CompleteDelivery(context.Context, string, string, string, []byte, time.Duration, store.BridgeLogEntry) error {
	return nil
}
func (s *webhookTestStore) ReleaseIdempotency(context.Context, string, string, string) error {
	return nil
}
func (s *webhookTestStore) LogBridge(context.Context, store.BridgeLogEntry) error { return nil }

func webhookSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newWebhookTestRouter(secret string) http.Handler {
	st := &webhookTestStore{tenant: &store.Tenant{
		ID:              uuid.New(),
		Name:            "demo",
		ChatwootInboxID: 7,
		ChatwootHMAC:    secret,
	}}
	return NewRouter(Deps{
		Store:        st,
		Bridge:       bridge.New(st, 24*time.Hour, false, 0),
		AdminToken:   "admin-token",
		ReplayWindow: 5 * time.Minute,
	})
}

func TestChatwootWebhookRejectsInvalidAndExpiredSignatures(t *testing.T) {
	body := []byte(`{"event":"conversation_created","inbox_id":7,"conversation":{"inbox_id":7}}`)
	now := time.Now()

	tests := []struct {
		name      string
		timestamp string
		signature string
	}{
		{name: "missing headers"},
		{name: "invalid digest", timestamp: strconv.FormatInt(now.Unix(), 10), signature: "sha256=deadbeef"},
		{
			name:      "expired timestamp",
			timestamp: strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10),
			signature: webhookSignature("webhook-secret", strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10), body),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhook/chatwoot", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Chatwoot-Timestamp", tt.timestamp)
			req.Header.Set("X-Chatwoot-Signature", tt.signature)
			recorder := httptest.NewRecorder()
			newWebhookTestRouter("webhook-secret").ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
			assert.JSONEq(t, `{"error":"invalid_signature"}`, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), "detail")
		})
	}
}

func TestChatwootWebhookAcceptsValidSignature(t *testing.T) {
	body := []byte(`{"event":"conversation_created","inbox_id":7,"conversation":{"inbox_id":7}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/webhook/chatwoot", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Chatwoot-Timestamp", timestamp)
	req.Header.Set("X-Chatwoot-Signature", webhookSignature("webhook-secret", timestamp, body))
	recorder := httptest.NewRecorder()

	newWebhookTestRouter("webhook-secret").ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"status":"skipped"}`, recorder.Body.String())
}

func TestChatwootWebhookUsesContactInboxFallback(t *testing.T) {
	body := []byte(`{"event":"conversation_created","conversation":{"contact_inbox":{"inbox_id":7}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/webhook/chatwoot", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Chatwoot-Timestamp", timestamp)
	req.Header.Set("X-Chatwoot-Signature", webhookSignature("webhook-secret", timestamp, body))
	recorder := httptest.NewRecorder()

	newWebhookTestRouter("webhook-secret").ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"status":"skipped"}`, recorder.Body.String())
}

func TestChatwootWebhookFailsClosedWithoutStoredSecret(t *testing.T) {
	body := []byte(`{"event":"conversation_created","inbox_id":7,"conversation":{"inbox_id":7}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/chatwoot", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	newWebhookTestRouter("").ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.JSONEq(t, `{"error":"signature_not_configured"}`, recorder.Body.String())
}
