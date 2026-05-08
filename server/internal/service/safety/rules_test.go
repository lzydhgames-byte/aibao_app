package safety

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRules_Success(t *testing.T) {
	rs, err := LoadRules(
		filepath.Join("testdata", "minimal_rules.yaml"),
		filepath.Join("testdata", "minimal_whitelist.yaml"),
		filepath.Join("testdata", "minimal_blacklist.yaml"),
	)
	require.NoError(t, err)
	require.NotNil(t, rs)

	assert.Contains(t, rs.Redlines, "violence")
	assert.Contains(t, rs.Redlines["violence"], "血腥")
	assert.Contains(t, rs.Redlines["horror"], "鬼")

	assert.Contains(t, rs.IPWhitelist, "奥特曼")
	assert.Contains(t, rs.IPWhitelist["奥特曼"], "爱宝奥特曼")

	assert.Contains(t, rs.IPBlacklist, "进击的巨人")

	assert.Contains(t, rs.AllRedlinesFlat, "血腥")
	assert.Contains(t, rs.AllRedlinesFlat, "鬼")
}

func TestLoadRules_MissingFile(t *testing.T) {
	_, err := LoadRules("/no/such/file.yaml", "x", "y")
	assert.Error(t, err)
}

func TestLoadRules_EmptyYAMLOK(t *testing.T) {
	tmp := t.TempDir()
	emptyRules := filepath.Join(tmp, "rules.yaml")
	emptyWL := filepath.Join(tmp, "wl.yaml")
	emptyBL := filepath.Join(tmp, "bl.yaml")

	require.NoError(t, os.WriteFile(emptyRules, []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(emptyWL, []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(emptyBL, []byte("[]\n"), 0o600))

	rs, err := LoadRules(emptyRules, emptyWL, emptyBL)
	require.NoError(t, err)
	assert.Empty(t, rs.AllRedlinesFlat)
	assert.Empty(t, rs.IPWhitelist)
	assert.Empty(t, rs.IPBlacklist)
}
