# Plan 6：BOOTSTRAP 首次相遇仪式 + MEMORY 深化与彩蛋串联

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让爱宝"认识"孩子，并且"记得"上次讲过什么。完成后端到端可演示：家长第一次创建孩子档案后，App 引导走完 6-8 题表单 → 后端调一次便宜 LLM 把答案润色成 80-150 字自然语言 `description` 写入 `children.profile` → 下一次生成故事时，System Prompt 里多了"孩子画像"段；并且 Worker 在每个故事完成后调一次便宜 LLM 写一句 30 字内的总结存进 `memories(memory_type='story_summary')`，下次故事开始前 Orchestrator 拉最近 3 条注入 Prompt 的"记忆上下文"段。两个能力叠加，第 2 个故事开始就有概率自然回调第 1 个故事里的元素（"还记得上次那只小恐龙吗？"）——"有生命的伙伴"卖点首次真正落地。

**Architecture:** 仍然是同步 + 异步两条线，**不引入任何新外部服务**。同步线：BOOTSTRAP 走两个新接口（`GET /bootstrap/questions` 纯静态、`POST /bootstrap/answers` 内部调一次便宜 LLM 后写 `children.profile`）；故事生成同步段在 `builder.Build` 之前多查一次 `MemorySelector.BuildContext` 把最近 3 条记忆拼成一行字塞进 `prompt.BuildInput.MemorySummary`（字段 Plan 4 已预留，模板 Plan 4 已写 `{{if .MemorySummary}}` 分支）。异步线：复用 Plan 4 的 Outbox Worker，扩 `MemoryUpdateHandler.Handle`——在既有 element extract + memory insert 之后多调一次 `Summarizer.Summarize` 拿 30 字总结，再插一行 `memory_type='story_summary'` 带 `story_id`。所有 LLM 调用走既有 `gateway/llm.Client`，用 `cfg.LLM.IntentModel`（便宜，~0.005 元/次）。**fail-open 哲学**：BOOTSTRAP LLM 失败仍存原始答案（description 为空）；MEMORY 总结 LLM 失败不影响故事生成、不影响主 memory_update。

**Tech Stack:**
- Go 1.24+ + Gin + GORM + PostgreSQL（已有）
- 复用 Plan 4 的 `gateway/llm.Client`（Doubao + Mock）—— **本 plan 不新增任何 gateway**
- 复用 Plan 4 Outbox Worker + memory_update handler 链
- 复用 Plan 1-5：repository / userctx / api.RespondError / metrics / pkg/config / safety/system_prompt.tmpl

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)
  - 第 3.5 USER 文件、3.6 MEMORY 文件、3.9 BOOTSTRAP 文件（**核心**——本 plan 的产品依据）
  - 第 5.1 BOOTSTRAP 仪式（**核心**——6-8 题的问题列表和顺序由此而来）
  - 第 5.4 串联机制（"记忆即关爱"——为什么必须自然回调而不是堆砌）
  - 第 7 章红线（孩子画像里"害怕的东西"是 PreCheck 的输入，本 plan 不直接用但要为 Plan 7 留口）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - 第 4 章数据流（**核心**——同步段新增一次便宜 LLM 调用是有意识的成本决策）
  - 第 5 章 `children.profile` JSONB 与 `memories` 表
  - 第 6 章 Gateway 抽象（复用，不新增）
  - 第 7 章 Prompt Builder（本 plan 只追加一个上下文段）
- Plan 4：[2026-05-08-plan-04-story-generation.md](2026-05-08-plan-04-story-generation.md)（**必读**——本 plan 直接扩 Plan 4 的 `MemoryUpdateHandler` 与 Orchestrator）
- Plan 5：[2026-05-09-plan-05-audio-tts-storage.md](2026-05-09-plan-05-audio-tts-storage.md)（参考其"事件载荷极简、handler 现取最新数据"的设计哲学）
- CLAUDE.md（4.2 内容安全；4.4 注释/文档风格 - 不写 LLM 套话注释；第 7 章必须解释知识点 + 同步落 `docs/knowledge/`）

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；新增 service+api 覆盖率 ≥ 70%
2. `make migrate-up` 应用 `000005_memories_story_id` 后，`\d memories` 显示新增列 `story_id bigint NULL` 和外键到 `stories(id) ON DELETE SET NULL`，并存在新的部分索引 `memories_child_story_summary_idx`
3. `make run-dev` 启动后能完成完整流程（curl/PowerShell 演示）：
   - 前置：登录 + 创建孩子（Plan 2 流程，profile 此时为 `{}`）
   - `GET /api/v1/bootstrap/questions` → 200，返回 7 个问题的固定 JSON spec（含 id/label/type/options/required）
   - `POST /api/v1/bootstrap/answers {child_id, answers:[...]}` → 200，返回 `{description: "...80-150 字...", version:1}`；数据库 `children.profile` 已写
   - `POST /api/v1/stories/generate` 第一次 → 200，故事文本可读；服务日志可见 `prompt.memory_context=empty`
   - 等待 5-15 秒（Worker 消费 memory_update）→ `psql ... SELECT memory_type, payload FROM memories WHERE child_id=...` 看到 2 行：一条原始 `story_summary`（Plan 4 既有），一条新的 `story_summary` 带 `story_id` 且 payload.summary 是 30 字内中文
   - `POST /api/v1/stories/generate` 第二次（同孩子）→ 200；服务日志可见 `prompt.memory_context=non_empty count=1+`；返回的故事文本在 manual 检查下显示出对前一故事元素/情感的自然延续可能性（不强制硬命中）
4. BOOTSTRAP 是**可选**的：profile 仍为 `{}` 的孩子调 `/stories/generate` 不报错，prompt builder 走"首次故事"分支
5. LLM 失败 fail-open 链路全部覆盖：
   - BOOTSTRAP 时 LLM 报错 → 仍 200 返回，`description` 为空、`answers` 已保存（用户可重试）
   - Worker memory summarize 报错 → 第一行 memory_update 已成功落表，故事生成完全不受影响
   - Orchestrator MemorySelector 报错 → 走空 MemorySummary 路径继续生成
6. 所有权校验：用户 A 拿用户 B 的 child_id 调 `/bootstrap/answers` → 403 `not_owner`
7. 业务 metrics 在 `/metrics` 可见：`memory_summary_duration_seconds`、`memory_summary_total{status="ok|fail"}`、`bootstrap_completion_total`
8. 单元测试**不打**任何真 LLM——`memory.Summarizer` / `bootstrap.Service` 测试全部用 `gateway/llm/mock.go`
9. `golangci-lint run ./...` 0 issues

---

## 范围决策记录（与用户对齐）

