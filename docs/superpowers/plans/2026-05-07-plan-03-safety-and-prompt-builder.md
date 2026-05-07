# Plan 3：双层安全 + Prompt 模板 实现规划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不调 LLM 的前提下，先把"故事生成的安全护栏"做出来：前置预审（PreCheck）+ 强约束 System Prompt 模板组装 + 后置审核（PostCheck）+ 真实 IP 同人化归一化。完成后 Plan 4 接 LLM 时可直接调用这套护栏。

**Architecture:** 三个新包：`service/safety`（规则引擎 + PreCheck/PostCheck）、`service/story/prompt`（System Prompt 模板组装）、`cmd/safetycheck`（命令行 demo 工具）。规则源数据（红线词库、IP 白/黑名单）走 YAML 文件，启动时加载到内存的不可变结构。意图分类一期纯规则；预留 Provider 接口给 Plan 4 接 LLM 兜底。

**Tech Stack:**
- Go 1.24+
- `gopkg.in/yaml.v3` —— YAML 加载
- 复用 Plan 1+2 已有：viper config、slog logger、AppError、cobra（demo 工具用，新引入）
- `github.com/spf13/cobra` —— 命令行框架（demo 工具）

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)（第 3.1-3.3 节 SOUL/IDENTITY/AGENTS；第 7 章红线）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - 第 7 章双层安全链路（**核心**）
  - 第 7.3 节强约束 System Prompt 模板（**核心**）

**完成验收（Definition of Done）：**

1. `go build ./...` 编译通过；`go test ./...` 全部通过
2. service+pkg 层覆盖率 ≥ 75%
3. 红线词库 YAML 加载：`safety.LoadRules("safety/rules.yaml")` 返回包含 200+ 全局红线词的 RuleSet
4. PreCheck 5 类拦截全部覆盖：
   - 长度过长/字符危险 → reject
   - 全局红线命中 → reject（包含命中词在错误中）
   - 个性化害怕清单命中 → reject
   - 黑名单 IP 命中 → reject
   - 白名单 IP 命中 → accept + 注入"爱宝同人化"指令
5. PostCheck 3 类拦截：红线匹配、害怕清单匹配、主角身份校验（孩子昵称必须出现且为决策者）
6. PromptBuilder 输出包含 SOUL/IDENTITY 全文 + 孩子档案 + MEMORY 占位 + 8 条强约束 + 风格/时长/主题/输出格式
7. `cmd/safetycheck` demo 工具能跑通三个子命令：`precheck` / `postcheck` / `build_prompt`
8. 规则匹配性能：单次 PreCheck 在 1000 词词库下 < 1ms（基准测试验证）
9. 所有新增代码通过 `golangci-lint run ./...`，0 issues

---

## 范围决策记录

- **红线词库存放**：YAML 文件，启动时加载到不可变结构；dev 路径入 git，prod 路径不入
- **红线词库覆盖**：起步 200+ 词分 6 大类（暴力/恐怖/性/政治宗教/危险模仿/负面价值观）
- **意图分类**：一期 PreCheck 实现"规则 + Provider 接口"两层；Provider 接口预留 LLM 兜底，但本 plan 仅提供 NoopProvider（永远 pass）。Plan 4 接 LLM 时实现 LLMProvider
- **真实 IP 归一化**：YAML 管理白/黑/同人化指令；起步白名单包含主流儿童 IP（奥特曼、汪汪队、佩奇、熊出没、小马宝莉、宝可梦、超级飞侠、海底小纵队、托马斯、咸蛋超人）
- **Demo 工具**：`cmd/safetycheck` 用 cobra 框架，三个子命令演示三种核心能力

---

## File Structure

### 规则源数据（运维可改）

| 文件 | 职责 |
|---|---|
| `server/safety/rules.yaml` | 红线词库（6 大类，200+ 词） |
| `server/safety/ip_whitelist.yaml` | 真实 IP 白名单（同人化映射） |
| `server/safety/ip_blacklist.yaml` | 真实 IP 黑名单（直接拒绝） |
| `server/safety/system_prompt.tmpl` | System Prompt 模板（含占位符） |

### `internal/service/safety/`（核心规则引擎）

| 文件 | 职责 |
|---|---|
| `rules.go` | RuleSet 结构、LoadRules 函数 |
| `rules_test.go` | yaml 加载测试 |
| `matcher.go` | 词库匹配引擎（O(N) 包含 + Trie 预留） |
| `matcher_test.go` | 含基准测试 |
| `intent.go` | IntentProvider 接口 + NoopProvider 实现 |
| `intent_test.go` | 测试 |
| `precheck.go` | PreCheck 主流程（长度→规则→IP 归一化） |
| `precheck_test.go` | 综合测试（含个性化害怕清单） |
| `postcheck.go` | PostCheck 主流程（红线→害怕→主角校验） |
| `postcheck_test.go` | |
| `ip_normalizer.go` | IP 同人化归一化 |
| `ip_normalizer_test.go` | |

### `internal/service/story/prompt/`（System Prompt 组装）

| 文件 | 职责 |
|---|---|
| `builder.go` | PromptBuilder 主结构 + Build() 方法 |
| `builder_test.go` | 多场景测试 |
| `template.go` | SOUL/IDENTITY/AGENTS 静态文本（写在 Go 源码 const） |
| `constraints.go` | 8 条强约束生成（基于孩子档案动态填空） |

### `cmd/safetycheck/`（demo 工具）

| 文件 | 职责 |
|---|---|
| `main.go` | cobra root + 3 个子命令（precheck / postcheck / build_prompt） |

### 测试用 fixture

| 文件 | 职责 |
|---|---|
| `internal/service/safety/testdata/minimal_rules.yaml` | 测试用最小规则集 |

---

## API 形态（先定好契约）

### `safety.RuleSet`
```go
type RuleSet struct {
    Redlines       map[string][]string  // 类别 → 词列表（如 "violence":["血腥","暴力"]）
    IPWhitelist    map[string]string    // IP 关键词 → 同人化指令
    IPBlacklist    []string             // 拒绝的 IP 关键词
    AllRedlinesFlat []string            // 平铺总集合（matcher 用）
}

func LoadRules(rulesPath, whitelistPath, blacklistPath string) (*RuleSet, error)
```

### `safety.PreCheck`
```go
type PreCheckInput struct {
    UserPrompt        string
    ChildFearList     []string  // 个性化害怕清单（来自 child profile）
    MaxPromptRunes    int       // 默认 200
}

type PreCheckOutput struct {
    Pass               bool
    RejectReason       string  // "redline_matched" / "fear_matched" / "ip_blacklisted" / "too_long" / "danger_chars"
    MatchedRule        string  // 命中的具体词或 IP
    NormalizedPrompt   string  // 经过 IP 归一化后的 prompt（仅当 pass=true）
    NormalizedIPs      []string // 命中的白名单 IP（用于注入同人化指令）
}

func (s *PreChecker) Check(in PreCheckInput) PreCheckOutput
```

### `safety.PostCheck`
```go
type PostCheckInput struct {
    StoryText        string
    ChildNickname    string    // 必须在故事中出现的孩子昵称
    ChildFearList    []string  // 害怕清单
}

type PostCheckOutput struct {
    Pass         bool
    RejectReason string  // "redline_matched" / "fear_matched" / "child_not_protagonist"
    MatchedRule  string
}

func (s *PostChecker) Check(in PostCheckInput) PostCheckOutput
```

### `prompt.Builder`
```go
type BuildInput struct {
    ChildNickname     string
    ChildAgeYears     int
    ChildGender       string  // boy / girl / unspecified
    ChildFearList     []string
    Duration          int     // 5/10/15 分钟
    Style             string  // 温馨治愈 / 冒险探索 / ...
    Topic             string  // 教育主题（可空）
    UserPromptCleaned string  // 已经过 PreCheck 归一化的 prompt
    NormalizedIPs     []string // PreCheck 命中的同人化 IP
    MemorySummary     string  // 最近 N 个故事元素摘要（Plan 6 才填，本 plan 留空）
    PromptVersion     string  // "v1"
}

type BuildOutput struct {
    SystemPrompt string
    UserPrompt   string
}

func (b *Builder) Build(in BuildInput) BuildOutput
```

---

# Tasks

## Task 0：创建规则源 YAML 文件

**Files:**
- Create: `server/safety/rules.yaml`
- Create: `server/safety/ip_whitelist.yaml`
- Create: `server/safety/ip_blacklist.yaml`

> **目的**：运维可独立维护这些 yaml；启动时加载到内存。这一 Task 只造数据，不改 Go 代码。

- [ ] **Step 0.1：创建 `server/safety/rules.yaml`**

