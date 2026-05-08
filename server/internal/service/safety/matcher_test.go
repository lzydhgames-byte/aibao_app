package safety

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatcher_FindFirst_Hit(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥", "暴力", "鬼"})
	hit, ok := m.FindFirst("我想要血腥的奥特曼故事")
	require.True(t, ok)
	assert.Equal(t, "血腥", hit)
}

func TestMatcher_FindFirst_Miss(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥", "暴力"})
	_, ok := m.FindFirst("讲个温馨的睡前故事")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_EmptyKeywords(t *testing.T) {
	m := NewKeywordMatcher(nil)
	_, ok := m.FindFirst("anything")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_EmptyInput(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥"})
	_, ok := m.FindFirst("")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_CaseInsensitiveASCII(t *testing.T) {
	m := NewKeywordMatcher([]string{"minecraft"})
	hit, ok := m.FindFirst("I love MineCraft a lot")
	require.True(t, ok)
	assert.Equal(t, "minecraft", hit)
}

func TestMatcher_FindFirst_PicksFirstHit(t *testing.T) {
	m := NewKeywordMatcher([]string{"暴力", "血腥"})
	hit, _ := m.FindFirst("血腥暴力")
	assert.Equal(t, "暴力", hit)
}

func BenchmarkMatcher_LargeKeywordSet(b *testing.B) {
	kws := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		kws = append(kws, "noise_"+strings.Repeat("x", i%5+1))
	}
	kws = append(kws, "血腥")
	m := NewKeywordMatcher(kws)
	input := "我想要一个长长的故事，里面有血腥的元素这是不允许的。"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.FindFirst(input)
	}
}
