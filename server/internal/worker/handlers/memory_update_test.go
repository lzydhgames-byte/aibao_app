package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

type fakeMem struct {
	created []*model.Memory
}

func (f *fakeMem) Create(_ context.Context, m *model.Memory) error {
	f.created = append(f.created, m)
	return nil
}
func (f *fakeMem) RecentByChild(_ context.Context, _ int64, _ string, _ int) ([]*model.Memory, error) {
	return f.created, nil
}
func (f *fakeMem) RecentByChildTypes(_ context.Context, _ int64, _ []string, _ int) ([]*model.Memory, error) {
	return f.created, nil
}

func TestMemoryUpdate_HandleHappy(t *testing.T) {
	mem := &fakeMem{}
	h := NewMemoryUpdateHandler(mem)
	payload, _ := json.Marshal(map[string]any{
		"story_id": 100, "child_id": 7, "title": "T", "summary": "S", "used_fallback": false,
	})
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: payload}))
	require.Len(t, mem.created, 1)
	assert.Equal(t, int64(7), mem.created[0].ChildID)
	assert.Equal(t, "story_summary", mem.created[0].MemoryType)
}

func TestMemoryUpdate_BadPayload(t *testing.T) {
	mem := &fakeMem{}
	h := NewMemoryUpdateHandler(mem)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: []byte("not json")})
	assert.Error(t, err)
}
