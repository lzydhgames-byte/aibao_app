//go:build integration

package outline_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/gateway/llm"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/idhash"
	"github.com/aibao/server/internal/service/cost"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/safety"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// fakeLLM returns canned responses in order.
type fakeLLM struct {
	responses []string
	idx       int
}

var errLLMExhausted = errors.New("fakeLLM responses exhausted")

func (f *fakeLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if f.idx >= len(f.responses) {
		return nil, errLLMExhausted
	}
	r := f.responses[f.idx]
	f.idx++
	return &llm.GenerateResponse{
		Text:         r,
		InputTokens:  600,
		OutputTokens: 400,
		Provider:     "doubao",
		Model:        "doubao-1.5-lite-32k",
	}, nil
}

func (f *fakeLLM) HealthCheck(ctx context.Context) error { return nil }

func newTestPreChecker(t *testing.T) *safety.PreChecker {
	t.Helper()
	rs := &safety.RuleSet{
		AllRedlinesFlat: []string{},
		IPWhitelist:     map[string]string{},
		IPBlacklist:     []string{},
	}
	return safety.NewPreChecker(rs, safety.NoopIntentProvider{})
}

func newTestService(t *testing.T, llmResponses ...string) *outline.Service {
	t.Helper()
	db := integrationDB(t)
	rdb := integrationRedis(t)

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-svctest
  entries:
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`)))
	pb, err := pkgcost.LoadFromViper(v)
	require.NoError(t, err)
	rec := cost.NewRecorder(pb, nil)

	return outline.NewService(outline.Deps{
		LLM:      &fakeLLM{responses: llmResponses},
		LLMModel: "doubao-1.5-lite-32k",
		Matcher:  safety.NewKeywordMatcher([]string{"血", "杀"}),
		PreCheck: newTestPreChecker(t),
		Cache:    outline.NewCache(rdb),
		Events:   outline.NewEventStore(db),
		Recorder: rec,
		IDHasher: idhash.New("test-secret"),
		Biz:      nil, // metric calls are nil-safe
	})
}

func cleanup(t *testing.T, outlineID string) {
	t.Helper()
	if outlineID == "" {
		return
	}
	db := integrationDB(t)
	db.Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)
	rdb := integrationRedis(t)
	rdb.Del(context.Background(), "outline:"+outlineID)
}

func TestPreview_Happy(t *testing.T) {
	resp := `{"title":"小宇的星空冒险","synopsis":"小宇遇到爱宝` + strings.Repeat("一", 60) + `","themes":["勇气"],"style":"冒险探索","educational_value":"学到勇敢"}`
	svc := newTestService(t, resp)

	res, err := svc.Preview(context.Background(), outline.PreviewInput{
		UserID: 1, ChildID: 7,
		ChildNickname: "小宇", ChildAge: 5,
		Prompt: "想听冒险故事", DurationMin: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	if !strings.HasPrefix(res.OutlineID, "ol_") {
		t.Errorf("bad outline_id: %s", res.OutlineID)
	}
	if res.Outline.Style != "冒险探索" {
		t.Errorf("style: %s", res.Outline.Style)
	}
	if res.Outline.SceneSeed == "" {
		t.Errorf("expected scene_seed injection")
	}
	if res.Outline.OutlinePromptVersion != outline.OutlinePromptVersion {
		t.Errorf("prompt_version: %s", res.Outline.OutlinePromptVersion)
	}
	if res.Outline.OutlineGroupID != res.OutlineID {
		t.Errorf("first preview: group_id should equal outline_id; got %s vs %s",
			res.Outline.OutlineGroupID, res.OutlineID)
	}
	cleanup(t, res.OutlineID)
}

func TestPreview_SchemaRepairRetry(t *testing.T) {
	bad := `{"title":"短","synopsis":"短","themes":[]}`
	good := `{"title":"小宇的太空旅行","synopsis":"小宇和爱宝出发去太空` + strings.Repeat("一", 60) + `","themes":["勇气"],"style":"冒险探索","educational_value":"学到坚持"}`
	svc := newTestService(t, bad, good)

	res, err := svc.Preview(context.Background(), outline.PreviewInput{
		UserID: 1, ChildID: 7,
		ChildNickname: "小宇", ChildAge: 5,
		Prompt: "去太空", DurationMin: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEmpty(t, res.OutlineID)
	cleanup(t, res.OutlineID)
}

func TestPreview_LLMFailedAfterRetry(t *testing.T) {
	bad1 := `{"x":"y"}`
	bad2 := `{"title":"ok","synopsis":"too short"}`
	svc := newTestService(t, bad1, bad2)

	_, err := svc.Preview(context.Background(), outline.PreviewInput{
		UserID: 1, ChildID: 7,
		ChildNickname: "小宇", ChildAge: 5,
		Prompt: "test", DurationMin: 5,
	})
	if err == nil {
		t.Fatalf("expected llm_failed error, got nil")
	}
}
