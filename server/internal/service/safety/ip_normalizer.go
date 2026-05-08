package safety

import "strings"

// IPVerdict is the IP normalizer's outcome.
type IPVerdict int

const (
	// IPNoMatch — no real IP keywords detected; pass through unchanged.
	IPNoMatch IPVerdict = iota
	// IPWhitelisted — one or more real IPs matched the whitelist; same-character
	// instructions returned for prompt injection.
	IPWhitelisted
	// IPBlacklisted — a blacklisted IP was found; the request must be rejected.
	IPBlacklisted
)

// IPNormalizeResult is the outcome of running IPNormalizer.Normalize.
type IPNormalizeResult struct {
	Verdict      IPVerdict
	MatchedIP    string   // populated when Verdict == IPBlacklisted
	MatchedIPs   []string // populated when Verdict == IPWhitelisted
	Instructions string   // joined whitelist instructions (when whitelisted)
}

// IPNormalizer scans user input for real-IP keywords and reports a verdict.
// Blacklist matches always take priority over whitelist.
type IPNormalizer struct {
	whitelist     map[string]string
	whitelistKeys []string
	blacklist     []string
}

// NewIPNormalizer constructs an IPNormalizer.
func NewIPNormalizer(whitelist map[string]string, blacklist []string) *IPNormalizer {
	keys := make([]string, 0, len(whitelist))
	for k := range whitelist {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return &IPNormalizer{
		whitelist:     whitelist,
		whitelistKeys: keys,
		blacklist:     blacklist,
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// Normalize scans input. Blacklist hits return IPBlacklisted; whitelist hits
// return IPWhitelisted with all matches and joined instructions.
func (n *IPNormalizer) Normalize(input string) IPNormalizeResult {
	if input == "" {
		return IPNormalizeResult{Verdict: IPNoMatch}
	}
	lowered := strings.ToLower(input)

	for _, b := range n.blacklist {
		if b == "" {
			continue
		}
		if strings.Contains(lowered, strings.ToLower(b)) {
			return IPNormalizeResult{Verdict: IPBlacklisted, MatchedIP: b}
		}
	}

	var hits []string
	var insns []string
	for _, k := range n.whitelistKeys {
		if strings.Contains(lowered, strings.ToLower(k)) {
			hits = append(hits, k)
			insns = append(insns, n.whitelist[k])
		}
	}
	if len(hits) == 0 {
		return IPNormalizeResult{Verdict: IPNoMatch}
	}
	return IPNormalizeResult{
		Verdict:      IPWhitelisted,
		MatchedIPs:   hits,
		Instructions: strings.Join(insns, "\n"),
	}
}
