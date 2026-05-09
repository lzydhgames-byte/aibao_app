package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMock_DefaultGenerate(t *testing.T) {
	m := NewMock()
	out, err := m.Generate(context.Background(), GenerateRequest{Model: "x"})
	require.NoError(t, err)
	assert.Contains(t, out.Text, "Mock")
	assert.Equal(t, 1, m.Calls)
}

func TestMock_ConfiguredError(t *testing.T) {
	m := NewMock()
	m.Err = errors.New("boom")
	_, err := m.Generate(context.Background(), GenerateRequest{})
	require.Error(t, err)
}

func TestMock_HealthCheckOK(t *testing.T) {
	require.NoError(t, NewMock().HealthCheck(context.Background()))
}

func TestMock_ImplementsClient(t *testing.T) {
	var c Client = NewMock()
	assert.NotNil(t, c)
}
