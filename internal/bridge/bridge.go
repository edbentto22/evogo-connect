// Package bridge implementa o core do connector: recebe um webhook do
// Chatwoot (Etapa 1) ou do evolution-go (Etapa 2), resolve o tenant
// correspondente, checa idempotência, faz o dispatch para o outro lado,
// e grava o audit log.
//
// Toda função aqui é **stateless** — toda persistência passa pelo Store.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/evogo"
	applog "github.com/edbentto22/evogo-connect/internal/log"
	"github.com/edbentto22/evogo-connect/internal/metrics"
	"github.com/edbentto22/evogo-connect/internal/store"
)

// Direction é o sentido do bridge.
type Direction string

const (
	// DirC2W: Chatwoot → WhatsApp (agente responde → cliente recebe)
	DirC2W Direction = "c2w"
	// DirW2C: WhatsApp → Chatwoot (cliente manda msg → agente recebe)
	DirW2C Direction = "w2c"
)

// ─── Errors ──────────────────────────────────────────────────────────────

// ErrSkipped é retornado quando o bridge decide não processar (duplicata,
// mensagem privada, kill switch, etc). Não é um erro operacional.
var ErrSkipped = errors.New("bridge: skipped")

// ErrPaused é retornado quando o kill switch está ativo.
var ErrPaused = errors.New("bridge: paused")

// ErrRateLimited é retornado quando o rate limiter recusa.
var ErrRateLimited = errors.New("bridge: rate limited")

// ErrInProgress pede ao emissor que retente enquanto outra execução mantém o
// claim. Diferente de uma entrega concluída, não deve produzir HTTP 200.
var ErrInProgress = errors.New("bridge: delivery in progress")

// ─── Core ────────────────────────────────────────────────────────────────

// Core é o motor do bridge.
type Core struct {
	Store          BridgeStore
	IdempotencyTTL time.Duration
	Paused         bool // valor carregado do env na inicialização
	LimitPerMin    int  // limite por minuto, por (tenant, direction)

	// limiters é um cache lazy de *rate.Limiter por chave `tenantID:direction`.
	// sync.Map porque o padrão de uso é read-heavy (cada webhook lê o limiter
	// do seu tenant) e write-rare (só na primeira request do tenant).
	limiters sync.Map
}

// BridgeStore contém apenas as operações persistentes usadas pelo core.
type BridgeStore interface {
	IsPaused(context.Context) (bool, error)
	GetTenantByChatwootInbox(context.Context, int) (*store.Tenant, error)
	ClaimIdempotency(context.Context, string, string, uuid.UUID, time.Duration, string) (store.IdempotencyClaim, error)
	CompleteDelivery(context.Context, string, string, string, []byte, time.Duration, store.BridgeLogEntry) error
	ReleaseIdempotency(context.Context, string, string, string) error
	LogBridge(context.Context, store.BridgeLogEntry) error
}

// New cria o Core.
func New(s BridgeStore, idempotencyTTL time.Duration, paused bool, limitPerMin int) *Core {
	return &Core{
		Store:          s,
		IdempotencyTTL: idempotencyTTL,
		Paused:         paused,
		LimitPerMin:    limitPerMin,
	}
}

// limiterKey compõe a chave do limiter pra sync.Map.
func limiterKey(tenantID uuid.UUID, dir Direction) string {
	return tenantID.String() + ":" + string(dir)
}

// getLimiter devolve o limiter pra (tenant, direction), criando sob demanda.
// O rate é `LimitPerMin / 60` por segundo, com burst igual ao limite/10
// (mínimo 1) — permite pequenos picos sem travar tudo.
func (c *Core) getLimiter(tenantID uuid.UUID, dir Direction) *rate.Limiter {
	key := limiterKey(tenantID, dir)
	if v, ok := c.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	perSec := rate.Limit(float64(c.LimitPerMin) / 60.0)
	burst := c.LimitPerMin / 10
	if burst < 1 {
		burst = 1
	}
	lim := rate.NewLimiter(perSec, burst)
	actual, _ := c.limiters.LoadOrStore(key, lim)
	return actual.(*rate.Limiter)
}

// IsPaused checa kill switch (DB tem prioridade sobre env, para toggle on-the-fly).
func (c *Core) IsPaused(ctx context.Context) (bool, error) {
	dbPaused, err := c.Store.IsPaused(ctx)
	if err != nil {
		return c.Paused, err
	}
	return c.Paused || dbPaused, nil
}

// ─── Etapa 1: Chatwoot → WhatsApp (reverse) ─────────────────────────────

