package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	childsvc "github.com/aibao/server/internal/service/child"
)

type fakeChildLookup struct {
	child *model.Child
	err   error
}

func (f *fakeChildLookup) GetByID(_ context.Context, _, _ int64) (*model.Child, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.child, nil
}

func bootstrapCounterVal(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

type fakeChildUpdater struct {
	calls     int
	lastInput childsvc.UpdateInput
	err       error
}

func (f *fakeChildUpdater) Update(_ context.Context, _, _ int64, in childsvc.UpdateInput) (any, error) {
	f.calls++
	f.lastInput = in
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func nullLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestSvc(updater ChildUpdater, mock *llm.MockClient) (*Service, *metrics.Business) {
	biz := metrics.NewBusiness(prometheus.NewRegistry())
	svc := NewServiceWithUpdater(updater, mock, "test-model", 0.8, biz, nullLogger())
	return svc, biz
}

func goodAnswers() []Answer {
	return []Answer{
		{QID: "personality_traits", Value: []interface{}{"勇敢", "好奇"}},
		{QID: "favorite_characters", Value: "奥特曼、小猪佩奇"},
		{QID: "fears", Value: "黑暗"},
		{QID: "family_members", Value: "妈妈、爸爸"},
		{QID: "story_style", Value: "温馨治愈"},
		{QID: "education_themes", Value: []interface{}{"勇敢", "友谊"}},
		{QID: "enable_storyline", Value: true},
	}
}

func TestSubmit_HappyPath(t *testing.T) {
	updater := &fakeChildUpdater{}
	mock := llm.NewMock()
	mock.Response = &llm.GenerateResponse{Text: "  这是一个勇敢好奇的孩子...  "}
	svc, _ := newTestSvc(updater, mock)

	p, err := svc.Submit(context.Background(), 1, 10, goodAnswers())
	require.NoError(t, err)
	assert.Equal(t, "这是一个勇敢好奇的孩子...", p.Description)
	assert.Equal(t, 1, updater.calls)
	require.NotNil(t, updater.lastInput.Profile)
	assert.Contains(t, string(*updater.lastInput.Profile), "勇敢")
}

func TestSubmit_MissingRequired(t *testing.T) {
	updater := &fakeChildUpdater{}
	svc, _ := newTestSvc(updater, llm.NewMock())

	// Drop favorite_characters (required).
	ans := goodAnswers()
	ans = append(ans[:1], ans[2:]...)

	_, err := svc.Submit(context.Background(), 1, 10, ans)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
	assert.Equal(t, "missing_required", ae.Reason)
	assert.Equal(t, 0, updater.calls)
}

func TestSubmit_InvalidSingleSelect(t *testing.T) {
	updater := &fakeChildUpdater{}
	svc, _ := newTestSvc(updater, llm.NewMock())
	ans := goodAnswers()
	for i, a := range ans {
		if a.QID == "story_style" {
			ans[i].Value = "无厘头"
		}
	}
	_, err := svc.Submit(context.Background(), 1, 10, ans)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_option", ae.Reason)
}

func TestSubmit_InvalidMultiSelect(t *testing.T) {
	updater := &fakeChildUpdater{}
	svc, _ := newTestSvc(updater, llm.NewMock())
	ans := goodAnswers()
	for i, a := range ans {
		if a.QID == "personality_traits" {
			ans[i].Value = []interface{}{"勇敢", "不存在"}
		}
	}
	_, err := svc.Submit(context.Background(), 1, 10, ans)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_option", ae.Reason)
}

func TestSubmit_TextTooLong(t *testing.T) {
	updater := &fakeChildUpdater{}
	svc, _ := newTestSvc(updater, llm.NewMock())
	ans := goodAnswers()
	long := ""
	for i := 0; i < 200; i++ {
		long += "字"
	}
	for i, a := range ans {
		if a.QID == "favorite_characters" {
			ans[i].Value = long
		}
	}
	_, err := svc.Submit(context.Background(), 1, 10, ans)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "text_too_long", ae.Reason)
}

func TestSubmit_LLMError_FailOpen(t *testing.T) {
	updater := &fakeChildUpdater{}
	mock := llm.NewMock()
	mock.Err = errors.New("upstream down")
	svc, _ := newTestSvc(updater, mock)

	p, err := svc.Submit(context.Background(), 1, 10, goodAnswers())
	require.NoError(t, err)
	assert.Equal(t, "", p.Description)
	assert.Equal(t, 1, updater.calls, "answers persisted even when LLM fails")
}

func TestSubmit_ThreadsNicknameIntoLLMPrompt(t *testing.T) {
	updater := &fakeChildUpdater{}
	mock := llm.NewMock()
	mock.Response = &llm.GenerateResponse{Text: "描述"}
	biz := metrics.NewBusiness(prometheus.NewRegistry())
	lookup := &fakeChildLookup{child: &model.Child{Nickname: "小宇"}}
	svc := NewServiceWithLookup(updater, lookup, mock, "test-model", 0.8, biz, nullLogger())

	_, err := svc.Submit(context.Background(), 1, 10, goodAnswers())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(mock.LastRequest.Messages), 2)
	userMsg := mock.LastRequest.Messages[len(mock.LastRequest.Messages)-1].Content
	assert.Contains(t, userMsg, "小宇")
	assert.Contains(t, userMsg, "孩子昵称")
}

func TestSubmit_LLMError_IncrementsFailFallbackCounter(t *testing.T) {
	updater := &fakeChildUpdater{}
	mock := llm.NewMock()
	mock.Err = errors.New("upstream down")
	biz := metrics.NewBusiness(prometheus.NewRegistry())
	svc := NewServiceWithUpdater(updater, mock, "test-model", 0.8, biz, nullLogger())

	_, err := svc.Submit(context.Background(), 1, 10, goodAnswers())
	require.NoError(t, err)
	assert.Equal(t, float64(1), bootstrapCounterVal(t, biz.LLMFailFallbackTotal.WithLabelValues("doubao", "test-model", "upstream_error")))
}

func TestSubmit_PropagatesNotOwner(t *testing.T) {
	updater := &fakeChildUpdater{
		err: apperr.New(apperr.CodePermissionDenied, "not_owner", "无权"),
	}
	svc, _ := newTestSvc(updater, llm.NewMock())
	_, err := svc.Submit(context.Background(), 1, 10, goodAnswers())
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodePermissionDenied, ae.Code)
	assert.Equal(t, "not_owner", ae.Reason)
}
