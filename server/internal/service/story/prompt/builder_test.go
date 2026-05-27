package prompt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const templatePath = "../../../../safety/system_prompt.tmpl"

func TestBuilder_BasicHappyPath(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)

	out := b.Build(BuildInput{
		ChildNickname:     "小宇",
		ChildAgeYears:     5,
		ChildGender:       "boy",
		Duration:          5,
		Style:             "温馨治愈",
		Topic:             "勇敢",
		UserPromptCleaned: "讲个奥特曼睡前故事",
		PromptVersion:     "v1",
	})

	assert.Contains(t, out.SystemPrompt, "你是「爱宝」")
	assert.Contains(t, out.SystemPrompt, "小宇")
	assert.Contains(t, out.SystemPrompt, "5")
	assert.Contains(t, out.SystemPrompt, "男孩")
	assert.Contains(t, out.SystemPrompt, "勇敢")
	assert.Contains(t, out.SystemPrompt, "温馨治愈")
	assert.Contains(t, out.SystemPrompt, "5 分钟")
	for _, n := range []string{"1.", "2.", "3.", "4.", "5.", "6.", "7.", "8."} {
		assert.Contains(t, out.SystemPrompt, n, "missing constraint number %s", n)
	}
	assert.Contains(t, out.SystemPrompt, "v1")
	// UserPrompt now carries the original user text followed by:
	//   - a random SceneSeed line
	//   - a variety-mandate sentence
	//   - a per-request nonce
	// (Plan 9c: defeats Doubao prompt-cache when the user repeats a prompt.)
	assert.Contains(t, out.UserPrompt, "讲个奥特曼睡前故事")
	assert.Contains(t, out.UserPrompt, "本次创作随机灵感")
	assert.Contains(t, out.UserPrompt, "多样性要求")
	assert.Contains(t, out.UserPrompt, "本次会话 ID")
}

func TestBuilder_FearListRendered(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		ChildFearList: []string{"蜘蛛", "雷"},
		Duration:      5,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "蜘蛛")
	assert.Contains(t, out.SystemPrompt, "雷")
}

func TestBuilder_NoFearListRendersAsNone(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "（无）")
}

func TestBuilder_IPInstructionsAppear(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname:            "小宇",
		ChildAgeYears:            5,
		ChildGender:              "boy",
		Duration:                 5,
		Style:                    "温馨治愈",
		NormalizedIPInstructions: "本故事中爱宝变身为爱宝奥特曼。",
		PromptVersion:            "v1",
	})
	assert.Contains(t, out.SystemPrompt, "本次故事中的特别变身指令")
	assert.Contains(t, out.SystemPrompt, "爱宝奥特曼")
}

func TestBuilder_NoIPInstructionsOmitsBlock(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.NotContains(t, out.SystemPrompt, "本次故事中的特别变身指令")
}

func TestBuilder_MemorySummaryRenders(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		MemorySummary: "上次救过一只小恐龙阿绿。",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "故事记忆上下文")
	assert.Contains(t, out.SystemPrompt, "阿绿")
	assert.NotContains(t, out.SystemPrompt, "首次相遇")
}

func TestBuilder_MemorySectionPosition(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		MemorySummary: "测试摘要",
		PromptVersion: "v1",
	})
	memIdx := strings.Index(out.SystemPrompt, "【故事记忆上下文】")
	consIdx := strings.Index(out.SystemPrompt, "【不可违反的 8 条强约束】")
	assert.GreaterOrEqual(t, memIdx, 0, "memory section should appear")
	assert.GreaterOrEqual(t, consIdx, 0, "constraints section should appear")
	assert.Less(t, memIdx, consIdx, "memory section should precede 8 constraints")
	assert.Contains(t, out.SystemPrompt, "尝试借用以下记忆里的角色或场景")
}

func TestBuilder_EmptyMemoryGoesElseBranch(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		MemorySummary: "",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "首次相遇")
	assert.NotContains(t, out.SystemPrompt, "最近的故事记忆")
}

func TestBuilder_NoTopicShowsAsPure(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		Topic:         "",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "无（纯娱乐）")
}

func TestBuild_StorylineSection_RendersWhenHookOrSummariesPresent(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname:            "小宇",
		ChildAgeYears:            5,
		ChildGender:              "boy",
		Duration:                 5,
		Style:                    "温馨治愈",
		StorylineHook:            "他们能找到宝藏吗",
		StorylineRecentSummaries: []string{"第二集摘要", "第一集摘要"},
		EpisodeNumber:            3,
		PromptVersion:            "v1",
	})
	assert.Contains(t, out.SystemPrompt, "上一集剧情")
	assert.Contains(t, out.SystemPrompt, "第 3 集")
	assert.Contains(t, out.SystemPrompt, "他们能找到宝藏吗")
	assert.Contains(t, out.SystemPrompt, "第二集摘要")
	assert.Contains(t, out.SystemPrompt, "第一集摘要")
}

func TestBuild_StorylineSection_OmittedWhenBothEmpty(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      5,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.NotContains(t, out.SystemPrompt, "上一集剧情")
}

func TestBuild_WithOutlineHints(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname:        "小宇",
		ChildAgeYears:        5,
		ChildGender:          "boy",
		Duration:             5,
		Style:                "冒险探索",
		UserPromptCleaned:    "想去太空",
		TitleHint:            "小宇的星空冒险",
		SynopsisHint:         "小宇遇到爱宝，他们一起穿越到星空之上……",
		EducationalValueHint: "学到勇气与团队合作",
		PromptVersion:        "v1",
	})
	assert.Contains(t, out.SystemPrompt, "本故事的预先设定")
	assert.Contains(t, out.SystemPrompt, "小宇的星空冒险")
	assert.Contains(t, out.SystemPrompt, "学到勇气与团队合作")
}

func TestBuild_WithoutOutlineHints_BackCompat(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname:     "小宇",
		ChildAgeYears:     5,
		ChildGender:       "boy",
		Duration:          5,
		Style:             "冒险探索",
		UserPromptCleaned: "想去太空",
		PromptVersion:     "v1",
	})
	assert.NotContains(t, out.SystemPrompt, "本故事的预先设定")
}

func TestBuilder_RuneCountRoughlyMatchesDuration(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	for _, dur := range []int{3, 5, 8} {
		out := b.Build(BuildInput{
			ChildNickname: "小宇",
			ChildAgeYears: 5,
			ChildGender:   "boy",
			Duration:      dur,
			Style:         "温馨治愈",
			PromptVersion: "v1",
		})
		center := dur * 280
		rmin := center * 95 / 100
		rmax := center * 105 / 100
		assert.Contains(t, out.SystemPrompt, fmt.Sprintf("%d–%d 个汉字", rmin, rmax),
			"duration=%d minutes should render the [%d, %d] hard range", dur, rmin, rmax)
	}
}
