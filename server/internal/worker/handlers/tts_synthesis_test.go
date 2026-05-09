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
	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/model"
)

type fakeStoryRW struct {
	story        *model.Story
	readyKey     string
	readyFormat  string
	readyBytes   int64
	readyDur     int
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
func (f *fakeStoryRW) MarkAudioReady(_ context.Context, _ int64, key, fmtStr string, sz int64, dur int) error {
	f.readyKey, f.readyFormat, f.readyBytes, f.readyDur = key, fmtStr, sz, dur
	return nil
}
func (f *fakeStoryRW) MarkAudioFailed(_ context.Context, id int64, msg string) error {
	f.failedID, f.failedErrMsg = id, msg
	return nil
}

func mkPayload(id int64) []byte {
	b, _ := json.Marshal(map[string]any{"story_id": id})
	return b
}

func TestTTSHandler_HappyPath(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "小宇冒险"}}
	mockTTS := tts.NewMock()
	mockSt := storage.NewMock()
	h := NewTTSSynthesisHandler(rw, rw, mockTTS, mockSt, TTSHandlerConfig{
		Provider: "mock", Model: "speech-01-turbo", VoiceID: "v",
		Format: "mp3", SampleRate: 32000, Bitrate: 128000, Speed: 1.0,
	}, nil)

	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{ID: 1, Payload: mkPayload(42)}))

	assert.True(t, strings.HasPrefix(rw.readyKey, "audio/7/42-"), "got: %s", rw.readyKey)
	assert.Equal(t, "mp3", rw.readyFormat)
	assert.True(t, rw.readyBytes > 0)
	assert.True(t, mockSt.Has(rw.readyKey))
}

func TestTTSHandler_AggregateIDFallback(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), storage.NewMock(),
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)
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
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), storage.NewMock(),
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)}))
	assert.Empty(t, rw.readyKey, "should not re-upload")
}

func TestTTSHandler_TTSFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockTTS := tts.NewMock()
	mockTTS.FailNext()
	h := NewTTSSynthesisHandler(rw, rw, mockTTS, storage.NewMock(),
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)

	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
	assert.NotEmpty(t, rw.failedErrMsg)
}

func TestTTSHandler_StorageFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockSt := storage.NewMock()
	mockSt.FailNext()
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), mockSt,
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
}

func TestTTSHandler_BadPayload(t *testing.T) {
	h := NewTTSSynthesisHandler(&fakeStoryRW{}, &fakeStoryRW{}, tts.NewMock(), storage.NewMock(),
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: []byte("not json")})
	require.Error(t, err)
}

func TestTTSHandler_StoryNotFound(t *testing.T) {
	rw := &fakeStoryRW{loadErr: errors.New("not found")}
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), storage.NewMock(),
		TTSHandlerConfig{Provider: "mock", Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(99)})
	require.Error(t, err)
}
