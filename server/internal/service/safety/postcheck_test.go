package safety

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestPostChecker(t *testing.T) *PostChecker {
	t.Helper()
	redlines := map[string][]string{
		"violence": {"血腥", "暴力"},
		"horror":   {"鬼"},
	}
	rs := &RuleSet{
		Redlines:        redlines,
		AllRedlinesFlat: flattenRedlines(redlines),
		WordToCategory:  buildWordToCategory(redlines),
	}
	return NewPostChecker(rs)
}

func TestPostCheck_Pass(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门，决定勇敢地走进竹林。爱宝跟在小宇身后，一起冒险。小宇说：'我们去找小恐龙！'于是小宇带着大家一路前行。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		ChildFearList: nil,
	})
	assert.True(t, out.Pass)
}

func TestPostCheck_RejectRedline(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇遇到了血腥的怪兽，但勇敢地战胜了它。小宇决定继续前行。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
	assert.Equal(t, "血腥", out.MatchedRule)
}

func TestPostCheck_RejectFear(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇在花园里看到一只大蜘蛛，决定友好地打招呼。小宇说：你好。小宇笑了。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		ChildFearList: []string{"蜘蛛"},
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "fear_matched", out.RejectReason)
	assert.Equal(t, "蜘蛛", out.MatchedRule)
}

func TestPostCheck_RejectChildAbsent(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "爱宝独自走进竹林，决定一个人冒险。爱宝跑得很快。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}

func TestPostCheck_DurationAdaptive_5min_2Mentions(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门。小宇决定勇敢地走进竹林。爱宝跟着。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		Duration:      5,
	})
	assert.True(t, out.Pass)
}

func TestPostCheck_DurationAdaptive_5min_1Mention(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门，决定勇敢地走进竹林。爱宝跟着。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		Duration:      5,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}

func TestPostCheck_DurationAdaptive_15min_3Mentions(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门。小宇走进竹林。小宇笑了。爱宝跟着。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		Duration:      15,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}

func TestPostCheck_DurationAdaptive_15min_4Mentions(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门。小宇走进竹林。小宇笑了。小宇又跑了。爱宝跟着。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		Duration:      15,
	})
	assert.True(t, out.Pass)
}

func TestPostCheck_RejectChildPassive(t *testing.T) {
	pc := newTestPostChecker(t)
	story := strings.Repeat("爱宝跑了。爱宝跳了。爱宝笑了。", 10) + "小宇也在场。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}

func TestPostCheck_NotContinuing_AllPreviousElementsAbsent(t *testing.T) {
	pc := newTestPostChecker(t)
	story := strings.Repeat("小宇和爱宝在公园玩耍。", 6)
	out := pc.Check(PostCheckInput{
		StoryText:         story,
		ChildNickname:     "小宇",
		Duration:          10,
		RequireContinuity: true,
		PreviousElements:  []string{"小恐龙阿绿", "竹林"},
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "not_continuing", out.RejectReason)
	assert.Equal(t, "no_previous_element_mentioned", out.MatchedRule)
}

func TestPostCheck_Continuing_HitAtLeastOnePass(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门，决定勇敢地走进竹林。爱宝跟在小宇身后，一起冒险。小宇说：'我们去找小恐龙！'小宇带着大家一路前行到竹林深处。"
	out := pc.Check(PostCheckInput{
		StoryText:         story,
		ChildNickname:     "小宇",
		Duration:          10,
		RequireContinuity: true,
		PreviousElements:  []string{"竹林", "外星人"},
	})
	assert.True(t, out.Pass)
}

func TestPostCheck_RequireContinuityFalse_Skipped(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门，决定勇敢地往前走。爱宝跟在小宇身后，一起冒险。小宇说：'我们出发！'小宇带着大家一路前行。"
	out := pc.Check(PostCheckInput{
		StoryText:         story,
		ChildNickname:     "小宇",
		Duration:          10,
		RequireContinuity: false,
		PreviousElements:  []string{"小恐龙阿绿", "竹林"},
	})
	assert.True(t, out.Pass)
}
