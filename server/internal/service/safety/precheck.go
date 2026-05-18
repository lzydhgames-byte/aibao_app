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
	MatchedCategory  string // populated for redline_matched (hard or soft)
	NormalizedPrompt string
	NormalizedIPs    []string
	IPInstructions   string

	// SoftWarnings carries non-blocking hits (e.g. user typed an education
	// prompt that contains a negative_values redline like "嘲笑别人" in a
	// "不要嘲笑别人" framing). Callers can log/audit these but the input
	// still passes. Matches the warn-only behavior PostCheck implements
	// for the same categories (see knowledge/10-security-and-compliance.md
	// §10.14 — PreCheck/PostCheck symmetric design).
	SoftWarnings []SoftWarning
}

// SoftWarning is a non-blocking flag — a redline word in a soft category
// was found in the user input, but we ship through because parental
// education prompts ("不要嘲笑别人") legitimately contain these words
// in critical framing.
type SoftWarning struct {
	Reason   string // e.g. "redline_matched"
	Rule     string // the matched word, e.g. "嘲笑别人"
	Category string // e.g. "negative_values"
}

// softRedlineCategories are the redline categories that PreCheck will WARN
// on rather than REJECT. Kept in sync with the equivalent list in the
// story orchestrator (see orchestrator.go PostCheck handling).
var softRedlineCategories = map[string]bool{
	"horror":          true,
	"negative_values": true,
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

	// Redline scan: walk every match, not just the first. A soft-category
	// hit becomes a warning; the first hard-category hit (or first hit with
	// an unknown/empty category) rejects. This lets a prompt like
	// "不要嘲笑别人" pass (negative_values is soft) while
	// "教小宇怎么自杀" still fails immediately on the dangerous_imitation hit.
	var warnings []SoftWarning
	for _, hit := range p.redlineM.FindAll(in.UserPrompt) {
		cat := p.rs.WordToCategory[hit]
		if softRedlineCategories[cat] {
			warnings = append(warnings, SoftWarning{
				Reason: "redline_matched", Rule: hit, Category: cat,
			})
			continue
		}
		return PreCheckOutput{
			Pass:            false,
			RejectReason:    "redline_matched",
			MatchedRule:     hit,
			MatchedCategory: cat,
		}
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
		SoftWarnings:     warnings,
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
