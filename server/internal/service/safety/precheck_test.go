package safety

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPreChecker(t *testing.T) *PreChecker {
	t.Helper()
	redlines := map[string][]string{
		"violence":        {"血腥", "暴力"},
		"horror":          {"鬼"},
		"negative_values": {"嘲笑别人"},
	}
	rs := &RuleSet{
		Redlines: redlines,
		IPWhitelist: map[string]string{
			"奥特曼": "本故事中爱宝变身为爱宝奥特曼。",
		},
		IPBlacklist:     []string{"进击的巨人"},
		AllRedlinesFlat: flattenRedlines(redlines),
		WordToCategory:  buildWordToCategory(redlines),
	}
	return NewPreChecker(rs, NewNoopIntentProvider())
}

func TestPreCheck_Pass_PlainStory(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个温馨的睡前故事",
		ChildFearList:  nil,
		MaxPromptRunes: 200,
	})
	assert.True(t, out.Pass)
	assert.Empty(t, out.MatchedRule)
	assert.Empty(t, out.NormalizedIPs)
}

func TestPreCheck_RejectTooLong(t *testing.T) {
	pc := newTestPreChecker(t)
	long := strings.Repeat("一", 250)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     long,
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "too_long", out.RejectReason)
}

func TestPreCheck_RejectDangerChars(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "正常文字\x00夹带控制字符",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "danger_chars", out.RejectReason)
}

func TestPreCheck_RejectGlobalRedline(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "我想要血腥的奥特曼故事",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
	assert.Equal(t, "血腥", out.MatchedRule)
}

func TestPreCheck_RejectFearList(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个有蜘蛛的故事",
		ChildFearList:  []string{"蜘蛛", "蛇"},
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "fear_matched", out.RejectReason)
	assert.Equal(t, "蜘蛛", out.MatchedRule)
}

func TestPreCheck_RejectBlacklistIP(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个进击的巨人风格的故事",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "ip_blacklisted", out.RejectReason)
	assert.Equal(t, "进击的巨人", out.MatchedRule)
}

func TestPreCheck_PassWithWhitelistIP(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个奥特曼睡前故事",
		MaxPromptRunes: 200,
	})
	require.True(t, out.Pass)
	assert.Contains(t, out.NormalizedIPs, "奥特曼")
	assert.NotEmpty(t, out.NormalizedPrompt)
}

func TestPreCheck_RedlineBeforeIP(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "奥特曼血腥打怪兽",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
}

func TestPreCheck_MaxRunesDefault(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt: "正常故事",
	})
	assert.True(t, out.Pass)
}

// Plan 9c 第三战：a negative_values redline ("嘲笑别人") in a
// parent-education prompt ("不要嘘笑别人") must pass with a soft warning
// rather than reject. Symmetric with the PostCheck warn-only behavior.
func TestPreCheck_SoftRedline_NegativeValues_Passes(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "不要嘲笑别人",
		MaxPromptRunes: 200,
	})
	require.True(t, out.Pass, "negative_values prompt should soft-pass")
	require.Len(t, out.SoftWarnings, 1)
	assert.Equal(t, "嘲笑别人", out.SoftWarnings[0].Rule)
	assert.Equal(t, "negative_values", out.SoftWarnings[0].Category)
}

// Hard-category redline (violence) must still reject, even when soft
// categories are also present in the same prompt.
func TestPreCheck_HardRedline_StillRejects(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "不要嘲笑别人，但讲血腥的故事",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
	assert.Equal(t, "violence", out.MatchedCategory)
	assert.Equal(t, "血腥", out.MatchedRule)
}