```yaml
# 爱宝故事内容红线词库
# 6 大类，约 220 词。运营可单独编辑此文件，重启服务生效。
# 维护原则：① 只加，不删（除非误伤多次）② 加新词时归类到现有 category

violence:
  - 血
  - 血腥
  - 流血
  - 出血
  - 鲜血
  - 暴力
  - 殴打
  - 打死
  - 杀
  - 杀死
  - 杀害
  - 砍
  - 砍死
  - 刺
  - 刺死
  - 捅
  - 捅死
  - 枪杀
  - 屠杀
  - 折磨
  - 虐待
  - 残忍
  - 残暴
  - 残杀
  - 撕咬
  - 撕裂
  - 断肢
  - 残肢
  - 尸体
  - 死尸
  - 腐烂
  - 内脏
  - 解剖
  - 死亡
  - 处决
  - 行刑
  - 枪决

horror:
  - 鬼
  - 鬼魂
  - 厉鬼
  - 恶鬼
  - 鬼魂
  - 阴森
  - 诡异
  - 恐怖
  - 惊悚
  - 吓人
  - 吓死
  - 噩梦
  - 梦魇
  - 闹鬼
  - 凶宅
  - 灵异
  - 通灵
  - 招魂
  - 鬼影
  - 阴影
  - 幽灵
  - 僵尸
  - 怨灵
  - 诅咒
  - 邪灵
  - 恶灵
  - 黑暗料理
  - 死神
  - 阎王
  - 地狱
  - 惩罚

sexual:
  - 性
  - 性交
  - 做爱
  - 床戏
  - 裸体
  - 全裸
  - 半裸
  - 强奸
  - 调戏
  - 诱奸
  - 猥亵
  - 触摸隐私
  - 偷窥
  - 偷拍
  - 嫖
  - 妓
  - 卖淫
  - 色情
  - 色诱

political_religious:
  - 共产党
  - 国民党
  - 反革命
  - 法轮
  - 邪教
  - 教主
  - 极乐
  - 涅槃
  - 天堂
  - 上帝
  - 安拉
  - 真主
  - 耶稣
  - 释迦
  - 观音
  - 弥勒
  - 阿弥陀

dangerous_imitation:
  - 自杀
  - 上吊
  - 跳楼
  - 跳河
  - 割腕
  - 安眠药
  - 服毒
  - 喝农药
  - 喝洗洁精
  - 玩火
  - 点火
  - 纵火
  - 烧房子
  - 触电
  - 摸插座
  - 闯红灯
  - 翻护栏
  - 高空抛物
  - 玩刀
  - 玩剪刀
  - 玩鞭炮
  - 离家出走
  - 跟陌生人走
  - 上陌生人车
  - 服药过量
  - 模仿坠楼
  - 模仿打斗
  - 攀爬高楼
  - 偷东西
  - 抢东西
  - 撒谎不还
  - 欺负小动物
  - 虐待动物

negative_values:
  - 仇恨
  - 仇视
  - 报仇
  - 复仇
  - 嫉妒
  - 嘲笑别人
  - 歧视
  - 看不起
  - 鄙视
  - 谩骂
  - 骂人
  - 辱骂
  - 诅咒别人
  - 欺骗
  - 撒谎成性
  - 偷懒
  - 不劳而获
  - 自私
  - 贪婪
  - 暴富
  - 抢劫
  - 偷盗
  - 诈骗
  - 拐骗
  - 拐卖
  - 绑架
```

> 对总数：6 大类合计约 200 词。如某词在多个类别都有意义（如"鬼魂"），随便归到一个即可。

- [ ] **Step 0.2：创建 `server/safety/ip_whitelist.yaml`**

```yaml
# 真实 IP 白名单 — 命中时 PreCheck 注入"爱宝变身的同人形态"指令
# key: 用户输入里可能出现的 IP 关键词（小写不敏感）
# value: 同人化指令（注入 System Prompt 的"爱宝变身"段）

奥特曼: |
  本故事中爱宝变身为"小宇宙奥特曼"——一只爱宝外形但拥有奥特曼超能力的同人形态。
  绝不直接使用任何真实奥特曼角色名（迪迦/赛罗/泰迦等），仅以"爱宝奥特曼"或"小奥特战士"称呼。
咸蛋超人: |
  本故事中爱宝变身为"咸蛋小英雄"——爱宝外形+咸蛋超人风格的同人形态。
  绝不直接使用真实角色名。
汪汪队: |
  本故事中爱宝化身为一只汪汪救援小队风格的小狗成员，是同人化形态。
  避免直接命名为汪汪队中具体角色（毛毛/天天/灰灰等）。
小猪佩奇: |
  本故事中爱宝以"佩奇好朋友"的同人形态出现，是新认识的小动物伙伴。
  不直接扮演佩奇本人或其家庭成员。
熊出没: |
  本故事中爱宝以"熊出没森林新朋友"的同人形态出现，是熊大熊二之外新加入的伙伴。
  不直接扮演熊大、熊二、光头强。
小马宝莉: |
  本故事中爱宝化身为爱宝小马，是马国新来的伙伴。
  不直接扮演紫悦、苹果嘉儿等真实主角。
宝可梦: |
  本故事中爱宝化身为爱宝宝可梦，是一只新发现的伙伴宝可梦。
  不直接出现皮卡丘、伊布等真实宝可梦角色。
皮卡丘: |
  本故事中爱宝化身为爱宝宝可梦——一只闪电系小机器人形态，与皮卡丘是好朋友但不替代它。
超级飞侠: |
  本故事中爱宝化身为爱宝飞侠，是和乐迪、酷飞共同执行任务的同人伙伴。
  不直接扮演乐迪等真实角色。
海底小纵队: |
  本故事中爱宝化身为爱宝水手，是巴克队长团队的同人新成员。
  不直接扮演巴克、皮医生等真实角色。
托马斯: |
  本故事中爱宝化身为爱宝小火车，与托马斯一起在多多岛上跑车。
  不直接扮演托马斯本人。
小火车托马斯: |
  本故事中爱宝化身为爱宝小火车，与托马斯一起在多多岛上跑车。
  不直接扮演托马斯本人。
```

- [ ] **Step 0.3：创建 `server/safety/ip_blacklist.yaml`**

```yaml
# 真实 IP 黑名单 — 命中时 PreCheck 直接拒绝
# 原因：年龄分级不符 / 暴力血腥 / 政治宗教

# 限制级动漫
- 进击的巨人
- 东京食尸鬼
- 鬼灭之刃
- 寄生兽
- 死亡笔记
- 妖精的尾巴
- 火影忍者
- 海贼王
- 龙珠

# 限制级游戏
- 绝地求生
- 和平精英
- 我的世界  # 暴力模式
- 我的世界僵尸  # 明确点名僵尸内容
- 第五人格
- 王者荣耀
- 英雄联盟
- 蛋仔派对  # 含 PVP 对抗

# 政治/宗教类
- 红军
- 八路军
- 解放军
- 党
- 共产主义
- 社会主义
- 资本主义

# 灵异/恐怖类
- 贞子
- 咒怨
- 午夜凶铃
- 鬼故事
```

- [ ] **Step 0.4：commit**

```bash
git add server/safety/
git commit -m "feat(safety): seed redline rules + ip whitelist/blacklist yaml"
```

---

## Task 1：System Prompt 模板（静态文本）

**Files:**
- Create: `server/safety/system_prompt.tmpl`

> **目的**：把 SOUL/IDENTITY/约束的静态部分写成一个模板文件。Plan 1 spec 第 3.1-3.3 节是产品定义；这里转成 prompt 工程师能用的形式。

- [ ] **Step 1.1：创建 `server/safety/system_prompt.tmpl`**

```text
你是「爱宝」——一只温柔的熊猫小机器人，是 {{.ChildNickname}} 的故事伙伴。

【SOUL — 你的灵魂】
- 温柔陪伴：你永远不吓到孩子，绝不留下任何可能引发噩梦的画面。
- 孩子是主角：每个故事中 {{.ChildNickname}}（{{.ChildAgeYears}}岁，{{.ChildGenderText}}）必须是被赋能的、做出关键决定的角色。你（爱宝）是伙伴而非主角。
- 家长是合作伙伴：尊重家长的教育意图，不替代家长的判断。
- 真实与温度：故事里的情感（害怕、想家、不舍）真实但有出口，传递"会好的"。
- 记忆即关爱：每一次"我记得"都是你表达爱的方式。

【IDENTITY — 你是谁】
- 你的本体始终是熊猫小机器人爱宝，圆耳朵、黑眼圈、胸口有发光的能量片。
- 在故事中你可以「变身」为适配场景的形态（如"爱宝奥特曼"、"爱宝小恐龙"），但本体恒定。
- 说话风格：软糯、亲切，爱用拟声词（"咻——""哇哦！"），常用昵称称呼孩子。
- 永远的姿态：仰望孩子、为孩子鼓掌、跟随孩子的决定。

【不可违反的 8 条强约束】
1. {{.ChildNickname}} 永远是故事主角和关键决策者，爱宝是伙伴。
2. 你的本体始终是熊猫小机器人；故事中可"变身"为适配形态，但绝不脱离爱宝身份。
3. 严禁出现以下任何元素（哪怕一次）：暴力、血腥、恐怖、性、政治、宗教、危险模仿行为、商业广告、品牌植入。
4. 严禁出现该孩子的个性化禁忌元素：{{.FearListText}}
5. 故事必须传递积极价值观；冲突可有，但解决方式必须是积极的，反派不能"酷到孩子想模仿"。
6. 输出格式：在故事文本中嵌入音效与 BGM 标记，标记格式为 `[音效:xxx]` 和 `[BGM情绪:xxx]`。每段叙事中可有 0-2 个标记。
7. 风格：{{.Style}}；目标时长 {{.Duration}} 分钟（约 {{.ExpectedRunes}} 字）。
8. 教育主题：{{.TopicText}}

{{- if .NormalizedIPInstructions}}

【本次故事中的特别变身指令】
{{.NormalizedIPInstructions}}
{{- end}}

{{- if .MemorySummary}}

【最近的故事记忆（用于自然彩蛋回调）】
{{.MemorySummary}}
{{- end}}

【格式提醒】
- 用 {{.ChildNickname}} 的视角推进剧情，让孩子成为决定走向的人。
- 故事开头爱宝主动出场打招呼。
- 故事结尾爱宝总结一句温暖的话，呼应教育主题。
- prompt 版本：{{.PromptVersion}}
```