| 维度 | 决策 |
|---|---|
| BOOTSTRAP 形态 | **表单驱动**（6-8 题），不做聊天驱动。理由：可控、可回填、可版本化、客户端实现成本最低 |
| BOOTSTRAP 是否必填 | **可选**。未走 BOOTSTRAP 的孩子 `profile={}`，故事照样生成（Plan 4 已能跑）|
| BOOTSTRAP LLM 模型 | `cfg.LLM.IntentModel`（豆包 lite，便宜）；temperature=0.3；max_tokens=300 |
| BOOTSTRAP 输出长度 | 80-150 字自然语言段落（system prompt 中限定）|
| profile JSONB schema | `{"description":"...","answers":{...原始...},"version":1}`——version 留升级口 |
| 问题列表 | 7 题固定：性格关键词 / 喜欢的角色 IP / 害怕的东西 / 家庭成员名字 / 喜欢的故事风格 / 教育主题倾向 / 是否开启连续剧（连续剧仅记录，Plan 7 才用）|
| 连续剧 | **本 plan 不实现**。`stories.storyline_id/episode_no` 列保持 NULL；BOOTSTRAP 答案存进 profile.answers 备查 |
| MEMORY 总结 LLM | `cfg.LLM.IntentModel`（同上）；temperature=0.2；max_tokens=80 |
| MEMORY 总结输出 | 30 字以内中文一句话，超出软截断 |
| 记忆注入策略 | 取该孩子最近 3 条 `memory_type IN ('story_summary','interest')`，按 `created_at DESC`，payload.summary 字段以 "；" 拼接为单行字符串 |
| 记忆为空时的 Prompt | 模板已有 `{{if .MemorySummary}}` 分支（Plan 4 实现），首次走"无记忆"路径不需要改 |
| Memory payload schema | 本 plan 标准化为 `{"type":"story_summary","summary":"...","story_id":N,"title":"...","used_fallback":false}`——Plan 4 既有的字段子集 + 新增 `summary`/`type` 显式字段 |
| Outbox payload 是否扩 | **不扩**。沿用 Plan 5 哲学——event payload 极简，handler 进来后用 story_id 现查 `stories.text_content`。这样后续如加"重生成故事"功能，summary 始终对应最新 text |
| memories.story_id 列 | **新增**（nullable FK + ON DELETE SET NULL）。理由：方便运维查"故事 X 衍生了哪些记忆"，且对话级 memory 没有 story 关联可以为空 |
| 失败兜底 | 全部 fail-open（不阻塞主流程）；metrics 计数 fail 让运维监控 |
| 鉴权 | BOOTSTRAP 两个接口都挂 JWT 中间件；POST 做所有权校验；不挂限流/预算（量小、零外部 LLM 成本——其实有一次 LLM 但每个孩子一辈子就一次） |
| 知识库联动 | 新增两条词条：(05.X) "向后兼容的接口预留：MemorySummary 字段 Plan 4 空跑、Plan 6 才填的设计"；(11.X) "记忆即软提示——为什么把历史记忆作为 system prompt 的 hint 段而不是 RAG/few-shot" |

---

## File Structure

### 数据迁移

| 文件 | 职责 |
|---|---|
| `server/migrations/000005_memories_story_id.up.sql` | `memories.story_id` 列 + 部分索引 |
| `server/migrations/000005_memories_story_id.down.sql` | 反向 |

### Data model + Repo

| 文件 | 修改 |
|---|---|
| `server/internal/model/story.go` | `Memory.StoryID *int64`；常量 `MemoryTypeStorySummary` / `MemoryTypeInterest` / `MemoryTypePreference` |
| `server/internal/repository/memory_repo.go` | 新增 `RecentByChildTypes(ctx, childID, types []string, limit)` —— 不破坏既有 `RecentByChild` |
| `server/internal/repository/memory_repo_test.go` | 集成测试（testcontainers）补 RecentByChildTypes 用例 |

### Memory 服务（新包）

| 文件 | 职责 |
|---|---|
| `server/internal/service/memory/summarizer.go` | `Summarizer` 结构 + `Summarize(ctx, storyText)` LLM 调用 |
| `server/internal/service/memory/summarizer_test.go` | 用 MockLLM 测系统 prompt、长度截断、fail-open |
| `server/internal/service/memory/selector.go` | `Selector` 结构 + `BuildContext(ctx, childID)` 拼字符串 |
| `server/internal/service/memory/selector_test.go` | fakeMemoryRepo 用例 |

### Bootstrap 服务（新包）

| 文件 | 职责 |
|---|---|
| `server/internal/service/bootstrap/questions.go` | 7 题固定定义 + `Questions()` 返回切片 |
| `server/internal/service/bootstrap/questions_test.go` | schema 完整性 + JSON 可序列化 |
| `server/internal/service/bootstrap/service.go` | `Service` 结构 + `Validate(answers)` + `BuildProfile(ctx, child, answers)` |
| `server/internal/service/bootstrap/service_test.go` | MockLLM + fakeChildRepo 全路径用例 |

### Worker handler 扩展

| 文件 | 修改 |
|---|---|
| `server/internal/worker/handlers/memory_update.go` | Handle 末尾追加：调 Summarizer，成功则插第二行 `memory_type='story_summary'` 带 `story_id` |
| `server/internal/worker/handlers/memory_update_test.go` | 补两个用例：summary 成功路径 + summary 失败路径（确保第一行 memory 仍 OK） |

### Orchestrator 扩展

| 文件 | 修改 |
|---|---|
| `server/internal/service/story/orchestrator.go` | 注入 `MemorySelector`；`builder.Build` 前调 `BuildContext` 拼入 `BuildInput.MemorySummary`；fail-open |
| `server/internal/service/story/orchestrator_test.go` | 调用点更新 + 两个用例（memory non-empty / memory error） |

### Safety template

| 文件 | 修改 |
|---|---|
| `server/safety/system_prompt.tmpl` | 验证既有"最近故事记忆"段渲染正确；若未有"无记忆"分支，补一段"（首次故事，无记忆上下文）" |

### API 层

| 文件 | 职责 |
|---|---|
| `server/internal/api/bootstrap.go` | `BootstrapHandler` + 2 个 endpoint + 所有权校验 |
| `server/internal/api/bootstrap_test.go` | handler 测试 |
| `server/internal/api/router.go` | `RouterDeps.Bootstrap`；JWT 组下挂载 |

### Metrics

| 文件 | 修改 |
|---|---|
| `server/internal/metrics/business.go` | +`MemorySummaryDuration` Histogram、`MemorySummaryTotal` CounterVec、`BootstrapCompletionTotal` Counter |
| `server/internal/metrics/business_test.go` | 用例追加 |

### Service child 微调

| 文件 | 修改 |
|---|---|
| `server/internal/service/child/child.go` | `UpdateInput` 加可选 `Profile *[]byte`；Update 时若非 nil 则覆盖（bootstrap 用，不污染既有用法） |
| `server/internal/service/child/child_test.go` | 用例追加 profile-only 更新 |

### 装配

| 文件 | 修改 |
|---|---|
| `server/cmd/server/main.go` | 构建 `Summarizer` / `Selector` / `bootstrap.Service`；注入 `MemoryUpdateHandler` + `StoryOrchestrator` + `RouterDeps.Bootstrap` |

---

## API 形态（先定好契约）

### GET `/api/v1/bootstrap/questions`
带 Bearer JWT。返回静态题库，**不**调 LLM。

**Response 200:**

```json
{
  "version": 1,
  "questions": [
    {
      "id": "personality_traits",
      "label": "你觉得孩子身上有哪些性格关键词？（多选，1-3 个）",
      "type": "multi_select",
      "required": true,
      "options": ["勇敢", "细心", "温柔", "调皮", "好奇", "安静", "开朗", "敏感"]
    },
    {
      "id": "favorite_characters",
      "label": "孩子最喜欢哪些角色或动画形象？（用顿号分隔，如：奥特曼、小猪佩奇）",
      "type": "text",
      "required": true,
      "max_length": 80
    },
    {
      "id": "fears",
      "label": "孩子目前比较怕什么？（1-3 项，用顿号分隔；可填'暂无'）",
      "type": "text",
      "required": true,
      "max_length": 60
    },
    {
      "id": "family_members",
      "label": "故事里可以提及哪些家人？（如：爸爸、妈妈、奶奶、弟弟）",
      "type": "text",
      "required": false,
      "max_length": 60
    },
    {
      "id": "story_style",
      "label": "孩子最喜欢哪种故事风格？",
      "type": "single_select",
      "required": true,
      "options": ["温馨治愈", "冒险探索", "搞笑欢乐", "神奇魔法", "科普认知"]
    },
    {
      "id": "education_themes",
      "label": "你希望故事里多一点哪些主题？（多选）",
      "type": "multi_select",
      "required": false,
      "options": ["勇敢", "友谊", "诚实", "分享", "坚持", "好奇心", "情绪管理"]
    },
    {
      "id": "enable_storyline",
      "label": "是否开启'连续剧'模式（同一系列故事会延续角色和情节）？",
      "type": "boolean",
      "required": true
    }
  ]
}
```

