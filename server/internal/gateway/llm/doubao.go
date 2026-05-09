package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// DoubaoConfig holds settings for the Doubao client.
type DoubaoConfig struct {
	APIKey         string
	BaseURL        string // e.g. https://ark.cn-beijing.volces.com/api/v3
	TimeoutSeconds int
}

// DoubaoClient calls Volcengine Ark's OpenAI-compatible chat completion API.
type DoubaoClient struct {
	c       *openai.Client
	timeout time.Duration
}

// NewDoubao constructs a DoubaoClient. Returns error if APIKey is missing.
func NewDoubao(cfg DoubaoConfig) (*DoubaoClient, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("doubao: api key required (set AIBAO_LLM_DOUBAO_API_KEY)")
	}
	cc := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		cc.BaseURL = cfg.BaseURL
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &DoubaoClient{
		c:       openai.NewClientWithConfig(cc),
		timeout: timeout,
	}, nil
}

// Generate calls Doubao with OpenAI-compatible messages.
func (d *DoubaoClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	msgs := make([]openai.ChatCompletionMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	creq := openai.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    msgs,
		Temperature: float32(req.Temperature),
	}
	if req.MaxTokens > 0 {
		creq.MaxTokens = req.MaxTokens
	}

	start := time.Now()
	resp, err := d.c.CreateChatCompletion(ctx, creq)
	latency := time.Since(start)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%w: empty choices", ErrUpstream)
	}
	return &GenerateResponse{
		Text:         resp.Choices[0].Message.Content,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		Provider:     "doubao",
		Model:        req.Model,
		Latency:      latency,
	}, nil
}

// HealthCheck issues a tiny ping-style call.
func (d *DoubaoClient) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := d.c.ListModels(ctx)
	return err
}
