# Plan 8：连续剧 + HEARTBEAT 伪推送（轻量版）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让爱宝的故事第一次具备"明天还想听"的钩子。完成本 plan 后端到端可演示两件事：(1) **连续剧**——家长 App 在第一次生成故事时显式传 `start_storyline:true`，后端创建一行 `storylines`、把该故事记为第 1 集；之后传 `storyline_id` 即可让爱宝把上一集的角色、场景、结尾钩子带进新一集 system prompt，生成"小宇和小恐龙第二天的冒险"。(2) **HEARTBEAT 伪推送**——App 启动时调 `GET /api/v1/heartbeat?child_id=N`，后端按本机时段拼一句"小宇下午好呀～"问候 + 返回该孩子最近活跃的连续剧列表（最多 5 条），客户端用这个数据渲染"继续小恐龙的冒险？"卡片。**本 plan 不做任何后台 cron / 后台预生成 / 真推送通道**（spec 二期）。

**Architecture:** 同步线只在既有 `Story.Orchestrator` 上加两个"扣"——**前扣**：若 `StorylineID != nil`，先通过新的 `StorylineContextBuilder` 拉最近 3 集的 30 字总结（复用 Plan 6 已经在写的 `memories.payload.summary`）+ 上一集的 `next_episode_hint`，塞进 `prompt.BuildInput` 三个新字段（`StorylineHook` / `StorylineRecentSummaries` / `EpisodeNumber`），由 system prompt 模板里新增的 `## 上一集剧情` 段消费；**后扣**：故事成功 persist 之后，**顺手** call 一次 doubao-lite（复用 Plan 6 的 `cfg.LLM.IntentModel`）生成 ≤20 字的"下集预告"，写回 `storylines.next_episode_hint` 并 `episode_count++`、`last_episode_at = NOW()`。`start_storyline:true` 分支在主 LLM 之前 pre-create 一行 storylines（status=active），把它的 id 写进 story 的 `storyline_id`、`episode_no=1`。HEARTBEAT 是一个**纯查询**的新接口（GET，无 DB 写入，无 LLM 调用，无外部 IO）：JWT → ownership check → 按时段算问候 → `storyline_repo.ListActiveByChild(childID, 5)`。续集失败兜底：PostCheck 新增 `not_continuing` 规则——若 `RequireContinuity=true` 且 LLM 输出**完全不**提及上一集的任何角色/场景名（5-8 个候选词），则视同 PostCheck fail，触发既有 fallback 链路（重试 1 次 → 走 fallback 模板，但 fallback 模板**不**继承 storyline_id，degrade 为独立故事）。

**Tech Stack:**
- Go 1.24+ + Gin + GORM + PostgreSQL（复用，**无新增依赖**）
- 复用 Plan 4 的 `gateway/llm.Client`（Doubao + Mock）—— hook 抽取走 `cfg.LLM.IntentModel`（豆包 lite，~0.003 元/次）
- 复用 Plan 6 的 `memory.Summarizer` 产出（不重新生成；续集上下文直接读 `memories.payload.summary`）
- 复用 Plan 6b 的 PostCheck duration-adaptive 主角阈值
- 复用 Plan 1-7：safety / prompt / repository / userctx / metrics / pkg/config / api.RespondError

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)
  - 第 3.7 HEARTBEAT 一期范围（**核心**——只做伪推送，不接 APNs/FCM）
  - 第 3.8 BOOT、3.9 BOOTSTRAP（确认 `enable_storyline` 字段已收集但本 plan 不自动用）
  - 第 5.3 重逢流程（HEARTBEAT 的产品意图：让爱宝"记得"）
  - 第 5.4 故事串联流程（**核心**——连续剧的角色/场景/钩子三件套）
  - 第 7 章红线（续集 prompt 不放宽任何安全约束）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - 第 4 章数据流（同步线新增"前扣 + 后扣"两个轻量 LLM 调用）
  - 第 5.1 stories / memories / outbox 表（**注意** `stories.storyline_id` 与 `episode_no` 是 Plan 4 留下的 NULLABLE 占位，本 plan 才真正启用并补上 FK 约束）
- Plan 4：[2026-05-08-plan-04-story-generation.md](2026-05-08-plan-04-story-generation.md)（**必读**——`stories.storyline_id/episode_no` 列就是 Plan 4 的占位）
- Plan 6：[2026-05-11-plan-06-bootstrap-and-memory.md](2026-05-11-plan-06-bootstrap-and-memory.md)（**必读**——本 plan 的 chapter_hook 抽取器是 `memory.Summarizer` 的双胞胎）
- Plan 6b：[2026-05-12-plan-06b-known-issue-fixes.md](2026-05-12-plan-06b-known-issue-fixes.md)（参考第 11.10 节"软提示位置 + 措辞"教训——续集 prompt 的硬约束程度比纯软提示高一档）
- Plan 7：[2026-05-13-plan-07-audio-mixing-bgm.md](2026-05-13-plan-07-audio-mixing-bgm.md)（参考其 plan 文档结构样式与"fail-open + metrics 计数 degraded"哲学）
- `server/migrations/000003_stories_and_outbox.up.sql`（确认 `storyline_id BIGINT NULL` + `episode_no INT NULL` 已就位）
- `server/internal/service/story/orchestrator.go`（本 plan 主修对象）
- `server/internal/service/memory/summarizer.go`（本 plan chapter_hook 的克隆模板）
- `server/internal/service/memory/selector.go`（本 plan storyline_context.Builder 的设计参照）
- `server/internal/service/bootstrap/questions.go`（确认 `enable_storyline` 已在问卷里，但**不**自动激活——客户端显式负责）
- CLAUDE.md（4.2 内容安全；4.4 不写套话注释；第 7 章必须解释知识点 + 同步落 `docs/knowledge/`）

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；新增 service/story/{chapter_hook,storyline_context} 与 repository/storyline_repo 覆盖率 ≥ 70%
2. `make migrate-up` 应用 `000007_storylines` 后：
   - `\d storylines` 显示 9 列 + `storylines_child_status_idx` 索引
   - `\d stories` 的 `storyline_id` 列下方出现新外键约束 `stories_storyline_fk → storylines(id) ON DELETE SET NULL`
