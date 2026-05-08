package safety

import "strings"

// PostCheckInput captures everything PostCheck needs.
type PostCheckInput struct {
	StoryText     string
	ChildNickname string
	ChildFearList []string
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

const minProtagonistOccurrences = 3

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
		if nickCount < minProtagonistOccurrences {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
		aibaoCount := strings.Count(in.StoryText, "爱宝")
		if aibaoCount > nickCount*2 {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
	}
	return PostCheckOutput{Pass: true}
}
