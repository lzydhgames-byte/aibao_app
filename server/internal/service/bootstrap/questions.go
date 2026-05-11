// Package bootstrap implements the "first encounter" form-driven onboarding
// that polishes parent-supplied answers into a natural-language child
// profile description used by the story system prompt.
package bootstrap

// QuestionType enumerates supported answer shapes.
type QuestionType string

const (
	// TypeText is a single free-form string answer.
	TypeText QuestionType = "text"
	// TypeSingleSelect is one option chosen from Options.
	TypeSingleSelect QuestionType = "single_select"
	// TypeMultiSelect is zero or more options chosen from Options.
	TypeMultiSelect QuestionType = "multi_select"
	// TypeBoolean is true/false.
	TypeBoolean QuestionType = "boolean"
)

// Question is one question in the BOOTSTRAP form.
type Question struct {
	ID        string       `json:"id"`
	Label     string       `json:"label"`
	Type      QuestionType `json:"type"`
	Required  bool         `json:"required"`
	Options   []string     `json:"options,omitempty"`
	MaxLength int          `json:"max_length,omitempty"`
}

// Version is bumped when the question set changes shape.
const Version = 1

// Questions returns the fixed 7-question BOOTSTRAP set.
// Keep order stable — clients render in this order.
func Questions() []Question {
	return []Question{
		{ID: "personality_traits", Label: "你觉得孩子身上有哪些性格关键词？（多选，1-3 个）", Type: TypeMultiSelect, Required: true,
			Options: []string{"勇敢", "细心", "温柔", "调皮", "好奇", "安静", "开朗", "敏感"}},
		{ID: "favorite_characters", Label: "孩子最喜欢哪些角色或动画形象？（用顿号分隔，如：奥特曼、小猪佩奇）", Type: TypeText, Required: true, MaxLength: 80},
		{ID: "fears", Label: "孩子目前比较怕什么？（1-3 项，用顿号分隔；可填'暂无'）", Type: TypeText, Required: true, MaxLength: 60},
		{ID: "family_members", Label: "故事里可以提及哪些家人？（如：爸爸、妈妈、奶奶、弟弟）", Type: TypeText, Required: false, MaxLength: 60},
		{ID: "story_style", Label: "孩子最喜欢哪种故事风格？", Type: TypeSingleSelect, Required: true,
			Options: []string{"温馨治愈", "冒险探索", "搞笑欢乐", "神奇魔法", "科普认知"}},
		{ID: "education_themes", Label: "你希望故事里多一点哪些主题？（多选）", Type: TypeMultiSelect, Required: false,
			Options: []string{"勇敢", "友谊", "诚实", "分享", "坚持", "好奇心", "情绪管理"}},
		{ID: "enable_storyline", Label: "是否开启'连续剧'模式（同一系列故事会延续角色和情节）？", Type: TypeBoolean, Required: true},
	}
}

// QuestionByID looks up a question definition; ok=false if not found.
func QuestionByID(id string) (Question, bool) {
	for _, q := range Questions() {
		if q.ID == id {
			return q, true
		}
	}
	return Question{}, false
}