3. `make run-dev` 启动后跑完整流程（curl/PowerShell 演示）：
   - 前置：登录 + 创建孩子 + （可选）走 BOOTSTRAP
   - `POST /api/v1/stories/generate {child_id, prompt, duration:5, style:..., start_storyline:true}` → 200，响应里 `storyline_id` 非空、`episode_no=1`；`SELECT * FROM storylines WHERE id=...` 看到 status=active、episode_count=1
   - 等待 5-15 秒（Worker 消费 memory_update → Plan 6 的 Summarizer 写 30 字总结）；同时观测 `storylines.next_episode_hint` 已在故事 persist 后被同步段更新为非空 ≤20 字字符串
   - `POST /api/v1/stories/generate {child_id, prompt, duration:5, style:..., storyline_id:<上一步返回的>}` → 200，响应 `episode_no=2`；故事文本中**至少出现** 1 个上一集角色/场景名（manual 检查或 metrics 看 not_continuing 没触发）
   - `GET /api/v1/heartbeat?child_id=N` → 200，响应含按本机时段的中文问候（`小宇下午好呀～...`）+ `active_storylines:[{id, title:"", episode_count:2, next_hint:"...", last_episode_at:"..."}]`
4. **互斥校验**：`POST /generate` 同时传 `start_storyline:true` 和 `storyline_id:N` → 400 `invalid_argument`
5. **所有权校验**：用户 A 拿用户 B 的 `storyline_id` 调 `/generate` 或拿用户 B 的 `child_id` 调 `/heartbeat` → 403 `not_owner`
6. **续集硬约束**：mock 一个 LLM 返回"小猫小狗去森林"——完全无视上一集"小恐龙救鳄鱼"——续集生成后 PostCheck 触发 `not_continuing`，重试 1 次仍失败 → 故事仍 ship（走 fallback 模板，但 `storyline_id` 字段降级为 NULL、不污染该 storyline 的连续性）
7. **HEARTBEAT 时段问候**正确：mock 系统时间在 5/12/15/20/23 点 → 分别返回 `早上好`/`中午好`/`下午好`/`晚上好`/`夜深了` 关键字
8. **零活跃连续剧**：HEARTBEAT 调用时 storylines 空 → 仍返回 200，`greeting` 非空、`active_storylines:[]`
9. **fail-open 链路全覆盖**：
   - chapter_hook LLM 报错 → 故事照常 ship，`storylines.next_episode_hint=""`，warning 日志 + `chapter_hook_extract_total{status="fail"}` +1
   - StorylineContextBuilder 报错（FK 损坏 / 该 storyline 已被删）→ 退化为"独立故事 prompt"（仍走 storyline_id 路径但 prompt 不带上一集段），warning 日志
10. 业务 metrics 在 `/metrics` 可见：
    - `storyline_created_total`（counter）
    - `storyline_episodes_total`（counter）
    - `chapter_hook_extract_duration_seconds`（histogram）
    - `chapter_hook_extract_total{status="ok|fail"}`（counter）
    - 复用 `llm_fail_fallback_total{provider,model,reason="chapter_hook"}`
11. `golangci-lint run ./...` 0 issues
12. 知识库新增 2 条词条：(05.X) "Pre-create 行 + 同事务父子写入：为什么 storyline 行要在主 LLM 之前先建"；(11.X) "硬约束 vs 软提示：续集 prompt 把'承接上一集'放到 system prompt 靠前段的设计权衡"

---

## 范围决策记录（与用户对齐）

