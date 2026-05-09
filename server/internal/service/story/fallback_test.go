package story

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fallbackDir = "../../../safety/fallback_stories"

func TestFallback_LoadExactMatch(t *testing.T) {
	f := NewFallback(filepath.Clean(fallbackDir))
	out, err := f.Load(FallbackKey{Style: "温馨治愈", Duration: 5}, "小宇")
	require.NoError(t, err)
	assert.Contains(t, out, "小宇")
	assert.NotContains(t, out, "{{NICK}}")
}

func TestFallback_LoadFallsBackTo10Min(t *testing.T) {
	f := NewFallback(filepath.Clean(fallbackDir))
	// 15min file does not exist; should fall back to 10min
	out, err := f.Load(FallbackKey{Style: "冒险探索", Duration: 15}, "小宇")
	require.NoError(t, err)
	assert.Contains(t, out, "小宇")
}

func TestFallback_UnknownStyleUsesWarm(t *testing.T) {
	f := NewFallback(filepath.Clean(fallbackDir))
	out, err := f.Load(FallbackKey{Style: "未知风格", Duration: 10}, "小宇")
	require.NoError(t, err)
	assert.Contains(t, out, "小宇")
}

func TestFallback_NicknameReplacement(t *testing.T) {
	f := NewFallback(filepath.Clean(fallbackDir))
	out, err := f.Load(FallbackKey{Style: "温馨治愈", Duration: 5}, "测试昵称")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "测试昵称"))
}

func TestFallback_DirNotFound(t *testing.T) {
	f := NewFallback("/no/such/dir")
	_, err := f.Load(FallbackKey{Style: "温馨治愈", Duration: 10}, "小宇")
	assert.ErrorIs(t, err, ErrNoFallback)
}
