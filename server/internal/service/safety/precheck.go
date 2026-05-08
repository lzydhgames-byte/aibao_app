package safety

import (
	"context"
	"unicode/utf8"
)

// DefaultMaxPromptRunes is the fallback max length when input doesn't supply one.
const DefaultMaxPromptRunes = 200

// PreCheckInput captures everything PreCheck needs.
type PreCheckInput struct {
	UserPrompt     string
	ChildFearList  []string
	MaxPromptRunes int // 0 → DefaultMaxPromptRunes
}

// PreCheckOutput is the verdict and (when pass) the normalized prompt + IP hits.
type PreCheckOutput struct {
	Pass             bool
	RejectReason     string
	MatchedRule      string
	NormalizedPrompt string
	NormalizedIPs    []string
	IPInstructions   string
}

// PreChecker is the front-line gate.
type PreChecker struct {
	rs       *RuleSet
	redlineM *KeywordMatcher
	ipNorm   *IPNormalizer
	intent   IntentProvider
}

// NewPreChecker constructs a PreChecker bound to a RuleSet.
func NewPreChecker(rs *RuleSet, intent IntentProvider) *PreChecker {
	return &PreChecker{
		rs:       rs,
		redlineM: NewKeywordMatcher(rs.AllRedlinesFlat),
		ipNorm:   NewIPNormalizer(rs.IPWhitelist, rs.IPBlacklist),
		intent:   intent,
	}
}

// Check runs the full pre-check pipeline.
func (p *PreChecker) Check(ctx context.Context, in PreCheckInput) PreCheckOutput {
	maxRunes := in.MaxPromptRunes
	if maxRunes <= 0 {
		maxRunes = DefaultMaxPromptRunes
	}

	if utf8.RuneCountInString(in.UserPrompt) > maxRunes {
		return PreCheckOutput{Pass: false, RejectReason: "too_long"}
	}
	if hasDangerChars(in.UserPrompt) {
		return PreCheckOutput{Pass: false, RejectReason: "danger_chars"}
	}
	if hit, ok := p.redlineM.FindFirst(in.UserPrompt); ok {
		return PreCheckOutput{Pass: false, RejectReason: "redline_matched", MatchedRule: hit}
	}
	if len(in.ChildFearList) > 0 {
		fearM := NewKeywordMatcher(in.ChildFearList)
		if hit, ok := fearM.FindFirst(in.UserPrompt); ok {
			return PreCheckOutput{Pass: false, RejectReason: "fear_matched", MatchedRule: hit}
		}
	}
	ipRes := p.ipNorm.Normalize(in.UserPrompt)
	if ipRes.Verdict == IPBlacklisted {
		return PreCheckOutput{Pass: false, RejectReason: "ip_blacklisted", MatchedRule: ipRes.MatchedIP}
	}
	if intent, err := p.intent.Classify(ctx, in.UserPrompt); err == nil && intent == IntentUnsafe {
		return PreCheckOutput{Pass: false, RejectReason: "intent_unsafe"}
	}
	return PreCheckOutput{
		Pass:             true,
		NormalizedPrompt: in.UserPrompt,
		NormalizedIPs:    ipRes.MatchedIPs,
		IPInstructions:   ipRes.Instructions,
	}
}

// hasDangerChars returns true if s contains a control character other than
// \n (0x0A), \r (0x0D), or \t (0x09).
func hasDangerChars(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
		if r == 0x7F {
			return true
		}
	}
	return false
}
