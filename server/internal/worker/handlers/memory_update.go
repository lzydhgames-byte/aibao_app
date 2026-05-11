// Package handlers contains Worker event handlers.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

// Summarizer is the minimal interface MemoryUpdateHandler needs from
// service/memory.Summarizer. Kept as an interface so tests can stub it
// without spinning up an LLM client.
type Summarizer interface {
	Summarize(ctx context.Context, storyText string) string
}

// MemoryUpdateHandler writes a memory record summarizing the just-finished
// story. Idempotent via INSERT (duplicate handler runs leave a tiny extra
// row, harmless and rare). For stricter idempotency, a unique index on
// (child_id, story_id) could be added later.
//
// Plan 6: in addition to the canonical orchestrator-emitted row, this
// handler optionally writes a second row whose payload.summary is the
// LLM-produced one-sentence (~30 char) version, suitable for cheap
// prompt-context injection on the next story.
type MemoryUpdateHandler struct {
	memories   repository.MemoryRepo
	stories    StoryReader
	summarizer Summarizer
}

// NewMemoryUpdateHandler constructs a handler. stories and summarizer may
// be nil; in that case the Plan 6 LLM-summary path is skipped.
func NewMemoryUpdateHandler(m repository.MemoryRepo, s StoryReader, sum Summarizer) *MemoryUpdateHandler {
	return &MemoryUpdateHandler{memories: m, stories: s, summarizer: sum}
}

// memoryUpdatePayload mirrors the orchestrator's emit shape.
type memoryUpdatePayload struct {
	StoryID      int64  `json:"story_id"`
	ChildID      int64  `json:"child_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	UsedFallback bool   `json:"used_fallback"`
}

// Handle parses payload and writes a memories row. Plan 6: also enqueues
// an LLM-derived summary row (fail-open — never returns the secondary error).
func (h *MemoryUpdateHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
	var p memoryUpdatePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	// Resolve story_id: payload value is the orchestrator's stale snapshot
	// (set to 0 before the transaction filled story.ID). Trust e.AggregateID
	// when payload value is missing — same pattern as tts_synthesis handler.
	storyID := p.StoryID
	if storyID == 0 && e.AggregateID != nil {
		storyID = *e.AggregateID
	}

	innerJSON, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("re-encode payload: %w", err)
	}
	var storyFK *int64
	if storyID > 0 {
		storyFK = &storyID
	}
	if err := h.memories.Create(ctx, &model.Memory{
		ChildID:    p.ChildID,
		MemoryType: model.MemoryTypeStorySummary,
		Payload:    innerJSON,
		Weight:     1.0,
		StoryID:    storyFK,
	}); err != nil {
		return err
	}

	// Plan 6: LLM-summarize fresh story text into a second, shorter memory.
	if h.summarizer == nil || h.stories == nil {
		return nil
	}
	if storyID == 0 {
		return nil
	}
	story, err := h.stories.FindByID(ctx, storyID)
	if err != nil || story == nil {
		return nil
	}
	summary := h.summarizer.Summarize(ctx, story.TextContent)
	if summary == "" {
		return nil
	}
	sumPayload, _ := json.Marshal(map[string]interface{}{
		"type":          "story_summary",
		"summary":       summary,
		"story_id":      storyID,
		"title":         p.Title,
		"used_fallback": p.UsedFallback,
	})
	_ = h.memories.Create(ctx, &model.Memory{
		ChildID:    p.ChildID,
		MemoryType: model.MemoryTypeStorySummary,
		Payload:    sumPayload,
		Weight:     1.2,
		StoryID:    storyFK,
	})
	return nil
}
