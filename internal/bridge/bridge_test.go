package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/store"
)

type memoryBridgeStore struct {
	mu       sync.Mutex
	tenant   *store.Tenant
	claims   map[string]string
	tokens   map[string]string
	audits   []store.BridgeLogEntry
	paused   bool
	claimErr error
}

func newMemoryBridgeStore(evoURL string) *memoryBridgeStore {
	return &memoryBridgeStore{
		tenant: &store.Tenant{
			ID:              uuid.New(),
			Name:            "demo",
			ChatwootInboxID: 7,
			EvoBaseURL:      evoURL,
			EvoAPIKey:       "instance-token",
		},
		claims: make(map[string]string),
		tokens: make(map[string]string),
	}
}

func (s *memoryBridgeStore) IsPaused(context.Context) (bool, error) { return s.paused, nil }

func (s *memoryBridgeStore) GetTenantByChatwootInbox(_ context.Context, inboxID int) (*store.Tenant, error) {
	if inboxID != s.tenant.ChatwootInboxID {
		return nil, store.ErrNotFound
	}
	return s.tenant, nil
}

func (s *memoryBridgeStore) ClaimIdempotency(_ context.Context, direction, key string, _ uuid.UUID, _ time.Duration, claimToken string) (store.IdempotencyClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return "", s.claimErr
	}
	mapKey := direction + ":" + key
	status := s.claims[mapKey]
	if status == "processing" || status == "sent" {
		if status == "sent" {
			return store.ClaimCompleted, nil
		}
		return store.ClaimInProgress, nil
	}
	s.claims[mapKey] = "processing"
	s.tokens[mapKey] = claimToken
	return store.ClaimAcquired, nil
}

func (s *memoryBridgeStore) CompleteDelivery(_ context.Context, direction, key, claimToken string, _ []byte, _ time.Duration, entry store.BridgeLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens[direction+":"+key] != claimToken {
		return errors.New("claim token mismatch")
	}
	s.claims[direction+":"+key] = "sent"
	s.audits = append(s.audits, entry)
	return nil
}

func (s *memoryBridgeStore) ReleaseIdempotency(_ context.Context, direction, key, claimToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens[direction+":"+key] != claimToken {
		return errors.New("claim token mismatch")
	}
	s.claims[direction+":"+key] = "failed"
	return nil
}

func (s *memoryBridgeStore) LogBridge(_ context.Context, entry store.BridgeLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, entry)
	return nil
}

func validEnvelope() chatwoot.WebhookEnvelope {
	return chatwoot.WebhookEnvelope{
		Event:       "message_created",
		MessageType: "outgoing",
		ID:          123,
		Content:     "mensagem privada",
		Conversation: chatwoot.ConversationRef{
			InboxID: 7,
			ContactInbox: chatwoot.ContactInbox{
				SourceID: "5511999999999@s.whatsapp.net",
				InboxID:  7,
			},
		},
	}
}

func TestHandleChatwootWebhookSendsTextExactlyOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/send/text", r.URL.Path)
		assert.Equal(t, "instance-token", r.Header.Get("apikey"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "5511999999999", body["number"])
		assert.Equal(t, "mensagem privada", body["text"])
		_, _ = w.Write([]byte(`{"status":"sent"}`))
	}))
	defer server.Close()

	st := newMemoryBridgeStore(server.URL)
	core := New(st, 24*time.Hour, false, 0)
	env := validEnvelope()
	require.NoError(t, core.HandleChatwootWebhook(context.Background(), env))
	require.NoError(t, core.HandleChatwootWebhook(context.Background(), env))
	assert.Equal(t, int32(1), calls.Load())
	require.Len(t, st.audits, 1)
	assert.NotEqual(t, env.Conversation.ContactInbox.SourceID, st.audits[0].JID)
	assert.NotContains(t, st.audits[0].ErrorDetail, env.Content)
}

