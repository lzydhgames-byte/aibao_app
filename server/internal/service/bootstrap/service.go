package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	childsvc "github.com/aibao/server/internal/service/child"
)

const bootstrapSystemPrompt = `你是儿童故事 App 的画像生成器。给你一组父母填写的孩子信息，请用 80-150 字的自然中文段落，描绘出这个孩子。要求：第三人称、温柔、具体（要把家人名字、害怕的东西、喜欢的角色等具体细节融入）；只输出段落本身，不要列表、不要标题、不要解释。`

// Answer is one submitted answer keyed by Question.ID.
// Value is interface{} — must match the Question's type at validation time.
type Answer struct {
	QID   string      `json:"q_id"`
	Value interface{} `json:"value"`
}

// Profile is the JSONB shape persisted into children.profile.
type Profile struct {
	Version     int                    `json:"version"`
	Description string                 `json:"description"`
	Answers     map[string]interface{} `json:"answers"`
}

// ChildUpdater is the minimal surface bootstrap needs from child service.
// Kept small so tests can inject fakes.
type ChildUpdater interface {
	Update(ctx context.Context, userID, childID int64, in childsvc.UpdateInput) (any, error)
}

// ChildLookup fetches a child for nickname threading into the LLM prompt.
type ChildLookup interface {
	GetByID(ctx context.Context, userID, childID int64) (*model.Child, error)
}

// childServiceAdapter adapts the concrete childsvc.Service into ChildUpdater + ChildLookup.
type childServiceAdapter struct{ inner *childsvc.Service }

func (a *childServiceAdapter) Update(ctx context.Context, userID, childID int64, in childsvc.UpdateInput) (any, error) {
	return a.inner.Update(ctx, userID, childID, in)
}

func (a *childServiceAdapter) GetByID(ctx context.Context, userID, childID int64) (*model.Child, error) {
	return a.inner.GetByID(ctx, userID, childID)
}

// Service builds a profile from BOOTSTRAP answers.
type Service struct {
	children    ChildUpdater
	lookup      ChildLookup
	llmClient   llm.Client
	model       string
	temperature float64
	biz         *metrics.Business
	logger      *slog.Logger
}

// NewService constructs.
func NewService(children *childsvc.Service, c llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Service {
	adapter := &childServiceAdapter{inner: children}
	return &Service{
		children:    adapter,
		lookup:      adapter,
		llmClient:   c,
		model:       model,
		temperature: temperature,
		biz:         biz,
		logger:      logger,
	}
}

// NewServiceWithUpdater is the test-friendly constructor that takes the
// ChildUpdater + ChildLookup interfaces directly. lookup may be nil for
// tests that don't exercise nickname threading.
func NewServiceWithUpdater(children ChildUpdater, c llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Service {
	return &Service{children: children, llmClient: c, model: model, temperature: temperature, biz: biz, logger: logger}
}

// NewServiceWithLookup is like NewServiceWithUpdater but also wires a ChildLookup.
func NewServiceWithLookup(children ChildUpdater, lookup ChildLookup, c llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Service {
	return &Service{children: children, lookup: lookup, llmClient: c, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Submit validates answers, calls LLM to render description (fail-open),
// and writes back into children.profile.
func (s *Service) Submit(ctx context.Context, userID, childID int64, answers []Answer) (*Profile, error) {
	answersMap, err := s.validate(answers)
	if err != nil {
		return nil, err
	}
	nickname := ""
	if s.lookup != nil {
		if c, lerr := s.lookup.GetByID(ctx, userID, childID); lerr == nil && c != nil {
			nickname = c.Nickname
		} else if lerr != nil && s.logger != nil {
			s.logger.Warn("bootstrap.lookup.fail", "err", lerr)
		}
	}
	description := s.renderDescription(ctx, nickname, answersMap) // "" on LLM error (fail-open)
	profile := &Profile{Version: Version, Description: description, Answers: answersMap}
	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "profile_marshal_failed", "服务暂时不可用")
	}
	if _, err := s.children.Update(ctx, userID, childID, childsvc.UpdateInput{Profile: &raw}); err != nil {
		return nil, err // childsvc maps not_owner / not_found
	}
	if s.biz != nil {
		s.biz.BootstrapCompletionTotal.Inc()
	}
	return profile, nil
}

func (s *Service) validate(answers []Answer) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(answers))
	for _, a := range answers {
		q, ok := QuestionByID(a.QID)
		if !ok {
			return nil, apperr.New(apperr.CodeInvalidArgument, "unknown_question", "未知问题 id: "+a.QID)
		}
		if err := checkAnswerShape(q, a.Value); err != nil {
			return nil, err
		}
		out[a.QID] = a.Value
	}
	for _, q := range Questions() {
		if !q.Required {
			continue
		}
		if _, ok := out[q.ID]; !ok {
			return nil, apperr.New(apperr.CodeInvalidArgument, "missing_required", "缺少必填问题: "+q.ID)
		}
	}
	return out, nil
}

func checkAnswerShape(q Question, v interface{}) error {
	switch q.Type {
	case TypeText:
		s, ok := v.(string)
		if !ok {
			return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为字符串")
		}
		if q.Required && strings.TrimSpace(s) == "" {
			return apperr.New(apperr.CodeInvalidArgument, "empty_text", q.ID+" 不能为空")
		}
		if q.MaxLength > 0 && len([]rune(s)) > q.MaxLength {
			return apperr.New(apperr.CodeInvalidArgument, "text_too_long", q.ID+" 超过最大长度")
		}
	case TypeSingleSelect:
		s, ok := v.(string)
		if !ok || !contains(q.Options, s) {
			return apperr.New(apperr.CodeInvalidArgument, "invalid_option", q.ID+" 选项不在白名单")
		}
	case TypeMultiSelect:
		arr, ok := v.([]interface{})
		if !ok {
			return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为数组")
		}
		for _, item := range arr {
			s, ok := item.(string)
			if !ok || !contains(q.Options, s) {
				return apperr.New(apperr.CodeInvalidArgument, "invalid_option", q.ID+" 含非法选项")
			}
		}
	case TypeBoolean:
		if _, ok := v.(bool); !ok {
			return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为 true/false")
		}
	}
	return nil
}

func contains(opts []string, v string) bool {
	for _, o := range opts {
		if o == v {
			return true
		}
	}
	return false
}

func (s *Service) renderDescription(ctx context.Context, nickname string, answers map[string]interface{}) string {
	userPayload, _ := json.Marshal(answers)
	userContent := fmt.Sprintf("孩子昵称：%s。其他档案信息（JSON）：%s", nickname, string(userPayload))
	out, err := s.llmClient.Generate(ctx, llm.GenerateRequest{
		Model:       s.model,
		Temperature: s.temperature,
		MaxTokens:   300,
		Messages: []llm.Message{
			{Role: "system", Content: bootstrapSystemPrompt},
			{Role: "user", Content: userContent},
		},
	})
	if err != nil {
		if s.biz != nil {
			s.biz.LLMFailFallbackTotal.WithLabelValues("doubao", s.model, "upstream_error").Inc()
		}
		if s.logger != nil {
			s.logger.Warn("bootstrap.render.fail", "err", err)
		}
		return ""
	}
	return strings.TrimSpace(out.Text)
}
