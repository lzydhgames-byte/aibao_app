package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/audio"
)

type fakeStoryRW struct {
	story        *model.Story
	readyKey     string
	readyFormat  string
	readyBytes   int64
	readyDur     int
	readyHasBGM  bool
	readyCalled  bool
	failedID     int64
	failedErrMsg string
	loadErr      error
}

func (f *fakeStoryRW) FindByID(_ context.Context, id int64) (*model.Story, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.story, nil
}
func (f *fakeStoryRW) MarkAudioReady(_ context.Context, _ int64, key, fmtStr string, sz int64, dur int, hasBGM bool) error {
	f.readyKey, f.readyFormat, f.readyBytes, f.readyDur, f.readyHasBGM = key, fmtStr, sz, dur, hasBGM
	f.readyCalled = true
	return nil
}
func (f *fakeStoryRW) MarkAudioFailed(_ context.Context, id int64, msg string) error {
	f.failedID, f.failedErrMsg = id, msg
	return nil
}

type stubComposer struct {
	resp *audio.ComposeResponse
	err  error
}

func (s *stubComposer) Compose(_ context.Context, _ audio.ComposeRequest) (*audio.ComposeResponse, error) {
	return s.resp, s.err
}

func okComposer(hasBGM bool) *stubComposer {
	return &stubComposer{resp: &audio.ComposeResponse{
		AudioBytes:       []byte("AUDIOBYTES"),
		AudioFormat:      "mp3",
		AudioDurationSec: 60,
		HasBGM:           hasBGM,
		Mood:             "warm",
	}}
}

func mkPayload(id int64) []byte {
	b, _ := json.Marshal(map[string]any{"story_id": id})
	return b
}

func defaultCfg() TTSHandlerConfig {
	return TTSHandlerConfig{
		Provider: "mock", Model: "m", VoiceID: "v",
		Format: "mp3", SampleRate: 32000, Bitrate: 128000, Speed: 1.0,
	}
}

func TestTTSHandler_HappyPath_HasBGM(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockSt := storage.NewMock()
	h := NewTTSSynthesisHandler(rw, rw, okComposer(true), mockSt, defaultCfg(), nil)

	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{ID: 1, Payload: mkPayload(42)}))

	assert.True(t, rw.readyCalled)
	assert.True(t, rw.readyHasBGM)
	assert.True(t, strings.HasPrefix(rw.readyKey, "audio/7/42-"), "got: %s", rw.readyKey)
	assert.Equal(t, "mp3", rw.readyFormat)
	assert.True(t, rw.readyBytes > 0)
	assert.True(t, mockSt.Has(rw.readyKey))
}

func TestTTSHandler_HappyPath_Degraded_NoBGM(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	h := NewTTSSynthesisHandler(rw, rw, okComposer(false), storage.NewMock(), defaultCfg(), nil)

	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)}))
	assert.True(t, rw.readyCalled)
	assert.False(t, rw.readyHasBGM)
}

func TestTTSHandler_AggregateIDFallback(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	h := NewTTSSynthesisHandler(rw, rw, okComposer(true), storage.NewMock(), defaultCfg(), nil)
	id := int64(42)
	payload, _ := json.Marshal(map[string]any{"story_id": 0})
	require.NoError(t, h.Handle(context.Background(),
		&model.OutboxEvent{ID: 1, Payload: payload, AggregateID: &id}))
	assert.NotEmpty(t, rw.readyKey)
}

func TestTTSHandler_AlreadyReady_Skip(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{
		ID: 42, ChildID: 7, TextContent: "x",
		AudioStatus: model.AudioStatusReady, AudioObjectKey: "audio/7/42-y.mp3",
	}}
	h := NewTTSSynthesisHandler(rw, rw, okComposer(true), storage.NewMock(), defaultCfg(), nil)
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)}))
	assert.False(t, rw.readyCalled, "should not re-upload")
}

func TestTTSHandler_ComposeFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	composer := &stubComposer{err: errors.New("tts down")}
	h := NewTTSSynthesisHandler(rw, rw, composer, storage.NewMock(), defaultCfg(), nil)

	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
	assert.NotEmpty(t, rw.failedErrMsg)
}

func TestTTSHandler_StorageFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockSt := storage.NewMock()
	mockSt.FailNext()
	h := NewTTSSynthesisHandler(rw, rw, okComposer(true), mockSt, defaultCfg(), nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
}

func TestTTSHandler_BadPayload(t *testing.T) {
	h := NewTTSSynthesisHandler(&fakeStoryRW{}, &fakeStoryRW{}, okComposer(true), storage.NewMock(),
		defaultCfg(), nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: []byte("not json")})
	require.Error(t, err)
}

func TestTTSHandler_StoryNotFound(t *testing.T) {
	rw := &fakeStoryRW{loadErr: errors.New("not found")}
	h := NewTTSSynthesisHandler(rw, rw, okComposer(true), storage.NewMock(), defaultCfg(), nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(99)})
	require.Error(t, err)
}