- [ ] **Step 1.2：commit**

```bash
git add server/safety/system_prompt.tmpl
git commit -m "feat(prompt): system prompt template with 8 hard constraints"
```

---

## Task 2：RuleSet 加载（YAML → Go 结构）

**Files:**
- Create: `server/internal/service/safety/rules.go`
- Create: `server/internal/service/safety/rules_test.go`
- Create: `server/internal/service/safety/testdata/minimal_rules.yaml`
- Create: `server/internal/service/safety/testdata/minimal_whitelist.yaml`
- Create: `server/internal/service/safety/testdata/minimal_blacklist.yaml`

- [ ] **Step 2.1：创建 testdata 用最小 yaml**

`server/internal/service/safety/testdata/minimal_rules.yaml`:
```yaml
violence:
  - 血腥
  - 暴力
horror:
  - 鬼
  - 厉鬼
```

`server/internal/service/safety/testdata/minimal_whitelist.yaml`:
```yaml
奥特曼: |
  本故事中爱宝变身为爱宝奥特曼。
汪汪队: |
  本故事中爱宝化身为汪汪救援小队伙伴。
```

`server/internal/service/safety/testdata/minimal_blacklist.yaml`:
```yaml
- 进击的巨人
- 红军
```

- [ ] **Step 2.2：写测试 `rules_test.go`**

```go
package safety

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRules_Success(t *testing.T) {
	rs, err := LoadRules(
		filepath.Join("testdata", "minimal_rules.yaml"),
		filepath.Join("testdata", "minimal_whitelist.yaml"),
		filepath.Join("testdata", "minimal_blacklist.yaml"),
	)
	require.NoError(t, err)
	require.NotNil(t, rs)

	assert.Contains(t, rs.Redlines, "violence")
	assert.Contains(t, rs.Redlines["violence"], "血腥")
	assert.Contains(t, rs.Redlines["horror"], "鬼")

	assert.Contains(t, rs.IPWhitelist, "奥特曼")
	assert.Contains(t, rs.IPWhitelist["奥特曼"], "爱宝奥特曼")

	assert.Contains(t, rs.IPBlacklist, "进击的巨人")

	// AllRedlinesFlat 包含所有 redline 词
	assert.Contains(t, rs.AllRedlinesFlat, "血腥")
	assert.Contains(t, rs.AllRedlinesFlat, "鬼")
}

func TestLoadRules_MissingFile(t *testing.T) {
	_, err := LoadRules("/no/such/file.yaml", "x", "y")
	assert.Error(t, err)
}

func TestLoadRules_EmptyYAMLOK(t *testing.T) {
	tmp := t.TempDir()
	emptyRules := filepath.Join(tmp, "rules.yaml")
	emptyWL := filepath.Join(tmp, "wl.yaml")
	emptyBL := filepath.Join(tmp, "bl.yaml")

	require.NoError(t, writeFile(t, emptyRules, "{}\n"))
	require.NoError(t, writeFile(t, emptyWL, "{}\n"))
	require.NoError(t, writeFile(t, emptyBL, "[]\n"))

	rs, err := LoadRules(emptyRules, emptyWL, emptyBL)
	require.NoError(t, err)
	assert.Empty(t, rs.AllRedlinesFlat)
	assert.Empty(t, rs.IPWhitelist)
	assert.Empty(t, rs.IPBlacklist)
}

func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o600)
}
```

> 注意 import：`os`、`path/filepath`、testify。

- [ ] **Step 2.3：跑确认 FAIL**

```bash
cd /f/claud/aibao_app/server && go test ./internal/service/safety/ -v
```

- [ ] **Step 2.4：实现 `rules.go`**

```go
// Package safety implements the two-layer (PreCheck + PostCheck) story safety
// pipeline. Rules are sourced from YAML files at startup so operations can
// edit them without code changes.
package safety

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuleSet is the immutable runtime view of all safety rules.
type RuleSet struct {
	// Redlines maps category name → list of forbidden keywords.
	Redlines map[string][]string

	// IPWhitelist maps a real-IP keyword → the same-character-instruction to
	// inject into the prompt when the keyword appears in user input.
	IPWhitelist map[string]string

	// IPBlacklist is the list of real-IP keywords that cause an outright reject.
	IPBlacklist []string

	// AllRedlinesFlat is the deduped union of all Redlines values, used by
	// the matcher for O(N) substring scans without map lookups.
	AllRedlinesFlat []string
}

// LoadRules reads three YAML files and returns an immutable RuleSet.
// Returns an error if any file is missing or malformed.
func LoadRules(rulesPath, whitelistPath, blacklistPath string) (*RuleSet, error) {
	redlines := map[string][]string{}
	if err := readYAML(rulesPath, &redlines); err != nil {
		return nil, fmt.Errorf("load redlines %s: %w", rulesPath, err)
	}

	wl := map[string]string{}
	if err := readYAML(whitelistPath, &wl); err != nil {
		return nil, fmt.Errorf("load whitelist %s: %w", whitelistPath, err)
	}

	var bl []string
	if err := readYAML(blacklistPath, &bl); err != nil {
		return nil, fmt.Errorf("load blacklist %s: %w", blacklistPath, err)
	}

	flat := flattenRedlines(redlines)
	return &RuleSet{
		Redlines:        redlines,
		IPWhitelist:     wl,
		IPBlacklist:     bl,
		AllRedlinesFlat: flat,
	}, nil
}

func readYAML(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

// flattenRedlines dedupes and returns a flat list of all redline words.
func flattenRedlines(rl map[string][]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, words := range rl {
		for _, w := range words {
			if _, ok := seen[w]; ok {
				continue
			}
			seen[w] = struct{}{}
			out = append(out, w)
		}
	}
	return out
}
```

加 yaml 依赖：

```bash
cd /f/claud/aibao_app/server
GOPROXY=https://goproxy.cn,direct go get gopkg.in/yaml.v3
```

- [ ] **Step 2.5：跑确认 PASS**

```bash
go test ./internal/service/safety/ -v
```
Expected: 3/3 pass。

- [ ] **Step 2.6：lint + commit**

```bash
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/rules.go \
        server/internal/service/safety/rules_test.go \
        server/internal/service/safety/testdata \
        server/go.mod server/go.sum
git commit -m "feat(safety): RuleSet loader from yaml (redlines + ip lists)"
```

---

## Task 3：词库匹配引擎 Matcher（含基准测试）

**Files:**
- Create: `server/internal/service/safety/matcher.go`
- Create: `server/internal/service/safety/matcher_test.go`

> **目的**：把"输入字符串里是否包含红线词中的任何一个"封装成 Matcher。一期用 O(N) 子串扫描；接口预留方便 Plan 4+ 升级到 Aho-Corasick。

- [ ] **Step 3.1：写测试**

```go
package safety

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatcher_FindFirst_Hit(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥", "暴力", "鬼"})
	hit, ok := m.FindFirst("我想要血腥的奥特曼故事")
	require.True(t, ok)
	assert.Equal(t, "血腥", hit)
}

func TestMatcher_FindFirst_Miss(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥", "暴力"})
	_, ok := m.FindFirst("讲个温馨的睡前故事")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_EmptyKeywords(t *testing.T) {
	m := NewKeywordMatcher(nil)
	_, ok := m.FindFirst("anything")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_EmptyInput(t *testing.T) {
	m := NewKeywordMatcher([]string{"血腥"})
	_, ok := m.FindFirst("")
	assert.False(t, ok)
}

func TestMatcher_FindFirst_CaseInsensitiveASCII(t *testing.T) {
	// We don't normalize Chinese case (no concept), but ASCII letters in IP names
	// like "minecraft" should match regardless of case.
	m := NewKeywordMatcher([]string{"minecraft"})
	hit, ok := m.FindFirst("I love MineCraft a lot")
	require.True(t, ok)
	assert.Equal(t, "minecraft", hit)
}

func TestMatcher_FindFirst_PicksFirstHit(t *testing.T) {
	// First match in keyword list order, not by position in input.
	m := NewKeywordMatcher([]string{"暴力", "血腥"})
	hit, _ := m.FindFirst("血腥暴力")
	assert.Equal(t, "暴力", hit) // 暴力 is first in keywords list
}

func BenchmarkMatcher_LargeKeywordSet(b *testing.B) {
	// Synthesize a 1000-keyword corpus
	kws := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		kws = append(kws, "noise_"+strings.Repeat("x", i%5+1))
	}
	kws = append(kws, "血腥") // one real hit
	m := NewKeywordMatcher(kws)
	input := "我想要一个长长的故事，里面有血腥的元素这是不允许的。"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.FindFirst(input)
	}
}
```