// HandleChatwootWebhook processa um webhook de mensagem outgoing do Chatwoot.
// Além de message_created, aceita message_outgoing, evento adicional emitido
// pelo fork fazer.ai com o mesmo message.webhook_data.
//
// Fluxo:
//  1. Verifica kill switch
//  2. Filtra: só processa eventos de mensagem criada/saída + outgoing + !private
//  3. Resolve tenant pelo inbox_id
//  4. Resolve JID via contact_inbox.source_id
//  5. Adquire claim idempotente por chatwoot_message_id
//  6. Envia texto/mídia via Evolution Go
//  7. Conclui a idempotência e grava audit log
func (c *Core) HandleChatwootWebhook(ctx context.Context, env chatwoot.WebhookEnvelope) error {
	start := time.Now()
	log := slog.Default().With(
		"event", env.Event,
		"message_id", env.ID,
		"message_type", env.MessageType,
		"inbox_id", env.Conversation.InboxID,
	)

	// 1. Kill switch
	paused, err := c.IsPaused(ctx)
	if err != nil {
		log.Warn("bridge: failed to check pause, using env", "err", err)
		paused = c.Paused
	}
	if paused {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "paused").Inc()
		return ErrPaused
	}

	// 2. Filtro de evento
	if env.Event != "message_created" && env.Event != "message_outgoing" {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), skippedEventStatus(env.Event)).Inc()
		return fmt.Errorf("%w: event=%s", ErrSkipped, env.Event)
	}
	if env.MessageType != "outgoing" {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "skipped_incoming").Inc()
		return fmt.Errorf("%w: message_type=%s", ErrSkipped, env.MessageType)
	}
	if env.Private {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "skipped_private").Inc()
		return fmt.Errorf("%w: private note", ErrSkipped)
	}
	if env.ID <= 0 {
		metrics.BridgeErrors.WithLabelValues("invalid_message_id", "bridge").Inc()
		return fmt.Errorf("bridge: invalid Chatwoot message id %d", env.ID)
	}

	// 3. Resolve tenant
	tenant, err := c.Store.GetTenantByChatwootInbox(ctx, env.Conversation.InboxID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			metrics.BridgeErrors.WithLabelValues("tenant_not_found", "bridge").Inc()
			return fmt.Errorf("bridge: tenant not found for inbox %d: %w", env.Conversation.InboxID, err)
		}
		return fmt.Errorf("bridge: get tenant: %w", err)
	}
	log = log.With("tenant", tenant.Name)

	// 4. Resolve JID via contact_inbox.source_id. O Fazer.ai pode serializar
	// esse objeto sem source_id em webhooks de mensagens outgoing, mas mantém
	// o identifier do mesmo contato em conversation.meta.sender.
	sourceID := strings.TrimSpace(env.Conversation.ContactInbox.SourceID)
	sourceField := "contact_inbox.source_id"
	if sourceID == "" {
		sourceID = env.Conversation.Meta.Sender.Identifier
		sourceField = "conversation.meta.sender.identifier"
		log.Info("bridge: using Chatwoot contact identifier fallback")
	}
	jid, number, jidErr := evogo.ParseDirectJID(sourceID)
	if jidErr != nil {
		errCode := "invalid_source_id"
		if sourceField == "conversation.meta.sender.identifier" {
			errCode = "invalid_contact_identifier"
		}
		metrics.BridgeErrors.WithLabelValues(errCode, "bridge").Inc()
		auditErr := c.logAudit(ctx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), "", nil, "error", errCode, "identificador WhatsApp inválido", 0)
		return errors.Join(fmt.Errorf("bridge: validate %s: %w", sourceField, jidErr), auditErr)
	}

	// 5. Reserva idempotente antes do efeito externo. Claims de processamento
	// expiram rapidamente para permitir retomada após crash.
	idempKey := fmt.Sprintf("c2w:%s:%d", tenant.Name, env.ID)
	claimToken := uuid.NewString()
	claimState, err := c.Store.ClaimIdempotency(ctx, string(DirC2W), idempKey, tenant.ID, 2*time.Minute, claimToken)
	if err != nil {
		return fmt.Errorf("bridge: claim idempotency: %w", err)
	}
	switch claimState {
	case store.ClaimCompleted:
		metrics.IdempotencyHits.Inc()
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "skipped_duplicate").Inc()
		log.Info("bridge: duplicate skipped")
		return nil
	case store.ClaimInProgress:
		metrics.IdempotencyHits.Inc()
		return ErrInProgress
	case store.ClaimAcquired:
		// Continua para o efeito externo.
	default:
		return fmt.Errorf("bridge: unexpected idempotency state %q", claimState)
	}

	if c.LimitPerMin > 0 && !c.getLimiter(tenant.ID, DirC2W).Allow() {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "rate_limited").Inc()
		metrics.BridgeErrors.WithLabelValues("rate_limited", "bridge").Inc()
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := c.Store.ReleaseIdempotency(persistCtx, string(DirC2W), idempKey, claimToken); err != nil {
			return errors.Join(ErrRateLimited, err)
		}
		return ErrRateLimited
	}

	// 6. Envia via Evolution Go com o token individual da instância.
	evo := evogo.NewClient(tenant.EvoBaseURL, tenant.EvoAPIKey)
	messageID := deterministicMessageID(tenant.ID, env.ID)

	var err2 error
	if len(env.Attachments) == 0 {
		_, err2 = evo.SendTextWithID(ctx, number, env.Content, messageID)
	} else {
		att := env.Attachments[0]
		mediaReq := evogo.SendMediaRequest{
			Number:   number,
			URL:      att.DataURL,
			Type:     normalizeMediaType(att.FileType),
			Filename: attachmentFilename(att),
			Caption:  env.Content,
			ID:       messageID,
		}
		_, err2 = evo.SendMedia(ctx, mediaReq)
	}

	latency := time.Since(start)
	latencyMS := int(latency.Milliseconds())
	if err2 != nil {
		errCode := "evo_send_failed"
		metrics.BridgeErrors.WithLabelValues(errCode, "bridge").Inc()
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "error").Inc()
		metrics.BridgeLatency.WithLabelValues(string(DirC2W)).Observe(latency.Seconds())
		log.Error("bridge: evo send failed", "err", err2, "jid_masked", applog.MaskPhone(jid))
		var transportErr *evogo.TransportError
		if errors.As(err2, &transportErr) {
			// Resultado ambíguo: mantém o claim até expirar. O retry usa o mesmo
			// message ID no upstream para reduzir risco de duplicação.
			return fmt.Errorf("bridge: evo send result unknown: %w", err2)
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		releaseErr := c.Store.ReleaseIdempotency(persistCtx, string(DirC2W), idempKey, claimToken)
		auditErr := c.logAudit(persistCtx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), jid, []byte(applog.ContentHash(env.Content)), "error", errCode, "falha no envio ao upstream", latencyMS)
		if releaseErr != nil || auditErr != nil {
			return errors.Join(fmt.Errorf("bridge: evo send: %w", err2), releaseErr, auditErr)
		}
		return fmt.Errorf("bridge: evo send: %w", err2)
	}

	// 7. Sucesso — conclui a chave antes da auditoria. Uma falha crítica de
	// persistência é propagada. Idempotência e auditoria são atômicas no DB.
	detail := json.RawMessage(fmt.Sprintf(`{"latency_ms":%d}`, latencyMS))
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	auditEntry := c.newAuditEntry(tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), jid, []byte(applog.ContentHash(env.Content)), "ok", "", "", latencyMS)
	if err := c.Store.CompleteDelivery(persistCtx, string(DirC2W), idempKey, claimToken, detail, c.IdempotencyTTL, auditEntry); err != nil {
		return fmt.Errorf("bridge: complete delivery: %w", err)
	}

	metrics.BridgeMessages.WithLabelValues(string(DirC2W), "ok").Inc()
	metrics.BridgeLatency.WithLabelValues(string(DirC2W)).Observe(latency.Seconds())
	log.Info("bridge: c2w delivered",
		"jid_masked", applog.MaskPhone(jid),
		"content_hash", applog.ContentHash(env.Content),
		"latency_ms", latency.Milliseconds(),
	)
	return nil
}

