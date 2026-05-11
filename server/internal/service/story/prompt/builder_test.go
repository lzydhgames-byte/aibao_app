package prompt

import (
	"fmt"
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
		Duration:          10,
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
	assert.Contains(t, out.SystemPrompt, "10 分钟")
	for _, n := range []string{"1.", "2.", "3.", "4.", "5.", "6.", "7.", "8."} {
		assert.Contains(t, out.SystemPrompt, n, "missing constraint number %s", n)
	}
	assert.Contains(t, out.SystemPrompt, "v1")
	assert.Equal(t, "讲个奥特曼睡前故事", out.UserPrompt)
}

func TestBuilder_FearListRendered(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		ChildFearList: []string{"蜘蛛", "雷"},
		Duration:      10,
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
		Duration:      10,
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
		Duration:                 10,
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
		Duration:      10,
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
		Duration:      10,
		Style:         "温馨治愈",
		MemorySummary: "上次救过一只小恐龙阿绿。",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "最近的故事记忆")
	assert.Contains(t, out.SystemPrompt, "阿绿")
	assert.NotContains(t, out.SystemPrompt, "首次相遇")
}

func TestBuilder_EmptyMemoryGoesElseBranch(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
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
		Duration:      10,
		Style:         "温馨治愈",
		Topic:         "",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "无（纯娱乐）")
}

func TestBuilder_RuneCountRoughlyMatchesDuration(t *testing.T) {
	b, err := NewBuilder(templatePath)
	require.NoError(t, err)
	for _, dur := range []int{5, 10, 15} {
		out := b.Build(BuildInput{
			ChildNickname: "小宇",
			ChildAgeYears: 5,
			ChildGender:   "boy",
			Duration:      dur,
			Style:         "温馨治愈",
			PromptVersion: "v1",
		})
		expected := dur * 120
		assert.Contains(t, out.SystemPrompt, fmt.Sprintf("约 %d 字", expected),
			"duration=%d minutes should map to about %d runes", dur, expected)
	}
}
