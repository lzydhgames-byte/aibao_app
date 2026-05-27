package outline_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/service/outline"
)

func TestParse_Happy(t *testing.T) {
	raw := `{
  "title": "小宇的星空冒险",
  "synopsis": "小宇遇到爱宝，他们一起穿越到星空之上。途中遇到流星雨，小宇展现出勇气，主动想办法保护小动物们，最终大家手拉手平安回家了。",
  "themes": ["勇气", "团队合作"],
  "style": "冒险探索",
  "educational_value": "学到遇到困难不害怕、和小伙伴一起想办法"
}`
	ro, err := outline.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ro.Style != "冒险探索" {
		t.Errorf("style: %s", ro.Style)
	}
	if len(ro.Themes) != 2 {
		t.Errorf("themes count: %d", len(ro.Themes))
	}
}

func TestParse_InvalidStyle(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"恐怖","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrInvalidStyle) {
		t.Fatalf("want ErrInvalidStyle, got %v", err)
	}
}

func TestParse_UnknownField(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x","extra":"bad"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrInvalidJSON) {
		t.Fatalf("want ErrInvalidJSON (unknown field), got %v", err)
	}
}

func TestParse_SynopsisTooShort(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"太短了","themes":["勇气"],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrSynopsisLength) {
		t.Fatalf("want ErrSynopsisLength, got %v", err)
	}
}

func TestParse_SynopsisTooLong(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 200) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrSynopsisLength) {
		t.Fatalf("want ErrSynopsisLength for too long, got %v", err)
	}
}

func TestParse_TitleTooShort(t *testing.T) {
	raw := `{"title":"太短","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrTitleLength) {
		t.Fatalf("want ErrTitleLength, got %v", err)
	}
}

func TestParse_ThemesEmpty(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":[],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrThemesCount) {
		t.Fatalf("want ErrThemesCount, got %v", err)
	}
}

func TestParse_ThemesTooMany(t *testing.T) {
	raw := `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":["a","b","c","d"],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrThemesCount) {
		t.Fatalf("want ErrThemesCount for too many, got %v", err)
	}
}

func TestParse_MarkdownFenced(t *testing.T) {
	raw := "```json\n" + `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x"}` + "\n```"
	_, err := outline.Parse(raw)
	if err != nil {
		t.Fatalf("markdown-fenced should parse: %v", err)
	}
}

func TestParse_PlainFenced(t *testing.T) {
	// no "json" tag, just ```
	raw := "```\n" + `{"title":"测试标题甲","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x"}` + "\n```"
	_, err := outline.Parse(raw)
	if err != nil {
		t.Fatalf("plain-fenced should parse: %v", err)
	}
}
