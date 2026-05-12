// Package handlers contains Worker event handlers.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/service/audio"
)

// StoryReader is the minimal read surface this handler needs.
type StoryReader interface {
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}

// StoryAudioWriter is the minimal write surface this handler needs.
type StoryAudioWriter interface {
	MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int, hasBGM bool) error
	MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error
}

// TTSHandlerConfig captures the synthesis params drawn from cfg.TTS.
type TTSHandlerConfig struct {
	Provider   string
	Model      string
	VoiceID    string
	Format     string
	SampleRate int
	Bitrate    int
	Speed      float64
}

// Composer is the minimal surface the handler needs from audio.Orchestrator.
type Composer interface {
	Compose(ctx context.Context, req audio.ComposeRequest) (*audio.ComposeResponse, error)
}

// TTSSynthesisHandler composes story audio (TTS + BGM) and uploads to storage.
type TTSSynthesisHandler struct {
	stories  StoryReader
	repo     StoryAudioWriter
	composer Composer
	storage  storage.Client
	cfg      TTSHandlerConfig
	bm       *metrics.Business
}

// NewTTSSynthesisHandler constructs the handler.
func NewTTSSynthesisHandler(
	stories StoryReader, repo StoryAudioWriter,
	composer Composer, s storage.Client,
	cfg TTSHandlerConfig, bm *metrics.Business,
) *TTSSynthesisHandler {
	return &TTSSynthesisHandler{stories: stories, repo: repo, composer: composer, storage: s, cfg: cfg, bm: bm}
}

type ttsSynthesisPayload struct {
	StoryID int64 `json:"story_id"`
}

// Handle is the worker entry point.
func (h *TTSSynthesisHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
	lg := logger.FromCtx(ctx).With("module", "tts_handler", "event_id", e.ID)

	var p ttsSynthesisPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	storyID := p.StoryID
	if storyID == 0 && e.AggregateID != nil {
		storyID = *e.AggregateID
	}
	if storyID == 0 {
		return errors.New("payload missing story_id and event missing aggregate_id")
	}

	story, err := h.stories.FindByID(ctx, storyID)
	if err != nil {
		return fmt.Errorf("load story %d: %w", storyID, err)
	}
	if story == nil {
		return fmt.Errorf("story %d not found", storyID)
	}
	if story.AudioStatus == model.AudioStatusReady && story.AudioObjectKey != "" {
		lg.Info("tts.skip.already_ready", "story_id", storyID, "key", story.AudioObjectKey)
		return nil
	}

	composed, err := h.composer.Compose(ctx, audio.ComposeRequest{
		StoryID:   story.ID,
		ChildID:   story.ChildID,
		StoryText: story.TextContent,
		Style:     story.Style,
		Voice: tts.SynthesizeRequest{
			VoiceID: h.cfg.VoiceID, Model: h.cfg.Model,
			Format: h.cfg.Format, SampleRate: h.cfg.SampleRate,
			Bitrate: h.cfg.Bitrate, Speed: h.cfg.Speed,
		},
	})
	if err != nil {
		if h.bm != nil {
			h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "fail").Inc()
			h.bm.AudioFailedTotal.WithLabelValues("tts").Inc()
		}
		if mErr := h.repo.MarkAudioFailed(ctx, storyID, err.Error()); mErr != nil {
			lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
		}
		return fmt.Errorf("audio compose: %w", err)
	}
	if h.bm != nil {
		h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "ok").Inc()
	}
	lg.Info("audio.compose.ok", "story_id", storyID,
		"bytes", len(composed.AudioBytes), "dur_sec", composed.AudioDurationSec,
		"has_bgm", composed.HasBGM, "mood", composed.Mood)

	key := buildObjectKey(story.ChildID, story.ID, h.cfg.Format)
	uStart := time.Now()
	err = h.storage.Upload(ctx, storage.UploadInput{
		Key: key, Body: bytes.NewReader(composed.AudioBytes), Size: int64(len(composed.AudioBytes)),
		ContentType: contentTypeFor(h.cfg.Format),
	})
	if h.bm != nil {
		h.bm.StorageUploadDuration.WithLabelValues("cos").Observe(time.Since(uStart).Seconds())
	}
	if err != nil {
		if h.bm != nil {
			h.bm.AudioFailedTotal.WithLabelValues("storage").Inc()
		}
		if mErr := h.repo.MarkAudioFailed(ctx, storyID, err.Error()); mErr != nil {
			lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
		}
		return fmt.Errorf("storage upload: %w", err)
	}
	lg.Info("storage.upload.ok", "story_id", storyID, "key", key)

	if err := h.repo.MarkAudioReady(ctx, storyID, key, h.cfg.Format,
		int64(len(composed.AudioBytes)), composed.AudioDurationSec, composed.HasBGM); err != nil {
		if h.bm != nil {
			h.bm.AudioFailedTotal.WithLabelValues("db").Inc()
		}
		return fmt.Errorf("mark audio ready: %w", err)
	}
	if h.bm != nil {
		h.bm.AudioReadyTotal.Inc()
	}
	return nil
}

// buildObjectKey returns "audio/{child_id}/{story_id}-{ts_nano}.{ext}".
func buildObjectKey(childID, storyID int64, format string) string {
	return fmt.Sprintf("audio/%d/%d-%d.%s", childID, storyID, time.Now().UnixNano(), format)
}

func contentTypeFor(format string) string {
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "application/octet-stream"
	}
}
