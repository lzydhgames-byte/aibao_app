package outlinecontract

import "context"

// Outline is the resolved outline DTO consumed by story orchestrator.
// All fields are post-validation, post-safety, ready for prompt injection.
// Spec §5.1: matches LLM JSON schema after schema repair + safety check;
// service/outline backfills SceneSeed + OutlineGroupID + VariantIndex +
// ParentOutlineID + OutlinePromptVersion before persisting to Redis.
type Outline struct {
	OutlineID            string
	Title                string
	Synopsis             string
	EducationalValue     string
	Themes               []string
	Style                string
	DurationMin          int
	SceneSeed            string
	OutlineGroupID       string
	VariantIndex         int
	ParentOutlineID      string
	OutlinePromptVersion string
}

// OutlineResolver resolves an outline_id to its full Outline payload,
// enforcing user_id + child_id + outline_id triple ownership.
// Spec §5.2 / §7.5.
//
// Returns:
//   - (*Outline, nil)               — success, outline is still pending and owned by (userID, childID)
//   - (nil, ErrOutlineExpired)      — cache miss / TTL elapsed / already in terminal state
//   - (nil, ErrOutlineForbidden)    — outline exists but belongs to different user/child
//   - (nil, ErrOutlineNotFound)     — refresh path only; outline_id was never cached
//   - (nil, other error)            — infrastructure failure (DB/Redis); caller maps to 500
type OutlineResolver interface {
	Resolve(ctx context.Context, outlineID string, userID, childID int64) (*Outline, error)
}