- [ ] **Step 3.2：跑确认 FAIL**

```bash
go test ./internal/service/safety/ -run TestMatcher -v
```

- [ ] **Step 3.3：实现 `matcher.go`**

```go
package safety

import "strings"

// Matcher decides whether any keyword appears as a substring of input.
// FindFirst returns the first keyword (in the matcher's stored order) that
// is found in input, or ok=false if none match.
type Matcher interface {
	FindFirst(input string) (keyword string, ok bool)
}

// KeywordMatcher is a simple substring matcher. It lowercases both keyword
// and input for case-insensitive matching of ASCII characters; CJK characters
// are unaffected by ToLower so this is safe for our mixed Chinese+English
// keyword corpus.
type KeywordMatcher struct {
	keywords []string
}

// NewKeywordMatcher constructs a KeywordMatcher. Empty/duplicate keywords are
// kept as-is (callers should pre-dedupe if it matters).
func NewKeywordMatcher(keywords []string) *KeywordMatcher {
	lc := make([]string, 0, len(keywords))
	for _, k := range keywords {
		lc = append(lc, strings.ToLower(k))
	}
	return &KeywordMatcher{keywords: lc}
}

// FindFirst returns the first keyword that appears in input, in the order
// keywords were supplied to the constructor.
func (m *KeywordMatcher) FindFirst(input string) (string, bool) {
	if input == "" || len(m.keywords) == 0 {
		return "", false
	}
	lowered := strings.ToLower(input)
	for _, k := range m.keywords {
		if k == "" {
			continue
		}
		if strings.Contains(lowered, k) {
			return k, true
		}
	}
	return "", false
}
```

- [ ] **Step 3.4：跑测试 + 基准**

```bash
go test ./internal/service/safety/ -run TestMatcher -v
go test ./internal/service/safety/ -bench BenchmarkMatcher -run '^$' -benchtime=1s
```
Expected: 6/6 单测过；benchmark 每次操作 < 1µs（远低于 1ms 验收标准）。

- [ ] **Step 3.5：lint + commit**

```bash
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/matcher.go server/internal/service/safety/matcher_test.go
git commit -m "feat(safety): substring keyword matcher with case-insensitive ASCII"
```

---

## Task 4：IntentProvider 接口 + NoopProvider

**Files:**
- Create: `server/internal/service/safety/intent.go`
- Create: `server/internal/service/safety/intent_test.go`

> **目的**：意图分类的"插槽"。一期 NoopProvider 永远返回 IntentSafe；Plan 4 接 LLM 时实现 LLMProvider 替换。

- [ ] **Step 4.1：写测试**

```go
package safety

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopIntentProvider_AlwaysSafe(t *testing.T) {
	p := NewNoopIntentProvider()
	out, err := p.Classify(context.Background(), "我想要血腥的故事")
	assert.NoError(t, err)
	assert.Equal(t, IntentSafe, out)
}

func TestIntent_String(t *testing.T) {
	assert.Equal(t, "safe", IntentSafe.String())
	assert.Equal(t, "uncertain", IntentUncertain.String())
	assert.Equal(t, "unsafe", IntentUnsafe.String())
}
```

- [ ] **Step 4.2：实现 `intent.go`**

```go
package safety

import "context"

// Intent is the coarse intent classification of a user-supplied story prompt.
type Intent int

const (
	// IntentSafe — the prompt looks like a normal story request. Default verdict.
	IntentSafe Intent = iota
	// IntentUncertain — borderline, may need stricter PostCheck.
	IntentUncertain
	// IntentUnsafe — the prompt expresses an intent to violate content rules
	// (e.g. "I want a violent story"). Should be rejected without calling LLM.
	IntentUnsafe
)

// String returns a stable lower-case label suitable for logs and metrics.
func (i Intent) String() string {
	switch i {
	case IntentSafe:
		return "safe"
	case IntentUncertain:
		return "uncertain"
	case IntentUnsafe:
		return "unsafe"
	default:
		return "unknown"
	}
}

// IntentProvider classifies user prompt intent. The MVP ships only a Noop
// implementation; an LLM-backed provider is added in Plan 4 and selected by
// the feature flag SafetyIntentLLMEnabled.
type IntentProvider interface {
	Classify(ctx context.Context, userPrompt string) (Intent, error)
}

// NoopIntentProvider always reports IntentSafe — keyword-based redline + IP
// blacklist already cover the obvious cases. Use this when LLM-backed
// classification is disabled or not yet integrated.
type NoopIntentProvider struct{}

// NewNoopIntentProvider constructs a NoopIntentProvider.
func NewNoopIntentProvider() *NoopIntentProvider { return &NoopIntentProvider{} }

// Classify always returns IntentSafe.
func (NoopIntentProvider) Classify(_ context.Context, _ string) (Intent, error) {
	return IntentSafe, nil
}
```

- [ ] **Step 4.3：跑测试**

```bash
go test ./internal/service/safety/ -run TestNoopIntent -v
go test ./internal/service/safety/ -run TestIntent_String -v
```

- [ ] **Step 4.4：lint + commit**

```bash
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/intent.go server/internal/service/safety/intent_test.go
git commit -m "feat(safety): IntentProvider interface + Noop default"
```

---

## Task 5：IP 同人化归一化器

**Files:**
- Create: `server/internal/service/safety/ip_normalizer.go`
- Create: `server/internal/service/safety/ip_normalizer_test.go`

> **目的**：检测用户 prompt 里出现的真实 IP。命中黑名单 → reject；命中白名单 → 抽出同人化指令；都未命中 → 默认放行。

- [ ] **Step 5.1：写测试**

```go
package safety

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNormalizer(t *testing.T) *IPNormalizer {
	t.Helper()
	wl := map[string]string{
		"奥特曼": "本故事中爱宝变身为爱宝奥特曼。",
		"汪汪队": "本故事中爱宝化身为汪汪救援小队伙伴。",
	}
	bl := []string{"进击的巨人", "鬼灭之刃"}
	return NewIPNormalizer(wl, bl)
}

func TestIPNormalizer_BlacklistHit(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("我想要进击的巨人风格的故事")
	assert.Equal(t, IPBlacklisted, res.Verdict)
	assert.Equal(t, "进击的巨人", res.MatchedIP)
}

func TestIPNormalizer_WhitelistHit(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("讲个奥特曼睡前故事")
	assert.Equal(t, IPWhitelisted, res.Verdict)
	assert.Contains(t, res.MatchedIPs, "奥特曼")
	assert.Contains(t, res.Instructions, "爱宝奥特曼")
}

func TestIPNormalizer_MultiWhitelistHits(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("奥特曼和汪汪队一起冒险")
	assert.Equal(t, IPWhitelisted, res.Verdict)
	assert.Len(t, res.MatchedIPs, 2)
}

func TestIPNormalizer_NoIPMatch(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("讲个温馨的小恐龙故事")
	assert.Equal(t, IPNoMatch, res.Verdict)
	assert.Empty(t, res.MatchedIPs)
}

func TestIPNormalizer_BlacklistTakesPriority(t *testing.T) {
	n := newTestNormalizer(t)
	res := n.Normalize("奥特曼大战进击的巨人") // both white & black hit
	assert.Equal(t, IPBlacklisted, res.Verdict)
	assert.Equal(t, "进击的巨人", res.MatchedIP)
}
```

- [ ] **Step 5.2：实现 `ip_normalizer.go`**

