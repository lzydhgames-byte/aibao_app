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
	// 时间 / 一天的不同片段
	"清晨的厨房里飘着早餐的香味",
	"幼儿园午睡刚醒，阳光从窗帘缝里漏进来",
	"放学路上突然下起的小雨",
	"放学后的操场，夕阳把影子拉得好长",
	"周末傍晚的小公园，秋千还在轻轻晃",
	"晚饭后阳台上，路灯刚刚亮起来",
	"清晨的小溪边，露珠还挂在草尖",
	"周日早上的早市，热气腾腾的包子摊前",

	// 天气
	"阴雨天的窗台上，水珠顺着玻璃往下滑",
	"刚下完雨的彩虹挂在远处的山顶",
	"夏天午后突然的雷阵雨刚停",
	"冬天的雪后，路上一片白茫茫",
	"大风天，落叶在脚边打着旋",
	"雾蒙蒙的早晨，对面楼都看不清",
	"晴天的草坪上，云的影子慢慢飘过",
	"傍晚的霞光把整面墙染成了橘红色",

	// 场所 / 日常
	"超市的零食货架前",
	"图书馆安静的角落，一本翻开的书",
	"理发店里转着花纹的红蓝白柱子",
	"医院走廊上一只迷路的小蜗牛",
	"小区门口的快递柜前堆着一摞盒子",
	"地铁站里电梯慢慢往上升",
	"周末的菜市场，活蹦乱跳的鱼缸边",
	"楼下小卖部门口的老花猫",

	// 场所 / 玩乐
	"游乐场最高的旋转木马刚停下",
	"动物园里大熊猫吃竹子的展区",
	"博物馆里恐龙骨架的大厅",
	"水族馆隧道里，鲨鱼从头顶游过",
	"科技馆按下按钮就升起的小火箭",
	"室内游泳池边，水汽蒙住了眼镜",
	"周末画画课的画板前",
	"轮滑场上刚学会站稳的小朋友",

	// 季节 / 节日
	"过年的厨房，到处都是包饺子的笑声",
	"中秋夜的院子里，月饼摆成一个圆圈",
	"端午包粽子，棕叶香味飘满屋",
	"元宵灯会，一只大兔子灯在飘",
	"儿童节早上，桌上多了一个神秘礼盒",
	"国庆游行队伍走过家门口",
	"圣诞节早上，袜子里鼓鼓囊囊",
	"开学第一天，新书包还硬邦邦的",

	// 自然 / 户外
	"夏天的海边沙滩，浪花轻轻拍过来",
	"森林边缘的一棵老橡树下",
	"星空下的草原，萤火虫飞来飞去",
	"山顶看日出，云海在脚下翻滚",
	"溪边搬开石头，下面藏着小螃蟹",
	"果园里苹果挂得满枝头",
	"麦田里风一吹，金色的波浪",
	"沙漠的傍晚，骆驼的影子拉得好长",

	// 交通工具
	"火车窗外飞速掠过的田野",
	"高速服务区里第一次看到的奶茶机",
	"飞机起飞那一刻，云朵就在窗外",
	"轮船甲板上，海鸥跟着船飞",
	"出租车里司机叔叔在哼着歌",
	"自行车后座，妈妈骑得风一样快",
	"夜班公交上一个空荡荡的车厢",
	"游乐场小火车钻进山洞那一刻",

	// 家里 / 亲人
	"奶奶家的院子，葡萄藤上挂满了紫色珍珠",
	"姥姥的针线筐里翻出一颗旧扣子",
	"爸爸的工具箱里第一次看到的锤子",
	"妈妈晾衣绳上飘着各种颜色的袜子",
	"爷爷的茶杯上一圈圈白色水汽",
	"哥哥的玩具柜最上层那个神秘箱子",
	"姐姐的化妆镜前一支没盖盖的口红",
	"全家围坐在火锅边，蒸汽升起来",

	// 想象 / 奇遇
	"放在桌上的玩具突然眨了一下眼睛",
	"墙上的画里跑出来一只小动物",
	"梦里推开一扇会发光的门",
	"打开抽屉，里面居然是另一个世界",
	"床底下传来轻轻的脚步声",
	"窗外飘来一只迷路的纸飞机",
	"信箱里塞着一封写给爱宝的信",
	"楼梯转角突然出现的小楼梯",

	// 学习 / 兴趣
	"钢琴课刚结束，琴键上还留着体温",
	"足球训练第一次踢中球门那一下",
	"舞蹈房整面墙的镜子前",
	"积木搭到最高那一刻摇摇欲坠",
	"第一次自己刷牙，泡沫蹦到镜子上",
	"科学小实验，瓶子里的气球鼓起来",
	"画完最后一笔，画纸上是一片星空",
	"读完一本绘本的最后一页",
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
	seed := pickSceneSeed()
	vars := templateVars{
		ChildNickname:            in.ChildNickname,
		ChildAgeYears:            in.ChildAgeYears,
		ChildGenderText:          genderText(in.ChildGender),
		FearListText:             fearListText(in.ChildFearList),
		Duration:                 in.Duration,
		ExpectedRunes:            expectedRunesForDuration(in.Duration),
		ExpectedRunesMin:         rmin,
		ExpectedRunesMax:         rmax,
		SceneSeed:                seed,
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

	// Even when the user's prompt is identical (e.g. "迪士尼乐园的一天"
	// asked twice), we want a fresh story. Two mechanisms layered onto the
	// user-role message:
	//   1) SceneSeed echoed in user role (LLM weighs user > system for
	//      novelty) — drives plot variation.
	//   2) Variety-mandate sentence — explicit "even if you've answered
	//      this before, write a different plot" steer.
	//   3) Per-request nonce hex — pure cache-buster on the Doubao side;
	//      LLM treats it as background noise but the bytes are different
	//      every call so prompt-prefix cache cannot match.
	userPrompt := in.UserPromptCleaned +
		"\n\n[本次创作随机灵感] " + seed +
		"\n[多样性要求] 即使主题或需求和之前讲过的故事相同，也请写一个完全不同的情节、不同的转折、不同的角色配置。" +
		"\n[本次会话 ID] " + randomNonceHex()
	return BuildOutput{
		SystemPrompt: buf.String(),
		UserPrompt:   userPrompt,
	}
}

// randomNonceHex returns a fresh 16-hex-char string for every call. Used
// solely as a prompt-cache-buster so two identical user prompts get two
// different LLM completions.
func randomNonceHex() string {
	const hex = "0123456789abcdef"
	b := make([]byte, 16)
	for i := range b {
		b[i] = hex[sceneRNG.Intn(16)]
	}
	return string(b)
}