func skippedEventStatus(event string) string {
	switch event {
	case "message_updated":
		return "skipped_message_updated"
	case "conversation_created":
		return "skipped_conversation_created"
	case "conversation_updated":
		return "skipped_conversation_updated"
	case "conversation_status_changed":
		return "skipped_conversation_status_changed"
	case "conversation_typing_on", "conversation_typing_off", "conversation_recording":
		return "skipped_conversation_activity"
	default:
		return "skipped_event"
	}
}

func attachmentFilename(att chatwoot.Attachment) string {
	if att.Extension == "" {
		return ""
	}
	return "attachment." + strings.TrimPrefix(att.Extension, ".")
}

func (c *Core) logAudit(ctx context.Context, tenantID uuid.UUID, dir Direction, extMsgID, jid string, payloadHash []byte, status, errCode, errDetail string, latencyMS int) error {
	if err := c.Store.LogBridge(ctx, c.newAuditEntry(tenantID, dir, extMsgID, jid, payloadHash, status, errCode, errDetail, latencyMS)); err != nil {
		return fmt.Errorf("bridge: log audit: %w", err)
	}
	return nil
}

func (c *Core) newAuditEntry(tenantID uuid.UUID, dir Direction, extMsgID, jid string, payloadHash []byte, status, errCode, errDetail string, latencyMS int) store.BridgeLogEntry {
	return store.BridgeLogEntry{
		TenantID:          tenantID,
		Direction:         string(dir),
		ExternalMessageID: extMsgID,
		JID:               applog.MaskPhone(jid),
		PayloadSHA256:     payloadHash,
		Status:            status,
		ErrorCode:         errCode,
		ErrorDetail:       errDetail,
		LatencyMS:         latencyMS,
	}
}

func deterministicMessageID(tenantID uuid.UUID, chatwootMessageID int64) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", tenantID, chatwootMessageID)))
	return fmt.Sprintf("CW%X", digest[:15])
}

func normalizeMediaType(chatwootType string) string {
	switch chatwootType {
	case "image":
		return "image"
	case "audio":
		return "audio"
	case "video":
		return "video"
	case "file":
		return "document"
	default:
		return "document"
	}
}