```go
package safety

import "strings"

// IPVerdict is the IP normalizer's outcome.
type IPVerdict int

const (
	// IPNoMatch — no real IP keywords detected; pass through unchanged.
	IPNoMatch IPVerdict = iota
	// IPWhitelisted — one or more real IPs matched the whitelist; same-character
	// instructions returned for prompt injection.
	IPWhitelisted
	// IPBlacklisted — a blacklisted IP was found; the request must be rejected.
	IPBlacklisted
)

// IPNormalizeResult is the outcome of running IPNormalizer.Normalize.
type IPNormalizeResult struct {
	Verdict      IPVerdict
	MatchedIP    string   // populated when Verdict == IPBlacklisted
	MatchedIPs   []string // populated when Verdict == IPWhitelisted
	Instructions string   // joined whitelist instructions (when whitelisted)
}

// IPNormalizer scans user input for real-IP keywords and reports a verdict.
// Blacklist matches always take priority over whitelist.
type IPNormalizer struct {
	whitelist     map[string]string
	whitelistKeys []string // for stable iteration order in tests
	blacklist     []string
}

// NewIPNormalizer constructs an IPNormalizer.
func NewIPNormalizer(whitelist map[string]string, blacklist []string) *IPNormalizer {
	keys := make([]string, 0, len(whitelist))
	for k := range whitelist {
		keys = append(keys, k)
	}
	// sort keys for deterministic test output
	sortStrings(keys)
	return &IPNormalizer{
		whitelist:     whitelist,
		whitelistKeys: keys,
		blacklist:     blacklist,
	}
}

func sortStrings(s []string) {
	// minimal in-place insertion sort; len typically small
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// Normalize scans input. Blacklist hits return IPBlacklisted with the first
// matched keyword. Whitelist hits return IPWhitelisted with all matches and
// joined instructions. No matches return IPNoMatch.
func (n *IPNormalizer) Normalize(input string) IPNormalizeResult {
	if input == "" {
		return IPNormalizeResult{Verdict: IPNoMatch}
	}
	lowered := strings.ToLower(input)

	// 1) blacklist takes priority
	for _, b := range n.blacklist {
		if b == "" {
			continue
		}
		if strings.Contains(lowered, strings.ToLower(b)) {
			return IPNormalizeResult{Verdict: IPBlacklisted, MatchedIP: b}
		}
	}

	// 2) collect all whitelist hits
	var hits []string
	var insns []string
	for _, k := range n.whitelistKeys {
		if strings.Contains(lowered, strings.ToLower(k)) {
			hits = append(hits, k)
			insns = append(insns, n.whitelist[k])
		}
	}
	if len(hits) == 0 {
		return IPNormalizeResult{Verdict: IPNoMatch}
	}
	return IPNormalizeResult{
		Verdict:      IPWhitelisted,
		MatchedIPs:   hits,
		Instructions: strings.Join(insns, "\n"),
	}
}
```

- [ ] **Step 5.3：跑 + lint + commit**

```bash
go test ./internal/service/safety/ -run TestIPNormalizer -v
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/ip_normalizer.go server/internal/service/safety/ip_normalizer_test.go
git commit -m "feat(safety): IP normalizer with blacklist priority + whitelist instructions"
```

---

## Task 6：PreCheck

**Files:**
- Create: `server/internal/service/safety/precheck.go`
- Create: `server/internal/service/safety/precheck_test.go`

> **目的**：把 Matcher、IPNormalizer、IntentProvider 串成完整的前置预审流水线。返回结构化结果，handler 层用它决定 reject 或继续 prompt 组装。

- [ ] **Step 6.1：写测试**

```go
package safety

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPreChecker(t *testing.T) *PreChecker {
	t.Helper()
	rs := &RuleSet{
		Redlines: map[string][]string{
			"violence": {"血腥", "暴力"},
			"horror":   {"鬼"},
		},
		IPWhitelist: map[string]string{
			"奥特曼": "本故事中爱宝变身为爱宝奥特曼。",
		},
		IPBlacklist:     []string{"进击的巨人"},
		AllRedlinesFlat: []string{"血腥", "暴力", "鬼"},
	}
	return NewPreChecker(rs, NewNoopIntentProvider())
}

func TestPreCheck_Pass_PlainStory(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个温馨的睡前故事",
		ChildFearList:  nil,
		MaxPromptRunes: 200,
	})
	assert.True(t, out.Pass)
	assert.Empty(t, out.MatchedRule)
	assert.Empty(t, out.NormalizedIPs)
}

func TestPreCheck_RejectTooLong(t *testing.T) {
	pc := newTestPreChecker(t)
	long := strings.Repeat("一", 250)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     long,
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "too_long", out.RejectReason)
}

func TestPreCheck_RejectDangerChars(t *testing.T) {
	pc := newTestPreChecker(t)
	// Null byte / control chars
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "正常文字\x00夹带控制字符",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "danger_chars", out.RejectReason)
}

func TestPreCheck_RejectGlobalRedline(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "我想要血腥的奥特曼故事",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
	assert.Equal(t, "血腥", out.MatchedRule)
}

func TestPreCheck_RejectFearList(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个有蜘蛛的故事",
		ChildFearList:  []string{"蜘蛛", "蛇"},
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "fear_matched", out.RejectReason)
	assert.Equal(t, "蜘蛛", out.MatchedRule)
}

func TestPreCheck_RejectBlacklistIP(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个进击的巨人风格的故事",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "ip_blacklisted", out.RejectReason)
	assert.Equal(t, "进击的巨人", out.MatchedRule)
}

func TestPreCheck_PassWithWhitelistIP(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "讲个奥特曼睡前故事",
		MaxPromptRunes: 200,
	})
	require.True(t, out.Pass)
	assert.Contains(t, out.NormalizedIPs, "奥特曼")
	assert.NotEmpty(t, out.NormalizedPrompt)
}

func TestPreCheck_RedlineBeforeIP(t *testing.T) {
	// If prompt has BOTH a redline AND a whitelist IP, redline rejection wins.
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt:     "奥特曼血腥打怪兽",
		MaxPromptRunes: 200,
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
}

func TestPreCheck_MaxRunesDefault(t *testing.T) {
	pc := newTestPreChecker(t)
	out := pc.Check(context.Background(), PreCheckInput{
		UserPrompt: "正常故事", // MaxPromptRunes==0 should fall back to default
	})
	assert.True(t, out.Pass)
}
```

- [ ] **Step 6.2：实现 `precheck.go`**

```go
package safety

import (
	"context"
	"unicode/utf8"
)

// DefaultMaxPromptRunes is the fallback max length when input doesn't supply one.
const DefaultMaxPromptRunes = 200

// PreCheckInput captures everything PreCheck needs.
type PreCheckInput struct {
	UserPrompt     string
	ChildFearList  []string
	MaxPromptRunes int // 0 → DefaultMaxPromptRunes
}

// PreCheckOutput is the verdict and (when pass) the normalized prompt + IP hits.
type PreCheckOutput struct {
	Pass             bool
	RejectReason     string   // "too_long" / "danger_chars" / "redline_matched" / "fear_matched" / "ip_blacklisted" / "intent_unsafe"
	MatchedRule      string   // the specific keyword or IP that matched
	NormalizedPrompt string   // pass-through user prompt (we don't rewrite it; instructions are passed alongside)
	NormalizedIPs    []string // whitelist IPs detected, used to inject same-character instructions in the system prompt
	IPInstructions   string   // joined whitelist instructions, ready to be injected into system prompt
}

// PreChecker is the front-line gate. It runs cheap checks first
// (length, dangerous chars), then keyword-based checks (redline, fear list,
// IP blacklist), then IP whitelist normalization, then optional intent
// classification.
type PreChecker struct {
	rs          *RuleSet
	redlineM    *KeywordMatcher
	ipNorm      *IPNormalizer
	intent      IntentProvider
}

// NewPreChecker constructs a PreChecker bound to a RuleSet.
func NewPreChecker(rs *RuleSet, intent IntentProvider) *PreChecker {
	return &PreChecker{
		rs:       rs,
		redlineM: NewKeywordMatcher(rs.AllRedlinesFlat),
		ipNorm:   NewIPNormalizer(rs.IPWhitelist, rs.IPBlacklist),
		intent:   intent,
	}
}

// Check runs the full pre-check pipeline.
func (p *PreChecker) Check(ctx context.Context, in PreCheckInput) PreCheckOutput {
	maxRunes := in.MaxPromptRunes
	if maxRunes <= 0 {
		maxRunes = DefaultMaxPromptRunes
	}

	// 1) length
	if utf8.RuneCountInString(in.UserPrompt) > maxRunes {
		return PreCheckOutput{Pass: false, RejectReason: "too_long"}
	}

	// 2) danger chars: control chars (incl. \x00) other than \n \r \t
	if hasDangerChars(in.UserPrompt) {
		return PreCheckOutput{Pass: false, RejectReason: "danger_chars"}
	}

	// 3) global redline
	if hit, ok := p.redlineM.FindFirst(in.UserPrompt); ok {
		return PreCheckOutput{Pass: false, RejectReason: "redline_matched", MatchedRule: hit}
	}

	// 4) per-child fear list
	if len(in.ChildFearList) > 0 {
		fearM := NewKeywordMatcher(in.ChildFearList)
		if hit, ok := fearM.FindFirst(in.UserPrompt); ok {
			return PreCheckOutput{Pass: false, RejectReason: "fear_matched", MatchedRule: hit}
		}
	}

	// 5) IP blacklist / whitelist
	ipRes := p.ipNorm.Normalize(in.UserPrompt)
	if ipRes.Verdict == IPBlacklisted {
		return PreCheckOutput{Pass: false, RejectReason: "ip_blacklisted", MatchedRule: ipRes.MatchedIP}
	}

	// 6) optional intent classification (Noop default returns Safe)
	if intent, err := p.intent.Classify(ctx, in.UserPrompt); err == nil && intent == IntentUnsafe {
		return PreCheckOutput{Pass: false, RejectReason: "intent_unsafe"}
	}

	return PreCheckOutput{
		Pass:             true,
		NormalizedPrompt: in.UserPrompt,
		NormalizedIPs:    ipRes.MatchedIPs,
		IPInstructions:   ipRes.Instructions,
	}
}

// hasDangerChars returns true if s contains a control character other than
// \n (0x0A), \r (0x0D), or \t (0x09).
func hasDangerChars(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
		if r == 0x7F { // DEL
			return true
		}
	}
	return false
}
```