| 维度 | 决策 |
|---|---|
| Storyline 启动 | **客户端显式**触发：`POST /generate {start_storyline:true}` → 后端创 storyline row 并把当前故事记为第 1 集，返回 `storyline_id`。**不**从 BOOTSTRAP `enable_storyline=true` 自动激活——客户端读 BOOTSTRAP 答案只用来决定"启动新故事时默认勾选连续剧 checkbox"，后端不读这个字段 |
| Storyline 推进 | `POST /generate {storyline_id:N}` → orchestrator 检测到该分支即拉上下文、写 `stories.storyline_id=N` `episode_no=current+1` |
| `start_storyline` 与 `storyline_id` 互斥 | 同时传两个 → 400。互斥校验在 API handler 层做，不漏到 service |
| 续集 prompt 上下文体量 | 最多 3 集 × 30 字总结 + 1 条 ≤20 字钩子 ≈ 110 字，加上模板包装段 < 200 token。**不**直接塞整篇前文（成本 + 串味儿风险） |
| 续集上下文数据源 | (1) 最近 3 集的 30 字总结：直接读 `memories.payload.summary`（Plan 6 已写）—— **不**重新生成。  (2) 上一集结尾钩子：从 `storylines.next_episode_hint` 拉 |
| 钩子提取 | 故事 persist 成功后**同步段**调一次 doubao-lite：`"请用 20 字内写下一集预告，承接刚才的故事氛围，但不要剧透关键转折。"` → 软截断到 20 字 → `UPDATE storylines SET next_episode_hint=$1, episode_count=episode_count+1, last_episode_at=NOW(), updated_at=NOW() WHERE id=$2`。**fail-open**：失败则 hint 维持原值（首次为 ""）、其他字段仍正常累加 |
| 钩子在哪一步调 | **同步段**，紧跟在 `CreateWithOutbox` 之后、`return story` 之前。理由：(a) 客户端立刻能在响应里看到 storyline 状态；(b) HEARTBEAT 接口能立刻看到新 hint；(c) 失败不影响故事 ship；(d) 一次 doubao-lite ≈ 200-500ms 不构成体验问题（故事生成本身 5-15s） |
| 钩子注入 prompt 的位置 | 借鉴 Plan 6b 11.10 教训：放在 system prompt **靠前**的新段 `## 上一集剧情`，位于 `## 身份与人格` 之后、`## 记忆上下文` 之前。措辞 **"下面是这个连续剧的上一集剧情和结尾钩子，请承接这些角色和场景写新一集，并自然地呼应上一集的结尾"**——比纯软提示更硬一档（"请承接"而非"可以参考"）|
| 续集 PostCheck 硬约束 | `RequireContinuity=true` 时，PostCheck 输入额外携带 5-8 个 `PreviousElements`（上一集的 `story_elements` 表中 element_type IN ('character','place') 按 recall_weight DESC 取 top 8）。LLM 输出若**完全不**包含其中任何一个子串 → 触发 `not_continuing` reason，复用既有 LLM 重试 1 次 + fallback 兜底链路 |
| 续集兜底降级 | 若续集走到 fallback 模板：(a) 故事仍 ship；(b) `stories.storyline_id` 写 **NULL**——不污染该连续剧的连续性，下一次仍可基于真正的上一集继续；(c) storylines 行**不**触发 IncrementEpisode（episode_count 不增）；(d) metrics 计 `not_continuing` |
| HEARTBEAT 是否后台预生成 | **不**做。本 plan 是"伪推送"——纯查询接口，客户端打开 App 时主动拉。spec 二期再上 APNs/FCM + cron 预生成 |
| HEARTBEAT 是否做 DB 写入 | **不**。完全只读：`storyline_repo.ListActiveByChild` + `child_repo.FindByID`。`updated_at` 字段不要因为查询被动 |
| HEARTBEAT 限流 | **不**挂限流（轻量纯查询、客户端启动时调 1 次）。挂 JWT + ownership 即可 |
| 问候时段切分 | 5-10 早上好 / 11-13 中午好 / 14-17 下午好 / 18-22 晚上好 / 23-4 夜深了。基于服务端本机时间 `time.Now().Hour()`（Docker 镜像里 `TZ=Asia/Shanghai`，本 plan 不引入用户时区字段） |
| HEARTBEAT 返回的 storylines 上限 | 5 条，按 `last_episode_at DESC NULLS LAST` 排序 |
| BOOTSTRAP `enable_storyline` 字段 | **保留**——客户端用来做"默认开启连续剧"的偏好提示，后端不读。Plan 8 文档明确"MVP 不自动激活，留给 spec 二期" |
| Storyline 标题 | 本 plan **不**自动生成（避免再加一次 LLM 调用）。`storylines.title` 默认空字符串，HEARTBEAT 卡片显示时客户端可用"《<上一集 story.title>》系列"做兜底。spec 二期再上 |
| Storyline 完结 | 本 plan 不提供"结束连续剧"的 API。`storylines.status` 字段就位但永远保持 `active`。spec 二期再上 `PATCH /storylines/:id/complete` |
| Storyline 删除 | **不**实现接口。`ON DELETE SET NULL` 保证 storyline 行被运维手工删除时关联故事降级为独立故事 |
| 测试策略 | chapter_hook 测试用 `gateway/llm/mock.go`；orchestrator 集成测试中 storyline 分支用 fake StorylineRepo；heartbeat handler 测试用 fake repos 直接构造数据 |

---

## File Structure

### 数据迁移

| 文件 | 职责 |
|---|---|
| `server/migrations/000007_storylines.up.sql` | 新建 `storylines` 表 + 索引；ALTER stories 增加 FK |
| `server/migrations/000007_storylines.down.sql` | DROP FK；DROP storylines |

### Data model

| 文件 | 修改/新增 |
|---|---|
| `server/internal/model/storyline.go` | 新增 `Storyline` 结构体 + `TableName()="storylines"` + 状态常量 `StorylineStatusActive/Completed/Abandoned` |
| `server/internal/model/story.go` | （无字段改动；`Story.StorylineID *int64` / `EpisodeNo *int` Plan 4 已存在）|
| `server/internal/service/safety/postcheck.go` | 在 `PostCheckReason` 一组常量里新增 `PostCheckReasonNotContinuing = "not_continuing"` |

### Repository

| 文件 | 修改/新增 |
|---|---|
| `server/internal/repository/storyline_repo.go` | **新增** `StorylineRepo` 接口 + GORM 实现：`Create` / `FindByID` / `ListActiveByChild` / `IncrementEpisode` |
| `server/internal/repository/storyline_repo_test.go` | **新增** 集成测试（testcontainers） |
| `server/internal/repository/story_repo.go` | **扩展** 新增 `RecentByStoryline(ctx, storylineID int64, limit int) ([]*model.Story, error)` 与对应接口方法 |
| `server/internal/repository/story_repo_test.go` | 补 `TestRecentByStoryline_*` |

### Service / Story

| 文件 | 修改/新增 |
|---|---|
| `server/internal/service/story/chapter_hook.go` | **新增** `ChapterHookExtractor` —— 镜像 `memory.Summarizer` |
| `server/internal/service/story/chapter_hook_test.go` | **新增** 单元测试，用 MockLLM |
| `server/internal/service/story/storyline_context.go` | **新增** `StorylineContextBuilder` + `StorylineContext` 结构体 |
| `server/internal/service/story/storyline_context_test.go` | **新增** 单元测试，fake repos |
| `server/internal/service/story/orchestrator.go` | **修改** —— `GenerateParams` 新增 `StartStoryline bool` + `StorylineID *int64`；Generate 加 storyline pre-create / 续集 context 拉取 / chapter_hook 后扣 |
| `server/internal/service/story/orchestrator_test.go` | 补 storyline 分支测试 |
| `server/internal/service/story/prompt/builder.go` | **修改** `BuildInput` 新增 `StorylineHook string` / `StorylineRecentSummaries []string` / `EpisodeNumber int` |
| `server/internal/service/story/prompt/builder_test.go` | 补 storyline 段渲染测试 |
| `server/internal/service/safety/postcheck.go` | **修改** `PostCheckInput` 新增 `RequireContinuity bool` / `PreviousElements []string`；Check 函数加 `not_continuing` 判断 |
| `server/internal/service/safety/postcheck_test.go` | 补 not_continuing 测试 |
| `server/safety/system_prompt.tmpl` | **修改** 在身份段后、记忆段前插入 `{{if .StorylineHook}}{{or len .StorylineRecentSummaries}}` 条件块 |

### API

