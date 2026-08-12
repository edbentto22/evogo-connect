// Package metrics expõe coletores Prometheus.
//
// Métricas:
//   - bridge_messages_total{direction, status} — contador de mensagens bridgeadas
//   - bridge_latency_seconds{direction} — histograma de latência por sentido
//   - bridge_errors_total{code, component} — erros categorizados
//   - http_requests_total{method, path, status} — tráfego HTTP
//   - idempotency_hits_total — mensagens deduplicadas
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// BridgeMessages conta mensagens processadas.
	// direction: "c2w" (chatwoot→whatsapp) ou "w2c" (whatsapp→chatwoot)
	// status: "ok", "skipped_duplicate", "error", "paused", "rate_limited"
	BridgeMessages = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_messages_total",
			Help: "Total de mensagens processadas pelo bridge.",
		},
		[]string{"direction", "status"},
	)

	// BridgeLatency mede latência end-to-end do bridge.
	BridgeLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bridge_latency_seconds",
			Help:    "Latência do bridge por sentido (segundos).",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"direction"},
	)

	// BridgeErrors conta erros categorizados.
	BridgeErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_errors_total",
			Help: "Total de erros do bridge, por código e componente.",
		},
		[]string{"code", "component"},
	)

	// HTTPRequests conta requisições HTTP.
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total de requisições HTTP.",
		},
		[]string{"method", "path", "status"},
	)

	// IdempotencyHits conta mensagens que caíram em idempotência.
	IdempotencyHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "idempotency_hits_total",
			Help: "Total de mensagens deduplicadas pela camada de idempotência.",
		},
	)
)
