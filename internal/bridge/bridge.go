// Package bridge implementa o core do connector: recebe um webhook do
// Chatwoot (Etapa 1) ou do evolution-go (Etapa 2), resolve o tenant
// correspondente, checa idempotência, faz o dispatch para o outro lado,
// e grava o audit log.
//
// Toda função aqui é **stateless** — toda persistência passa pelo Store.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

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

// ─── Core ────────────────────────────────────────────────────────────────

// Core é o motor do bridge.
type Core struct {
	Store          *store.Store
	IdempotencyTTL time.Duration
	Paused         bool // valor carregado do env na inicialização
	// RateLimit por tenant+direction — passado pelo caller
}

// New cria o Core.
func New(s *store.Store, idempotencyTTL time.Duration, paused bool) *Core {
	return &Core{
		Store:          s,
		IdempotencyTTL: idempotencyTTL,
		Paused:         paused,
	}
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

// HandleChatwootWebhook processa um webhook do Chatwoot (MessageCreated de
// outgoing). Retorna ErrSkipped/ErrPaused/ErrRateLimited para casos não-erro.
//
// Fluxo:
//  1. Verifica kill switch
//  2. Valida HMAC (feito no handler antes de chegar aqui)
//  3. Filtra: só processa message_created + outgoing + !private
//  4. Resolve tenant pelo inbox_id
//  5. Resolve JID do WhatsApp via contact_map (source_id = JID)
//  6. Checagem de idempotência por chatwoot_message_id
//  7. Envia texto/mídia via evolution-go
//  8. Grava audit log + idempotency_keys
func (c *Core) HandleChatwootWebhook(ctx context.Context, env chatwoot.WebhookEnvelope, signature, hmacToken, hmacMode string, rawBody []byte) error {
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
	if env.Event != "message_created" {
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "skipped_event").Inc()
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

	// 4. Valida HMAC (defesa em profundidade — handler já validou, mas
	//    checamos de novo aqui dentro caso o handler tenha sido bypassado)
	if hmacToken != "" {
		if !chatwoot.VerifySignature(signature, hmacToken, string(rawBody), hmacMode) {
			metrics.BridgeErrors.WithLabelValues("hmac_invalid", "bridge").Inc()
			c.logAudit(ctx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), env.Conversation.ContactInbox.SourceID, nil, "error", "hmac_invalid", "HMAC mismatch in core", 0)
			return fmt.Errorf("bridge: HMAC invalid for inbox %d", env.Conversation.InboxID)
		}
	}

	// 5. Resolve JID via contact_map
	jid := env.Conversation.ContactInbox.SourceID
	if jid == "" {
		metrics.BridgeErrors.WithLabelValues("empty_source_id", "bridge").Inc()
		c.logAudit(ctx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), "", nil, "error", "empty_source_id", "contact_inbox.source_id is empty", 0)
		return fmt.Errorf("bridge: empty source_id")
	}
	// Para Etapa 1, contact_map precisa estar populado (via `connect add-contact`).
	// Verificamos se o JID é bem formado.
	if !strings.Contains(jid, "@") {
		jid = jid + "@s.whatsapp.net"
	}

	// 6. Idempotência
	idempKey := fmt.Sprintf("c2w:%s:%d", tenant.Name, env.ID)
	existing, found, err := c.Store.CheckIdempotency(ctx, string(DirC2W), idempKey)
	if err != nil {
		return fmt.Errorf("bridge: check idempotency: %w", err)
	}
	if found {
		metrics.IdempotencyHits.Inc()
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "skipped_duplicate").Inc()
		log.Info("bridge: duplicate skipped", "idemp_status", existing.Status)
		return nil // idempotente: responde 200 sem reenviar
	}

	// 7. Envia via evolution-go
	evo := evogo.NewClient(tenant.EvoBaseURL, tenant.EvoAPIKey)
	number := strings.Split(jid, "@")[0] // evolution-go recebe só dígitos, sem @s.whatsapp.net

	var err2 error
	if len(env.Attachments) == 0 {
		// Texto puro
		_, err2 = evo.SendText(ctx, tenant.EvoInstanceName, number, env.Content)
	} else {
		// Mídia: envia a primeira (Etapa 1 simplifica; Etapa 5 amplia)
		att := env.Attachments[0]
		mediaReq := evogo.SendMediaRequest{
			Number:    number,
			MediaType: normalizeMediaType(att.FileType),
			Media:     att.FileURL,
			FileName:  att.FileName,
			Caption:   env.Content,
		}
		_, err2 = evo.SendMedia(ctx, tenant.EvoInstanceName, mediaReq)
	}

	latency := time.Since(start)
	latencyMS := int(latency.Milliseconds())
	if err2 != nil {
		errCode := "evo_send_failed"
		errDetail := err2.Error()
		metrics.BridgeErrors.WithLabelValues(errCode, "bridge").Inc()
		metrics.BridgeMessages.WithLabelValues(string(DirC2W), "error").Inc()
		metrics.BridgeLatency.WithLabelValues(string(DirC2W)).Observe(latency.Seconds())
		log.Error("bridge: evo send failed", "err", err2, "jid_masked", applog.MaskPhone(jid))
		// Grava idempotência como failed + audit log
		_ = c.Store.RecordIdempotency(ctx, string(DirC2W), idempKey, tenant.ID, "failed", []byte(errDetail), c.IdempotencyTTL)
		c.logAudit(ctx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), jid, []byte(applog.ContentHash(env.Content)), "error", errCode, errDetail, latencyMS)
		return fmt.Errorf("bridge: evo send: %w", err2)
	}

	// 8. Sucesso — grava idempotência + audit
	detail := json.RawMessage(fmt.Sprintf(`{"latency_ms":%d}`, latencyMS))
	_ = c.Store.RecordIdempotency(ctx, string(DirC2W), idempKey, tenant.ID, "sent", detail, c.IdempotencyTTL)
	c.logAudit(ctx, tenant.ID, DirC2W, fmt.Sprintf("%d", env.ID), jid, []byte(applog.ContentHash(env.Content)), "ok", "", "", latencyMS)

	metrics.BridgeMessages.WithLabelValues(string(DirC2W), "ok").Inc()
	metrics.BridgeLatency.WithLabelValues(string(DirC2W)).Observe(latency.Seconds())
	log.Info("bridge: c2w delivered",
		"jid_masked", applog.MaskPhone(jid),
		"content_hash", applog.ContentHash(env.Content),
		"latency_ms", latency.Milliseconds(),
	)
	return nil
}

func (c *Core) logAudit(ctx context.Context, tenantID uuid.UUID, dir Direction, extMsgID, jid string, payloadHash []byte, status, errCode, errDetail string, latencyMS int) {
	if err := c.Store.LogBridge(ctx, store.BridgeLogEntry{
		TenantID:          tenantID,
		Direction:         string(dir),
		ExternalMessageID: extMsgID,
		JID:               jid,
		PayloadSHA256:     payloadHash,
		Status:            status,
		ErrorCode:         errCode,
		ErrorDetail:       errDetail,
		LatencyMS:         latencyMS,
	}); err != nil {
		slog.Default().Error("bridge: log audit failed", "err", err, "tenant", tenantID)
	}
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