| 文件 | 修改/新增 |
|---|---|
| `server/internal/api/story.go` | **修改** `generateReq` 新增 `StartStoryline bool` + `StorylineID *int64`；互斥校验；响应里带 `storyline_id` / `episode_no` |
| `server/internal/api/heartbeat.go` | **新增** `HeartbeatHandler` 全套 |
| `server/internal/api/heartbeat_test.go` | **新增** 单元测试 |
| `server/internal/api/router.go` | **修改** `RouterDeps` 新增 `Heartbeat *HeartbeatHandler`；挂 `/api/v1/heartbeat`（JWT auth 组，不挂 rate limit） |

### Metrics + main

| 文件 | 修改/新增 |
|---|---|
| `server/internal/metrics/business.go` | 新增 `StorylineCreatedTotal` / `StorylineEpisodesTotal` / `ChapterHookExtractDuration` / `ChapterHookExtractTotal` |
| `server/cmd/server/main.go` | 装配 `StorylineRepo` / `ChapterHookExtractor` / `StorylineContextBuilder`；注入 Orchestrator 与 HeartbeatHandler |

### Docs / Knowledge

| 文件 | 修改/新增 |
|---|---|
| `docs/devlog/2026-05-14-plan-08-shipped.md` | 完工日志 |
| `docs/knowledge/05-software-design.md` | 新增"Pre-create 行 + 父子写入"词条 |
| `docs/knowledge/11-llm-prompt.md`（无则新建） | 新增"硬约束 vs 软提示"词条 |
| `CLAUDE.md` | 更新当前阶段、已落地能力列表、可演示接口列表 |
| `MEMORY.md` | 追加 Plan 8 决策摘要 |

---

## API 形态

### 改：`POST /api/v1/stories/generate`

请求体新增两个**互斥**字段：

```json
{
  "child_id": 7,
  "prompt": "想听一个关于小恐龙的冒险",
  "duration": 5,
  "style": "adventure",
  "topic": "",
  "start_storyline": true,
  "storyline_id": null
}
```

约束：
- `start_storyline` 与 `storyline_id` 不能同时非零值。同时传 → 400 `invalid_argument` `user_msg="不能同时启动新连续剧和续接已有连续剧"`
- `storyline_id` 非 nil 时：(a) 必须能查到该 row；(b) `storylines.child_id == req.child_id`；(c) `storylines.child_id` 所属用户 == ctx.UserID。任一失败 → 403 `not_owner` 或 404 `storyline_not_found`

响应（在既有 storyJSON 上追加两个字段）：

```json
{
  "id": 123,
  "title": "小恐龙的森林冒险",
  "child_id": 7,
  "duration_minutes": 5,
  "style": "adventure",
  "audio_status": "pending",
  "storyline_id": 42,
  "episode_no": 1,
  "created_at": "..."
}
```

`storyline_id` / `episode_no` 在非连续剧故事中为 `null`。

### 新：`GET /api/v1/heartbeat?child_id=N`

JWT 必需。Query 必填 `child_id`。

成功响应：

```json
{
  "greeting": "小宇下午好呀～想继续之前的冒险吗？",
  "active_storylines": [
    {
      "id": 42,
      "title": "",
      "episode_count": 3,
      "next_hint": "明天小宇要带小恐龙找妈妈",
      "last_episode_at": "2026-05-14T15:23:01+08:00"
    }
  ]
}
```

零活跃：

```json
{
  "greeting": "小宇下午好呀～",
  "active_storylines": []
}
```

时段切片（基于服务端 `time.Now().Hour()`，Docker `TZ=Asia/Shanghai`）：

| 小时区间 | 问候模板 |
|---|---|
| 5-10 | `{name}早上好呀～` |
| 11-13 | `{name}中午好呀～` |
| 14-17 | `{name}下午好呀～` |
| 18-22 | `{name}晚上好呀～` |
| 23-4 | `{name}夜深了，今天还想听一个故事吗？` |

`{name}` = `child.Nickname`（创建孩子档案时填的昵称）。

若 `active_storylines` 非空，问候后**追加** "想继续之前的冒险吗？"（除"夜深了"那条本身已带）。

错误：
- 无 JWT → 401
- `child_id` 不属于 ctx.UserID → 403 `not_owner`
- `child_id` 不存在 → 404 `child_not_found`

---

## 数据模型字段约定

### `storylines` 表（新增）

| 列 | 类型 | 约束 | 备注 |
|---|---|---|---|
| `id` | `BIGSERIAL` | PK | |
| `child_id` | `BIGINT` | NOT NULL, FK → `children(id)` ON DELETE CASCADE | |
| `title` | `VARCHAR(200)` | NOT NULL DEFAULT '' | 本 plan 不自动填，留二期 |
| `status` | `VARCHAR(20)` | NOT NULL DEFAULT `'active'` | enum: `active` / `completed` / `abandoned`（本 plan 只用 active） |
| `next_episode_hint` | `VARCHAR(200)` | NOT NULL DEFAULT '' | LLM 抽取的 ≤20 字下集预告 |
| `episode_count` | `INT` | NOT NULL DEFAULT 0 | 已写成功的集数 |
| `last_episode_at` | `TIMESTAMPTZ` | NULL | 最近一集落库时间，HEARTBEAT 排序用 |
| `created_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT NOW() | |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL DEFAULT NOW() | |

索引：

```sql
CREATE INDEX storylines_child_status_idx
  ON storylines(child_id, status, last_episode_at DESC NULLS LAST);
```

### `stories` 表（仅补 FK 约束，**不**新增列）

```sql
ALTER TABLE stories
  ADD CONSTRAINT stories_storyline_fk
  FOREIGN KEY (storyline_id) REFERENCES storylines(id) ON DELETE SET NULL;