### POST `/api/v1/bootstrap/answers`
带 Bearer JWT。

**Request:**

```json
{
  "child_id": 7,
  "answers": [
    {"q_id": "personality_traits", "value": ["勇敢", "好奇"]},
    {"q_id": "favorite_characters", "value": "奥特曼、小恐龙"},
    {"q_id": "fears", "value": "打雷、黑暗"},
    {"q_id": "family_members", "value": "爸爸、妈妈、奶奶"},
    {"q_id": "story_style", "value": "冒险探索"},
    {"q_id": "education_themes", "value": ["勇敢", "好奇心"]},
    {"q_id": "enable_storyline", "value": false}
  ]
}
```

**Response 200:**

```json
{
  "child_id": 7,
  "version": 1,
  "description": "小宇是一个勇敢又充满好奇心的孩子，喜欢和奥特曼、小恐龙一起冒险，目前对打雷和黑暗有些害怕。家里有爸爸、妈妈和奶奶。最喜欢的故事风格是冒险探索，希望多接触勇敢和好奇心方面的成长主题。"
}
```

**错误：**
- 400 `invalid_argument`（缺必填、type 不匹配、option 不在白名单）
- 400 `child_not_found`
- 403 `not_owner`
- 401 无 token

🎓 **为什么 description 是 LLM 润色而不是模板拼接？** 模板拼接（"喜欢 X、害怕 Y、想学 Z"）出来的句子刚性、僵硬，传给故事 LLM 之后它会照搬这个生硬感。让一个便宜 LLM 先把答案"叙述化"成自然中文段落，故事 LLM 接到的是一个"活的孩子画像"，写出来的故事也会更柔软。一次性 0.005 元，每个孩子一辈子一次，性价比极高。

---

## 数据模型字段约定（仅新增/变化）

### memories 表新增列

| 字段 | 类型 | 说明 |
|---|---|---|
| story_id | bigint NULL REFERENCES stories(id) ON DELETE SET NULL | 仅 story_summary 类型会填 |

索引（新增）：

```sql
CREATE INDEX memories_child_story_summary_idx
    ON memories(child_id, created_at DESC)
    WHERE memory_type = 'story_summary';
```

### children.profile JSONB schema（标准化）

```json
{
  "version": 1,
  "description": "小宇是一个勇敢又好奇心强的孩子……",
  "answers": {
    "personality_traits": ["勇敢", "好奇"],
    "favorite_characters": "奥特曼、小恐龙",
    "fears": "打雷、黑暗",
    "family_members": "爸爸、妈妈、奶奶",
    "story_style": "冒险探索",
    "education_themes": ["勇敢", "好奇心"],
    "enable_storyline": false
  }
}
```

- 未走 BOOTSTRAP 的孩子 `profile = {}`，prompt builder 检测到没有 `description` 字段时走"首次故事"分支
- `version` 留升级口：未来如果改问题，老 profile 仍可读

### memories.payload JSONB schema（标准化）

Plan 4 既有 + 本 plan 显式化：

```json
{
  "type": "story_summary",
  "summary": "小宇和爱宝一起救了迷路的小恐龙，学会了勇敢。",
  "story_id": 42,
  "title": "小宇和爱宝的森林冒险",
  "used_fallback": false
}
```

🎓 **为什么 memories.payload 用 jsonb 而不是规范化列？** 记忆类型未来会增长（feedback / preference / interaction），每种 schema 不同。jsonb 让我们不必为每种新增类型迁库；同时 PostgreSQL 对 jsonb 有索引能力，未来真的需要按字段查询时可以补 GIN。

---

# Tasks

## Task 0：迁移文件 `000005_memories_story_id`

**Files:**
- Create: `server/migrations/000005_memories_story_id.up.sql`
- Create: `server/migrations/000005_memories_story_id.down.sql`

- [ ] **Step 0.1：up SQL**

`server/migrations/000005_memories_story_id.up.sql`：

```sql
ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS story_id BIGINT NULL REFERENCES stories(id) ON DELETE SET NULL;

-- Partial index for the hot read path: latest story_summary per child.
CREATE INDEX IF NOT EXISTS memories_child_story_summary_idx
    ON memories(child_id, created_at DESC)
    WHERE memory_type = 'story_summary';
```

- [ ] **Step 0.2：down SQL**

```sql
DROP INDEX IF EXISTS memories_child_story_summary_idx;
ALTER TABLE memories
    DROP COLUMN IF EXISTS story_id;
```

- [ ] **Step 0.3：跑迁移**

```bash
cd server && make migrate-up
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\d memories" | grep story_id
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\di memories_child_story_summary_idx"
```

Expected：`story_id | bigint`；索引存在并显示 `WHERE memory_type = 'story_summary'::text`。

- [ ] **Step 0.4：commit**

```bash
git add server/migrations/000005_memories_story_id.up.sql \
        server/migrations/000005_memories_story_id.down.sql
git commit -m "feat(db): memories.story_id + partial index for story_summary lookups"
```

---

## Task 1：扩展业务 metrics

**Files:**
- Modify: `server/internal/metrics/business.go`
- Modify: `server/internal/metrics/business_test.go`

- [ ] **Step 1.1：新增 3 个指标**

在 `Business` struct 上加：

```go
MemorySummaryDuration   prometheus.Histogram
MemorySummaryTotal      *prometheus.CounterVec // labels: status (ok|fail)
BootstrapCompletionTotal prometheus.Counter
```

`NewBusiness(reg *prometheus.Registry)` 中注册：

```go
b.MemorySummaryDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "memory_summary_duration_seconds",
    Help:    "Latency of the cheap LLM call that summarizes a finished story into a one-sentence memory.",
    Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),
})
b.MemorySummaryTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "memory_summary_total",
    Help: "Count of memory-summary LLM calls.",
}, []string{"status"})
b.BootstrapCompletionTotal = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "bootstrap_completion_total",
    Help: "Count of successful BOOTSTRAP answer submissions.",
})
reg.MustRegister(b.MemorySummaryDuration, b.MemorySummaryTotal, b.BootstrapCompletionTotal)
```

- [ ] **Step 1.2：测试**

按 `business_test.go` 既有模式断言 3 个 metric 存在、label 正确。

- [ ] **Step 1.3：跑 + commit**

```bash
cd server && go test ./internal/metrics/...
git add server/internal/metrics/business.go server/internal/metrics/business_test.go
git commit -m "feat(metrics): memory_summary + bootstrap_completion business metrics"
```

---

## Task 2：扩展 Memory model

**Files:**
- Modify: `server/internal/model/story.go`

- [ ] **Step 2.1：Memory 加 StoryID 字段 + 常量**

```go
type Memory struct {
    ID         int64     `gorm:"primaryKey;column:id"`
    ChildID    int64     `gorm:"column:child_id;index"`
    MemoryType string    `gorm:"column:memory_type"`
    Payload    []byte    `gorm:"column:payload;type:jsonb"`
    Weight     float64   `gorm:"column:weight"`
    StoryID    *int64    `gorm:"column:story_id"` // Plan 6: nullable FK
    CreatedAt  time.Time `gorm:"column:created_at"`
}

// Memory type constants.
const (
    MemoryTypeStorySummary = "story_summary"
    MemoryTypeInterest     = "interest"
    MemoryTypePreference   = "preference"
)
```

- [ ] **Step 2.2：build + commit**

```bash
cd server && go build ./...
git add server/internal/model/story.go
git commit -m "feat(model): Memory.StoryID nullable FK + memory type constants"
```

---

## Task 3：MemoryRepo 新增 RecentByChildTypes

**Files:**
- Modify: `server/internal/repository/memory_repo.go`
- Modify: `server/internal/repository/memory_repo_test.go`

**目的：** 既有 `RecentByChild` 只接受单 type，Plan 6 需要 `IN (...)` 多 type。新增方法、不改老方法（向后兼容）。

