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
// characters. Calibrated against real Minimax t2a_v2 (audiobook_female_1)
// output: ~300 chars/min observed; we target 320 chars/min so the LLM
// slightly over-writes rather than under-fills the audio window.
//
// Returns the CENTER of the target band; the prompt template renders a
// ±10% window around this value as a hard constraint.
func expectedRunesForDuration(durationMin int) int {
	return durationMin * 320
}

// expectedRuneBand returns the inclusive [min, max] rune range the LLM
// must hit, computed as ±10% around expectedRunesForDuration.
func expectedRuneBand(durationMin int) (int, int) {
	c := expectedRunesForDuration(durationMin)
	return c * 9 / 10, c * 11 / 10
}
