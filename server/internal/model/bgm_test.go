package model

import "testing"

func TestMoodFromStyle(t *testing.T) {
	cases := map[string]string{
		"温馨治愈":    MoodWarm,
		"温馨":      MoodWarm,
		"治愈":      MoodWarm,
		"冒险探索":    MoodAdventure,
		"冒险":      MoodAdventure,
		"搞笑欢乐":    MoodFunny,
		"搞笑":      MoodFunny,
		"神奇魔法":    MoodMagic,
		"神奇":      MoodMagic,
		"魔法":      MoodMagic,
		"科普认知":    MoodCurious,
		"科普":      MoodCurious,
		" 治愈 ":    MoodWarm,
		"悬疑":      MoodWarm, // unknown → warm
		"":        MoodWarm,
		"奇怪的风格":   MoodWarm,
	}
	for in, want := range cases {
		if got := MoodFromStyle(in); got != want {
			t.Errorf("MoodFromStyle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMoodFromCueLabel(t *testing.T) {
	cases := map[string]string{
		"温馨":   MoodWarm,
		"治愈":   MoodWarm,
		"冒险":   MoodAdventure,
		"搞笑":   MoodFunny,
		"神奇":   MoodMagic,
		"魔法":   MoodMagic,
		"科普":   MoodCurious,
		"好奇":   MoodCurious,
		"认知":   MoodCurious,
		"unknown": MoodWarm, // fallback
		"":     MoodWarm,
	}
	for in, want := range cases {
		if got := MoodFromCueLabel(in); got != want {
			t.Errorf("MoodFromCueLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