```

> 🎓 **小知识：ON DELETE SET NULL**——它的意思是"如果父行（storylines）被删，所有子行（stories）的 `storyline_id` 列自动改成 NULL，但子行本身不删"。**为什么需要**：连续剧只是"故事的分组关系"，删一个 storyline 不能连带删孩子已经听过的故事——那些故事本身是孩子的回忆，应该独立存在。换成 `CASCADE` 就会一并删故事，破坏数据完整性。

### `Storyline` Go 模型

```go
type Storyline struct {
    ID               int64
    ChildID          int64
    Title            string
    Status           string  // active / completed / abandoned
    NextEpisodeHint  string
    EpisodeCount     int
    LastEpisodeAt    *time.Time
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

func (Storyline) TableName() string { return "storylines" }
```

---

## Tasks

### Task 0：迁移 `000007_storylines`

- [ ] 新建 `server/migrations/000007_storylines.up.sql`：
  - `CREATE TABLE storylines (...)` 按上节字段
  - `CREATE INDEX storylines_child_status_idx`
  - `ALTER TABLE stories ADD CONSTRAINT stories_storyline_fk FOREIGN KEY (storyline_id) REFERENCES storylines(id) ON DELETE SET NULL;`
- [ ] 新建 `server/migrations/000007_storylines.down.sql`：
  - `ALTER TABLE stories DROP CONSTRAINT IF EXISTS stories_storyline_fk;`
  - `DROP TABLE IF EXISTS storylines;`
- [ ] 本地 `make migrate-up` → `make migrate-down` → `make migrate-up` 来回跑一遍验证幂等
- [ ] `\d storylines` + `\d stories` 截图（贴 devlog）

### Task 1：业务 metrics 新增 4 个

- [ ] 修 `server/internal/metrics/business.go`，在既有 `Business` struct 后追加 4 个字段：
  - `StorylineCreatedTotal prometheus.Counter`
  - `StorylineEpisodesTotal prometheus.Counter`
  - `ChapterHookExtractDuration prometheus.Histogram`（buckets `0.05, 0.1, 0.2, 0.5, 1, 2, 5`）
  - `ChapterHookExtractTotal *prometheus.CounterVec`（label `status`）
- [ ] `Register` 函数登记新指标
- [ ] 复用 `LLMFailFallbackTotal{provider="doubao", model=<lite>, reason="chapter_hook"}`（**不**新增 counter）
- [ ] `go test ./internal/metrics/...` 通过

### Task 2：model 与常量

- [ ] 新建 `server/internal/model/storyline.go`，定义 `Storyline` struct + `TableName()` + 3 个状态常量
- [ ] 修 `server/internal/service/safety/postcheck.go`，在既有 `PostCheckReason*` 常量组里新增 `PostCheckReasonNotContinuing = "not_continuing"`

### Task 3：`StorylineRepo` 接口 + 实现 + 测试

- [ ] 新建 `server/internal/repository/storyline_repo.go`：
  ```go
  type StorylineRepo interface {
      Create(ctx context.Context, s *model.Storyline) error
      FindByID(ctx context.Context, id int64) (*model.Storyline, error)
      ListActiveByChild(ctx context.Context, childID int64, limit int) ([]*model.Storyline, error)
      IncrementEpisode(ctx context.Context, id int64, hint string) error
  }
  ```
- [ ] GORM 实现：
  - `Create` 直接 `Create(s)`
  - `FindByID` `First` + `gorm.ErrRecordNotFound` 转 `apperr.CodeNotFound`
  - `ListActiveByChild` `Where("child_id=? AND status=?", ..., "active").Order("last_episode_at DESC NULLS LAST").Limit(limit)`
  - `IncrementEpisode` 用 `Updates(map[string]any{...})` 或原生 SQL：
    ```sql
    UPDATE storylines
    SET next_episode_hint = $1,
        episode_count = episode_count + 1,
        last_episode_at = NOW(),
        updated_at = NOW()
    WHERE id = $2
    ```
- [ ] 新建 `storyline_repo_test.go`（testcontainers）：
  - `TestCreate_Success`
  - `TestFindByID_NotFound_Apperr`
  - `TestListActiveByChild_OrderedByLastEpisodeDesc`
  - `TestIncrementEpisode_BumpsCountAndTimestamp`

### Task 4：扩展 `StoryRepo` —— `RecentByStoryline`

- [ ] 修 `server/internal/repository/story_repo.go`：
  - 接口（如有 interface）加 `RecentByStoryline(ctx, storylineID, limit) ([]*model.Story, error)`
  - 实现：`Where("storyline_id = ?", storylineID).Order("episode_no DESC NULLS LAST, created_at DESC").Limit(limit)`
- [ ] `story_repo_test.go` 补 `TestRecentByStoryline_OrderAndLimit`

### Task 5：`ChapterHookExtractor`（镜像 `memory.Summarizer`）

- [ ] 新建 `server/internal/service/story/chapter_hook.go`：

  ```go
  package story

  const chapterHookSystemPrompt = `你是儿童故事的下集预告员。请用 20 字以内一句话写下一集预告，承接刚才故事的氛围，但不要剧透关键转折。直接给出预告句子本身，不要解释、不要标点收尾。`

  type ChapterHookExtractor struct {
      client      llm.Client
      model       string
      temperature float64
      biz         *metrics.Business
      logger      *slog.Logger
  }

  func NewChapterHookExtractor(client llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *ChapterHookExtractor { ... }

  // Extract returns a <=20-char Chinese sentence or "" on any error.
  func (e *ChapterHookExtractor) Extract(ctx context.Context, storyText string) string { ... }
  ```

  实现要点：
  - `MaxTokens: 60`，`Temperature: 0.4`
  - 计 `ChapterHookExtractDuration.Observe(dur)` 与 `ChapterHookExtractTotal.WithLabelValues("ok|fail").Inc()`
  - 失败时 `LLMFailFallbackTotal.WithLabelValues("doubao", model, "chapter_hook").Inc()` + `logger.Warn("story.chapter_hook.fail", "err", err)`
  - 成功 trim 后用 `truncateChinese(s, 20)` 软截断（如已有 helper 抽到 pkg 共享，否则本文件内复制一份）

- [ ] 新建 `chapter_hook_test.go`，用 MockLLM：
  - `TestExtract_Success_TrimmedTo20`
  - `TestExtract_LLMError_ReturnsEmpty`
  - `TestExtract_EmptyInput_NoLLMCall`

### Task 6：`StorylineContextBuilder`

- [ ] 新建 `server/internal/service/story/storyline_context.go`：

  ```go
  type StorylineContext struct {
      StorylineID            int64
      Title                  string
      EpisodeNumber          int      // 即将生成的这一集的 episode_no
      RecentSummaries        []string // 上一集在前；最多 3 条；每条 ≤30 字
      PreviousHook           string   // 来自 storylines.next_episode_hint
      PreviousElements       []string // 上一集的角色/场景名 top 5-8
  }

  type StorylineContextBuilder struct {
      storylineRepo StorylineRepo
      storyRepo     StoryRepo  // 需 RecentByStoryline + 一个能读 story_elements 的方法
      memoryRepo    MemoryRepo // 已有；读 memory.payload.summary
      logger        *slog.Logger
  }

  func (b *StorylineContextBuilder) Build(ctx, storylineID int64) (*StorylineContext, error)
  ```

  实现：
  1. `sl, err := storylineRepo.FindByID(...)` —— err 直接返
  2. `recent, _ := storyRepo.RecentByStoryline(ctx, storylineID, 3)`
  3. 对每个 `recent[i].ID`，从 `memories` 表里取 `memory_type='story_summary' AND payload->>'story_id'=...` 的最新一条，提 `payload.summary`。无则跳过
  4. 取 `recent[0]`（最新一集）的 `story_elements`：element_type IN ('character','place') ORDER BY recall_weight DESC LIMIT 8 → `PreviousElements`
  5. `EpisodeNumber = sl.EpisodeCount + 1`
  6. `PreviousHook = sl.NextEpisodeHint`

- [ ] **fail-open**：任一子查询失败 → log warn + 该字段空值，仍返回非 nil 的 `*StorylineContext`（不 propagate err 到 orchestrator）—— 但 `Build` 函数签名仍保留 `error` 给上层做"完全没找到 storyline"的硬错（用来挡 not_owner）

- [ ] 测试 `storyline_context_test.go`：
  - `TestBuild_Success_WithAll4Fields`
  - `TestBuild_NoMemories_SummariesEmpty`
  - `TestBuild_NoPreviousElements_StillReturns`
  - `TestBuild_StorylineNotFound_ReturnsError`

### Task 7：`prompt.BuildInput` + 模板段

- [ ] 修 `server/internal/service/story/prompt/builder.go`，在 `BuildInput` struct 加：
  ```go
  StorylineHook            string
  StorylineRecentSummaries []string
  EpisodeNumber            int
  ```
- [ ] 修 `server/safety/system_prompt.tmpl`，在身份段（`## 身份与人格`）之后、记忆段（`{{if .MemorySummary}}...`）之前**插入**：

  ```text
  {{if or .StorylineHook (gt (len .StorylineRecentSummaries) 0)}}
  ## 上一集剧情（这是连续剧第 {{.EpisodeNumber}} 集）

  下面是这个连续剧的上一集剧情和结尾钩子，请**承接这些角色和场景**写新一集，并自然地呼应上一集的结尾。不要重新介绍角色——孩子已经认识他们了。

  {{if gt (len .StorylineRecentSummaries) 0}}最近几集回顾：
  {{range .StorylineRecentSummaries}}- {{.}}
  {{end}}{{end}}

  {{if .StorylineHook}}上一集结尾留下的钩子：{{.StorylineHook}}{{end}}
  {{end}}
  ```

- [ ] 单测 `builder_test.go` 加 `TestBuild_StorylineSection_RendersWhenHookOrSummariesPresent` + `TestBuild_StorylineSection_OmittedWhenBothEmpty`

### Task 8：PostCheck 续集硬约束

- [ ] 修 `server/internal/service/safety/postcheck.go`：
  - `PostCheckInput` 加 `RequireContinuity bool` / `PreviousElements []string`
  - 在既有规则列表后追加新规则：
    ```go
    if in.RequireContinuity && len(in.PreviousElements) > 0 {
        hit := false
        for _, e := range in.PreviousElements {
            if e == "" { continue }
            if strings.Contains(in.StoryText, e) { hit = true; break }
        }
        if !hit {
            return PostCheckOutput{Pass: false, RejectReason: PostCheckReasonNotContinuing, MatchedRule: "no_previous_element_mentioned"}
        }
    }
    ```
- [ ] 单测：
  - `TestPostCheck_NotContinuing_AllPreviousElementsAbsent`
  - `TestPostCheck_NotContinuing_HitAtLeastOnePass`
  - `TestPostCheck_RequireContinuityFalse_Skipped`

### Task 9：`Orchestrator` 主修改

- [ ] 修 `server/internal/service/story/orchestrator.go`，`GenerateParams` 加：
  ```go
  StartStoryline bool
  StorylineID    *int64
  ```
- [ ] `Deps` 加：
  ```go
  Storylines      StorylineRepo
  StorylineCtxBld *StorylineContextBuilder
  ChapterHook     *ChapterHookExtractor
  Biz             *metrics.Business
  ```
- [ ] `Generate` 流程改写（伪代码）：
  ```
  1. child check + budget check + precheck（不变）
  2. 准备 storylineCtx：
     - 若 p.StartStoryline:
         sl := &Storyline{ChildID: child.ID, Status: "active"}
         storylineRepo.Create(ctx, sl)    // 拿到 sl.ID
         biz.StorylineCreatedTotal.Inc()
         currentStorylineID := &sl.ID
         episodeNumber := 1
         storylineCtx := nil               // 第 1 集没有上下文
     - elif p.StorylineID != nil:
         slCtx, err := storylineCtxBld.Build(ctx, *p.StorylineID)
         若 err（not found）→ 返 404
         若 slCtx.* child 不匹配 → 403 not_owner（在 builder 里或 orchestrator 里查 storyline.ChildID == p.ChildID）
         currentStorylineID := p.StorylineID
         episodeNumber := slCtx.EpisodeNumber
         storylineCtx := slCtx
     - else:
         currentStorylineID := nil; episodeNumber := 0
  3. memCtx + builder.Build —— 把 storylineCtx 三字段塞进 BuildInput
  4. LLM 调用（不变，含 1 次重试）
  5. PostCheck —— 新增 RequireContinuity = (storylineCtx != nil)、PreviousElements = storylineCtx.PreviousElements
  6. fallback 逻辑（不变；若 fallback 触发且本来是续集，则 currentStorylineID 强制回退为 nil）
  7. 构造 story，写 storyline_id / episode_no：
     story.StorylineID = currentStorylineID
     if currentStorylineID != nil { eno := episodeNumber; story.EpisodeNo = &eno }
  8. CreateWithOutbox（不变）
  9. 后扣：仅当 currentStorylineID != nil 且未走 fallback：
        hint := chapterHook.Extract(ctx, llmText)
        _ = storylineRepo.IncrementEpisode(ctx, *currentStorylineID, hint)  // fail-open
        biz.StorylineEpisodesTotal.Inc()
  10. return story
  ```
- [ ] **互斥校验**：`if p.StartStoryline && p.StorylineID != nil { return apperr 400 invalid_argument }`（也可推到 API 层，二选一；本 plan 推到 API 层避免 service 层多一个不必要分支）
- [ ] **所有权校验**：在 `StorylineContextBuilder.Build` 内部对 `sl.ChildID != p.ChildID` 返 `not_owner`；orchestrator 转 403
- [ ] orchestrator_test 补：
  - `TestGenerate_StartStoryline_CreatesRowAndEpisode1`
  - `TestGenerate_ContinueStoryline_PassesEpisode2AndContext`
  - `TestGenerate_StorylineNotOwnedByChild_Returns403`
  - `TestGenerate_PostCheckNotContinuing_FallsbackWithNullStorylineID`
  - `TestGenerate_ChapterHookFails_StoryStillShipsHintEmpty`

### Task 10：API `story.go` 与互斥校验

- [ ] 修 `server/internal/api/story.go`：
  - `generateReq` 新增 `StartStoryline bool json:"start_storyline"` + `StorylineID *int64 json:"storyline_id"`
  - 校验：`if req.StartStoryline && req.StorylineID != nil { 400 invalid_argument "不能同时启动新连续剧和续接已有连续剧" }`
  - `Generate` 调用传入新字段
  - `storyJSON(story)` 输出加 `storyline_id` + `episode_no`（指针类型，nil → JSON null）
- [ ] 单测 `story_test.go` 补：
  - `TestGenerate_BothStartAndContinue_400`
  - `TestGenerate_Response_IncludesStorylineFields`

### Task 11：`heartbeat.go` + router

- [ ] 新建 `server/internal/api/heartbeat.go`：
  ```go
  type HeartbeatHandler struct {
      children      ChildRepo
      storylines    StorylineRepo
      now           func() time.Time   // 注入便于测试
  }

  func (h *HeartbeatHandler) RegisterRoutes(g *gin.RouterGroup) {
      g.GET("/heartbeat", h.heartbeat)
  }
  ```
- [ ] handler 逻辑：
  1. 取 ctx.UserID（无则 401）
  2. `child_id` query parse → `child := children.FindByID(...)` → not found 404 / `child.UserID != uid` → 403
  3. `lines, _ := storylines.ListActiveByChild(ctx, childID, 5)` —— err 不中断，空切片继续
  4. `hour := h.now().Hour()` → 切片到问候模板 → 替换 `{name}` 为 `child.Nickname`
  5. 若 `len(lines) > 0` 且当前 hour 不属于"夜深了"段 → 问候追加 `想继续之前的冒险吗？`
  6. 200 JSON 返回
- [ ] 新建 `heartbeat_test.go`：
  - `TestHeartbeat_TimeSlices_Morning/Noon/Afternoon/Evening/LateNight`
  - `TestHeartbeat_AppendsContinuationPromptWhenStorylinesExist`
  - `TestHeartbeat_NoStorylines_GreetingOnly`
  - `TestHeartbeat_ChildNotOwnedByUser_403`
  - `TestHeartbeat_ChildNotFound_404`
- [ ] 修 `router.go`：`RouterDeps` 加 `Heartbeat *HeartbeatHandler`；在既有 JWT-auth 组内 `deps.Heartbeat.RegisterRoutes(authed)`；不挂 rate limit/budget middleware

### Task 12：`main.go` 装配

- [ ] 在既有 wiring 段内：
  ```go
  storylineRepo := repository.NewStorylineRepo(db)
  chapterHook := story.NewChapterHookExtractor(llmClient, cfg.LLM.IntentModel, 0.4, biz, logger)
  storylineCtxBld := story.NewStorylineContextBuilder(storylineRepo, storyRepo, memoryRepo, logger)

  orch, err := story.NewOrchestrator(story.Deps{
      // 既有字段...
      Storylines:      storylineRepo,
      StorylineCtxBld: storylineCtxBld,
      ChapterHook:     chapterHook,
      Biz:             biz,
  })

  hbHandler := api.NewHeartbeatHandler(childRepo, storylineRepo, time.Now)

  routerDeps.Heartbeat = hbHandler
  ```

### Task 13：端到端冒烟（手动）

- [ ] `make migrate-up` 确认 000007 应用
- [ ] `make run-dev`
- [ ] curl 脚本（或 PowerShell）：
  1. 登录 → JWT
  2. 创建孩子 nickname="小宇"
  3. POST /generate `{child_id, prompt:"小恐龙的冒险", duration:5, style:"adventure", start_storyline:true}` → 验返回 `storyline_id != null, episode_no=1`
  4. 5-15s 后 `psql -c "SELECT id, episode_count, next_episode_hint, last_episode_at FROM storylines WHERE id=...;"` → `episode_count=1, hint 非空（如失败 fail-open 为空也算 pass，记 devlog）`
  5. POST /generate `{child_id, prompt:"继续小恐龙的冒险", duration:5, style:"adventure", storyline_id:<上一步 id>}` → 验返回 `episode_no=2`；故事文本 grep 上一集角色名应至少命中 1 个
  6. GET /heartbeat?child_id=<id> → 验问候格式 + active_storylines[0].episode_count=2
  7. 模拟"同时传两个"调用 → 验 400
  8. 用用户 B 的 JWT 调上面的 storyline_id → 验 403
- [ ] 截 4 张图（POST 启动 / POST 续集 / SELECT storylines / GET heartbeat）贴 devlog

### Task 14：devlog + CLAUDE/MEMORY/knowledge 同步

- [ ] 新增 `docs/devlog/2026-05-14-plan-08-shipped.md`：完工总结、踩坑、token 成本观察
- [ ] 更新 `CLAUDE.md`：
  - 第 2 节"当前阶段"改为 "Plan 1+2+3+4+5+6+6b+7+8 全部实现并通过冒烟"
  - 已落地能力列表追加"连续剧"和"HEARTBEAT 伪推送"
  - 可演示接口追加 `POST /generate` 的 storyline 形态 + `GET /heartbeat`
  - 下一步改为 Plan 9（Flutter 客户端）/ Plan 10（部署上线）
- [ ] 更新 `MEMORY.md`：追加 Plan 8 关键决策（客户端显式触发；钩子同步段写；fallback 续集不污染 storyline）
- [ ] `docs/knowledge/05-software-design.md` 新增词条："Pre-create 父行 + 同事务关联子行"——讲清楚为什么 storylines 行要在主 LLM 调用**之前**先 INSERT、而不是在 story 写完之后再补。
- [ ] `docs/knowledge/11-llm-prompt.md`（如无则新建并加索引）新增词条："硬约束 vs 软提示"——讲清楚为什么续集 prompt 用"请承接"而非"可以参考"，以及 system prompt 中段落位置的影响（Plan 6b 11.10 教训的延伸）

---

## 教学段落（边做边学）

🎓 **Pre-create 行 + 父子写入**

我们这里有个看起来反直觉的设计：**先**写 storylines 行（父），**再**写 stories 行（子）。如果第 1 集 LLM 生成失败、我们没走到 `CreateWithOutbox`——岂不是留下了一个空的 storyline？

可以这样想：storyline 是"档案夹"，story 是"放进档案夹的纸"。先建空档案夹**没问题**，运维查询时只会看到 `episode_count=0` 的空 storyline，HEARTBEAT 的 `ListActiveByChild` 也会忽略它（按 `last_episode_at DESC NULLS LAST` 它会排到最后，加上前 5 条限制基本看不见）。**为什么需要先建**：因为 story 的 `storyline_id` 是 NOT NULL 的外键（其实是 NULL 允许，但我们要让它非 NULL），等 story 写完再回头去建 storyline 等于多一次事务、多一份耦合——pre-create 反而最干净。

替代方案是把 "Create storyline + Create story" 包在同一个 GORM 事务里——这是更严谨的做法，但本 plan 偷懒只在 story 写失败时**不**做任何 storyline 清理（接受少量孤儿 storyline），换来代码简洁。devlog 里记一笔，spec 二期可以加 GC。

🎓 **硬约束 vs 软提示**（Plan 6b 11.10 的延伸）

Plan 6 的 `## 记忆上下文` 段写的是"可以参考下面这些过往记忆"——**软提示**，LLM 经常视而不见。Plan 6b 把它前移并改措辞为"请自然地呼应这些记忆"，**召回率有明显提升**。

Plan 8 的续集场景**更严格**——如果 LLM 完全不提上一集的小恐龙，整个"连续剧"价值就归零。所以我们采取三层叠加：
1. **位置靠前**：放到身份段之后、记忆段之前（更前 = LLM 更不容易遗忘）
2. **措辞硬**："请**承接这些角色和场景**写新一集"——用"请"+ 强动词
3. **后置兜底**：PostCheck 的 `not_continuing` 规则——LLM 完全无视上一集元素 → 重试 / fallback

这三层是阶梯式的"软 → 硬"过渡：前两层成本 0、对 LLM 的"语言暗示"够强；最后一层是"语言不行就用代码兜底"。生产观测下我们会调 `RequireContinuity` 的阈值（目前是"至少 1 个元素命中"，将来可以提到 2 个）。

---

## 本 Plan 显式**不**做的事

- ❌ **后台 cron / 后台预生成**——HEARTBEAT 是纯查询，App 启动时主动拉。spec 二期再上
- ❌ **APNs / FCM 真推送通道**——spec 二期
- ❌ **生日 / 节日 / 季节性故事自动触发**——spec 二期
- ❌ **storyline 自动完结（episode_count >= N）**——本 plan 不做；status 永远 active
- ❌ **`PATCH /storylines/:id/complete` 等 storyline 管理 API**——spec 二期
- ❌ **storyline 自动起标题**——避免再多一次 LLM 调用；二期再补
- ❌ **基于 BOOTSTRAP `enable_storyline=true` 的自动激活**——明确推给客户端 UI 决策
- ❌ **跨 storyline 的横向联动**（小恐龙在 A 连续剧里也出现在 B 里）——spec 二期
- ❌ **续集生成时的"剧情走向选择"分支**（家长选 A/B 路径）——spec 二期
- ❌ **HEARTBEAT 的"今天讲什么"个性化推荐**——本 plan 只回打招呼 + 活跃连续剧列表

---

## 风险与开放问题

| 风险 | 缓解 |
|---|---|
| chapter_hook LLM 调用让同步段延迟 +200-500ms | 接受；故事生成本身 5-15s，用户感知差异 < 5%。如观测到长尾 > 1s，把 hook 改到 outbox 异步段 |
| 续集 `PreviousElements` 选不到角色名（上一集 element extract 全为 IP 实体） | builder 兜底用 `recent[0].title` 切词作为候选；仍空则 `RequireContinuity=false` 跳过硬约束（log warn） |
| storyline 行越来越多（每次 start_storyline 都新建） | 本 plan 接受；后期上 `status='abandoned'` 自动归档脚本 |
| LLM 把"下一集预告"写超 20 字 | `truncateChinese(s, 20)` 软截断；可能截在词中——可接受 |
| 时区固定写死 Asia/Shanghai | 接受；MVP 用户都在国内。客户端时区字段留到二期 |
| 续集 PostCheck 反着误伤（LLM 用了上一集角色但换了同义词如"小龙"代替"小恐龙"） | 接受少量误伤；触发后走 1 次重试，仍败则 fallback。监控 `not_continuing` 命中率，若 > 5% 调整阈值 |

---

## 验收清单（合卷）

- [ ] Task 0-14 全部 checkbox 完成
- [ ] DoD 12 条全过
- [ ] 端到端冒烟 4 张截图入 devlog
- [ ] CLAUDE.md / MEMORY.md / 知识库 同步
- [ ] PR 描述里贴 `storyline_episodes_total` 与 `chapter_hook_extract_total` 的 metrics 截图
- [ ] **不**提交 commit（按惯例留给用户复审）
