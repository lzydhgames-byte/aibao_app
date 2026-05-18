// rules-lint validates safety/rules.yaml structure:
//   1. file parses as map[string][]string
//   2. all 6 expected categories present
//   3. no duplicate words within a category
//   4. no duplicate words across categories (would collapse on the flat scan)
//   5. no empty words
//   6. no leading/trailing whitespace in words (a copy-paste hazard)
//
// Exits 0 on clean, 1 on any issue (suitable for CI hook).
//
// Usage:
//
//	go run ./cmd/rules-lint                      # uses safety/rules.yaml
//	go run ./cmd/rules-lint -file=other.yaml     # custom path
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// expectedCategories must match safety/rules.yaml top-level keys.
// Adding a new category? Add it here and to PostCheck/PreCheck category
// classification (see internal/service/safety/precheck.go softRedlineCategories
// and internal/service/story/orchestrator.go PostCheck soft handling).
var expectedCategories = []string{
	"violence",
	"horror",
	"sexual",
	"political_religious",
	"dangerous_imitation",
	"negative_values",
}

func main() {
	file := flag.String("file", "safety/rules.yaml", "path to rules.yaml")
	flag.Parse()

	data, err := os.ReadFile(*file)
	if err != nil {
		fail("read %s: %v", *file, err)
	}

	var m map[string][]string
	if err := yaml.Unmarshal(data, &m); err != nil {
		fail("parse YAML: %v", err)
	}

	var issues []string

	// (2) presence of expected categories
	for _, cat := range expectedCategories {
		if _, ok := m[cat]; !ok {
			issues = append(issues, fmt.Sprintf("missing category: %q", cat))
		}
	}
	// also flag unexpected categories so typos like "violenc" surface
	for cat := range m {
		known := false
		for _, e := range expectedCategories {
			if e == cat {
				known = true
				break
			}
		}
		if !known {
			issues = append(issues, fmt.Sprintf("unexpected category: %q (typo?)", cat))
		}
	}

	// (3) intra-category dup, (5) empty, (6) whitespace
	for cat, words := range m {
		seen := map[string]int{}
		for i, w := range words {
			if w == "" {
				issues = append(issues, fmt.Sprintf("%s[%d]: empty word", cat, i))
				continue
			}
			if trim := strings.TrimSpace(w); trim != w {
				issues = append(issues, fmt.Sprintf("%s[%d]: leading/trailing whitespace in %q", cat, i, w))
			}
			seen[w]++
		}
		for w, n := range seen {
			if n > 1 {
				issues = append(issues, fmt.Sprintf("%s: duplicate %q × %d", cat, w, n))
			}
		}
	}

	// (4) cross-category dup
	word2cats := map[string][]string{}
	for cat, words := range m {
		for _, w := range words {
			word2cats[w] = append(word2cats[w], cat)
		}
	}
	for w, cats := range word2cats {
		if len(cats) > 1 {
			sort.Strings(cats)
			issues = append(issues, fmt.Sprintf("cross-category duplicate: %q in %v", w, cats))
		}
	}

	// Stats (always print)
	cats := make([]string, 0, len(m))
	for c := range m {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	total := 0
	fmt.Printf("rules-lint: %s\n", *file)
	for _, c := range cats {
		fmt.Printf("  %-22s %3d words\n", c+":", len(m[c]))
		total += len(m[c])
	}
	fmt.Printf("  %-22s %3d words\n", "TOTAL:", total)

	if len(issues) == 0 {
		fmt.Println("OK — no issues found.")
		return
	}

	sort.Strings(issues)
	fmt.Printf("\n%d issue(s):\n", len(issues))
	for _, s := range issues {
		fmt.Printf("  - %s\n", s)
	}
	os.Exit(1)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "rules-lint: "+format+"\n", args...)
	os.Exit(1)
}
