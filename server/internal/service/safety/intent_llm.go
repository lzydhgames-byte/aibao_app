package safety

import (
	"context"
	"strings"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/pkg/logger"
)

// LLMIntentProvider asks an LLM whether the user prompt expresses an unsafe
// intent (e.g. "I want a violent story"). On any LLM failure, returns
// IntentSafe and logs — we never block a user because the LLM hiccupped.
type LLMIntentProvider struct {
	c     llm.Client
	model string
}

// NewLLMIntentProvider constructs a provider backed by an LLM client.
func NewLLMIntentProvider(c llm.Client, model string) *LLMIntentProvider {
	return &LLMIntentProvider{c: c, model: model}
}

const intentSystemPrompt = `你是一个儿童故事请求安全分类器。判断输入是否表达了"想要不适合儿童的故事内容"的意图。
仅输出三个单词之一（不带任何其他字符）：
- "safe" —— 正常的儿童故事请求
- "unsafe" —— 明显想要暴力/血腥/恐怖/性等不适合儿童的内容
- "uncertain" —— 模糊不清的请求

只回答一个单词，不要解释。`

// Classify asks the LLM and parses its single-word response.
func (p *LLMIntentProvider) Classify(ctx context.Context, userPrompt string) (Intent, error) {
	resp, err := p.c.Generate(ctx, llm.GenerateRequest{
		Model: p.model,
		Messages: []llm.Message{
			{Role: "system", Content: intentSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
		MaxTokens:   8,
	})
	if err != nil {
		logger.FromCtx(ctx).Warn("safety.intent_llm.fail_fallback_safe", "err", err.Error())
		return IntentSafe, nil
	}
	switch strings.TrimSpace(strings.ToLower(resp.Text)) {
	case "safe":
		return IntentSafe, nil
	case "unsafe":
		return IntentUnsafe, nil
	case "uncertain":
		return IntentUncertain, nil
	default:
		logger.FromCtx(ctx).Warn("safety.intent_llm.unparseable", "raw", resp.Text)
		return IntentSafe, nil
	}
}