- [ ] **Step 3.1：扩接口与实现**

```go
type MemoryRepo interface {
    Create(ctx context.Context, m *model.Memory) error
    RecentByChild(ctx context.Context, childID int64, memoryType string, limit int) ([]*model.Memory, error)
    // RecentByChildTypes filters by IN (types...). If types is empty, returns nothing.
    RecentByChildTypes(ctx context.Context, childID int64, types []string, limit int) ([]*model.Memory, error)
}

func (r *memoryRepo) RecentByChildTypes(ctx context.Context, childID int64, types []string, limit int) ([]*model.Memory, error) {
    if len(types) == 0 {
        return []*model.Memory{}, nil
    }
    if limit <= 0 {
        limit = 10
    }
    var out []*model.Memory
    err := r.db.WithContext(ctx).
        Where("child_id = ? AND memory_type IN ?", childID, types).
        Order("created_at DESC").
        Limit(limit).
        Find(&out).Error
    return out, err
}
```

- [ ] **Step 3.2：集成测试**

按既有 testcontainers 模式补两个用例：
- 插入 3 条 story_summary + 2 条 interest + 1 条 preference；查 `["story_summary","interest"]` limit=3 → 应得 3 条按 created_at desc
- `types=[]` → 返回空数组、不报错

- [ ] **Step 3.3：跑 + commit**

```bash
cd server && go test ./internal/repository/... -run TestMemory
git add server/internal/repository/memory_repo.go server/internal/repository/memory_repo_test.go
git commit -m "feat(repo): MemoryRepo.RecentByChildTypes for multi-type recall"
```

---

## Task 4：`service/memory.Summarizer`

**Files:**
- Create: `server/internal/service/memory/summarizer.go`
- Create: `server/internal/service/memory/summarizer_test.go`

**目的：** 给一段故事文本，调便宜 LLM 返回 30 字内中文一句话总结。失败返回空 + 记日志，**不返回 error**（fail-open 由上层用空判断决定是否插记忆）。

- [ ] **Step 4.1：summarizer.go**

```go
// Package memory provides post-story memory writing (summarizer) and
// pre-story memory injection (selector). Both layers are deliberately
// fail-open: a failure here must NEVER block story generation.
package memory

import (
    "context"
    "log/slog"
    "strings"
    "time"
    "unicode/utf8"

    "github.com/aibao/server/internal/gateway/llm"
    "github.com/aibao/server/internal/metrics"
)

const summarizerSystemPrompt = `你是一个儿童故事总结器。请用一句不超过 30 个汉字的中文，总结下面这个儿童故事的主要情节和情感。只输出这句话本身，不要加引号、不要解释、不要其他说明。`

// Summarizer turns a finished story text into a single-sentence memory.
type Summarizer struct {
    client      llm.Client
    model       string
    temperature float64
    biz         *metrics.Business
    logger      *slog.Logger
}

// NewSummarizer constructs a Summarizer.
func NewSummarizer(client llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Summarizer {
    return &Summarizer{client: client, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Summarize returns a <=30-char Chinese sentence or "" on any error.
func (s *Summarizer) Summarize(ctx context.Context, storyText string) string {
    if strings.TrimSpace(storyText) == "" {
        return ""
    }
    start := time.Now()
    out, err := s.client.Generate(ctx, llm.GenerateRequest{
        Model:       s.model,
        Temperature: s.temperature,
        MaxTokens:   80,
        Messages: []llm.Message{
            {Role: "system", Content: summarizerSystemPrompt},
            {Role: "user", Content: storyText},
        },
    })
    dur := time.Since(start).Seconds()
    if s.biz != nil {
        s.biz.MemorySummaryDuration.Observe(dur)
    }
    if err != nil {
        if s.biz != nil {
            s.biz.MemorySummaryTotal.WithLabelValues("fail").Inc()
        }
        s.logger.Warn("memory.summarize.fail", "err", err)
        return ""
    }
    if s.biz != nil {
        s.biz.MemorySummaryTotal.WithLabelValues("ok").Inc()
    }
    return truncateChinese(strings.TrimSpace(out.Text), 30)
}

// truncateChinese trims to N runes if longer (soft cap). LLM is asked
// for <=30 chars; this is belt-and-suspenders.
func truncateChinese(s string, n int) string {
    if utf8.RuneCountInString(s) <= n {
        return s
    }
    runes := []rune(s)
    return string(runes[:n])
}
```

- [ ] **Step 4.2：summarizer_test.go**

用 `gateway/llm.MockLLM` 覆盖：
- 正常路径：mock 返回 `"小宇和爱宝救了小恐龙，学会了勇敢"` → Summarize 返回原文不截断
- 超长返回：mock 返回 50 字 → 截到 30
- LLM 报错：Summarize 返回 ""；metrics `MemorySummaryTotal{status=fail}` 计数 +1
- 空 storyText：直接返 ""，不调 LLM（断言 MockLLM 调用次数=0）

- [ ] **Step 4.3：跑 + commit**

```bash
cd server && go test ./internal/service/memory/...
git add server/internal/service/memory/summarizer.go server/internal/service/memory/summarizer_test.go
git commit -m "feat(memory): Summarizer turns finished story into one-sentence memory"
```

---

## Task 5：`service/memory.Selector`

**Files:**
- Create: `server/internal/service/memory/selector.go`
- Create: `server/internal/service/memory/selector_test.go`

**目的：** 查孩子近期记忆，拼成一行字。给 Orchestrator 用。

- [ ] **Step 5.1：selector.go**

```go
package memory

import (
    "context"
    "encoding/json"
    "log/slog"
    "strings"

    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/repository"
)

// Selector reads recent memories and renders them as a single line of
// "soft hints" for the story system prompt.
type Selector struct {
    repo   repository.MemoryRepo
    types  []string
    limit  int
    logger *slog.Logger
}

// NewSelector constructs a Selector with sensible defaults.
func NewSelector(repo repository.MemoryRepo, logger *slog.Logger) *Selector {
    return &Selector{
        repo:   repo,
        types:  []string{model.MemoryTypeStorySummary, model.MemoryTypeInterest},
        limit:  3,
        logger: logger,
    }
}

// BuildContext returns "" when no memories exist OR on repo error
// (fail-open). On success returns "；" joined summaries newest-first.
func (s *Selector) BuildContext(ctx context.Context, childID int64) string {
    rows, err := s.repo.RecentByChildTypes(ctx, childID, s.types, s.limit)
    if err != nil {
        s.logger.Warn("memory.selector.fail", "child_id", childID, "err", err)
        return ""
    }
    if len(rows) == 0 {
        return ""
    }
    parts := make([]string, 0, len(rows))
    for _, m := range rows {
        if sum := extractSummary(m.Payload); sum != "" {
            parts = append(parts, sum)
        }
    }
    return strings.Join(parts, "；")
}

func extractSummary(payload []byte) string {
    if len(payload) == 0 {
        return ""
    }
    var p struct {
        Summary string `json:"summary"`
    }
    if err := json.Unmarshal(payload, &p); err != nil {
        return ""
    }
    return strings.TrimSpace(p.Summary)
}
```

- [ ] **Step 5.2：selector_test.go**

用 fakeMemoryRepo（按 Plan 4 模式）：
- 3 条记忆 → 输出 "A；B；C" 顺序为 created_at desc
- 0 条 → ""
- repo 报错 → ""，不 panic
- 某一条 payload 缺 summary 字段 → 跳过、其他正常

- [ ] **Step 5.3：跑 + commit**

```bash
cd server && go test ./internal/service/memory/...
git add server/internal/service/memory/selector.go server/internal/service/memory/selector_test.go
git commit -m "feat(memory): Selector builds prompt context from recent memories (fail-open)"
```

---

## Task 6：BOOTSTRAP 问题定义

**Files:**
- Create: `server/internal/service/bootstrap/questions.go`
- Create: `server/internal/service/bootstrap/questions_test.go`

