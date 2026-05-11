package safety

import "strings"

// PostCheckInput captures everything PostCheck needs.
type PostCheckInput struct {
	StoryText     string
	ChildNickname string
	ChildFearList []string
	Duration      int
}

// PostCheckOutput is the verdict.
type PostCheckOutput struct {
	Pass         bool
	RejectReason string
	MatchedRule  string
}

// PostChecker validates LLM output before returning it to the caller.
type PostChecker struct {
	rs       *RuleSet
	redlineM *KeywordMatcher
}

// NewPostChecker constructs a PostChecker bound to a RuleSet.
func NewPostChecker(rs *RuleSet) *PostChecker {
	return &PostChecker{
		rs:       rs,
		redlineM: NewKeywordMatcher(rs.AllRedlinesFlat),
	}
}

func minProtagonistFor(duration int) int {
	switch duration {
	case 5:
		return 2
	case 10:
		return 3
	case 15:
		return 4
	default:
		return 3
	}
}

// Check runs the full post-check pipeline.
func (p *PostChecker) Check(in PostCheckInput) PostCheckOutput {
	if hit, ok := p.redlineM.FindFirst(in.StoryText); ok {
		return PostCheckOutput{Pass: false, RejectReason: "redline_matched", MatchedRule: hit}
	}
	if len(in.ChildFearList) > 0 {
		fearM := NewKeywordMatcher(in.ChildFearList)
		if hit, ok := fearM.FindFirst(in.StoryText); ok {
			return PostCheckOutput{Pass: false, RejectReason: "fear_matched", MatchedRule: hit}
		}
	}
	if in.ChildNickname != "" {
		nickCount := strings.Count(in.StoryText, in.ChildNickname)
		if nickCount < minProtagonistFor(in.Duration) {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
		aibaoCount := strings.Count(in.StoryText, "爱宝")
		if aibaoCount > nickCount*2 {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
	}
	return PostCheckOutput{Pass: true}
}
