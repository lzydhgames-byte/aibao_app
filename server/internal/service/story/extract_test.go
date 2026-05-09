package story

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtract_FromIPInstructions(t *testing.T) {
	story := "小宇推开门，爱宝奥特曼一起走进竹林。"
	ipNames := []string{"奥特曼"}
	got := ExtractElements(story, ipNames)
	// At least 'character' for "爱宝奥特曼"
	hasCharacter := false
	for _, e := range got {
		if e.ElementType == "character" && e.Name == "爱宝奥特曼" {
			hasCharacter = true
		}
	}
	assert.True(t, hasCharacter, "expected character 爱宝奥特曼, got %+v", got)
}

func TestExtract_KnownPlaces(t *testing.T) {
	story := "小宇走进了星星城堡，后来又去了花园和海底。"
	got := ExtractElements(story, nil)
	names := elementNames(got, "place")
	assert.Contains(t, names, "城堡")
}

func TestExtract_DedupesElements(t *testing.T) {
	story := "城堡里的城堡，进了城堡又出城堡。"
	got := ExtractElements(story, nil)
	names := elementNames(got, "place")
	count := 0
	for _, n := range names {
		if n == "城堡" {
			count++
		}
	}
	assert.Equal(t, 1, count, "expected 城堡 once, got %d", count)
}

func TestExtract_EmptyStory(t *testing.T) {
	got := ExtractElements("", nil)
	assert.Empty(t, got)
}

func elementNames(elems []*ExtractedElement, kind string) []string {
	out := []string{}
	for _, e := range elems {
		if e.ElementType == kind {
			out = append(out, e.Name)
		}
	}
	return out
}
