package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/aibao/server/internal/pkg/logger"
)

// Mixer mixes a TTS audio byte stream with a BGM file on disk into a single mp3.
type Mixer interface {
	// MixWithBGM returns (mixedBytes, durationSec, error).
	// On any error the caller is expected to fall back to ttsBytes unchanged.
	MixWithBGM(ctx context.Context, ttsBytes []byte, bgmPath string) ([]byte, int, error)
}

// FFmpegMixer is the production Mixer that shells out to ffmpeg.
type FFmpegMixer struct {
	Binary      string
	BGMVolumeDB float64
	SampleRate  int
	BitrateKbps int
	Timeout     time.Duration
}

// NewFFmpegMixer constructs an FFmpegMixer.
func NewFFmpegMixer(binary string, bgmVolDB float64, sr, br int, timeout time.Duration) *FFmpegMixer {
	return &FFmpegMixer{Binary: binary, BGMVolumeDB: bgmVolDB, SampleRate: sr, BitrateKbps: br, Timeout: timeout}
}

// ErrMixerUnavailable means ffmpeg cannot be located/executed.
var ErrMixerUnavailable = errors.New("ffmpeg unavailable")

// ErrMixerFailed wraps a ffmpeg execution failure.
type ErrMixerFailed struct {
	Stderr string
	Err    error
}

func (e *ErrMixerFailed) Error() string {
	return fmt.Sprintf("ffmpeg mix failed: %v; stderr: %s", e.Err, head(e.Stderr, 500))
}
func (e *ErrMixerFailed) Unwrap() error { return e.Err }

// MixWithBGM runs ffmpeg to combine ttsBytes (mp3) with bgmPath (mp3) and
// returns the resulting mp3 bytes plus an estimated duration in seconds.
func (m *FFmpegMixer) MixWithBGM(ctx context.Context, ttsBytes []byte, bgmPath string) ([]byte, int, error) {
	if _, err := exec.LookPath(m.Binary); err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrMixerUnavailable, err)
	}
	lg := logger.FromCtx(ctx).With("module", "mixer")

	ttsTmp, err := os.CreateTemp("", "aibao-tts-*.mp3")
	if err != nil {
		return nil, 0, fmt.Errorf("create tts tmp: %w", err)
	}
	defer os.Remove(ttsTmp.Name())
	if _, err := ttsTmp.Write(ttsBytes); err != nil {
		ttsTmp.Close()
		return nil, 0, fmt.Errorf("write tts tmp: %w", err)
	}
	ttsTmp.Close()

	outTmp, err := os.CreateTemp("", "aibao-mix-*.mp3")
	if err != nil {
		return nil, 0, fmt.Errorf("create out tmp: %w", err)
	}
	outPath := outTmp.Name()
	outTmp.Close()
	defer os.Remove(outPath)

	cctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	filter := fmt.Sprintf(
		"[1:a]volume=%.1fdB[bgm];[0:a][bgm]amix=inputs=2:duration=first:dropout_transition=0[out]",
		m.BGMVolumeDB,
	)

	args := []string{
		"-y",
		"-i", ttsTmp.Name(),
		"-stream_loop", "-1", "-i", bgmPath,
		"-filter_complex", filter,
		"-map", "[out]",
		"-c:a", "libmp3lame",
		"-b:a", fmt.Sprintf("%dk", m.BitrateKbps),
		"-ar", strconv.Itoa(m.SampleRate),
		"-ac", "2",
		outPath,
	}

	cmd := exec.CommandContext(cctx, m.Binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	tStart := time.Now()
	if err := cmd.Run(); err != nil {
		lg.Warn("mixer.ffmpeg.fail", "err", err.Error(), "stderr_head", head(stderr.String(), 500))
		return nil, 0, &ErrMixerFailed{Stderr: stderr.String(), Err: err}
	}
	elapsed := time.Since(tStart)

	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, 0, fmt.Errorf("read output: %w", err)
	}

	dur := parseDurationFromStderr(stderr.String())
	if dur <= 0 && m.BitrateKbps > 0 {
		dur = int(float64(len(out)*8) / float64(m.BitrateKbps*1000))
	}

	lg.Info("mixer.ok", "elapsed_ms", elapsed.Milliseconds(),
		"out_bytes", len(out), "dur_sec", dur)
	return out, dur, nil
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var durationRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+)`)

// parseDurationFromStderr extracts "Duration: HH:MM:SS" from ffmpeg stderr.
// Returns 0 if not found.
func parseDurationFromStderr(s string) int {
	m := durationRe.FindStringSubmatch(s)
	if len(m) != 4 {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	mn, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	return h*3600 + mn*60 + sec
}
