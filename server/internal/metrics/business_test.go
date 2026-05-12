package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessMetrics_Registered(t *testing.T) {
	reg := prometheus.NewRegistry()
	bm := NewBusiness(reg)
	require.NotNil(t, bm)

	bm.StoryGenerateTotal.WithLabelValues("ok").Inc()
	bm.StoryGenerateDuration.Observe(12.3)
	bm.LLMCallDuration.WithLabelValues("doubao").Observe(8.0)
	bm.LLMCallTotal.WithLabelValues("doubao", "ok").Inc()
	bm.SafetyFailTotal.WithLabelValues("pre", "redline_matched").Inc()
	bm.OutboxPendingCount.Set(3)
	bm.OutboxDeadTotal.WithLabelValues("memory_update").Inc()
	bm.LLMBudgetUsedYuan.Set(12.5)
	bm.ExternalAPIErrorTotal.WithLabelValues("doubao").Inc()
	bm.TTSCallDuration.WithLabelValues("minimax").Observe(2.0)
	bm.TTSCallTotal.WithLabelValues("minimax", "ok").Inc()
	bm.StorageUploadDuration.WithLabelValues("cos").Observe(0.4)
	bm.AudioPendingCount.Set(2)
	bm.AudioReadyTotal.Inc()
	bm.AudioFailedTotal.WithLabelValues("tts").Inc()
	bm.MemorySummaryDuration.Observe(0.5)
	bm.MemorySummaryTotal.WithLabelValues("ok").Inc()
	bm.MemorySummaryTotal.WithLabelValues("fail").Inc()
	bm.BootstrapCompletionTotal.Inc()
	bm.LLMFailFallbackTotal.WithLabelValues("doubao", "doubao-lite", "upstream_error").Inc()
	bm.AudioMixDuration.Observe(1.5)
	bm.AudioMixTotal.WithLabelValues("ok").Inc()
	bm.AudioMixTotal.WithLabelValues("degraded").Inc()
	bm.AudioMixTotal.WithLabelValues("failed").Inc()
	bm.BGMNotFoundTotal.WithLabelValues("warm").Inc()

	mf, err := reg.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(mf))
	for _, f := range mf {
		names = append(names, f.GetName())
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{
		"story_generate_total",
		"story_generate_duration_seconds",
		"llm_call_duration_seconds",
		"llm_call_total",
		"safety_fail_total",
		"outbox_pending_count",
		"outbox_dead_total",
		"llm_budget_used_yuan",
		"external_api_error_total",
		"tts_call_duration_seconds",
		"tts_call_total",
		"storage_upload_duration_seconds",
		"audio_pending_count",
		"audio_ready_total",
		"audio_failed_total",
		"memory_summary_duration_seconds",
		"memory_summary_total",
		"bootstrap_completion_total",
		"llm_fail_fallback_total",
		"audio_mix_duration_seconds",
		"audio_mix_total",
		"bgm_not_found_total",
	} {
		assert.Contains(t, joined, want, "missing metric %s", want)
	}
}