- [ ] **Step 6.1：questions.go**

```go
// Package bootstrap implements the "first encounter" form-driven onboarding
// that polishes parent-supplied answers into a natural-language child
// profile description used by the story system prompt.
package bootstrap

// QuestionType enumerates supported answer shapes.
type QuestionType string

const (
    TypeText         QuestionType = "text"
    TypeSingleSelect QuestionType = "single_select"
    TypeMultiSelect  QuestionType = "multi_select"
    TypeBoolean      QuestionType = "boolean"
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
```

- [ ] **Step 6.2：测试**

- 7 个问题、id 不重复、required 至少 5 个
- JSON 可序列化 round-trip
- `QuestionByID("favorite_characters").Type == TypeText`
- `QuestionByID("unknown")` → ok=false

- [ ] **Step 6.3：跑 + commit**

```bash
cd server && go test ./internal/service/bootstrap/...
git add server/internal/service/bootstrap/questions.go server/internal/service/bootstrap/questions_test.go
git commit -m "feat(bootstrap): fixed 7-question BOOTSTRAP form definition"
```

---

## Task 7：BOOTSTRAP Service（validate + LLM polish）

**Files:**
- Create: `server/internal/service/bootstrap/service.go`
- Create: `server/internal/service/bootstrap/service_test.go`
- Modify: `server/internal/service/child/child.go`（UpdateInput 加 Profile）
- Modify: `server/internal/service/child/child_test.go`

- [ ] **Step 7.1：child.UpdateInput 加 Profile**

`server/internal/service/child/child.go`：

```go
type UpdateInput struct {
    Nickname *string
    Gender   *string
    Birthday *time.Time
    Profile  *[]byte // Plan 6: BOOTSTRAP-rendered profile JSON. nil = don't touch.
}
```

`Update` 函数末尾、`s.repo.Update` 前加：

```go
if in.Profile != nil {
    c.Profile = *in.Profile
}
```

补一个测试：`TestUpdate_ProfileOnly` 断言只传 Profile 时 Nickname/Gender 不变。

- [ ] **Step 7.2：service.go**

```go
package bootstrap

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"

    "github.com/aibao/server/internal/gateway/llm"
    "github.com/aibao/server/internal/metrics"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/pkg/apperr"
    "github.com/aibao/server/internal/repository"
    childsvc "github.com/aibao/server/internal/service/child"
)

const bootstrapSystemPrompt = `你是儿童故事 App 的画像生成器。给你一组父母填写的孩子信息，请用 80-150 字的自然中文段落，描绘出这个孩子。要求：第三人称、温柔、具体（要把家人名字、害怕的东西、喜欢的角色等具体细节融入）；只输出段落本身，不要列表、不要标题、不要解释。`

// Answer is one submitted answer keyed by Question.ID.
// Value is interface{} — must match the Question's type at validation time.
type Answer struct {
    QID   string      `json:"q_id"`
    Value interface{} `json:"value"`
}

// Profile is the JSONB shape persisted into children.profile.
type Profile struct {
    Version     int                    `json:"version"`
    Description string                 `json:"description"`
    Answers     map[string]interface{} `json:"answers"`
}

// Service builds a profile from BOOTSTRAP answers.
type Service struct {
    children    *childsvc.Service
    llmClient   llm.Client
    model       string
    temperature float64
    biz         *metrics.Business
    logger      *slog.Logger
}

// NewService constructs.
func NewService(children *childsvc.Service, c llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Service {
    return &Service{children: children, llmClient: c, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Submit validates answers, calls LLM to render description (fail-open),
// and writes back into children.profile.
func (s *Service) Submit(ctx context.Context, userID, childID int64, answers []Answer) (*Profile, error) {
    answersMap, err := s.validate(answers)
    if err != nil {
        return nil, err
    }
    description := s.renderDescription(ctx, answersMap) // "" on LLM error (fail-open)
    profile := &Profile{Version: Version, Description: description, Answers: answersMap}
    raw, err := json.Marshal(profile)
    if err != nil {
        return nil, apperr.Wrap(err, apperr.CodeInternal, "profile_marshal_failed", "服务暂时不可用")
    }
    if _, err := s.children.Update(ctx, userID, childID, childsvc.UpdateInput{Profile: &raw}); err != nil {
        return nil, err // childsvc maps not_owner / not_found
    }
    if s.biz != nil {
        s.biz.BootstrapCompletionTotal.Inc()
    }
    return profile, nil
}

func (s *Service) validate(answers []Answer) (map[string]interface{}, error) {
    out := make(map[string]interface{}, len(answers))
    for _, a := range answers {
        q, ok := QuestionByID(a.QID)
        if !ok {
            return nil, apperr.New(apperr.CodeInvalidArgument, "unknown_question", "未知问题 id: "+a.QID)
        }
        if err := checkAnswerShape(q, a.Value); err != nil {
            return nil, err
        }
        out[a.QID] = a.Value
    }
    // Required completeness
    for _, q := range Questions() {
        if !q.Required {
            continue
        }
        if _, ok := out[q.ID]; !ok {
            return nil, apperr.New(apperr.CodeInvalidArgument, "missing_required", "缺少必填问题: "+q.ID)
        }
    }
    return out, nil
}

func checkAnswerShape(q Question, v interface{}) error {
    switch q.Type {
    case TypeText:
        s, ok := v.(string)
        if !ok {
            return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为字符串")
        }
        if q.Required && strings.TrimSpace(s) == "" {
            return apperr.New(apperr.CodeInvalidArgument, "empty_text", q.ID+" 不能为空")
        }
        if q.MaxLength > 0 && len([]rune(s)) > q.MaxLength {
            return apperr.New(apperr.CodeInvalidArgument, "text_too_long", q.ID+" 超过最大长度")
        }
    case TypeSingleSelect:
        s, ok := v.(string)
        if !ok || !contains(q.Options, s) {
            return apperr.New(apperr.CodeInvalidArgument, "invalid_option", q.ID+" 选项不在白名单")
        }
    case TypeMultiSelect:
        arr, ok := v.([]interface{})
        if !ok {
            return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为数组")
        }
        for _, item := range arr {
            s, ok := item.(string)
            if !ok || !contains(q.Options, s) {
                return apperr.New(apperr.CodeInvalidArgument, "invalid_option", q.ID+" 含非法选项")
            }
        }
    case TypeBoolean:
        if _, ok := v.(bool); !ok {
            return apperr.New(apperr.CodeInvalidArgument, "invalid_value", q.ID+" 应为 true/false")
        }
    }
    return nil
}

func contains(opts []string, v string) bool {
    for _, o := range opts {
        if o == v {
            return true
        }
    }
    return false
}

func (s *Service) renderDescription(ctx context.Context, answers map[string]interface{}) string {
    userPayload, _ := json.Marshal(answers)
    out, err := s.llmClient.Generate(ctx, llm.GenerateRequest{
        Model:       s.model,
        Temperature: s.temperature,
        MaxTokens:   300,
        Messages: []llm.Message{
            {Role: "system", Content: bootstrapSystemPrompt},
            {Role: "user", Content: fmt.Sprintf("孩子信息（JSON）：%s", string(userPayload))},
        },
    })
    if err != nil {
        s.logger.Warn("bootstrap.render.fail", "err", err)
        return ""
    }
    return strings.TrimSpace(out.Text)
}

// Touch model so we can use it without unused-import warnings if extended later.
var _ = model.MemoryTypeStorySummary
```

- [ ] **Step 7.3：service_test.go**

用 MockLLM + fakeChildRepo（参考 Plan 4 / Plan 2 模式）：
- 全部 required 答全、LLM 正常 → Submit 返回 Profile.Description 非空、childRepo 收到 Update 调用、`BootstrapCompletionTotal` +1
- 缺 required → 返回 `apperr` Code=InvalidArgument code="missing_required"
- single_select option 不在白名单 → "invalid_option"
- multi_select 含非白名单元素 → "invalid_option"
- text 超长 → "text_too_long"
- LLM 报错 → 仍 200，`Profile.Description == ""`，answers 已保存
- not_owner → childsvc 返回的错误透传

