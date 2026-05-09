package tts

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MinimaxConfig holds settings for the Minimax client.
type MinimaxConfig struct {
	BaseURL        string
	GroupID        string
	APIKey         string
	TimeoutSeconds int
}

// MinimaxClient calls Minimax T2A v2.
type MinimaxClient struct {
	cfg  MinimaxConfig
	http *http.Client
}

// NewMinimax constructs a MinimaxClient.
func NewMinimax(cfg MinimaxConfig) (*MinimaxClient, error) {
	if cfg.GroupID == "" {
		return nil, errors.New("minimax: group id required (set AIBAO_TTS_MINIMAX_GROUP_ID)")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("minimax: api key required (set AIBAO_TTS_MINIMAX_API_KEY)")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.chat"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &MinimaxClient{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}, nil
}

type minimaxT2AReq struct {
	Model        string              `json:"model"`
	Text         string              `json:"text"`
	VoiceSetting minimaxVoiceSetting `json:"voice_setting"`
	AudioSetting minimaxAudioSetting `json:"audio_setting"`
}

type minimaxVoiceSetting struct {
	VoiceID string  `json:"voice_id"`
	Speed   float64 `json:"speed"`
	Vol     float64 `json:"vol"`
	Pitch   float64 `json:"pitch"`
}

type minimaxAudioSetting struct {
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
	Channel    int    `json:"channel"`
}

type minimaxT2AResp struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	ExtraInfo struct {
		AudioLength int `json:"audio_length"`
	} `json:"extra_info"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// Synthesize calls /v1/t2a_v2.
func (m *MinimaxClient) Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	body := minimaxT2AReq{
		Model: req.Model,
		Text:  req.Text,
		VoiceSetting: minimaxVoiceSetting{
			VoiceID: req.VoiceID,
			Speed:   req.Speed,
			Vol:     1.0,
			Pitch:   0,
		},
		AudioSetting: minimaxAudioSetting{
			SampleRate: req.SampleRate,
			Bitrate:    req.Bitrate,
			Format:     req.Format,
			Channel:    1,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal req: %w", err)
	}

	url := fmt.Sprintf("%s/v1/t2a_v2?GroupId=%s", m.cfg.BaseURL, m.cfg.GroupID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("new req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := m.http.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: http %d: %s", ErrUpstream, resp.StatusCode, truncate(string(rb), 200))
	}

	var parsed minimaxT2AResp
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
	}
	if parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: minimax %d: %s", ErrUpstream, parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}
	if parsed.Data.Audio == "" {
		return nil, fmt.Errorf("%w: empty audio payload", ErrUpstream)
	}

	audioBytes, err := hex.DecodeString(parsed.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("%w: hex decode: %v", ErrUpstream, err)
	}

	durSec := parsed.ExtraInfo.AudioLength / 1000

	return &SynthesizeResponse{
		Audio:           audioBytes,
		Format:          req.Format,
		DurationSeconds: durSec,
		Provider:        "minimax",
		Latency:         latency,
	}, nil
}

// HealthCheck does NOT call Minimax; just validates config presence.
func (m *MinimaxClient) HealthCheck(_ context.Context) error {
	if m.cfg.GroupID == "" || m.cfg.APIKey == "" {
		return errors.New("minimax: not configured")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
