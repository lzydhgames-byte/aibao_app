package tts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimax_NewRequiresKeys(t *testing.T) {
	_, err := NewMinimax(MinimaxConfig{})
	require.Error(t, err)
	_, err = NewMinimax(MinimaxConfig{GroupID: "g"})
	require.Error(t, err)
}

func TestMinimax_HappyPath(t *testing.T) {
	wantBody := []string{"speech-01-turbo", "小宇", "female-tianmei", "32000", "mp3"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/t2a_v2", r.URL.Path)
		assert.Equal(t, "test-group", r.URL.Query().Get("GroupId"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		for _, w := range wantBody {
			assert.Contains(t, body, w)
		}
		fakeAudio := []byte{0xff, 0xfb, 0x90, 0x40}
		hexed := hex.EncodeToString(fakeAudio)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"audio": hexed, "status": 2},
			"extra_info": map[string]any{"audio_length": 12345},
			"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}))
	defer srv.Close()

	c, err := NewMinimax(MinimaxConfig{
		BaseURL: srv.URL, GroupID: "test-group", APIKey: "test-key", TimeoutSeconds: 5,
	})
	require.NoError(t, err)

	resp, err := c.Synthesize(context.Background(), SynthesizeRequest{
		Text: "小宇", Model: "speech-01-turbo", VoiceID: "female-tianmei",
		Format: "mp3", SampleRate: 32000, Bitrate: 128000, Speed: 1.0,
	})
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xfb, 0x90, 0x40}, resp.Audio)
	assert.Equal(t, 12, resp.DurationSeconds)
	assert.Equal(t, "minimax", resp.Provider)
}

func TestMinimax_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "tts upstream error"))
}

func TestMinimax_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"audio": ""},
			"base_resp": map[string]any{"status_code": 1004, "status_msg": "rate limit"},
		})
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1004")
}

func TestMinimax_BadHex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"audio": "not-hex!!!"},
			"base_resp": map[string]any{"status_code": 0},
		})
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hex decode")
}

func TestMinimax_EmptyAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"audio": ""},
			"base_resp": map[string]any{"status_code": 0},
		})
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty audio")
}

func TestMinimax_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}