- [ ] **Step 7.4：跑 + commit**

```bash
cd server && go test ./internal/service/bootstrap/... ./internal/service/child/...
git add server/internal/service/bootstrap/service.go \
        server/internal/service/bootstrap/service_test.go \
        server/internal/service/child/child.go \
        server/internal/service/child/child_test.go
git commit -m "feat(bootstrap): validate answers, LLM-polish into profile.description"
```

---

## Task 8：API handler `bootstrap.go`

**Files:**
- Create: `server/internal/api/bootstrap.go`
- Create: `server/internal/api/bootstrap_test.go`

- [ ] **Step 8.1：handler**

```go
package api

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/aibao/server/internal/api/userctx"
    "github.com/aibao/server/internal/service/bootstrap"
)

// BootstrapHandler exposes BOOTSTRAP form endpoints.
type BootstrapHandler struct {
    svc *bootstrap.Service
}

// NewBootstrapHandler constructs.
func NewBootstrapHandler(svc *bootstrap.Service) *BootstrapHandler {
    return &BootstrapHandler{svc: svc}
}

// RegisterRoutes mounts under v1 (caller is the JWT-auth group).
func (h *BootstrapHandler) RegisterRoutes(g *gin.RouterGroup) {
    g.GET("/bootstrap/questions", h.questions)
    g.POST("/bootstrap/answers", h.answers)
}

type questionsResp struct {
    Version   int                   `json:"version"`
    Questions []bootstrap.Question  `json:"questions"`
}

func (h *BootstrapHandler) questions(c *gin.Context) {
    c.JSON(http.StatusOK, questionsResp{
        Version:   bootstrap.Version,
        Questions: bootstrap.Questions(),
    })
}

type answersReq struct {
    ChildID int64                `json:"child_id" binding:"required"`
    Answers []bootstrap.Answer   `json:"answers"  binding:"required"`
}

func (h *BootstrapHandler) answers(c *gin.Context) {
    uid, ok := userctx.FromContext(c.Request.Context())
    if !ok {
        RespondUnauthorized(c)
        return
    }
    var req answersReq
    if err := c.ShouldBindJSON(&req); err != nil {
        RespondBadRequest(c, "invalid_argument", err.Error())
        return
    }
    profile, err := h.svc.Submit(c.Request.Context(), uid, req.ChildID, req.Answers)
    if err != nil {
        RespondError(c, err)
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "child_id":    req.ChildID,
        "version":     profile.Version,
        "description": profile.Description,
    })
}
```

> 注：`RespondUnauthorized` / `RespondBadRequest` / `RespondError` 如不存在，按既有 `api/audio.go` 调用风格替换。Plan 5 已统一使用 `RespondError(c, err)`，照搬即可。

- [ ] **Step 8.2：handler 测试**

按 Plan 5 `audio_test.go` 模式：
- GET /questions 200，body.questions 长度=7
- POST /answers 缺 JWT → 401
- POST /answers child 不属于 user → 403 not_owner（fake svc 返 apperr.CodePermissionDenied）
- POST /answers 正常 → 200，body 含 description

- [ ] **Step 8.3：跑 + commit**

```bash
cd server && go test ./internal/api/... -run Bootstrap
git add server/internal/api/bootstrap.go server/internal/api/bootstrap_test.go
git commit -m "feat(api): GET /bootstrap/questions + POST /bootstrap/answers"
```

---

## Task 9：扩展 MemoryUpdateHandler

**Files:**
- Modify: `server/internal/worker/handlers/memory_update.go`
- Modify: `server/internal/worker/handlers/memory_update_test.go`

**目的：** 既有 handler 落第一条原始 memory 后，额外调 Summarizer 拿 30 字总结，落第二条 `story_summary`（payload 含 `summary` / `story_id`）。Summarizer 失败不影响第一条 — fail-open。

- [ ] **Step 9.1：扩 handler**

```go
type MemoryUpdateHandler struct {
    memories   repository.MemoryRepo
    stories    repository.StoryRepo // Plan 6: re-read text by story_id (fresh)
    summarizer *memorysvc.Summarizer
}

func NewMemoryUpdateHandler(m repository.MemoryRepo, s repository.StoryRepo, sum *memorysvc.Summarizer) *MemoryUpdateHandler {
    return &MemoryUpdateHandler{memories: m, stories: s, summarizer: sum}
}

func (h *MemoryUpdateHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
    var p memoryUpdatePayload
    if err := json.Unmarshal(e.Payload, &p); err != nil {
        return fmt.Errorf("decode payload: %w", err)
    }
    // Existing behavior: write the canonical memory_update row.
    innerJSON, err := json.Marshal(p)
    if err != nil {
        return fmt.Errorf("re-encode payload: %w", err)
    }
    storyIDPtr := p.StoryID
    if err := h.memories.Create(ctx, &model.Memory{
        ChildID:    p.ChildID,
        MemoryType: model.MemoryTypeStorySummary,
        Payload:    innerJSON,
        Weight:     1.0,
        StoryID:    &storyIDPtr,
    }); err != nil {
        return err
    }

    // Plan 6 addition: LLM-summarize fresh story text into a short memory.
    // Fail-open — primary memory has already been written.
    if h.summarizer == nil || h.stories == nil {
        return nil
    }
    story, err := h.stories.FindByID(ctx, p.StoryID)
    if err != nil || story == nil {
        return nil
    }
    summary := h.summarizer.Summarize(ctx, story.TextContent)
    if summary == "" {
        return nil
    }
    sumPayload, _ := json.Marshal(map[string]interface{}{
        "type":          "story_summary",
        "summary":       summary,
        "story_id":      p.StoryID,
        "title":         p.Title,
        "used_fallback": p.UsedFallback,
    })
    _ = h.memories.Create(ctx, &model.Memory{
        ChildID:    p.ChildID,
        MemoryType: model.MemoryTypeStorySummary,
        Payload:    sumPayload,
        Weight:     1.2, // slightly higher recall than the raw row
        StoryID:    &storyIDPtr,
    })
    return nil
}
```

> 🎓 **为什么写两行而不是更新一行？** memories 表是 append-only 的"事件日志"，每次故事完成都该是新一行（即使重复跑也只是多一行，对召回有轻微噪声但绝不破坏一致性）。这同时避免了"先查再更新"的 race。Plan 4 故意把 idempotency 留松，本 plan 沿用。

- [ ] **Step 9.2：测试**

补两个用例：
- Summary 成功 → 检查 fakeMemoryRepo 收到 **2** 次 Create；第二次 payload.summary 非空、StoryID 不为 nil
- Summary 失败（Summarizer 返 ""）→ 只收到 1 次 Create；handler 不返回 error

- [ ] **Step 9.3：跑 + commit**

```bash
cd server && go test ./internal/worker/...
git add server/internal/worker/handlers/memory_update.go \
        server/internal/worker/handlers/memory_update_test.go
git commit -m "feat(worker): memory_update also writes LLM-summarized story memory"
```

---

## Task 10：Orchestrator 注入 MemorySummary

**Files:**
- Modify: `server/internal/service/story/orchestrator.go`
- Modify: `server/internal/service/story/orchestrator_test.go`

**目的：** 调 LLM 生成故事**之前**，先调 `memory.Selector.BuildContext(ctx, childID)` 拿"记忆上下文"字符串塞进 `prompt.BuildInput.MemorySummary`。selector 已 fail-open（错时返 ""），所以本层不再做兜底。

- [ ] **Step 10.1：注入字段**

`Orchestrator` 结构体加 `memorySelector *memorysvc.Selector`；构造函数尾参传入；在 `Generate` 内 `builder.Build(...)` 前：

