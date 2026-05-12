// Package audio implements the post-LLM audio orchestration:
//
//	parse cue markers → call TTS on clean text → pick BGM by mood →
//	ffmpeg-mix TTS+BGM → return final mp3 bytes.
//
// cue_parser.go is pure string processing — no I/O, no external deps.
package audio

import (
	"regexp"
	"strings"

	"github.com/aibao/server/internal/model"
)

// CueType is "sfx" (sound effect) or "bgm" (background music mood).
type CueType string

const (
	CueTypeSFX CueType = "sfx"
	CueTypeBGM CueType = "bgm"
)

// Cue records one marker extracted from the story text.
type Cue struct {
	Type       CueType
	Label      string // e.g. "门铃" or "温馨"
	CharOffset int    // byte offset INTO CleanText where this cue WOULD have been
}

// ParseResult is the full output of Parse.
type ParseResult struct {
	CleanText string // text with all [音效:...] and [BGM情绪:...] markers stripped
	Cues      []Cue
	BGMMood   string // resolved mood key (e.g. "warm"); always non-empty after Parse
}

// cueRe matches [音效:xxx] or [BGM情绪:xxx]. Extracted in textual order.
var cueRe = regexp.MustCompile(`\[(音效|BGM情绪):([^\]]+)\]`)

// Parse extracts cues from text and returns clean text plus offsets relative
// to the clean text. If no recognizable [BGM情绪:...] cue is present, BGMMood
// falls back to MoodFromStyle(fallbackStyle); if that also yields "", returns
// MoodWarm.
func Parse(text, fallbackStyle string) ParseResult {
	var (
		clean strings.Builder
		cues  []Cue
		last  int
	)
	bgmMood := ""

	idxs := cueRe.FindAllStringSubmatchIndex(text, -1)
	for _, m := range idxs {
		start, end := m[0], m[1]
		kindStart, kindEnd := m[2], m[3]
		labelStart, labelEnd := m[4], m[5]

		clean.WriteString(text[last:start])
		offset := clean.Len()

		kind := text[kindStart:kindEnd]
		label := strings.TrimSpace(text[labelStart:labelEnd])

		var ct CueType
		if kind == "音效" {
			ct = CueTypeSFX
		} else {
			ct = CueTypeBGM
			if bgmMood == "" {
				if mm := model.MoodFromCueZh(label); mm != "" {
					bgmMood = mm
				}
			}
		}
		cues = append(cues, Cue{Type: ct, Label: label, CharOffset: offset})
		last = end
	}
	clean.WriteString(text[last:])

	if bgmMood == "" {
		bgmMood = model.MoodFromStyle(fallbackStyle)
	}
	if bgmMood == "" {
		bgmMood = model.MoodWarm
	}
	return ParseResult{
		CleanText: clean.String(),
		Cues:      cues,
		BGMMood:   bgmMood,
	}
}
