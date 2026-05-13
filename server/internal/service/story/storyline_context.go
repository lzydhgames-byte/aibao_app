package story

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

// StorylineContext is the assembled "previous-episode" view passed to the
// prompt builder and (parts of) PostCheck for sequel generation.
type StorylineContext struct {
	StorylineID      int64
	Title            string
	EpisodeNumber    int      // sl.EpisodeCount + 1 (the upcoming episode)
	RecentSummaries  []string // newest first; up to 3
	PreviousHook     string   // sl.NextEpisodeHint (may be "")
	PreviousElements []string // top 5-8 character/place names from latest episode
}

// storylineRepoForCtx is the minimum StorylineRepo surface needed.
type storylineRepoForCtx interface {
	FindByID(ctx context.Context, id int64) (*model.Storyline, error)
}

// storyRepoForCtx is the minimum StoryRepo surface needed.
type storyRepoForCtx interface {
	RecentByStoryline(ctx context.Context, storylineID int64, limit int) ([]*model.Story, error)
	ElementsByStory(ctx context.Context, storyID int64, types []string, limit int) ([]*model.StoryElement, error)
}

// memoryRepoForCtx is the minimum MemoryRepo surface needed.
type memoryRepoForCtx interface {
	RecentByChildTypes(ctx context.Context, childID int64, types []string, limit int) ([]*model.Memory, error)
}

// StorylineContextBuilder assembles a previous-episode context for sequel generation.
type StorylineContextBuilder struct {
	storylineRepo storylineRepoForCtx
	storyRepo     storyRepoForCtx
	memoryRepo    memoryRepoForCtx
	logger        *slog.Logger
}

// NewStorylineContextBuilder constructs a builder.
func NewStorylineContextBuilder(
	storylineRepo storylineRepoForCtx,
	storyRepo storyRepoForCtx,
	memoryRepo memoryRepoForCtx,
	logger *slog.Logger,
) *StorylineContextBuilder {
	return &StorylineContextBuilder{
		storylineRepo: storylineRepo,
		storyRepo:     storyRepo,
		memoryRepo:    memoryRepo,
		logger:        logger,
	}
}

// Build assembles the context. Returns error only if the storyline does not
// exist (used by callers to map to 404). All other sub-queries are fail-open.
func (b *StorylineContextBuilder) Build(ctx context.Context, storylineID int64) (*StorylineContext, error) {
	sl, err := b.storylineRepo.FindByID(ctx, storylineID)
	if err != nil {
		return nil, err
	}

	out := &StorylineContext{
		StorylineID:   sl.ID,
		Title:         sl.Title,
		EpisodeNumber: sl.EpisodeCount + 1,
		PreviousHook:  sl.NextEpisodeHint,
	}

	recent, err := b.storyRepo.RecentByStoryline(ctx, storylineID, 3)
	if err != nil {
		if b.logger != nil {
			b.logger.Warn("storyline_context.recent_stories.fail", "storyline_id", storylineID, "err", err)
		}
		recent = nil
	}

	if len(recent) > 0 {
		// Pull a generous batch of recent summaries for this child, then filter
		// per-story-id. fail-open.
		mems, mErr := b.memoryRepo.RecentByChildTypes(ctx, sl.ChildID, []string{model.MemoryTypeStorySummary}, 30)
		if mErr != nil {
			if b.logger != nil {
				b.logger.Warn("storyline_context.memories.fail", "child_id", sl.ChildID, "err", mErr)
			}
			mems = nil
		}
		for _, s := range recent {
			if sum := findSummaryForStory(mems, s.ID); sum != "" {
				out.RecentSummaries = append(out.RecentSummaries, sum)
			}
		}

		// PreviousElements from the latest episode.
		elems, eErr := b.storyRepo.ElementsByStory(ctx, recent[0].ID, []string{"character", "place"}, 8)
		if eErr != nil {
			if b.logger != nil {
				b.logger.Warn("storyline_context.elements.fail", "story_id", recent[0].ID, "err", eErr)
			}
		}
		for _, e := range elems {
			if e.Name != "" {
				out.PreviousElements = append(out.PreviousElements, e.Name)
			}
		}
	}

	return out, nil
}

// findSummaryForStory returns the first matching memory's payload.summary
// where the memory's story_id matches. Memories without StoryID fall back
// to a payload.story_id JSON lookup for backward compatibility.
func findSummaryForStory(mems []*model.Memory, storyID int64) string {
	for _, m := range mems {
		if m == nil {
			continue
		}
		match := false
		if m.StoryID != nil && *m.StoryID == storyID {
			match = true
		} else if len(m.Payload) > 0 {
			var p struct {
				StoryID int64  `json:"story_id"`
				Summary string `json:"summary"`
			}
			if json.Unmarshal(m.Payload, &p) == nil && p.StoryID == storyID {
				match = true
			}
		}
		if !match {
			continue
		}
		var p struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			continue
		}
		if s := strings.TrimSpace(p.Summary); s != "" {
			return s
		}
	}
	return ""
}

// Compile-time assertion: ensure repository concrete types satisfy our consumer
// interfaces (helps catch drift when repo signatures change).
var (
	_ storyRepoForCtx     = (repository.StoryRepo)(nil)
	_ storylineRepoForCtx = (repository.StorylineRepo)(nil)
	_ memoryRepoForCtx    = (repository.MemoryRepo)(nil)
)