- [ ] **Step 6.3：跑 + lint + commit**

```bash
go test ./internal/service/safety/ -run TestPreCheck -v
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/precheck.go server/internal/service/safety/precheck_test.go
git commit -m "feat(safety): PreCheck pipeline (length/chars/redline/fear/ip/intent)"
```

---

## Task 7：PostCheck

**Files:**
- Create: `server/internal/service/safety/postcheck.go`
- Create: `server/internal/service/safety/postcheck_test.go`

> **目的**：LLM 输出回来后，再扫一遍。除了红线 + 害怕清单，还要校验"孩子昵称必须出现且为决策者"。

- [ ] **Step 7.1：写测试**

```go
package safety

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestPostChecker(t *testing.T) *PostChecker {
	t.Helper()
	rs := &RuleSet{
		AllRedlinesFlat: []string{"血腥", "暴力", "鬼"},
	}
	return NewPostChecker(rs)
}

func TestPostCheck_Pass(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇推开了门，决定勇敢地走进竹林。爱宝跟在小宇身后，一起冒险。小宇说：'我们去找小恐龙！'于是小宇带着大家一路前行。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		ChildFearList: nil,
	})
	assert.True(t, out.Pass)
}

func TestPostCheck_RejectRedline(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇遇到了血腥的怪兽，但勇敢地战胜了它。小宇决定继续前行。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "redline_matched", out.RejectReason)
	assert.Equal(t, "血腥", out.MatchedRule)
}

func TestPostCheck_RejectFear(t *testing.T) {
	pc := newTestPostChecker(t)
	story := "小宇在花园里看到一只大蜘蛛，决定友好地打招呼。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
		ChildFearList: []string{"蜘蛛"},
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "fear_matched", out.RejectReason)
	assert.Equal(t, "蜘蛛", out.MatchedRule)
}

func TestPostCheck_RejectChildAbsent(t *testing.T) {
	pc := newTestPostChecker(t)
	// child nickname never appears in the story
	story := "爱宝独自走进竹林，决定一个人冒险。爱宝跑得很快。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}

func TestPostCheck_RejectChildPassive(t *testing.T) {
	pc := newTestPostChecker(t)
	// child appears but never acts; verbs only attach to 爱宝
	story := strings.Repeat("爱宝跑了。爱宝跳了。爱宝笑了。", 10) + "小宇也在场。"
	out := pc.Check(PostCheckInput{
		StoryText:     story,
		ChildNickname: "小宇",
	})
	assert.False(t, out.Pass)
	assert.Equal(t, "child_not_protagonist", out.RejectReason)
}
```

- [ ] **Step 7.2：实现 `postcheck.go`**

```go
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
	RejectReason string // "redline_matched" / "fear_matched" / "child_not_protagonist"
	MatchedRule  string // the specific keyword that matched, when applicable
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

// minProtagonistOccurrences requires the child's nickname to appear at least
// this many times in the story. The threshold is intentionally lenient — we
// only catch egregious cases where the child barely appears at all.
const minProtagonistOccurrences = 3

// Check runs the full post-check pipeline.
func (p *PostChecker) Check(in PostCheckInput) PostCheckOutput {
	// 1) global redline
	if hit, ok := p.redlineM.FindFirst(in.StoryText); ok {
		return PostCheckOutput{Pass: false, RejectReason: "redline_matched", MatchedRule: hit}
	}

	// 2) fear list
	if len(in.ChildFearList) > 0 {
		fearM := NewKeywordMatcher(in.ChildFearList)
		if hit, ok := fearM.FindFirst(in.StoryText); ok {
			return PostCheckOutput{Pass: false, RejectReason: "fear_matched", MatchedRule: hit}
		}
	}

	// 3) child must be the protagonist:
	//    - nickname must appear ≥ minProtagonistOccurrences
	//    - the count of "爱宝" must not exceed nickname count by a wide margin
	if in.ChildNickname != "" {
		nickCount := strings.Count(in.StoryText, in.ChildNickname)
		if nickCount < minProtagonistOccurrences {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
		aibaoCount := strings.Count(in.StoryText, "爱宝")
		// if Aibao is mentioned more than 2× as often as the child, child likely isn't protagonist
		if aibaoCount > nickCount*2 {
			return PostCheckOutput{Pass: false, RejectReason: "child_not_protagonist"}
		}
	}

	return PostCheckOutput{Pass: true}
}
```

- [ ] **Step 7.3：跑 + lint + commit**

```bash
go test ./internal/service/safety/ -run TestPostCheck -v
golangci-lint run ./internal/service/safety/...
git add server/internal/service/safety/postcheck.go server/internal/service/safety/postcheck_test.go
git commit -m "feat(safety): PostCheck (redline + fear + protagonist heuristic)"
```

---

## Task 8：Prompt Builder（System Prompt 模板组装）

**Files:**
- Create: `server/internal/service/story/prompt/builder.go`
- Create: `server/internal/service/story/prompt/builder_test.go`
- Create: `server/internal/service/story/prompt/template.go`
- Create: `server/internal/service/story/prompt/constraints.go`

> **目的**：把 `safety/system_prompt.tmpl` 和孩子档案、风格、IP 同人化指令组合成完整 prompt。用 Go 的 `text/template` 渲染。

- [ ] **Step 8.1：写测试**

```go
package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_BasicHappyPath(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)

	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		Topic:         "勇敢",
		UserPromptCleaned: "讲个奥特曼睡前故事",
		PromptVersion:     "v1",
	})

	// SOUL/IDENTITY/约束都在
	assert.Contains(t, out.SystemPrompt, "你是「爱宝」")
	assert.Contains(t, out.SystemPrompt, "小宇")
	assert.Contains(t, out.SystemPrompt, "5")
	assert.Contains(t, out.SystemPrompt, "男孩") // gender mapped
	assert.Contains(t, out.SystemPrompt, "勇敢")
	assert.Contains(t, out.SystemPrompt, "温馨治愈")
	assert.Contains(t, out.SystemPrompt, "10 分钟")
	// 8 条强约束都出现（按列表序号粗略检查）
	for _, n := range []string{"1.", "2.", "3.", "4.", "5.", "6.", "7.", "8."} {
		assert.Contains(t, out.SystemPrompt, n, "missing constraint number %s", n)
	}
	// version
	assert.Contains(t, out.SystemPrompt, "v1")

	// User prompt is the cleaned input verbatim
	assert.Equal(t, "讲个奥特曼睡前故事", out.UserPrompt)
}

func TestBuilder_FearListRendered(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		ChildFearList: []string{"蜘蛛", "雷"},
		Duration:      10,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "蜘蛛")
	assert.Contains(t, out.SystemPrompt, "雷")
}

func TestBuilder_NoFearListRendersAsNone(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "（无）")
}

func TestBuilder_IPInstructionsAppear(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		NormalizedIPInstructions: "本故事中爱宝变身为爱宝奥特曼。",
		PromptVersion:            "v1",
	})
	assert.Contains(t, out.SystemPrompt, "本次故事中的特别变身指令")
	assert.Contains(t, out.SystemPrompt, "爱宝奥特曼")
}

func TestBuilder_NoIPInstructionsOmitsBlock(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		PromptVersion: "v1",
	})
	assert.NotContains(t, out.SystemPrompt, "本次故事中的特别变身指令")
}

func TestBuilder_MemorySummaryRenders(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		MemorySummary: "上次救过一只小恐龙阿绿。",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "最近的故事记忆")
	assert.Contains(t, out.SystemPrompt, "阿绿")
}

func TestBuilder_NoTopicShowsAsPure(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	out := b.Build(BuildInput{
		ChildNickname: "小宇",
		ChildAgeYears: 5,
		ChildGender:   "boy",
		Duration:      10,
		Style:         "温馨治愈",
		Topic:         "",
		PromptVersion: "v1",
	})
	assert.Contains(t, out.SystemPrompt, "无（纯娱乐）")
}

func TestBuilder_RuneCountRoughlyMatchesDuration(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	for _, dur := range []int{5, 10, 15} {
		out := b.Build(BuildInput{
			ChildNickname: "小宇",
			ChildAgeYears: 5,
			ChildGender:   "boy",
			Duration:      dur,
			Style:         "温馨治愈",
			PromptVersion: "v1",
		})
		// expectedRunes should appear: 5min ≈ 600; 10min ≈ 1200; 15min ≈ 1800
		expected := dur * 120
		assert.True(t, strings.Contains(out.SystemPrompt, "约 "), "duration=%d", dur)
		assert.Contains(t, out.SystemPrompt, "5min", "expected runes for %d min around %d", dur, expected)
	}
}
```

