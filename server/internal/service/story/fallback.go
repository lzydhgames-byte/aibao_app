// Package story orchestrates story generation: pre-check, prompt build, LLM
// call, post-check, and persistence with outbox event.
package story

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoFallback is returned when no fallback template matches the given key.
var ErrNoFallback = errors.New("no fallback template")

// FallbackKey identifies which template to load.
type FallbackKey struct {
	Style    string // 温馨治愈 / 冒险探索 / 搞笑欢乐 / 神奇魔法 / 科普认知
	Duration int    // 3 / 5 / 8 minutes
}

// Fallback loads pre-written stories from disk and substitutes the child's
// nickname for the {{NICK}} placeholder. Used when LLM generation fails.
type Fallback struct {
	dir string
}

// NewFallback constructs a Fallback that loads templates from dir.
func NewFallback(dir string) *Fallback {
	return &Fallback{dir: dir}
}

// styleFile maps a Chinese style label to a filename prefix.
func styleFile(style string) string {
	switch style {
	case "温馨治愈":
		return "warm"
	case "冒险探索":
		return "adventure"
	case "搞笑欢乐":
		return "funny"
	case "神奇魔法":
		return "magic"
	case "科普认知":
		return "magic" // closest fallback
	default:
		return "warm"
	}
}

// Load returns a fallback story text with {{NICK}} replaced by nickname.
// Tries exact (style, duration) match first; falls back to (style, 5min)
// which is the "middle" slot guaranteed to exist for every style.
func (f *Fallback) Load(key FallbackKey, nickname string) (string, error) {
	prefix := styleFile(key.Style)
	candidates := []string{
		fmt.Sprintf("%s_%dmin.txt", prefix, key.Duration),
		fmt.Sprintf("%s_5min.txt", prefix),
		"warm_5min.txt",
	}
	for _, c := range candidates {
		path := filepath.Join(f.dir, c)
		data, err := os.ReadFile(path)
		if err == nil {
			text := strings.ReplaceAll(string(data), "{{NICK}}", nickname)
			return text, nil
		}
	}
	return "", ErrNoFallback
}
