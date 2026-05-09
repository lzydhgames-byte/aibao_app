// Package handlers contains Worker event handlers.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

// MemoryUpdateHandler writes a memory record summarizing the just-finished
// story. Idempotent via INSERT (duplicate handler runs leave a tiny extra
// row, harmless and rare). For stricter idempotency, a unique index on
// (child_id, story_id) could be added later.
type MemoryUpdateHandler struct {
	memories repository.MemoryRepo
}

// NewMemoryUpdateHandler constructs a handler.
func NewMemoryUpdateHandler(m repository.MemoryRepo) *MemoryUpdateHandler {
	return &MemoryUpdateHandler{memories: m}
}

// memoryUpdatePayload mirrors the orchestrator's emit shape.
type memoryUpdatePayload struct {
	StoryID      int64  `json:"story_id"`
	ChildID      int64  `json:"child_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	UsedFallback bool   `json:"used_fallback"`
}

// Handle parses payload and writes a memories row.
func (h *MemoryUpdateHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
	var p memoryUpdatePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	innerJSON, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("re-encode payload: %w", err)
	}
	return h.memories.Create(ctx, &model.Memory{
		ChildID:    p.ChildID,
		MemoryType: "story_summary",
		Payload:    innerJSON,
		Weight:     1.0,
	})
}