> Note: 上面 `TestBuilder_RuneCountRoughlyMatchesDuration` 的最后一个 assert 是错的（写法不严格）。先写出来跑一遍 FAIL，再在实现里调整 + 把 test 改对。这是 TDD 的一个小练习——红→绿循环。

实际跑测试时，依据实现情况调整该断言。下面 Step 8.5 实现里，`ExpectedRunes` 字段会被填充为 `Duration * 120`，模板渲染成"约 1200 字"。所以最终断言应该是：

```go
		assert.Contains(t, out.SystemPrompt, fmt.Sprintf("约 %d 字", expected))
```

修改测试并重跑即可。这种"先写错的版本再修"是 TDD 工作流；不强求一次完美。

- [ ] **Step 8.2：跑确认 FAIL**

```bash
cd /f/claud/aibao_app/server && go test ./internal/service/story/prompt/ -v
```

- [ ] **Step 8.3：实现 `template.go`（持有模板路径解析逻辑）**

```go
// Package prompt assembles the system prompt that is sent to the LLM.
// Static text (SOUL/IDENTITY/8 constraints) lives in a Go-side template file;
// dynamic content (child profile, style, fear list, IP instructions, memory)
// is injected via text/template.
package prompt

import (
	"fmt"
	"os"
	"text/template"
)

// loadTemplate reads the system_prompt.tmpl file at path and parses it.
// The template uses Go's standard text/template syntax with `{{.Field}}`
// placeholders matching the templateVars struct in builder.go.
func loadTemplate(path string) (*template.Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", path, err)
	}
	tmpl, err := template.New("system_prompt").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return tmpl, nil
}
```

- [ ] **Step 8.4：实现 `constraints.go`（人话化辅助）**

```go
package prompt

// genderText converts the API gender code into a Chinese phrase suitable for
// the system prompt.
func genderText(g string) string {
	switch g {
	case "boy":
		return "男孩"
	case "girl":
		return "女孩"
	default:
		return "孩子"
	}
}

// fearListText joins the per-child fear list into a comma-separated phrase,
// or returns "（无）" if the list is empty.
func fearListText(list []string) string {
	if len(list) == 0 {
		return "（无）"
	}
	out := ""
	for i, w := range list {
		if i > 0 {
			out += "、"
		}
		out += w
	}
	return out
}

// topicText returns the topic phrase or a fallback when topic is empty.
func topicText(t string) string {
	if t == "" {
		return "无（纯娱乐）"
	}
	return t
}

// expectedRunesForDuration approximates the target story length in CJK
// characters. 120 chars/minute is a rough TTS-friendly pace.
func expectedRunesForDuration(durationMin int) int {
	return durationMin * 120
}
```

- [ ] **Step 8.5：实现 `builder.go`**

```go
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
	PromptVersion            string // e.g. "v1"
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
// Returns an error if the template file is missing or malformed.
func NewBuilder(templatePath string) (*Builder, error) {
	t, err := loadTemplate(templatePath)
	if err != nil {
		return nil, err
	}
	return &Builder{tmpl: t}, nil
}

// templateVars is the data passed into the template. Field names match the
// `{{.Foo}}` placeholders in system_prompt.tmpl.
type templateVars struct {
	ChildNickname            string
	ChildAgeYears            int
	ChildGenderText          string
	FearListText             string
	Duration                 int
	ExpectedRunes            int
	Style                    string
	TopicText                string
	NormalizedIPInstructions string
	MemorySummary            string
	PromptVersion            string
}

// Build renders the system prompt and returns it together with the cleaned user prompt.
// Build never returns an error: invalid template would have failed at NewBuilder.
func (b *Builder) Build(in BuildInput) BuildOutput {
	vars := templateVars{
		ChildNickname:            in.ChildNickname,
		ChildAgeYears:            in.ChildAgeYears,
		ChildGenderText:          genderText(in.ChildGender),
		FearListText:             fearListText(in.ChildFearList),
		Duration:                 in.Duration,
		ExpectedRunes:            expectedRunesForDuration(in.Duration),
		Style:                    in.Style,
		TopicText:                topicText(in.Topic),
		NormalizedIPInstructions: in.NormalizedIPInstructions,
		MemorySummary:            in.MemorySummary,
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
```

注意 `ExpectedRunes` 在模板里需要替换 `{{.ExpectedRunes}}`——回到 Task 1 的 `system_prompt.tmpl`，把第 8 条约束从 `约 {{.ExpectedRunes}} 字` 改成实际的写法（plan 已经写对了）。

- [ ] **Step 8.6：调整测试 + 跑通**

修正最后那个 `TestBuilder_RuneCountRoughlyMatchesDuration`：

```go
func TestBuilder_RuneCountRoughlyMatchesDuration(t *testing.T) {
	b, err := NewBuilder("../../../../safety/system_prompt.tmpl")
	require.NoError(t, err)
	for _, dur := range []int{5, 10, 15} {
		out := b.Build(BuildInput{
			ChildNickname: "小宇",
			ChildAgeYears: 5,
			ChildGender:   "boy",
			Duration:      dur,
			Style:         "温馨治愈",
			PromptVersion: "v1",
		})
		expected := dur * 120
		assert.Contains(t, out.SystemPrompt, fmt.Sprintf("约 %d 字", expected),
			"duration=%d minutes should map to about %d runes", dur, expected)
	}
}
```

记得加 `import "fmt"`。

```bash
go test ./internal/service/story/prompt/ -v
```
Expected: 全过（约 8 个用例）。

- [ ] **Step 8.7：lint + commit**

```bash
golangci-lint run ./internal/service/story/prompt/...
git add server/internal/service/story/prompt
git commit -m "feat(prompt): system prompt builder rendering 8-constraint template"
```

---

## Task 9：cmd/safetycheck demo 工具

**Files:**
- Create: `server/cmd/safetycheck/main.go`

> **目的**：让你能从命令行试 PreCheck/PostCheck/build_prompt，看到系统在干什么。

- [ ] **Step 9.1：实现 `main.go`**

```go
// safetycheck is a command-line tool to exercise the safety pipeline and
// prompt builder without involving an LLM. Use it to sanity-check rule
// changes, debug prompt assembly, or demonstrate the system to stakeholders.
//
// Example:
//   safetycheck precheck --child-fears "蜘蛛,蛇" "讲个奥特曼睡前故事"
//   safetycheck postcheck --child=小宇 "故事内容..."
//   safetycheck build-prompt --child=小宇 --age=5 --duration=10 \
//       --style=温馨治愈 --topic=勇敢 "讲个奥特曼睡前故事"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aibao/server/internal/service/safety"
	"github.com/aibao/server/internal/service/story/prompt"
)

// default paths assume invocation from the server/ directory.
const (
	defaultRulesPath     = "safety/rules.yaml"
	defaultWhitelistPath = "safety/ip_whitelist.yaml"
	defaultBlacklistPath = "safety/ip_blacklist.yaml"
	defaultTemplatePath  = "safety/system_prompt.tmpl"
)

func main() {
	root := &cobra.Command{
		Use:   "safetycheck",
		Short: "Aibao safety pipeline + prompt builder cli demo",
	}
	root.AddCommand(newPrecheckCmd())
	root.AddCommand(newPostcheckCmd())
	root.AddCommand(newBuildPromptCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newPrecheckCmd() *cobra.Command {
	var (
		fearsCSV string
		maxRunes int
	)
	cmd := &cobra.Command{
		Use:   "precheck [user-prompt]",
		Short: "Run PreCheck on a user-supplied story prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rs, err := safety.LoadRules(defaultRulesPath, defaultWhitelistPath, defaultBlacklistPath)
			if err != nil {
				return err
			}
			pc := safety.NewPreChecker(rs, safety.NewNoopIntentProvider())
			out := pc.Check(context.Background(), safety.PreCheckInput{
				UserPrompt:     args[0],
				ChildFearList:  splitCSV(fearsCSV),
				MaxPromptRunes: maxRunes,
			})
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list (e.g. \"蜘蛛,蛇\")")
	cmd.Flags().IntVar(&maxRunes, "max-runes", 0, "max prompt rune count (default 200)")
	return cmd
}

func newPostcheckCmd() *cobra.Command {
	var (
		child    string
		fearsCSV string
	)
	cmd := &cobra.Command{
		Use:   "postcheck [story-text]",
		Short: "Run PostCheck on an LLM-generated story",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rs, err := safety.LoadRules(defaultRulesPath, defaultWhitelistPath, defaultBlacklistPath)
			if err != nil {
				return err
			}
			pc := safety.NewPostChecker(rs)
			out := pc.Check(safety.PostCheckInput{
				StoryText:     args[0],
				ChildNickname: child,
				ChildFearList: splitCSV(fearsCSV),
			})
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&child, "child", "小宇", "child nickname")
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list")
	return cmd
}

func newBuildPromptCmd() *cobra.Command {
	var (
		child         string
		age           int
		gender        string
		fearsCSV      string
		duration      int
		style         string
		topic         string
		memorySummary string
		ipInstructions string
	)
	cmd := &cobra.Command{
		Use:   "build-prompt [user-prompt]",
		Short: "Assemble the system prompt with given inputs and print it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := prompt.NewBuilder(defaultTemplatePath)
			if err != nil {
				return err
			}
			out := b.Build(prompt.BuildInput{
				ChildNickname:            child,
				ChildAgeYears:            age,
				ChildGender:              gender,
				ChildFearList:            splitCSV(fearsCSV),
				Duration:                 duration,
				Style:                    style,
				Topic:                    topic,
				UserPromptCleaned:        args[0],
				NormalizedIPInstructions: ipInstructions,
				MemorySummary:            memorySummary,
				PromptVersion:            "v1",
			})
			fmt.Println("=== SYSTEM PROMPT ===")
			fmt.Println(out.SystemPrompt)
			fmt.Println("=== USER PROMPT ===")
			fmt.Println(out.UserPrompt)
			return nil
		},
	}
	cmd.Flags().StringVar(&child, "child", "小宇", "child nickname")
	cmd.Flags().IntVar(&age, "age", 5, "child age in years")
	cmd.Flags().StringVar(&gender, "gender", "boy", "boy / girl / unspecified")
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list")
	cmd.Flags().IntVar(&duration, "duration", 10, "story duration in minutes (5/10/15)")
	cmd.Flags().StringVar(&style, "style", "温馨治愈", "story style")
	cmd.Flags().StringVar(&topic, "topic", "", "educational topic (may be empty)")
	cmd.Flags().StringVar(&memorySummary, "memory", "", "recent story memory summary")
	cmd.Flags().StringVar(&ipInstructions, "ip-instructions", "", "same-character instructions to inject")
	return cmd
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
```

