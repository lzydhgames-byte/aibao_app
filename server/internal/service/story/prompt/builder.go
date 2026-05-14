package prompt

import (
	"bytes"
	"fmt"
	"math/rand"
	"text/template"
	"time"
)

// sceneSeeds is the rotating pool of opening-scene hints injected into the
// prompt. Each generation picks one at random — this is the cheapest fix
// for the homogenization problem (every LLM output starts at "夜晚/床边/月光").
//
// Keep entries short and concrete (a place + a mood/time) so the LLM has
// enough handhold to vary the opening without sacrificing coherence with
// the user's prompt or storyline continuity.
var sceneSeeds = []string{
	"清晨的厨房里飘着早餐的香味",
	"周末傍晚的小公园，秋千还在轻轻晃",
	"放学路上突然下起的小雨",
	"周末的阳台上，云朵奇形怪状",
	"夏天的海边沙滩，浪花轻轻拍过来",
	"冬天的雪后，路上一片白茫茫",
	"幼儿园午睡刚醒，阳光从窗帘缝里漏进来",
	"超市的零食货架前",
	"森林边缘的一棵老橡树下",
	"星空下的草原，萤火虫飞来飞去",
	"奶奶家的院子，葡萄藤上挂满了紫色珍珠",
	"动物园里大熊猫吃竹子的展区",
	"图书馆安静的角落，一本翻开的书",
	"游乐场最高的旋转木马刚停下",
	"阴雨天的窗台上，水珠顺着玻璃往下滑",
	"清晨的小溪边，露珠还挂在草尖",
	"放学后的操场，夕阳把影子拉得好长",
	"博物馆里恐龙骨架的大厅",
	"火车窗外飞速掠过的田野",
	"过年的厨房，到处都是包饺子的笑声",
}

var sceneRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

func pickSceneSeed() string {
	return sceneSeeds[sceneRNG.Intn(len(sceneSeeds))]
}

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
	SceneSeed                string
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
		SceneSeed:                pickSceneSeed(),
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
