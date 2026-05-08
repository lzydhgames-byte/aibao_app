package safety

import "context"

// Intent is the coarse intent classification of a user-supplied story prompt.
type Intent int

const (
	// IntentSafe — the prompt looks like a normal story request. Default verdict.
	IntentSafe Intent = iota
	// IntentUncertain — borderline, may need stricter PostCheck.
	IntentUncertain
	// IntentUnsafe — the prompt expresses an intent to violate content rules
	// (e.g. "I want a violent story"). Should be rejected without calling LLM.
	IntentUnsafe
)

// String returns a stable lower-case label suitable for logs and metrics.
func (i Intent) String() string {
	switch i {
	case IntentSafe:
		return "safe"
	case IntentUncertain:
		return "uncertain"
	case IntentUnsafe:
		return "unsafe"
	default:
		return "unknown"
	}
}

// IntentProvider classifies user prompt intent.
type IntentProvider interface {
	Classify(ctx context.Context, userPrompt string) (Intent, error)
}

// NoopIntentProvider always reports IntentSafe.
type NoopIntentProvider struct{}

// NewNoopIntentProvider constructs a NoopIntentProvider.
func NewNoopIntentProvider() *NoopIntentProvider { return &NoopIntentProvider{} }

// Classify always returns IntentSafe.
func (NoopIntentProvider) Classify(_ context.Context, _ string) (Intent, error) {
	return IntentSafe, nil
}
