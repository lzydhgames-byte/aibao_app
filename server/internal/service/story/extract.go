package story

import "strings"

// ExtractedElement is a story element ready to be persisted.
type ExtractedElement struct {
	ElementType  string // character / place / object / event
	Name         string
	Description  string
	RecallWeight float64
}

// commonPlaces are stock fantasy-friendly locations we recognize.
var commonPlaces = []string{
	"竹林", "森林", "城堡", "花园", "海底", "山洞", "河边", "村庄",
	"太空", "月亮", "星星", "彩虹", "云朵", "宇宙", "海岛",
}

// commonObjects are stock items.
var commonObjects = []string{
	"魔法棒", "宝石", "钥匙", "宝箱", "灯笼", "翅膀",
}

// ExtractElements runs heuristic extraction over a story text.
//   - For each whitelist IP keyword that appears, registers a "character"
//     element named "爱宝<IP>" (matches the same-character convention).
//   - Scans for known places and objects.
//
// Returns deduped elements.
func ExtractElements(story string, normalizedIPs []string) []*ExtractedElement {
	if story == "" {
		return nil
	}
	out := []*ExtractedElement{}
	seen := map[string]struct{}{}

	add := func(kind, name string, weight float64) {
		key := kind + ":" + name
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, &ExtractedElement{
			ElementType:  kind,
			Name:         name,
			RecallWeight: weight,
		})
	}

	for _, ip := range normalizedIPs {
		// We assume the prompt instructs the LLM to render the same-character
		// form as "爱宝<IP>". Register that as a character element.
		add("character", "爱宝"+ip, 1.5)
	}
	for _, p := range commonPlaces {
		if strings.Contains(story, p) {
			add("place", p, 1.0)
		}
	}
	for _, o := range commonObjects {
		if strings.Contains(story, o) {
			add("object", o, 0.8)
		}
	}
	return out
}
