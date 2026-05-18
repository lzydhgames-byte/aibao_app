package prompt

// genderText converts the API gender code into a Chinese phrase suitable for
// the system prompt.
func genderText(g string) string {
	switch g {
	case "boy":
		return "男孩"
	case "girl":
		return "女孩"
	default:
		return "孩子"
	}
}

// fearListText joins the per-child fear list into a comma-separated phrase,
// or returns "（无）" if the list is empty.
func fearListText(list []string) string {
	if len(list) == 0 {
		return "（无）"
	}
	out := ""
	for i, w := range list {
		if i > 0 {
			out += "、"
		}
		out += w
	}
	return out
}

// topicText returns the topic phrase or a fallback when topic is empty.
func topicText(t string) string {
	if t == "" {
		return "无（纯娱乐）"
	}
	return t
}

// expectedRunesForDuration approximates the target story length in CJK
// characters. Calibrated against real Minimax t2a_v2 (audiobook_female_1):
//
//   - Round 1 (Plan 9c first battle): 120 chars/min — way too low, output
//     was 1/4 of expected duration.
//   - Round 2 (Plan 9c second battle): 320 chars/min — fixed the 1-min
//     fallback problem, but 15-story sample averaged +14% over duration,
//     worst-case +33% (5min slot ran 6:38).
//   - Round 3 (Plan 9c third battle, today): 280 chars/min — empirical
//     median from the round-2 sample. We accept slightly higher fallback
//     risk in exchange for ~10% lower TTS bill per story.
//
// Returns the CENTER of the target band; the prompt template renders a
// ±5% window around this value as a hard constraint (was ±10% in round 2).
func expectedRunesForDuration(durationMin int) int {
	return durationMin * 280
}

// expectedRuneBand returns the inclusive [min, max] rune range the LLM
// must hit, computed as ±5% around expectedRunesForDuration. Tighter
// window (was ±10%) discourages the LLM from interpreting the upper bound
// as an aspirational target.
func expectedRuneBand(durationMin int) (int, int) {
	c := expectedRunesForDuration(durationMin)
	return c * 95 / 100, c * 105 / 100
}

// ExpectedRuneBand exposes expectedRuneBand for callers outside this
// package (e.g. the story orchestrator's length-retry guard).
func ExpectedRuneBand(durationMin int) (int, int) { return expectedRuneBand(durationMin) }

// CountCJKRunes counts the number of CJK ideographs in s, ignoring
// punctuation, whitespace and [音效:xxx] / [BGM情绪:xxx] cue markers.
// Used by the orchestrator's length guard to decide whether to ask the
// LLM to rewrite a too-short story.
func CountCJKRunes(s string) int {
	n := 0
	inBracket := false
	for _, r := range s {
		if r == '[' {
			inBracket = true
			continue
		}
		if r == ']' {
			inBracket = false
			continue
		}
		if inBracket {
			continue
		}
		// Common CJK Unified Ideographs block.
		if r >= 0x4E00 && r <= 0x9FFF {
			n++
		}
	}
	return n
}
