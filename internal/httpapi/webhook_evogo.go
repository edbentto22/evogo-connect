package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edbentto22/evogo-connect/internal/bridge"
	"github.com/edbentto22/evogo-connect/internal/evogo"
	"github.com/edbentto22/evogo-connect/internal/metrics"
)

// evogoWebhookHandler recebe eventos de uma única instância Evolution Go. A
// autenticação usa um segredo aleatório no caminho porque a versão suportada
// da Evolution Go não oferece assinatura/header configurável para webhooks.
func evogoWebhookHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.Param("instance")
		tenant, err := d.Store.GetTenantByEvoInstance(c.Request.Context(), instance)
		if err != nil || tenant.EvoWebhookSecret == "" || !sameWebhookSecret(c.Param("secret"), tenant.EvoWebhookSecret) {
			metrics.BridgeErrors.WithLabelValues("evo_webhook_unauthorized", "http").Inc()
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		body := http.MaxBytesReader(c.Writer, c.Request.Body, 5*1024*1024)
		raw, err := io.ReadAll(body)
		if err != nil {
			respondErr(c, http.StatusBadRequest, "read_body", err)
			return
		}
		var env evogo.WebhookEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			respondErr(c, http.StatusBadRequest, "decode_envelope", err)
			return
		}
		if env.Instance != "" && env.Instance != tenant.EvoInstanceName {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
		defer cancel()
		err = d.Bridge.HandleEvogoWebhook(ctx, tenant, env)
		if err == nil || errors.Is(err, bridge.ErrSkipped) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		switch {
		case errors.Is(err, bridge.ErrPaused), errors.Is(err, bridge.ErrInProgress):
			respondErr(c, http.StatusServiceUnavailable, "retry", err)
		case errors.Is(err, bridge.ErrRateLimited):
			respondErr(c, http.StatusTooManyRequests, "rate_limited", err)
		default:
			respondErr(c, http.StatusServiceUnavailable, "bridge_error", err)
		}
	}
}

func sameWebhookSecret(got, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}