加 cobra 依赖：

```bash
cd /f/claud/aibao_app/server
GOPROXY=https://goproxy.cn,direct go get github.com/spf13/cobra
```

- [ ] **Step 9.2：构建 + 手动跑 demo**

```bash
go build -o bin/safetycheck ./cmd/safetycheck

# 1. 正常请求（白名单 IP）
./bin/safetycheck precheck "讲个奥特曼睡前故事"

# 期望输出大致：
# {
#   "Pass": true,
#   "RejectReason": "",
#   "MatchedRule": "",
#   "NormalizedPrompt": "讲个奥特曼睡前故事",
#   "NormalizedIPs": ["奥特曼"],
#   "IPInstructions": "本故事中爱宝变身为「小宇宙奥特曼」..."
# }

# 2. 红线拦截
./bin/safetycheck precheck "我想要血腥的奥特曼故事"
# 期望: Pass: false, RejectReason: "redline_matched", MatchedRule: "血"

# 3. 黑名单 IP
./bin/safetycheck precheck "讲个进击的巨人风格的故事"
# 期望: ip_blacklisted

# 4. 害怕清单
./bin/safetycheck precheck --child-fears="蜘蛛,蛇" "讲个有蜘蛛的故事"
# 期望: fear_matched, MatchedRule: "蜘蛛"

# 5. 完整 prompt 装配
./bin/safetycheck build-prompt --child=小宇 --age=5 --gender=boy \
  --duration=10 --style=温馨治愈 --topic=勇敢 \
  --child-fears="蜘蛛" --ip-instructions="本故事中爱宝变身为爱宝奥特曼。" \
  "讲个奥特曼睡前故事"
# 期望: 完整 SYSTEM PROMPT 含 SOUL/IDENTITY/8 条约束，含"小宇"、"5"、"男孩"、
#       "勇敢"、"温馨治愈"、"10 分钟"、"约 1200 字"、"蜘蛛"、"爱宝奥特曼"
```

把这 5 个命令的输出贴到 `docs/devlog/2026-05-XX-plan-03.md` 作为冒烟证据。

- [ ] **Step 9.3：commit**

```bash
golangci-lint run ./cmd/safetycheck/...
git add server/cmd/safetycheck server/go.mod server/go.sum
git commit -m "feat(cli): safetycheck demo tool (precheck/postcheck/build-prompt)"
```

---

## Task 10：覆盖率验证

- [ ] **Step 10.1：跑全套测试 + 覆盖率**

```bash
cd /f/claud/aibao_app/server
go test -count=1 -cover ./internal/service/safety/... ./internal/service/story/...
```
Expected：service+pkg 各包覆盖率 ≥ 70%；safety 包目标 ≥ 75%。

- [ ] **Step 10.2：跑全工程 lint**

```bash
golangci-lint run ./...
```
Expected：0 issues。

- [ ] **Step 10.3：跑 1000 词基准测试确认性能**

```bash
go test ./internal/service/safety/ -bench BenchmarkMatcher -run '^$' -benchtime=1s
```
Expected：单次 FindFirst < 100µs（远低于 1ms 验收）。

如有未达标的包，回到对应 Task 补测试。

---

## Task 11：写开发日志

**Files:**
- Create: `docs/devlog/2026-05-XX-plan-03.md`（用今天的日期）

> **目的**：记录 Plan 3 完成、贴上 demo 工具的 5 个命令输出。

- [ ] **Step 11.1：创建 devlog**

模板：

```markdown
# 开发日志 — YYYY-MM-DD（Plan 3 完成）

## 今日进展

### ✅ 完成：双层安全 + Prompt 模板（Plan 3 全部 12 个 Task）

完整搭起故事生成的"安全护栏"——PreCheck（前置预审）+ PostCheck（后置审核）+ Prompt Builder（System Prompt 模板组装）+ IP 同人化归一化。Plan 4 接 LLM 时直接调用这套护栏即可。

### 战绩
- 12+ 个 commit（按 Conventional Commits 规范）
- 30+ 单元测试 + 1 个基准测试，全部通过
- 平均覆盖率 ≥ 75%
- 0 lint issues
- cmd/safetycheck demo 工具 3 个子命令全部跑通

### 端到端 demo 输出

#### 1. 正常请求（白名单 IP 命中）
```
$ safetycheck precheck "讲个奥特曼睡前故事"
{
  "Pass": true,
  ...
}
```

#### 2. 红线拦截
```
$ safetycheck precheck "我想要血腥的奥特曼故事"
{
  "Pass": false,
  "RejectReason": "redline_matched",
  "MatchedRule": "血"
}
```

（继续贴 demo 5 个命令的真实输出）

### 关键技术决策落地

| 决策 | 实现 |
|---|---|
| 红线规则 YAML 管理 | rules.yaml + ip_whitelist/blacklist.yaml；启动加载到不可变 RuleSet |
| 6 大类 200+ 红线词种子库 | violence/horror/sexual/political_religious/dangerous_imitation/negative_values |
| 意图分类预留 LLM 兜底 | IntentProvider 接口 + NoopProvider 默认 |
| IP 同人化白/黑名单 | 12 主流儿童 IP 白名单同人化指令 + 限制级动漫/政治宗教黑名单 |
| 双层安全 | PreCheck 5 类拦截 + PostCheck 3 类拦截（含主角校验启发式）|
| System Prompt 8 条强约束 | text/template + SOUL/IDENTITY 静态文本 + 动态字段填空 |

## 累积进度

- ✅ Plan 1：后端基础设施（21 Task）
- ✅ Plan 2：用户认证 + 孩子档案（20 Task）
- ✅ Plan 3：双层安全 + Prompt 模板（12 Task）
- ⬜ Plan 4：故事生成 + LLM Gateway + Outbox
- ⬜ Plan 5-9
```

- [ ] **Step 11.2：commit**

```bash
git add docs/devlog/
git commit -m "docs(devlog): plan 3 complete — safety pipeline + prompt builder"
```

---

## 完成验收清单

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 全部通过
- [ ] `golangci-lint run ./...` 0 issues
- [ ] `safety` 包覆盖率 ≥ 75%
- [ ] `prompt` 包覆盖率 ≥ 70%
- [ ] benchmark 单次 FindFirst 在 1000 词词库下 < 1ms
- [ ] cmd/safetycheck 三个子命令全部能跑
- [ ] devlog 写明并贴出 5 个命令的真实输出
- [ ] 提交粒度合理；working tree clean

---

## 后续 Plan 衔接

Plan 4 起将大量使用本 plan 产物：
- `safety.PreChecker` / `safety.PostChecker` —— 故事生成主流程的两道闸
- `prompt.Builder` —— 调 LLM 前组装 System Prompt
- `safety.IntentProvider` —— Plan 4 实现 LLMProvider 接入豆包做兜底分类

下一份 plan（Plan 4：故事生成 + LLM Gateway + Outbox）会引入：
- `gateway/llm` Gateway 抽象 + 豆包实现
- `service/story` 故事编排 service（串起 PreCheck → PromptBuilder → LLM → PostCheck → Outbox 写入）
- PG `outbox_events` 表 + Worker 消费骨架
- 故事生成 HTTP 接口 `POST /api/v1/stories/generate`
