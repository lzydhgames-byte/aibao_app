// Package metrics defines Prometheus metric vectors used across the server.
// Infrastructure-level HTTP metrics live here; business metrics (story_generate_total,
// llm_call_duration_seconds, etc) will be added by their respective packages
// using the same registry.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the core HTTP-layer metric vectors.
type Metrics struct {
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
}

// New registers core HTTP metrics on the given registry and returns the
// Metrics struct holding the registered vectors. In main.go, pass a
// prometheus.NewRegistry() so the registry is also exposed at /metrics.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total HTTP requests by path and status.",
			},
			[]string{"path", "status"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration by path and status.",
				Buckets: prometheus.ExponentialBuckets(0.005, 2, 12), // 5ms .. ~20s
			},
			[]string{"path", "status"},
		),
	}
	reg.MustRegister(m.HTTPRequests, m.HTTPDuration)
	return m
}