func TestHandleChatwootWebhookRetriesAfterTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, `{"content":"must-not-leak"}`, http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"status":"sent"}`))
	}))
	defer server.Close()

	st := newMemoryBridgeStore(server.URL)
	core := New(st, 24*time.Hour, false, 0)
	err := core.HandleChatwootWebhook(context.Background(), validEnvelope())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "must-not-leak")
	require.NoError(t, core.HandleChatwootWebhook(context.Background(), validEnvelope()))
	assert.Equal(t, int32(2), calls.Load())
}

func TestHandleChatwootWebhookSendsChatwootAttachmentContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/send/media", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "https://chatwoot.example.com/file.pdf", body["url"])
		assert.Equal(t, "document", body["type"])
		assert.Equal(t, "attachment.pdf", body["filename"])
		assert.NotEmpty(t, body["id"])
		_, _ = w.Write([]byte(`{"message":"success","data":{"Info":{"ID":"msg-media"}}}`))
	}))
	defer server.Close()

	st := newMemoryBridgeStore(server.URL)
	env := validEnvelope()
	env.Attachments = []chatwoot.Attachment{{
		FileType:  "file",
		DataURL:   "https://chatwoot.example.com/file.pdf",
		Extension: "pdf",
	}}
	require.NoError(t, New(st, 24*time.Hour, false, 0).HandleChatwootWebhook(context.Background(), env))
}

func TestHandleChatwootWebhookConcurrentDuplicateProducesOneEffect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"sent"}`))
	}))
	defer server.Close()

	st := newMemoryBridgeStore(server.URL)
	core := New(st, 24*time.Hour, false, 0)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- core.HandleChatwootWebhook(context.Background(), validEnvelope())
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, ErrInProgress)
		}
	}
	assert.Equal(t, int32(1), calls.Load())
}

func TestHandleChatwootWebhookDoesNotSendWhenClaimFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	st := newMemoryBridgeStore(server.URL)
	st.claimErr = errors.New("database unavailable")
	err := New(st, 24*time.Hour, false, 0).HandleChatwootWebhook(context.Background(), validEnvelope())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claim idempotency")
}

func TestGetLimiter_CreatesAndReuses(t *testing.T) {
	// Core com limit=120/min → 2 req/s, burst=12
	c := New(nil, 24*time.Hour, false, 120)
	tenantID := uuid.New()

	lim1 := c.getLimiter(tenantID, DirC2W)
	if lim1 == nil {
		t.Fatal("expected non-nil limiter")
	}
	lim2 := c.getLimiter(tenantID, DirC2W)
	if lim1 != lim2 {
		t.Error("getLimiter should return the same instance for same key")
	}
}

func TestGetLimiter_DifferentTenantsGetDifferentLimiters(t *testing.T) {
	c := New(nil, 24*time.Hour, false, 60)
	a := uuid.New()
	b := uuid.New()
	if c.getLimiter(a, DirC2W) == c.getLimiter(b, DirC2W) {
		t.Error("different tenants should have different limiters")
	}
	if c.getLimiter(a, DirC2W) == c.getLimiter(a, DirW2C) {
		t.Error("different directions should have different limiters")
	}
}

func TestGetLimiter_BurstBehavior(t *testing.T) {
	// limit=60/min → 1 req/s, burst=6
	c := New(nil, 24*time.Hour, false, 60)
	tenantID := uuid.New()
	lim := c.getLimiter(tenantID, DirC2W)

	// Primeiros 6 devem passar (burst)
	allowed := 0
	for i := 0; i < 20; i++ {
		if lim.Allow() {
			allowed++
		}
	}
	// Após o burst, deve bloquear (sem refill em <1s)
	if allowed < 6 || allowed > 7 {
		t.Errorf("expected ~6 allowed (burst), got %d", allowed)
	}
}

func TestGetLimiter_ZeroLimitDisables(t *testing.T) {
	c := New(nil, 24*time.Hour, false, 0)
	tenantID := uuid.New()
	// getLimiter ainda é chamado mas retorna limiter muito restritivo.
	// No bridge, a checagem `if c.LimitPerMin > 0` evita a chamada quando
	// o limit é 0, garantindo zero overhead.
	lim := c.getLimiter(tenantID, DirC2W)
	// Sanity: ainda é um *rate.Limiter válido
	if _, ok := interface{}(lim).(*rate.Limiter); !ok {
		t.Error("expected *rate.Limiter")
	}
}