```go
memCtx := o.memorySelector.BuildContext(ctx, child.ID)
o.logger.Debug("orchestrator.memory_context", "child_id", child.ID, "len", len(memCtx))

buildIn := prompt.BuildInput{
    // ... 既有字段 ...
    MemorySummary: memCtx,
}
```

如果 `memorySelector == nil`（旧测试构造路径），降级为 `memCtx = ""`，确保不 panic。

- [ ] **Step 10.2：测试**

补两个用例（fakeMemorySelector）：
- selector 返 "上次救了小恐龙" → 断言传给 builder 的 BuildInput.MemorySummary 等于该字符串
- selector 返 ""（无记忆 / 错误已被吞）→ 断言 BuildInput.MemorySummary == ""，故事仍生成成功

- [ ] **Step 10.3：跑 + commit**

```bash
cd server && go test ./internal/service/story/...
git add server/internal/service/story/orchestrator.go \
        server/internal/service/story/orchestrator_test.go
git commit -m "feat(story): orchestrator injects memory context into system prompt"
```

---

## Task 11：System Prompt 模板检查

**Files:**
- Modify (可能): `server/safety/system_prompt.tmpl`

**目的：** 模板已含 `{{if .MemorySummary}}...{{end}}` 段（Plan 4 写好）。本 plan 验证：（1）非空时正确插入；（2）为空时不留空白行；（3）给"无记忆"分支加一行明确兜底文字，让 LLM 知道这是首次故事。

- [ ] **Step 11.1：审视既有片段**

打开 `server/safety/system_prompt.tmpl`，定位：

```
{{- if .MemorySummary}}

【最近的故事记忆（用于自然彩蛋回调）】
{{.MemorySummary}}
{{- end}}
```

替换为：

```
{{- if .MemorySummary}}

【最近的故事记忆（用于自然彩蛋回调，不要刻意提及"还记得吗"）】
{{.MemorySummary}}
{{- else}}

【首次相遇】这是孩子和爱宝的第一个故事，请自然地建立彼此信任。
{{- end}}
```

🎓 **为什么"非空才显式提示不要刻意提及"？** LLM 对显式指令非常敏感。如果你说"请回忆"，它会硬塞一句"还记得上次吗？"——立刻显得刻意；如果你只给"事实"+"风格约束"，回调会自然地从角色/场景里浮出来。这就是"软提示"哲学。

- [ ] **Step 11.2：测试**

`prompt/builder_test.go` 已经覆盖 `MemorySummary` 渲染（Plan 4 写过），补一个"空 MemorySummary 走 else 分支"断言。

- [ ] **Step 11.3：跑 + commit**

```bash
cd server && go test ./internal/service/story/prompt/...
git add server/safety/system_prompt.tmpl server/internal/service/story/prompt/builder_test.go
git commit -m "feat(prompt): explicit first-encounter branch + soft callback hint"
```

---

## Task 12：Router 注册

**Files:**
- Modify: `server/internal/api/router.go`

- [ ] **Step 12.1：扩 RouterDeps + 挂载**

```go
type RouterDeps struct {
    // ... existing ...
    Bootstrap *BootstrapHandler // Plan 6
}
```

在 `if deps.JWT != nil { auth := ... }` 块中（在 `Child`/`Audio` 旁）：

```go
if deps.Bootstrap != nil {
    deps.Bootstrap.RegisterRoutes(auth)
}
```

- [ ] **Step 12.2：build + commit**

```bash
cd server && go build ./... && go test ./internal/api/...
git add server/internal/api/router.go
git commit -m "feat(api): mount bootstrap handler under JWT-auth group"
```

---

## Task 13：main.go 装配

**Files:**
- Modify: `server/cmd/server/main.go`

**目的：** 构造 Summarizer / Selector / bootstrap.Service，注入既有 MemoryUpdateHandler、StoryOrchestrator、RouterDeps。

- [ ] **Step 13.1：构造**

在 main.go 中现有 LLM client 之后、worker 装配之前：

```go
memorySummarizer := memorysvc.NewSummarizer(
    llmClient,
    cfg.LLM.IntentModel, // cheap path
    0.2,
    bizMetrics,
    logger,
)
memorySelector := memorysvc.NewSelector(memRepo, logger)

bootstrapSvc := bootstrap.NewService(
    childSvc,
    llmClient,
    cfg.LLM.IntentModel,
    0.3,
    bizMetrics,
    logger,
)
bootstrapHandler := api.NewBootstrapHandler(bootstrapSvc)
```

`MemoryUpdateHandler` 构造点替换为：

```go
memHandler := handlers.NewMemoryUpdateHandler(memRepo, storyRepo, memorySummarizer)
```

`Orchestrator` 构造点末参追加 `memorySelector`。

`RouterDeps`：

```go
api.RouterDeps{
    // ... existing ...
    Bootstrap: bootstrapHandler,
}
```

- [ ] **Step 13.2：build + commit**

```bash
cd server && go build ./...
git add server/cmd/server/main.go
git commit -m "feat(server): wire memory summarizer/selector + bootstrap into main"
```

---

## Task 14：端到端冒烟（手动）

**Files:** 无新增；本任务是 runbook。

**前提：** Plan 1-5 已可正常 `make run-dev`；测试库或 dev 库已 `make migrate-up`。

- [ ] **Step 14.1：启动 + 登录 + 创建孩子**

PowerShell：

```powershell
$env:AIBAO_LLM_DOUBAO_API_KEY="<your key>"
cd server; make run-dev
# 另开终端
$base = "http://localhost:8080/api/v1"

# 走 Plan 2 流程拿 token；这里假设你已有 $token / $childId
```

- [ ] **Step 14.2：拉问题**

```powershell
curl -s -H "Authorization: Bearer $token" "$base/bootstrap/questions" | jq '.questions | length'
# Expected: 7
```

- [ ] **Step 14.3：提交答案**

PowerShell 注意 UTF-8 byte body（Plan 4 教训，见知识库 06-testing.md 6.11）：

```powershell
$body = @{
  child_id = $childId
  answers = @(
    @{ q_id="personality_traits";  value=@("勇敢","好奇") }
    @{ q_id="favorite_characters"; value="奥特曼、小恐龙" }
    @{ q_id="fears";               value="打雷、黑暗" }
    @{ q_id="family_members";      value="爸爸、妈妈、奶奶" }
    @{ q_id="story_style";         value="冒险探索" }
    @{ q_id="education_themes";    value=@("勇敢","好奇心") }
    @{ q_id="enable_storyline";    value=$false }
  )
} | ConvertTo-Json -Depth 6
$bytes = [System.Text.Encoding]::UTF8.GetBytes($body)

curl.exe -s -X POST "$base/bootstrap/answers" `
  -H "Authorization: Bearer $token" `
  -H "Content-Type: application/json; charset=utf-8" `
  --data-binary "@-" <<< $bytes | jq
```

Expected：`description` 字段非空、80-150 字中文段落。

DB 验证：

```bash
docker exec aibao-postgres-dev psql -U aibao -d aibao -c \
  "SELECT id, profile->'description' FROM children WHERE id=$childId;"
```

- [ ] **Step 14.4：生成第 1 个故事**

```powershell
$story1 = curl.exe -s -X POST "$base/stories/generate" `
  -H "Authorization: Bearer $token" `
  -H "Content-Type: application/json; charset=utf-8" `
  --data-binary "@-" <<< ([System.Text.Encoding]::UTF8.GetBytes(@{
      child_id=$childId; prompt="讲一个森林冒险睡前故事"; duration=10;
      style="冒险探索"; topic="勇敢"
    } | ConvertTo-Json)) | ConvertFrom-Json
$story1.id
```

等待 10-20 秒让 Worker 消费：

```bash
docker exec aibao-postgres-dev psql -U aibao -d aibao -c \
  "SELECT memory_type, story_id, payload->'summary' FROM memories WHERE child_id=$childId ORDER BY created_at;"
```

