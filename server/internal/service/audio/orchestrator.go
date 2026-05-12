package audio

import (
	"context"
	"errors"
	"time"

	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/repository"
)

// BGMPicker is the minimal surface Orchestrator needs from BGMRepo.
type BGMPicker interface {
	PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error)
}

// ComposeRequest is the input to Compose.
type ComposeRequest struct {
	StoryID   int64
	ChildID   int64
	StoryText string                // raw LLM output (with cues)
	Style     string                // story.Style for mood fallback
	Voice     tts.SynthesizeRequest // voice/model/format pre-filled by caller; Text overwritten
}

// ComposeResponse is what Orchestrator returns.
type ComposeResponse struct {
	AudioBytes       []byte
	AudioFormat      string
	AudioDurationSec int
	HasBGM           bool
	Mood             string
	BGMFilename      string
	ParseResult      ParseResult
}

// Orchestrator wires parser → tts → bgm pick → cache → mixer.
type Orchestrator struct {
	tts   tts.Client
	bgm   BGMPicker
	cache BGMCache
	mixer Mixer
	bm    *metrics.Business
}

// NewOrchestrator constructs an Orchestrator. Any of bm may be nil for tests.
func NewOrchestrator(t tts.Client, bgm BGMPicker, c BGMCache, m Mixer, bm *metrics.Business) *Orchestrator {
	return &Orchestrator{tts: t, bgm: bgm, cache: c, mixer: m, bm: bm}
}

// Compose runs the full pipeline. TTS failure is hard-error; every later
// failure degrades gracefully to pure TTS audio.
func (o *Orchestrator) Compose(ctx context.Context, req ComposeRequest) (*ComposeResponse, error) {
	lg := logger.FromCtx(ctx).With("module", "audio.orchestrator", "story_id", req.StoryID)

	// 1. Parse cues.
	pr := Parse(req.StoryText, req.Style)
	lg.Info("audio.parse", "mood", pr.BGMMood, "cue_count", len(pr.Cues),
		"clean_len", len(pr.CleanText))

	// 2. TTS on clean text.
	voiceReq := req.Voice
	voiceReq.Text = pr.CleanText
	ttsResp, err := o.tts.Synthesize(ctx, voiceReq)
	if err != nil {
		return nil, err
	}

	base := &ComposeResponse{
		AudioBytes:       ttsResp.Audio,
		AudioFormat:      voiceReq.Format,
		AudioDurationSec: ttsResp.DurationSeconds,
		HasBGM:           false,
		Mood:             pr.BGMMood,
		ParseResult:      pr,
	}

	// 3. Pick BGM by mood.
	asset, err := o.bgm.PickByMood(ctx, pr.BGMMood)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			if o.bm != nil {
				o.bm.BGMNotFoundTotal.WithLabelValues(pr.BGMMood).Inc()
			}
			lg.Warn("audio.bgm.not_found", "mood", pr.BGMMood)
			return o.degrade(ctx, base, "bgm_not_found"), nil
		}
		lg.Warn("audio.bgm.pick.err", "mood", pr.BGMMood, "err", err.Error())
		return o.degrade(ctx, base, "bgm_pick_err"), nil
	}
	if asset == nil {
		if o.bm != nil {
			o.bm.BGMNotFoundTotal.WithLabelValues(pr.BGMMood).Inc()
		}
		lg.Warn("audio.bgm.not_found", "mood", pr.BGMMood)
		return o.degrade(ctx, base, "bgm_not_found"), nil
	}

	// 4. Local cache.
	bgmPath, err := o.cache.GetLocalPath(ctx, asset)
	if err != nil {
		lg.Warn("audio.bgm.cache.err", "filename", asset.Filename, "err", err.Error())
		return o.degrade(ctx, base, "bgm_unavailable"), nil
	}

	// 5. Mix.
	tStart := time.Now()
	mixed, dur, err := o.mixer.MixWithBGM(ctx, ttsResp.Audio, bgmPath)
	if o.bm != nil {
		o.bm.AudioMixDuration.Observe(time.Since(tStart).Seconds())
	}
	if err != nil {
		status := "failed"
		if errors.Is(err, ErrMixerUnavailable) {
			status = "degraded"
		}
		if o.bm != nil {
			o.bm.AudioMixTotal.WithLabelValues(status).Inc()
		}
		lg.Warn("audio.mix.fail", "err", err.Error())
		return o.degrade(ctx, base, "mixer_fail"), nil
	}
	if o.bm != nil {
		o.bm.AudioMixTotal.WithLabelValues("ok").Inc()
	}

	base.AudioBytes = mixed
	base.AudioDurationSec = dur
	base.HasBGM = true
	base.BGMFilename = asset.Filename
	lg.Info("audio.mix.ok", "mood", pr.BGMMood, "bgm", asset.Filename,
		"out_bytes", len(mixed), "dur_sec", dur)
	return base, nil
}

func (o *Orchestrator) degrade(ctx context.Context, base *ComposeResponse, reason string) *ComposeResponse {
	if o.bm != nil {
		o.bm.AudioMixTotal.WithLabelValues("degraded").Inc()
	}
	logger.FromCtx(ctx).Warn("audio.mix.degraded", "reason", reason, "mood", base.Mood)
	return base
}
