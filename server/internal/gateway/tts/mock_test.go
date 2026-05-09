package tts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMock_HappyPath(t *testing.T) {
	var _ Client = NewMock()
	m := NewMock()
	resp, err := m.Synthesize(context.Background(), SynthesizeRequest{
		Text: "小宇在森林里冒险", Format: "mp3",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Audio)
	assert.Equal(t, "mp3", resp.Format)
	assert.Equal(t, 1, m.Calls())
}

func TestMock_FailNext(t *testing.T) {
	m := NewMock()
	m.FailNext()
	_, err := m.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	_, err = m.Synthesize(context.Background(), SynthesizeRequest{Text: "y"})
	require.NoError(t, err)
}

func TestMock_EmptyText(t *testing.T) {
	m := NewMock()
	_, err := m.Synthesize(context.Background(), SynthesizeRequest{Text: ""})
	require.Error(t, err)
}

func TestMock_HealthCheck(t *testing.T) {
	require.NoError(t, NewMock().HealthCheck(context.Background()))
}
