package outline

import (
	"fmt"
	"strings"
)

// OutlinePromptVersion bumps on every prompt template change.
// Tracked in Redis payload + outline_events + cost_events for A/B / regression analysis.
// Spec §5.4.
const OutlinePromptVersion = "v20260525-1"

// OutlinePromptInput is the input for BuildPrompt.
type OutlinePromptInput struct {
	ChildNickname string
	ChildAge      int
	UserPrompt    string
	DurationMin   int
}

// BuildPrompt assembles system + user messages for outline LLM call.
// Returns (systemMsg, userMsg).
func BuildPrompt(in OutlinePromptInput) (string, string) {
	sys := strings.TrimSpace(`
你是儿童故事策划师。家长会用一句话告诉你孩子今晚想听什么样的故事，
你需要返回一个**结构化大纲 JSON**，让家长在生成正文前确认。

要求：
1. 必须返回合法 JSON，包含字段：title / synopsis / themes / style / educational_value
2. title：故事标题，5-12 个汉字
3. synopsis：3-5 句梗概，80-120 汉字，必须以孩子（昵称见用户消息）为主角
4. themes：1-3 个教育主题，从中国传统教育价值观词库选（如"勇气"/"友谊"/"诚实"）
5. style：必须从下列 5 选 1：温馨治愈 / 冒险探索 / 搞笑欢乐 / 神奇魔法 / 科普认知
6. educational_value：1 句话说明孩子能学到什么，自然口语
7. 不要包含任何成人内容、暴力、恐怖、政治词汇
8. 不要让孩子之外的角色（如奥特曼）抢主角位置——孩子永远是主角，IP 是陪伴
`)
	user := fmt.Sprintf(strings.TrimSpace(`
孩子昵称：%s
孩子年龄：%d 岁
本次需求：%s
时长档位：%d 分钟（仅作时长参考，无需写入 JSON 的 duration_min 字段）

请返回大纲 JSON。
`), in.ChildNickname, in.ChildAge, in.UserPrompt, in.DurationMin)
	return sys, user
}