Expected：至少 2 行（其中一行 payload.summary 是 30 字内中文）。

- [ ] **Step 14.5：生成第 2 个故事 — 验证记忆注入**

```powershell
$story2 = curl.exe -s -X POST "$base/stories/generate" ` # 同样 body
$story2.text
```

在服务日志中应看到：

```
DEBUG orchestrator.memory_context child_id=... len=30+
```

manual 检查：`$story2.text` 中是否自然带出第 1 个故事的角色/场景元素。**不强制硬命中**——记忆是软提示，命中是概率事件。

- [ ] **Step 14.6：metrics 检查**

```bash
curl -s http://localhost:8080/metrics | grep -E "memory_summary_|bootstrap_completion_"
```

Expected：三个指标都有值。

- [ ] **Step 14.7：fail-open 检查（可选）**

临时把 `AIBAO_LLM_DOUBAO_API_KEY` 改成无效值，重启服务，重跑 14.3：应当仍 200，但 `description == ""`。说明 BOOTSTRAP fail-open 工作正常。

> ⚠️ 该步会让"生成故事"也失败（因为故事 LLM 同 key）；测完务必恢复 key。

---

## Task 15：devlog + CLAUDE.md + MEMORY.md + 知识库

**Files:**
- Create: `docs/devlog/2026-05-11-plan-06-shipped.md`
- Modify: `CLAUDE.md` 第 2 章已完成清单
- Modify: `MEMORY.md`
- Modify: `docs/knowledge/README.md`
- Modify (新增条目): `docs/knowledge/05-software-design.md`、`docs/knowledge/11-prompt-and-llm.md`（如不存在则新建）

- [ ] **Step 15.1：devlog**

按 [docs/devlog/](../../devlog/) 既有日记风格写一篇，覆盖：
- 本次 ship 的两个能力（BOOTSTRAP + MEMORY 串联）
- 关键决策回顾（为什么不用聊天驱动 BOOTSTRAP、为什么写两行 memory 而不是 upsert、fail-open 哲学）
- 遗留 / 下一步（连续剧 Plan 7、subjective profile 演进、记忆 weight 衰减策略）

- [ ] **Step 15.2：CLAUDE.md 更新**

第 2 章"已落地的能力"加：
- BOOTSTRAP 首次相遇仪式（7 题表单 + LLM 润色 description）
- MEMORY 深化（每个故事一条 LLM 总结 + Orchestrator 拉最近 3 条注入 prompt）

"端到端可演示接口" 加：
- `GET /api/v1/bootstrap/questions`
- `POST /api/v1/bootstrap/answers`

当前阶段改为：Plan 1-6 全部实现，下一步写 Plan 7（连续剧 / storyline）。

- [ ] **Step 15.3：MEMORY.md 加决策摘要**

- BOOTSTRAP = 表单驱动而非对话驱动
- memory 是 append-only event log（不 upsert）
- LLM 失败一律 fail-open，不阻塞主流程
- `cfg.LLM.IntentModel` 是 lite 路径，story = pro 路径——本 plan 两次新增 LLM 调用都走 lite

- [ ] **Step 15.4：知识库新增 2 条词条**

**05-software-design.md** 追加：

```markdown
## 5.X 向后兼容的接口预留 / Forward-Compatible Field

**定义：** 在数据结构（DB 列、Go struct、JSON schema、prompt 模板等）中**提前留出**未来才会用到的字段，并以"空值"或"无效但合法的默认"占位，使得后续 plan 可以"只灌内容、不动接口"。

**生活类比：** 装修毛坯房时预埋的网线管——现在不接网线，但管道走好。后面要换 5 类→7 类网线时不用砸墙。

**为什么需要：** 如果不预留，后续 plan 要么需要修改老代码（增加联调风险），要么用兼容补丁绕过（积累技术债）。预留只占几行字段定义和 nil 检查，回报远大于成本。

**在本项目中怎么用：** Plan 4 在 `prompt.BuildInput` 里加了 `MemorySummary` 字段 + 模板 `{{if .MemorySummary}}` 分支，但当时不灌任何内容（始终为空字符串）。Plan 6 增加 MemorySelector 才真正灌入。**Plan 4 实现期，单元测试已覆盖"空 MemorySummary 走 else 分支"**——所以 Plan 6 接入时没出现任何已有测试报红。

**何时引入：** Plan 4 设计期；Plan 6 真正落地。
```

**11-prompt-and-llm.md**（若不存在，按既有 `README.md` 结构新建）：

```markdown
## 11.X 记忆即软提示 / Memory as Soft Hint

**定义：** 把"模型应该回忆的历史信息"作为 system prompt 中的**事实段落**塞入，而不是作为指令（"请回忆 X"）或 few-shot（"上一轮：…\n本轮：…"）。

**生活类比：** 跟一个新认识的朋友聊天前，先有人在你耳边低声说一句"她最近养了只小猫，叫嘟嘟"。你不会主动喊"听说你有只猫叫嘟嘟！"——而是当对话自然走到宠物话题时顺势带出来。

**为什么需要：** 显式指令式记忆（"请回忆上一个故事"）会让 LLM 输出僵硬的"还记得吗"开头；few-shot 式记忆会占用大量 token 且让模型陷入模仿语调。软提示是"事实 + 风格约束"——LLM 把记忆当作背景知识，自然地在情节里浮现。这同时也让"无记忆"分支非常优雅：模板只是少一段事实，不需要重写。

**在本项目中怎么用：** Plan 6 的 `MemorySelector.BuildContext` 把最近 3 条 memories.summary 用"；"拼成一行，塞入 system prompt 的"【最近的故事记忆】"段；且加一句"用于自然彩蛋回调，不要刻意提及'还记得吗'"。

**何时引入：** Plan 6 BOOTSTRAP + MEMORY 串联。
```

`docs/knowledge/README.md` 索引同步追加这两条。

- [ ] **Step 15.5：commit**

```bash
git add docs/devlog/2026-05-11-plan-06-shipped.md \
        CLAUDE.md MEMORY.md \
        docs/knowledge/README.md \
        docs/knowledge/05-software-design.md \
        docs/knowledge/11-prompt-and-llm.md
git commit -m "docs: ship Plan 6 (bootstrap + memory injection) + knowledge entries"
```

---

# 附：执行顺序与依赖

Tasks 0-13 严格串行（每一步 build 都依赖上一步）；Task 14 是手动冒烟，Task 15 是文档收尾。

| 串行段 | Tasks | 备注 |
|---|---|---|
| 数据层 | 0, 1, 2, 3 | migration + model + repo + metrics |
| 服务层 | 4, 5, 6, 7 | summarizer + selector + bootstrap |
| 接入层 | 8, 9, 10, 11, 12 | api + worker + orchestrator + prompt + router |
| 装配 | 13 | main.go |
| 验证 | 14 | 手动冒烟 |
| 收尾 | 15 | devlog + 知识库 |

# 附：Plan 6 不做什么（明确边界）

- **连续剧 storyline / episode_no**：列已存在但本 plan 不写不读，留 Plan 7
- **memory weight 衰减 / 重要性评分**：所有新 memory weight 固定 1.0~1.2，留未来
- **BOOTSTRAP 可重做 / version 升级流程**：本 plan 只支持 v1 写入；老 profile 读时降级，但没有"重做表单"接口
- **subjective profile 演进**：每次 BOOTSTRAP 都是完全覆盖；增量演进留 Plan 7+
- **memory 检索的语义匹配**：本 plan 用"最近 N 条"，不做向量相似度
- **故事冲突避免（不要给打雷怕的孩子讲雷暴故事）**：profile.answers.fears 已存，但 PreCheck 复用 Plan 3 的红线词路径——把 fears 接入 PreCheck 留 Plan 7
- **离线/无 LLM 模式**：本 plan 两个 LLM 新调用点均无降级模板（fail-open = description/summary 为空），不做"模板拼接 fallback 描述"
