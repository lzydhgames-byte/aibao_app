package safety

import "strings"

// Matcher decides whether any keyword appears as a substring of input.
// FindFirst returns the first keyword (in the matcher's stored order) that
// is found in input, or ok=false if none match.
type Matcher interface {
	FindFirst(input string) (keyword string, ok bool)
}

// KeywordMatcher is a simple substring matcher. It lowercases both keyword
// and input for case-insensitive matching of ASCII characters; CJK characters
// are unaffected by ToLower so this is safe for our mixed Chinese+English
// keyword corpus.
type KeywordMatcher struct {
	keywords []string
}

// NewKeywordMatcher constructs a KeywordMatcher.
func NewKeywordMatcher(keywords []string) *KeywordMatcher {
	lc := make([]string, 0, len(keywords))
	for _, k := range keywords {
		lc = append(lc, strings.ToLower(k))
	}
	return &KeywordMatcher{keywords: lc}
}

// FindFirst returns the first keyword that appears in input.
func (m *KeywordMatcher) FindFirst(input string) (string, bool) {
	if input == "" || len(m.keywords) == 0 {
		return "", false
	}
	lowered := strings.ToLower(input)
	for _, k := range m.keywords {
		if k == "" {
			continue
		}
		if strings.Contains(lowered, k) {
			return k, true
		}
	}
	return "", false
}
