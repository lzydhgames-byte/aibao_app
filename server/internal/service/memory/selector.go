package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/aibao/server/internal/model"
)

// MemoryRepoLike is the minimum surface Selector needs from the memory repo.
// Declared here (rather than depending on repository.MemoryRepo) so tests can
// inject a tiny fake without pulling in DB plumbing.
type MemoryRepoLike interface {
	RecentByChildTypes(ctx context.Context, childID int64, types []string, limit int) ([]*model.Memory, error)
}

// Selector reads recent memories and renders them as a single line of
// "soft hints" for the story system prompt.
type Selector struct {
	repo   MemoryRepoLike
	types  []string
	limit  int
	logger *slog.Logger
}

// NewSelector constructs a Selector that pulls story_summary + interest types.
func NewSelector(repo MemoryRepoLike, limit int, logger *slog.Logger) *Selector {
	if limit <= 0 {
		limit = 3
	}
	return &Selector{
		repo:   repo,
		types:  []string{model.MemoryTypeStorySummary, model.MemoryTypeInterest},
		limit:  limit,
		logger: logger,
	}
}

// BuildContext returns "" when no memories exist OR on repo error
// (fail-open). On success returns "；" joined summaries newest-first.
func (s *Selector) BuildContext(ctx context.Context, childID int64) string {
	rows, err := s.repo.RecentByChildTypes(ctx, childID, s.types, s.limit)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("memory.selector.fail", "child_id", childID, "err", err)
		}
		return ""
	}
	if len(rows) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rows))
	for _, m := range rows {
		if sum := extractSummary(m.Payload); sum != "" {
			parts = append(parts, sum)
		}
	}
	return strings.Join(parts, "；")
}

func extractSummary(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.Summary)
}
