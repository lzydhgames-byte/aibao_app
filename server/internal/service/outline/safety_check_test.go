package outline_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/safety"
)

// newTestMatcher creates a safety.Matcher with the given redline words.
// Uses project's actual API: NewKeywordMatcher returns *KeywordMatcher,
// which implements Matcher interface.
func newTestMatcher(t *testing.T, words ...string) safety.Matcher {
	t.Helper()
	return safety.NewKeywordMatcher(words)
}

// validSynopsis returns a 70-rune protagonist-bearing synopsis for happy cases.
// nickname is N runes; padding is (70-N) runes of "一".
func validSynopsis(nickname string) string {
	pad := strings.Repeat("一", 70-len([]rune(nickname)))
	return nickname + pad
}

func TestSafetyCheck_OK(t *testing.T) {
	m := newTestMatcher(t, "血", "杀", "鬼")
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "小宇的星空冒险",
			Synopsis:         validSynopsis("小宇"),
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "学到勇敢",
		},
		ChildNickname: "小宇",
	})
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
}

func TestSafetyCheck_ProtagonistMissing(t *testing.T) {
	m := newTestMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "奥特曼大冒险",
			Synopsis:         strings.Repeat("一", 70), // no nickname inside
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
	})
	if !errors.Is(res.Reason, outline.ErrSafetyProtagonistMissing) {
		t.Fatalf("want ProtagonistMissing, got %v", res.Reason)
	}
}

func TestSafetyCheck_Redline(t *testing.T) {
	m := newTestMatcher(t, "杀", "血")
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "小宇的冒险",
			Synopsis:         "小宇遇到恶龙要杀" + strings.Repeat("一", 60),
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
	})
	if !errors.Is(res.Reason, outline.ErrSafetyRedline) {
		t.Fatalf("want Redline, got %v", res.Reason)
	}
}

func TestSafetyCheck_ChildFears(t *testing.T) {
	m := newTestMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "小宇的冒险",
			Synopsis:         "小宇遇到一只大狗" + strings.Repeat("一", 60),
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
		ChildFears:    []string{"大狗"},
	})
	if !errors.Is(res.Reason, outline.ErrSafetyChildFears) {
		t.Fatalf("want ChildFears, got %v", res.Reason)
	}
}

func TestSafetyCheck_IPWhitelistInTitle(t *testing.T) {
	m := newTestMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "小宇和奥特曼的冒险", // 奥特曼 in title = abuse of protagonist seat
			Synopsis:         "小宇和爱宝一起" + strings.Repeat("一", 60),
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
		IPWhitelist:   []string{"奥特曼"},
	})
	if !errors.Is(res.Reason, outline.ErrSafetyIPMisuse) {
		t.Fatalf("want IPMisuse, got %v", res.Reason)
	}
}

// IP whitelist appearing in synopsis (as companion) is OK.
func TestSafetyCheck_IPWhitelistInSynopsisOK(t *testing.T) {
	m := newTestMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:            "小宇的太空旅行",
			Synopsis:         "小宇和奥特曼" + strings.Repeat("一", 60), // IP in synopsis as companion
			Themes:           []string{"勇气"},
			Style:            "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
		IPWhitelist:   []string{"奥特曼"},
	})
	if !res.OK {
		t.Fatalf("expected OK (IP as companion in synopsis), got %+v", res)
	}
}
