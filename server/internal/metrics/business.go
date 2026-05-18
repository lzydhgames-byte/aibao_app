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

	// Plan 5
	TTSCallDuration       *prometheus.HistogramVec // labels: provider
	TTSCallTotal          *prometheus.CounterVec   // labels: provider, status
	StorageUploadDuration *prometheus.HistogramVec // labels: provider
	AudioPendingCount     prometheus.Gauge
	AudioReadyTotal       prometheus.Counter
	AudioFailedTotal      *prometheus.CounterVec // labels: stage (tts/storage/db)

	// Plan 9c third battle: cost-tracking. Counts CJK runes actually sent
	// to TTS (basis for the bill) and how many of those exceeded the
	// duration-slot's expected ceiling. Lets us watch
	// rate(tts_chars_excess_total) / rate(tts_chars_total) — if it
	// trends above ~10% we are over-paying for audio that ran past the
	// requested duration. Labels: duration_min (3/5/8).
	TTSCharsTotal       *prometheus.CounterVec
	TTSCharsExcessTotal *prometheus.CounterVec

	// Plan 6
	MemorySummaryDuration    prometheus.Histogram
	MemorySummaryTotal       *prometheus.CounterVec // labels: status (ok|fail)
	BootstrapCompletionTotal prometheus.Counter

	// Plan 6b
	LLMFailFallbackTotal *prometheus.CounterVec // labels: provider, model, reason

	// Plan 7
	AudioMixDuration prometheus.Histogram     // no labels
	AudioMixTotal    *prometheus.CounterVec   // labels: status (ok/degraded/failed)
	BGMNotFoundTotal *prometheus.CounterVec   // labels: mood

	// Plan 8
	StorylineCreatedTotal      prometheus.Counter
	StorylineEpisodesTotal     prometheus.Counter
	ChapterHookExtractDuration prometheus.Histogram
	ChapterHookExtractTotal    *prometheus.CounterVec // labels: status (ok/fail)
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
		TTSCallDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "tts_call_duration_seconds",
				Help:    "TTS API call duration by provider.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
			}, []string{"provider"},
		),
		TTSCallTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tts_call_total",
				Help: "Total TTS API calls by provider and status.",
			}, []string{"provider", "status"},
		),
		TTSCharsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tts_chars_total",
				Help: "Total CJK runes sent to TTS by duration slot. Cost basis.",
			}, []string{"duration_min"},
		),
		TTSCharsExcessTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tts_chars_excess_total",
				Help: "Cumulative runes ABOVE the duration slot's expected ceiling. Pure waste.",
			}, []string{"duration_min"},
		),
		StorageUploadDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "storage_upload_duration_seconds",
				Help:    "Object storage upload duration by provider.",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),
			}, []string{"provider"},
		),
		AudioPendingCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audio_pending_count",
				Help: "Stories with audio_status='pending' waiting for synthesis.",
			},
		),
		AudioReadyTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "audio_ready_total",
				Help: "Stories that successfully reached audio_status='ready'.",
			},
		),
		AudioFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audio_failed_total",
				Help: "Stories that ended in audio_status='failed', labeled by failing stage.",
			}, []string{"stage"},
		),
		MemorySummaryDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "memory_summary_duration_seconds",
				Help:    "Latency of the cheap LLM call that summarizes a finished story into a one-sentence memory.",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),
			},
		),
		MemorySummaryTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "memory_summary_total",
				Help: "Count of memory-summary LLM calls.",
			}, []string{"status"},
		),
		BootstrapCompletionTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "bootstrap_completion_total",
				Help: "Count of successful BOOTSTRAP answer submissions.",
			},
		),
		LLMFailFallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_fail_fallback_total",
				Help: "Count of LLM call fail-open events (upstream error or unparseable), by provider/model/reason.",
			}, []string{"provider", "model", "reason"},
		),
		AudioMixDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "audio_mix_duration_seconds",
				Help:    "End-to-end audio mixing duration (TTS + BGM via ffmpeg).",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s..~51s
			},
		),
		AudioMixTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audio_mix_total",
				Help: "Count of audio mix attempts by status (ok/degraded/failed).",
			}, []string{"status"},
		),
		BGMNotFoundTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bgm_not_found_total",
				Help: "Count of BGM lookups that returned no row for a mood.",
			}, []string{"mood"},
		),
		StorylineCreatedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "storyline_created_total",
				Help: "Count of new storylines created.",
			},
		),
		StorylineEpisodesTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "storyline_episodes_total",
				Help: "Count of episodes appended to storylines.",
			},
		),
		ChapterHookExtractDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "chapter_hook_extract_duration_seconds",
				Help:    "Latency of the chapter hook extraction LLM call.",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),
			},
		),
		ChapterHookExtractTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "chapter_hook_extract_total",
				Help: "Count of chapter hook extraction attempts by status (ok/fail).",
			}, []string{"status"},
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
		b.TTSCallDuration,
		b.TTSCallTotal,
		b.TTSCharsTotal,
		b.TTSCharsExcessTotal,
		b.StorageUploadDuration,
		b.AudioPendingCount,
		b.AudioReadyTotal,
		b.AudioFailedTotal,
		b.MemorySummaryDuration,
		b.MemorySummaryTotal,
		b.BootstrapCompletionTotal,
		b.LLMFailFallbackTotal,
		b.AudioMixDuration,
		b.AudioMixTotal,
		b.BGMNotFoundTotal,
		b.StorylineCreatedTotal,
		b.StorylineEpisodesTotal,
		b.ChapterHookExtractDuration,
		b.ChapterHookExtractTotal,
	)
	return b
}
