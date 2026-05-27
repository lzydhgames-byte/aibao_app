package outline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidJSON    = errors.New("outline parser: invalid JSON")
	ErrInvalidStyle   = errors.New("outline parser: invalid style enum")
	ErrSynopsisLength = errors.New("outline parser: synopsis length out of range")
	ErrThemesCount    = errors.New("outline parser: themes count out of range")
	ErrTitleLength    = errors.New("outline parser: title length out of range")
)

// validStyles is the 5-enum whitelist (spec §5.1).
var validStyles = map[string]bool{
	"温馨治愈": true, "冒险探索": true, "搞笑欢乐": true, "神奇魔法": true, "科普认知": true,
}

// RawOutline is the JSON shape returned by the outline LLM call.
// Sprint B Task 17 Service.Preview wraps this into outlinecontract.Outline
// after injecting scene_seed + outline_id + group/variant metadata.
type RawOutline struct {
	Title            string   `json:"title"`
	Synopsis         string   `json:"synopsis"`
	Themes           []string `json:"themes"`
	Style            string   `json:"style"`
	EducationalValue string   `json:"educational_value"`
}

// Parse extracts the structured outline from the LLM response text.
// Strict schema: unknown fields rejected; ranges validated.
// Spec §5.1: caller may attempt 1 schema repair retry on failure.
func Parse(raw string) (*RawOutline, error) {
	raw = strings.TrimSpace(raw)
	// strip markdown code fence if present (```json or just ```)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var ro RawOutline
	if err := dec.Decode(&ro); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if n := utf8.RuneCountInString(ro.Title); n < 5 || n > 16 {
		return nil, fmt.Errorf("%w: got %d runes", ErrTitleLength, n)
	}
	if n := utf8.RuneCountInString(ro.Synopsis); n < 60 || n > 160 {
		return nil, fmt.Errorf("%w: got %d runes", ErrSynopsisLength, n)
	}
	if len(ro.Themes) < 1 || len(ro.Themes) > 3 {
		return nil, fmt.Errorf("%w: got %d themes", ErrThemesCount, len(ro.Themes))
	}
	if !validStyles[ro.Style] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStyle, ro.Style)
	}
	return &ro, nil
}
