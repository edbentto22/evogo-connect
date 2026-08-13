// Package httpapi implementa o servidor HTTP (Gin) com os endpoints do
// evogo-connect:
//
//	GET  /healthz                  — liveness
//	GET  /readyz                   — readiness (checa DB)
//	GET  /metrics                  — Prometheus
//	POST /webhook/chatwoot         — webhook do Chatwoot (Etapa 1)
//	POST /admin/pause              — kill switch
//	POST /admin/resume             — kill switch off
//	GET  /admin/tenants            — listar tenants (debug)
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/edbentto22/evogo-connect/internal/bridge"
	"github.com/edbentto22/evogo-connect/internal/store"
)

// Deps agrupa as dependências dos handlers.
type Deps struct {
	Store        HTTPStore
	Bridge       *bridge.Core
	AdminToken   string
	ReplayWindow time.Duration
}

// HTTPStore contém as operações persistentes usadas pela camada HTTP.
type HTTPStore interface {
	Ping(context.Context) error
	GetTenantByChatwootInbox(context.Context, int) (*store.Tenant, error)
	GetTenantByEvoInstance(context.Context, string) (*store.Tenant, error)
	SetPaused(context.Context, bool, string) error
	IsPaused(context.Context) (bool, error)
	ListTenants(context.Context) ([]store.Tenant, error)
}

// NewRouter monta o router Gin.
func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Store.Ping(ctx); err != nil {
			slog.Warn("readiness check failed", "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Webhook do Chatwoot (Etapa 1)
	r.POST("/webhook/chatwoot", chatwootWebhookHandler(d))
	r.POST("/webhook/evo/:instance/:secret", evogoWebhookHandler(d))

	// Admin
	admin := r.Group("/admin", adminAuth(d.AdminToken))
	admin.POST("/pause", pauseHandler(d))
	admin.POST("/resume", resumeHandler(d))
	admin.GET("/tenants", listTenantsHandler(d))
	admin.GET("/paused", isPausedHandler(d))

	return r
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Default().Info("http",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}

func adminAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("X-Admin-Token")
		if h == "" || h != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
