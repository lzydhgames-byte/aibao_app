package bootstrap

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuestions_Count(t *testing.T) {
	qs := Questions()
	assert.GreaterOrEqual(t, len(qs), 6)
}

func TestQuestions_FieldsNonEmpty(t *testing.T) {
	qs := Questions()
	ids := map[string]bool{}
	requiredCount := 0
	for _, q := range qs {
		assert.NotEmpty(t, q.ID, "id empty")
		assert.NotEmpty(t, q.Label, "label empty for "+q.ID)
		assert.NotEmpty(t, q.Type, "type empty for "+q.ID)
		assert.False(t, ids[q.ID], "duplicate id: "+q.ID)
		ids[q.ID] = true
		if q.Type == TypeSingleSelect || q.Type == TypeMultiSelect {
			assert.NotEmpty(t, q.Options, "options empty for "+q.ID)
		}
		if q.Required {
			requiredCount++
		}
	}
	assert.GreaterOrEqual(t, requiredCount, 4)
}

func TestQuestionByID(t *testing.T) {
	q, ok := QuestionByID("personality_traits")
	require.True(t, ok)
	assert.Equal(t, TypeMultiSelect, q.Type)

	q2, ok := QuestionByID("favorite_characters")
	require.True(t, ok)
	assert.Equal(t, TypeText, q2.Type)

	_, ok = QuestionByID("unknown")
	assert.False(t, ok)
}

func TestQuestions_JSONRoundTrip(t *testing.T) {
	qs := Questions()
	b, err := json.Marshal(qs)
	require.NoError(t, err)
	var back []Question
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, len(qs), len(back))
	assert.Equal(t, qs[0].ID, back[0].ID)
}
