package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aibao/server/internal/model"
)

type fakeMemoryRepo struct {
	rows []*model.Memory
	err  error
}

func (f *fakeMemoryRepo) RecentByChildTypes(_ context.Context, _ int64, _ []string, _ int) ([]*model.Memory, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func TestSelector_JoinsSummariesInOrder(t *testing.T) {
	repo := &fakeMemoryRepo{rows: []*model.Memory{
		{Payload: []byte(`{"summary":"A"}`)},
		{Payload: []byte(`{"summary":"B"}`)},
		{Payload: []byte(`{"summary":"C"}`)},
	}}
	s := NewSelector(repo, 3, quietLogger())
	got := s.BuildContext(context.Background(), 1)
	assert.Equal(t, "A；B；C", got)
}

func TestSelector_SkipsBadJSON(t *testing.T) {
	repo := &fakeMemoryRepo{rows: []*model.Memory{
		{Payload: []byte(`{"summary":"A"}`)},
		{Payload: []byte(`not json`)},
		{Payload: []byte(`{"summary":"C"}`)},
	}}
	s := NewSelector(repo, 3, quietLogger())
	got := s.BuildContext(context.Background(), 1)
	assert.Equal(t, "A；C", got)
}

func TestSelector_RepoErrorReturnsEmpty(t *testing.T) {
	repo := &fakeMemoryRepo{err: errors.New("db down")}
	s := NewSelector(repo, 3, quietLogger())
	assert.NotPanics(t, func() {
		got := s.BuildContext(context.Background(), 1)
		assert.Equal(t, "", got)
	})
}

func TestSelector_EmptyResult(t *testing.T) {
	repo := &fakeMemoryRepo{rows: nil}
	s := NewSelector(repo, 3, quietLogger())
	assert.Equal(t, "", s.BuildContext(context.Background(), 1))
}
