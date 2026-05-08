// Package safety implements the two-layer (PreCheck + PostCheck) story safety
// pipeline. Rules are sourced from YAML files at startup so operations can
// edit them without code changes.
package safety

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuleSet is the immutable runtime view of all safety rules.
type RuleSet struct {
	// Redlines maps category name → list of forbidden keywords.
	Redlines map[string][]string

	// IPWhitelist maps a real-IP keyword → the same-character-instruction to
	// inject into the prompt when the keyword appears in user input.
	IPWhitelist map[string]string

	// IPBlacklist is the list of real-IP keywords that cause an outright reject.
	IPBlacklist []string

	// AllRedlinesFlat is the deduped union of all Redlines values, used by
	// the matcher for O(N) substring scans without map lookups.
	AllRedlinesFlat []string
}

// LoadRules reads three YAML files and returns an immutable RuleSet.
// Returns an error if any file is missing or malformed.
func LoadRules(rulesPath, whitelistPath, blacklistPath string) (*RuleSet, error) {
	redlines := map[string][]string{}
	if err := readYAML(rulesPath, &redlines); err != nil {
		return nil, fmt.Errorf("load redlines %s: %w", rulesPath, err)
	}

	wl := map[string]string{}
	if err := readYAML(whitelistPath, &wl); err != nil {
		return nil, fmt.Errorf("load whitelist %s: %w", whitelistPath, err)
	}

	var bl []string
	if err := readYAML(blacklistPath, &bl); err != nil {
		return nil, fmt.Errorf("load blacklist %s: %w", blacklistPath, err)
	}

	flat := flattenRedlines(redlines)
	return &RuleSet{
		Redlines:        redlines,
		IPWhitelist:     wl,
		IPBlacklist:     bl,
		AllRedlinesFlat: flat,
	}, nil
}

func readYAML(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

// flattenRedlines dedupes and returns a flat list of all redline words.
func flattenRedlines(rl map[string][]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, words := range rl {
		for _, w := range words {
			if _, ok := seen[w]; ok {
				continue
			}
			seen[w] = struct{}{}
			out = append(out, w)
		}
	}
	return out
}
