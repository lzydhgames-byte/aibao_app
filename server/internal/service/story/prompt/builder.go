package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// BuildInput is the structured input for assembling a system prompt.
type BuildInput struct {
	ChildNickname            string
	ChildAgeYears            int
	ChildGender              string // "boy" / "girl" / "unspecified"
	ChildFearList            []string
	Duration                 int    // minutes: 5/10/15
	Style                    string // "温馨治愈" / "冒险探索" / ...
	Topic                    string // educational topic (may be empty)
	UserPromptCleaned        string // PreCheck-cleaned user prompt
	NormalizedIPs            []string
	NormalizedIPInstructions string // joined whitelist instructions
	MemorySummary            string // recent story elements (Plan 6)
	StorylineHook            string   // Plan 8: previous episode's next-episode hint
	StorylineRecentSummaries []string // Plan 8: up to 3 previous episode summaries, newest first
	EpisodeNumber            int      // Plan 8: the upcoming episode number (>=2 for sequels)
	PromptVersion            string   // e.g. "v1"
}

// BuildOutput is the assembled prompt.
type BuildOutput struct {
	SystemPrompt string
	UserPrompt   string
}

// Builder renders the system prompt template.
type Builder struct {
	tmpl *template.Template
}

// NewBuilder loads the system_prompt template from disk and returns a Builder.
func NewBuilder(templatePath string) (*Builder, error) {
	t, err := loadTemplate(templatePath)
	if err != nil {
		return nil, err
	}
	return &Builder{tmpl: t}, nil
}

// templateVars is the data passed into the template.
type templateVars struct {
	ChildNickname            string
	ChildAgeYears            int
	ChildGenderText          string
	FearListText             string
	Duration                 int
	ExpectedRunes            int
	ExpectedRunesMin         int
	ExpectedRunesMax         int
	Style                    string
	TopicText                string
	NormalizedIPInstructions string
	MemorySummary            string
	StorylineHook            string
	StorylineRecentSummaries []string
	EpisodeNumber            int
	PromptVersion            string
}

// Build renders the system prompt and returns it together with the cleaned user prompt.
func (b *Builder) Build(in BuildInput) BuildOutput {
	rmin, rmax := expectedRuneBand(in.Duration)
	vars := templateVars{
		ChildNickname:            in.ChildNickname,
		ChildAgeYears:            in.ChildAgeYears,
		ChildGenderText:          genderText(in.ChildGender),
		FearListText:             fearListText(in.ChildFearList),
		Duration:                 in.Duration,
		ExpectedRunes:            expectedRunesForDuration(in.Duration),
		ExpectedRunesMin:         rmin,
		ExpectedRunesMax:         rmax,
		Style:                    in.Style,
		TopicText:                topicText(in.Topic),
		NormalizedIPInstructions: in.NormalizedIPInstructions,
		MemorySummary:            in.MemorySummary,
		StorylineHook:            in.StorylineHook,
		StorylineRecentSummaries: in.StorylineRecentSummaries,
		EpisodeNumber:            in.EpisodeNumber,
		PromptVersion:            in.PromptVersion,
	}
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, vars); err != nil {
		// Template was already validated at NewBuilder. If this fails it's a
		// programmer bug — surface it loudly.
		panic(fmt.Sprintf("system_prompt template execution failed: %v", err))
	}
	return BuildOutput{
		SystemPrompt: buf.String(),
		UserPrompt:   in.UserPromptCleaned,
	}
}
