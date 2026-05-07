package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/metrics"
)

func TestMetrics_RecordsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	r := gin.New()
	r.Use(Metrics(m))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	mf, err := reg.Gather()
	require.NoError(t, err)
	var seen bool
	for _, f := range mf {
		if f.GetName() == "http_requests_total" {
			for _, met := range f.GetMetric() {
				if met.GetCounter().GetValue() > 0 {
					seen = true
				}
			}
		}
	}
	assert.True(t, seen, "expected counter to be incremented")
}
