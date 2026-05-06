package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPMetrics_Registered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)
	require.NotNil(t, m)

	m.HTTPRequests.WithLabelValues("/health", "200").Inc()
	m.HTTPDuration.WithLabelValues("/health", "200").Observe(0.012)

	got := testutil.CollectAndCount(m.HTTPRequests)
	assert.Equal(t, 1, got)

	mf, err := reg.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(mf))
	for _, f := range mf {
		names = append(names, f.GetName())
	}
	joined := strings.Join(names, ",")
	assert.Contains(t, joined, "http_requests_total")
	assert.Contains(t, joined, "http_request_duration_seconds")
}
