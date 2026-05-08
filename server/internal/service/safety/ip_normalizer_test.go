package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestNormalizer(t *testing.T) *IPNormalizer {
	t.Helper()
	wl := map[string]string{
		"奥特曼": "本故事中爱宝变身为爱宝奥特曼。",
		"汪汪队": "本故事中爱宝化身为汪汪救援小队伙伴。",
	}
	bl := []string{"进击的巨人", "鬼灭之刃"}
	return NewIPNormalizer(wl, bl)
}

func TestIPNormalizer_BlacklistHit(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("我想要进击的巨人风格的故事")
	assert.Equal(t, IPBlacklisted, res.Verdict)
	assert.Equal(t, "进击的巨人", res.MatchedIP)
}

func TestIPNormalizer_WhitelistHit(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("讲个奥特曼睡前故事")
	assert.Equal(t, IPWhitelisted, res.Verdict)
	assert.Contains(t, res.MatchedIPs, "奥特曼")
	assert.Contains(t, res.Instructions, "爱宝奥特曼")
}

func TestIPNormalizer_MultiWhitelistHits(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("奥特曼和汪汪队一起冒险")
	assert.Equal(t, IPWhitelisted, res.Verdict)
	assert.Len(t, res.MatchedIPs, 2)
}

func TestIPNormalizer_NoIPMatch(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("讲个温馨的小恐龙故事")
	assert.Equal(t, IPNoMatch, res.Verdict)
	assert.Empty(t, res.MatchedIPs)
}

func TestIPNormalizer_BlacklistTakesPriority(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("奥特曼大战进击的巨人")
	assert.Equal(t, IPBlacklisted, res.Verdict)
	assert.Equal(t, "进击的巨人", res.MatchedIP)
}
