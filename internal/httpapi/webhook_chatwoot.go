package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edbentto22/evogo-connect/internal/bridge"
	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/metrics"
)

// chatwootWebhookHandler recebe POST /webhook/chatwoot
//
// Validação:
//  1. Lê body cru (pra HMAC)
//  2. Resolve tenant pelo `inbox_id` (top-level OU dentro de conversation)
//  3. Valida HMAC com o token do tenant
//  4. Decodifica envelope completo
//  5. Chama bridge.Core.HandleChatwootWebhook
//
// Resposta:
//   - 200: processado (inclusive se skipped/duplicata — é idempotente)
//   - 401: HMAC inválido
//   - 503: kill switch ativo
func chatwootWebhookHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Lê body cru pra HMAC
		rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, 5*1024*1024)) // 5MB max
		if err != nil {
			respondErr(c, http.StatusBadRequest, "read_body", err)
			return
		}

		// Decode envelope completo
		var env chatwoot.WebhookEnvelope
		if err := json.Unmarshal(rawBody, &env); err != nil {
			respondErr(c, http.StatusBadRequest, "decode_envelope", err)
			return
		}

		// Resolve inbox_id (top-level OU dentro de conversation OU contact_inbox)
		inboxID := env.InboxID
		if inboxID == 0 {
			inboxID = env.Conversation.InboxID
		}
		if inboxID == 0 {
			inboxID = env.Conversation.ContactInbox.InboxID
		}
		if inboxID == 0 {
			inboxID = extractInboxIDFromBody(rawBody)
		}
		if inboxID == 0 {
			respondErr(c, http.StatusBadRequest, "missing_inbox", errors.New("inbox_id not found in payload"))
			return
		}

		// Resolve tenant
		tenant, err := d.Store.GetTenantByChatwootInbox(c.Request.Context(), inboxID)
		if err != nil {
			respondErr(c, http.StatusNotFound, "tenant_not_found", err)
			return
		}

		// Valida HMAC se tenant tiver token configurado
		signature := c.GetHeader("X-Chatwoot-Signature")
		if tenant.ChatwootHMAC != "" {
			if !chatwoot.VerifySignature(signature, tenant.ChatwootHMAC, string(rawBody), "plain") {
				metrics.BridgeErrors.WithLabelValues("hmac_invalid", "http").Inc()
				respondErr(c, http.StatusUnauthorized, "invalid_signature", errors.New("HMAC invalid"))
				return
			}
		}

		// Processa via core
		ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
		defer cancel()
		err = d.Bridge.HandleChatwootWebhook(ctx, env, signature, tenant.ChatwootHMAC, "plain", rawBody)
		if err != nil {
			switch {
			case errors.Is(err, bridge.ErrPaused):
				respondErr(c, http.StatusServiceUnavailable, "paused", err)
			case errors.Is(err, bridge.ErrSkipped):
				c.JSON(http.StatusOK, gin.H{"status": "skipped", "reason": err.Error()})
			default:
				respondErr(c, http.StatusInternalServerError, "bridge_error", err)
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func respondErr(c *gin.Context, code int, label string, err error) {
	metrics.HTTPRequests.WithLabelValues(c.Request.Method, c.FullPath(), strconv.Itoa(code)).Inc()
	c.JSON(code, gin.H{"error": label, "detail": err.Error()})
}
