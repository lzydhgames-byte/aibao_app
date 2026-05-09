package metrics

import "github.com/prometheus/client_golang/prometheus"

// Business holds the business-level metric vectors.
// Story-generation pipeline + LLM calls + safety + outbox + budget.
type Business struct {
	StoryGenerateTotal     *prometheus.CounterVec
	StoryGenerateDuration  prometheus.Histogram
	LLMCallDuration        *prometheus.HistogramVec
	LLMCallTotal           *prometheus.CounterVec
	SafetyFailTotal        *prometheus.CounterVec
	OutboxPendingCount     prometheus.Gauge
	OutboxDeadTotal        *prometheus.CounterVec
	LLMBudgetUsedYuan      prometheus.Gauge
	ExternalAPIErrorTotal  *prometheus.CounterVec
}

// NewBusiness registers all business metrics on reg and returns the bundle.
func NewBusiness(reg prometheus.Registerer) *Business {
	b := &Business{
		StoryGenerateTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "story_generate_total",
				Help: "Total story generation outcomes by status (ok/fail/fallback).",
			}, []string{"status"},
		),
		StoryGenerateDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "story_generate_duration_seconds",
				Help:    "Story generation end-to-end duration.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8), // 0.5s..~64s
			},
		),
		LLMCallDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "llm_call_duration_seconds",
				Help:    "LLM API call duration by provider.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
			}, []string{"provider"},
		),
		LLMCallTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_call_total",
				Help: "Total LLM API calls by provider and status.",
			}, []string{"provider", "status"},
		),
		SafetyFailTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "safety_fail_total",
				Help: "Safety pipeline rejections by stage and reason.",
			}, []string{"stage", "reason"},
		),
		OutboxPendingCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "outbox_pending_count",
				Help: "Current count of outbox_events with status='pending'.",
			},
		),
		OutboxDeadTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "outbox_dead_total",
				Help: "Cumulative outbox events that hit max retries (status='dead').",
			}, []string{"event_type"},
		),
		LLMBudgetUsedYuan: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "llm_budget_used_yuan",
				Help: "Today's accumulated LLM cost in yuan.",
			},
		),
		ExternalAPIErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "external_api_error_total",
				Help: "External API error count by provider.",
			}, []string{"provider"},
		),
	}
	reg.MustRegister(
		b.StoryGenerateTotal,
		b.StoryGenerateDuration,
		b.LLMCallDuration,
		b.LLMCallTotal,
		b.SafetyFailTotal,
		b.OutboxPendingCount,
		b.OutboxDeadTotal,
		b.LLMBudgetUsedYuan,
		b.ExternalAPIErrorTotal,
	)
	return b
}
