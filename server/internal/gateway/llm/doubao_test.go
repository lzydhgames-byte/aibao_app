package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDoubao_RejectsEmptyAPIKey(t *testing.T) {
	_, err := NewDoubao(DoubaoConfig{APIKey: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api key required")
}

func TestNewDoubao_DefaultsTimeout(t *testing.T) {
	c, err := NewDoubao(DoubaoConfig{APIKey: "fake-key"})
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNewDoubao_AcceptsCustomBaseURL(t *testing.T) {
	c, err := NewDoubao(DoubaoConfig{APIKey: "fake-key", BaseURL: "https://custom.example.com"})
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNewDoubao_ImplementsClient(t *testing.T) {
	c, err := NewDoubao(DoubaoConfig{APIKey: "fake-key"})
	require.NoError(t, err)
	var _ Client = c
}
