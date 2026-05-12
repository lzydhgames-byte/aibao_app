package audio_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/audio"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type stubTTS struct {
	resp     *tts.SynthesizeResponse
	err      error
	lastText string
}

func (s *stubTTS) Synthesize(_ context.Context, r tts.SynthesizeRequest) (*tts.SynthesizeResponse, error) {
	s.lastText = r.Text
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}
func (s *stubTTS) HealthCheck(_ context.Context) error { return nil }

type stubBGM struct {
	asset *model.BGMAsset
	err   error
}

func (s *stubBGM) PickByMood(_ context.Context, _ string) (*model.BGMAsset, error) {
	return s.asset, s.err
}

type stubCache struct {
	path string
	err  error
}

func (s *stubCache) GetLocalPath(_ context.Context, _ *model.BGMAsset) (string, error) {
	return s.path, s.err
}

type stubMixer struct {
	out []byte
	dur int
	err error
}

func (s *stubMixer) MixWithBGM(_ context.Context, _ []byte, _ string) ([]byte, int, error) {
	return s.out, s.dur, s.err
}

func newOrch(ts tts.Client, bgm audio.BGMPicker, c audio.BGMCache, m audio.Mixer) *audio.Orchestrator {
	bm := metrics.NewBusiness(prometheus.NewRegistry())
	return audio.NewOrchestrator(ts, bgm, c, m, bm)
}

func TestOrchestrator_HappyPath(t *testing.T) {
	ts := &stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 10}}
	o := newOrch(ts,
		&stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3", ObjectKey: "bgm/warm/warm_01.mp3"}},
		&stubCache{path: "/tmp/warm_01.mp3"},
		&stubMixer{out: []byte("MIXED"), dur: 12},
	)
	resp, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "[BGM情绪:温馨]Hello", Style: "温馨治愈",
		Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.True(t, resp.HasBGM)
	require.Equal(t, []byte("MIXED"), resp.AudioBytes)
	require.Equal(t, 12, resp.AudioDurationSec)
	require.Equal(t, model.MoodWarm, resp.Mood)
	require.Equal(t, "warm_01.mp3", resp.BGMFilename)
	// cue parser stripped the marker
	require.Equal(t, "Hello", ts.lastText)
}

func TestOrchestrator_BGMNotFound_Degrades(t *testing.T) {
	o := newOrch(
		&stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
		&stubBGM{err: repository.ErrNotFound},
		&stubCache{},
		&stubMixer{},
	)
	resp, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "无 cue", Style: "冒险探索",
		Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.False(t, resp.HasBGM)
	require.Equal(t, []byte("TTS"), resp.AudioBytes)
	require.Equal(t, model.MoodAdventure, resp.Mood)
}

func TestOrchestrator_MixerUnavailable_Degrades(t *testing.T) {
	o := newOrch(
		&stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
		&stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3"}},
		&stubCache{path: "/tmp/warm_01.mp3"},
		&stubMixer{err: audio.ErrMixerUnavailable},
	)
	resp, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "x", Style: "温馨治愈", Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.False(t, resp.HasBGM)
	require.Equal(t, []byte("TTS"), resp.AudioBytes)
}

func TestOrchestrator_MixerFailed_Degrades(t *testing.T) {
	o := newOrch(
		&stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
		&stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3"}},
		&stubCache{path: "/tmp/warm_01.mp3"},
		&stubMixer{err: &audio.ErrMixerFailed{Err: errors.New("ffmpeg boom"), Stderr: "bad"}},
	)
	resp, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "x", Style: "温馨", Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.False(t, resp.HasBGM)
	require.Equal(t, []byte("TTS"), resp.AudioBytes)
}

func TestOrchestrator_TTSFails_HardError(t *testing.T) {
	o := newOrch(
		&stubTTS{err: errors.New("tts down")},
		&stubBGM{}, &stubCache{}, &stubMixer{},
	)
	_, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "x", Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.Error(t, err)
}

func TestOrchestrator_CacheError_Degrades(t *testing.T) {
	o := newOrch(
		&stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
		&stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3"}},
		&stubCache{err: errors.New("cos 500")},
		&stubMixer{},
	)
	resp, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "x", Style: "温馨", Voice: tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.False(t, resp.HasBGM)
	require.Equal(t, []byte("TTS"), resp.AudioBytes)
}

func TestOrchestrator_CueParserStripsMarkers(t *testing.T) {
	ts := &stubTTS{resp: &tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 3}}
	o := newOrch(ts,
		&stubBGM{err: repository.ErrNotFound},
		&stubCache{},
		&stubMixer{},
	)
	_, err := o.Compose(context.Background(), audio.ComposeRequest{
		StoryText: "前[音效:门铃]中[BGM情绪:温馨]后",
		Voice:     tts.SynthesizeRequest{Format: "mp3"},
	})
	require.NoError(t, err)
	require.Equal(t, "前中后", ts.lastText)
}
