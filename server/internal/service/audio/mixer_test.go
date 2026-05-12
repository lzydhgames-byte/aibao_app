package audio_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aibao/server/internal/service/audio"
	"github.com/stretchr/testify/require"
)

func TestFFmpegMixer_Unavailable(t *testing.T) {
	m := audio.NewFFmpegMixer("/no/such/binary-xyz", -18, 32000, 128, 5*time.Second)
	_, _, err := m.MixWithBGM(context.Background(), []byte("x"), "/tmp/nope.mp3")
	require.ErrorIs(t, err, audio.ErrMixerUnavailable)
}

// generateSineMp3 uses ffmpeg to emit a short MP3 sine wave at the given path.
func generateSineMp3(t *testing.T, path string, freqHz, durationSec int) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi",
		"-i", "sine=frequency="+strconv.Itoa(freqHz)+":duration="+strconv.Itoa(durationSec)+":sample_rate=22050",
		"-q:a", "9", "-acodec", "libmp3lame",
		path,
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "ffmpeg fixture generate failed: %s", string(out))
}

func TestFFmpegMixer_RealMix(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg not on PATH; skipping real mix test: %v", err)
	}

	dir := t.TempDir()
	ttsPath := filepath.Join(dir, "tts.mp3")
	bgmPath := filepath.Join(dir, "bgm.mp3")
	generateSineMp3(t, ttsPath, 440, 2)
	generateSineMp3(t, bgmPath, 220, 3)

	tts, err := os.ReadFile(ttsPath)
	require.NoError(t, err)
	require.NotEmpty(t, tts)

	m := audio.NewFFmpegMixer("ffmpeg", -18, 32000, 128, 30*time.Second)
	out, dur, err := m.MixWithBGM(context.Background(), tts, bgmPath)
	require.NoError(t, err)
	require.True(t, len(out) > 1024, "mixed output should be a real mp3, got %d bytes", len(out))
	require.GreaterOrEqual(t, dur, 0)
}

func TestFFmpegMixer_BGMMissing(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg not on PATH: %v", err)
	}
	m := audio.NewFFmpegMixer("ffmpeg", -18, 32000, 128, 10*time.Second)
	_, _, err := m.MixWithBGM(context.Background(), []byte("not real mp3"), "/tmp/does-not-exist-aibao-xyz.mp3")
	require.Error(t, err)
	var fe *audio.ErrMixerFailed
	require.ErrorAs(t, err, &fe)
	require.NotEmpty(t, fe.Stderr)
}
