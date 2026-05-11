# Plan 6b：Plan 6 已知问题修复（小补丁集）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Plan 6 端到端真链路打通后，冒烟暴露出 5 个已知问题（BOOTSTRAP 润色不知道孩子昵称、PostCheck 主角出现次数阈值对短故事过严、记忆软提示驱动不出彩蛋、孩子昵称 UTF-8 编码未校验、3 个 fail-open 路径无可观测指标）。Plan 6b 把这 5 个问题各自最小代价地修掉。**不引入任何新产品能力、不新增包、不新增外部依赖、不需要新迁移**——只做参数透传 + 一个模板措辞调整 + 一处 UTF-8 校验 + 一个新 Prometheus Counter 注入到现有 3 个 fail-open 现场。每个修复都有对应单测；冒烟回放复用 Plan 6 的脚本与孩子档案。

**Architecture:** 5 个修复彼此独立：(1) `bootstrap.Service.Submit` 多接 `nickname`，`renderDescription` 的 user prompt 拼接昵称；(2) `safety.PostCheckInput` 新增 `Duration int`，把硬编码常量 `minProtagonistOccurrences=3` 替换为按时长返回 2/3/4 的小函数；(3) `safety/system_prompt.tmpl` 把"已有记忆"分支从模板末尾上移到 IDENTITY 与"8 条强约束"之间作为独立"## 故事记忆上下文"段，并把措辞从"自然回调"改为"请尝试借用以下记忆里的角色或场景"；(4) `api/child.go` POST/PATCH 在 binding 后做 `utf8.ValidString(nickname)` 拒非法字节；(5) `metrics.Business` 加 `LLMFailFallbackTotal` CounterVec（labels: provider/model/reason），注入到 `safety/intent_llm.go`、`memory/summarizer.go`、`bootstrap/service.go` 现有的 3 个 warn 日志处。

**Tech Stack:**
- 完全复用 Plan 1-6 既有栈（Go + Gin + GORM + Prometheus + slog + apperr + testify）
- **不引入任何新依赖、不新增包、不新增外部 SDK、不需要迁移**

**前置阅读：**
- [Plan 6 - 2026-05-11-plan-06-bootstrap-and-memory.md](2026-05-11-plan-06-bootstrap-and-memory.md)（本 Plan 是它的直接 follow-up；交付的 BOOTSTRAP / Summarizer / Selector / 双分支模板 是本 Plan 的修改对象）
- [Plan 6 devlog - 2026-05-12.md](../../devlog/2026-05-12.md)（**核心** ——"Known Issues"小节即本 Plan 的需求规格；冒烟实测数据是验收基线）
- [MEMORY.md](../../../MEMORY.md) Plan 6 "关键技术教训"段（outbox payload 不可后改、fail-open 必须可观测 → 本 Plan 落实"必须可观测"）
- [CLAUDE.md](../../../CLAUDE.md)（4.2 内容安全；4.4 注释/文档风格；第 7 章必须解释知识点 + 同步落 `docs/knowledge/`）
- [docs/knowledge/05-software-design.md 5.16](../../knowledge/05-software-design.md)（outbox payload 不可后改 —— 本 Plan 不重复修，仅守住不要回退）
- [docs/knowledge/09-observability.md 9.14](../../knowledge/09-observability.md)（fail-open 必须配指标告警 —— 本 Plan 的 Issue 5 直接落地此原则）
- [docs/knowledge/11-llm-engineering.md 11.10](../../knowledge/11-llm-engineering.md)（软提示 vs 硬提示的取舍 —— 本 Plan 的 Issue 3 是它的一次小步实践）

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；新增/改动文件单测覆盖率 ≥ 70%（含 5 个修复各自的 happy + 边界用例）
2. 5 个 fix 各自有独立单测，能在没有 DB / 没有真 LLM 的前提下跑通：
   - `bootstrap.Service.Submit` 把 nickname 写进 user prompt（MockLLM 断言收到的 message 文本包含昵称）
   - `safety.PostChecker.Check` 在 `Duration=5` 且昵称出现 2 次的故事文本上 Pass；`Duration=15` 且只出现 3 次时 Fail
   - `prompt.Builder.Build` 渲染后 `SystemPrompt` 中"## 故事记忆上下文"段的 index < "【不可违反的 8 条强约束】"段的 index（顺序断言）
   - `api/child.go` POST/PATCH 收到含 `\xc8\xed\xa1\xa1` 字节的 nickname 时返回 400 `invalid_nickname`
   - 3 个 fail-open 现场（intent_llm / summarizer / bootstrap.renderDescription）在 MockLLM 强制返回 error 时各自把 `llm_fail_fallback_total{provider=...,model=...,reason=...}` +1（用 `prometheus/testutil.ToFloat64`）
3. `make run-dev` 启动后回放 Plan 6 冒烟脚本：
   - (a) `POST /api/v1/bootstrap/answers`（孩子昵称="小宇"）→ 200，`children.profile.description` 文本中至少包含一次"小宇"，**不再出现"小明"**或其他编造昵称
   - (b) 用 `duration=5` 生成短故事，即使昵称只在文本里出现 2 次（爱宝出场更突出时常见），PostCheck **不再** Fail 为 `child_not_protagonist`；用 `duration=15` 时阈值收紧到 4 次，恢复 Plan 6 旧行为
   - (c) 同孩子连续生成两个故事：第二个故事的 `system_prompt` 渲染日志可见"## 故事记忆上下文"段已位于 IDENTITY 之后、约束之前；manual 阅读第二个故事文本，**有较高概率**自然提到第一个故事中的角色/场景（不强制硬命中，但比 Plan 6 显著改善）
   - (d) `POST /api/v1/children` 直接构造一个 `\xc8\xed\xa1\xa1` 字节的 nickname → 返回 400 `{"reason":"invalid_nickname","user_msg":"昵称包含非法字节，请确保为 UTF-8"}`
   - (e) `curl http://localhost:8080/metrics | grep llm_fail_fallback_total` 至少出现一次（在 mock 模式下故意触发一次 LLM 错误后能看到对应 label 的 counter）
4. `golangci-lint run ./...` 0 issues
5. devlog [2026-05-13.md](../../devlog/2026-05-13.md) 写入"Plan 6b 完成 + Plan 6 5 个 known issue 关闭"小结
6. MEMORY.md 在"Plan 6 关键技术教训"段后追加一句"Plan 6b: 把 fail-open 现场补齐指标 + nickname UTF-8 + 软提示加权"
7. CLAUDE.md "当前阶段"小节翻页到"Plan 6b 完成，下一步进入 Plan 7"

---

## 范围决策记录（与用户对齐）

| Issue | 决策 | 不做什么（避免越界）|
|---|---|---|
| 1. BOOTSTRAP nickname 未传 | `Submit` 签名加 `nickname string` 参数；`api/bootstrap.go` 端用既有的 child 所有权校验链路顺手把 `child.Nickname` 取出来传进去。`renderDescription` 的 user prompt 改为 `孩子昵称：{nickname}。其他档案信息（JSON）：{json}` | 不重新设计 BOOTSTRAP 问题列表；不把昵称塞进 answers map（昵称已经是 `children.nickname` 列、不属于问卷答案） |
| 2. PostCheck `minProtagonistOccurrences=3` 太严 | `PostCheckInput` 新增 `Duration int`；新 helper `minProtagonistFor(duration) int`：5→2, 10→3, 15→4, 其他→3（默认守住旧行为）。Orchestrator 把 `p.Duration` 透传进来 | 不引入"自适应窗口"或"AI 二次判定"等复杂方案；不动 `aibaoCount > nickCount*2` 那一支（爱宝抢戏的判定与时长无关） |
| 3. 软提示彩蛋不工作 | 模板 2 个动作：(a) 把 `{{ if .MemorySummary }}` 分支从末尾移到 IDENTITY 段之后、"8 条强约束"之前，单独以"## 故事记忆上下文"标题成段；(b) 措辞从"可以自然回调，不要刻意提及'还记得吗'"改为"**请在新故事中尝试借用以下记忆里的角色或场景**，让孩子感受到熟悉的延续，但不要使用'还记得吗'这样的明确提示句"。`{{ else }}` 首次相遇分支**留在原位**不动 | 不加"第二轮编辑 LLM"做强制回调；不改 Selector 拼字符串的算法；不动注入字段名 |
| 4. nickname GBK 编码遗留 | POST + PATCH 在 ShouldBindJSON 后立刻 `utf8.ValidString(nickname)`，非法返回 400 `invalid_nickname` | 不做编码自动转换；不补"批量修复历史脏数据"脚本（Plan 6 已手动 UPDATE 修复一条；后续都走 UTF-8 入口拦截即可） |
| 5. fail-open 无可观测 | 新增 `LLMFailFallbackTotal *CounterVec`（labels: `provider`, `model`, `reason`）。注入到 3 个现场：intent_llm（reason=`upstream_error` 或 `unparseable`）、summarizer（reason=`upstream_error`）、bootstrap.renderDescription（reason=`upstream_error`） | 不引入告警规则（Prometheus 告警归运维侧 Plan 8+）；不改 fail-open 的语义（仍然不阻塞主流程） |

---

## File Structure

### Metrics

| 文件 | 改动 |
|---|---|
| `server/internal/metrics/business.go` | 新增字段 `LLMFailFallbackTotal *prometheus.CounterVec`；在 `NewBusiness` 里 `NewCounterVec` + `MustRegister` |
| `server/internal/metrics/business_test.go`（若已存在则补；不存在则新建） | 用 `testutil.CollectAndCount` 断言指标已注册 |

### Safety

| 文件 | 改动 |
|---|---|
| `server/internal/service/safety/postcheck.go` | `PostCheckInput` 加 `Duration int`；删除 `const minProtagonistOccurrences = 3`；新增 `minProtagonistFor(duration int) int`；`Check` 用新 helper |
| `server/internal/service/safety/postcheck_test.go` | 补 3 条：duration=5/10/15 各自的 nickname 出现次数边界用例 |
| `server/internal/service/safety/intent_llm.go` | 构造函数加 `biz *metrics.Business` 入参（保留旧构造函数签名兼容，或全局替换调用方）；在 2 处 warn 现场 Inc 计数器 |
| `server/internal/service/safety/intent_llm_test.go` | 触发 MockLLM 返回 error 与 unparseable 两条路径，断言 counter 各 +1 |

### Memory

| 文件 | 改动 |
|---|---|
| `server/internal/service/memory/summarizer.go` | 在 `if err != nil` 分支中（已有 `biz` 字段）增加 `s.biz.LLMFailFallbackTotal.WithLabelValues(provider, s.model, "upstream_error").Inc()`。provider 由调用方传或在构造函数补一个 `provider string` 字段；优先后者（一行字段 + 一行 setter） |
| `server/internal/service/memory/summarizer_test.go` | 复用既有 fail-open 测试，追加 counter 断言 |

### Bootstrap

| 文件 | 改动 |
|---|---|
| `server/internal/service/bootstrap/service.go` | `Submit(ctx, userID, childID int64, nickname string, answers []Answer)` 新签名；`renderDescription(ctx, nickname, answers)` 把 nickname 嵌进 user prompt；构造函数补 `provider string` 字段，在 fail-open warn 处 Inc `LLMFailFallbackTotal` |
| `server/internal/service/bootstrap/service_test.go` | (a) 断言 LLM 收到的 user message 文本含昵称；(b) fail-open counter 断言 |
| `server/internal/api/bootstrap.go` | 调用 `Submit` 前先用 ChildRepo（或注入 ChildReader 接口）取 child；把 `child.Nickname` 传入 Submit。所有权校验与既有逻辑合并到同一次查询，不重复读 DB |
| `server/internal/api/bootstrap_test.go` | 改造既有用例适配新签名；新增一条"用户 A 拿用户 B child_id"→403 用例保持回归 |

### Template + Prompt Builder

| 文件 | 改动 |
|---|---|
| `server/safety/system_prompt.tmpl` | (a) 移除当前位于模板尾部的 `{{ if .MemorySummary }} ... {{ else }} ... {{ end }}` 块的"有记忆"分支；(b) 在 IDENTITY 段（"【IDENTITY — 你是谁】"那段）与"【不可违反的 8 条强约束】"之间插入新的 "【故事记忆上下文】" 段（含 `{{ if .MemorySummary }} ... {{ end }}`）；(c) "首次相遇"`{{ else }}` 分支留在原位置（模板末尾"格式提醒"之前），单独包裹独立 `{{ if not .MemorySummary }}` 块即可 |
| `server/internal/service/story/prompt/builder_test.go` | 新增 `TestBuilder_MemorySectionPosition`：渲染后 `strings.Index(out.SystemPrompt, "【故事记忆上下文】") < strings.Index(out.SystemPrompt, "【不可违反的 8 条强约束】")`；同时断言措辞含"尝试借用以下记忆里的角色或场景" |

### Story Orchestrator

| 文件 | 改动 |
|---|---|
| `server/internal/service/story/orchestrator.go` | `PostCheckInput{Duration: p.Duration, ...}` 透传时长（一行新增字段） |
| `server/internal/service/story/orchestrator_test.go` | 已有 PostCheck 失败/成功用例上加一条"duration=5 且昵称出现 2 次的 LLM 输出仍 Pass"的覆盖 |

### API Layer (child)

| 文件 | 改动 |
|---|---|
| `server/internal/api/child.go` | `create` 和 `update` 在 ShouldBindJSON 之后立即用 `utf8.ValidString` 校验 nickname（update 时只在 `req.Nickname != nil` 时校验 `*req.Nickname`）；不通过返回 400 `invalid_nickname` |
| `server/internal/api/child_test.go` | 新增两条：POST 含 `\xc8\xed\xa1\xa1` 的 raw JSON body → 400；PATCH 同样 → 400 |

### Docs

| 文件 | 改动 |
|---|---|
| `docs/devlog/2026-05-13.md` | 新建：Plan 6b 完成记录 |
| `MEMORY.md` | 追加 Plan 6b 一句话教训 |
| `CLAUDE.md` | 第 2 节"当前阶段"刷新 |

---

## API 形态

**没有新接口。** 仅改动既有接口的错误响应：

- `POST /api/v1/children` —— 当 `nickname` 字段不是有效 UTF-8 字节序列时，新增一种 400 响应：
  ```json
  { "reason": "invalid_nickname", "user_msg": "昵称包含非法字节，请确保为 UTF-8" }
  ```
- `PATCH /api/v1/children/:id` —— 同上（仅当 `nickname` 字段存在且解码后非 UTF-8 时）
- `POST /api/v1/bootstrap/answers` —— 行为变更（润色 prompt 内部多塞了昵称），**响应结构不变**

`/metrics` 暴露面新增一项指标：

- `llm_fail_fallback_total{provider="doubao",model="ep-...",reason="upstream_error"|"unparseable"}` —— 累积计数器，每次 LLM 调用走 fail-open 兜底时 +1

---

## Tasks

### Task 0：metrics 新增 LLMFailFallbackTotal

**Files:**
- `server/internal/metrics/business.go`

**Steps:**
- [ ] 在 `Business` struct 中添加字段 `LLMFailFallbackTotal *prometheus.CounterVec`
- [ ] 在 `NewBusiness` 内用 `prometheus.NewCounterVec` 构造：Name=`llm_fail_fallback_total`，Help=`Count of LLM calls that fell back to safe default due to error or unparseable response.`，Labels=`{"provider","model","reason"}`
- [ ] 追加到 `reg.MustRegister(...)` 列表
- [ ] `go build ./...` 通过
- [ ] 若有 `business_test.go`，补一行断言 `b.LLMFailFallbackTotal` 非 nil；没有则跳过

**Commit:** `feat(metrics): add llm_fail_fallback_total counter for fail-open observability`

---

### Task 1：safety/intent_llm 注入 fail-open counter

**Files:**
- `server/internal/service/safety/intent_llm.go`
- `server/internal/service/safety/intent_llm_test.go`
- 调用方（main.go / wire 处）适配新构造函数

**Steps:**
- [ ] `LLMIntentProvider` 加字段 `biz *metrics.Business` 和 `provider string`
- [ ] 新构造函数 `NewLLMIntentProviderWithMetrics(c llm.Client, provider, model string, biz *metrics.Business) *LLMIntentProvider`；保留旧 `NewLLMIntentProvider` 内部调用新函数（biz=nil，provider="unknown"）便于现存测试不爆
- [ ] 在 `safety.intent_llm.fail_fallback_safe`（upstream error）处加：`if p.biz != nil { p.biz.LLMFailFallbackTotal.WithLabelValues(p.provider, p.model, "upstream_error").Inc() }`
- [ ] 在 `safety.intent_llm.unparseable` 处同样加，reason=`"unparseable"`
- [ ] 改 `server/cmd/server/main.go`（或 wire 文件）传 provider 字符串（例如配置里 `cfg.LLM.Provider`，或硬编码 `"doubao"`）
- [ ] 测试：构造两次 MockLLM——一次返回 error，一次返回 `"hello"` 这种 unparseable 文本——分别断言 counter 对应 label 值为 1（用 `testutil.ToFloat64(provider.biz.LLMFailFallbackTotal.WithLabelValues("mock","test","upstream_error"))`）

**Commit:** `feat(safety): instrument intent_llm fail-open with llm_fail_fallback_total`

---

### Task 2：memory/summarizer 注入 fail-open counter

**Files:**
- `server/internal/service/memory/summarizer.go`
- `server/internal/service/memory/summarizer_test.go`
- 构造调用方（main.go）

**Steps:**
- [ ] `Summarizer` 加 `provider string` 字段
- [ ] `NewSummarizer` 签名兼容性优先：增加 `NewSummarizerWithProvider`，老 `NewSummarizer` 透传 `provider="unknown"` 调它
- [ ] 在 `if err != nil` 分支中，紧挨着既有 `MemorySummaryTotal.WithLabelValues("fail").Inc()` 后面再加一行 `s.biz.LLMFailFallbackTotal.WithLabelValues(s.provider, s.model, "upstream_error").Inc()`
- [ ] main.go 调用方改用新构造函数传 provider
- [ ] 测试：MockLLM 强制返回 error → 调用 `Summarize` → 断言 counter +1 且返回 `""`

**Commit:** `feat(memory): instrument summarizer fail-open with llm_fail_fallback_total`

---

### Task 3：bootstrap.Service 注入 fail-open counter + nickname 传参

**Files:**
- `server/internal/service/bootstrap/service.go`
- `server/internal/service/bootstrap/service_test.go`
- `server/internal/api/bootstrap.go`
- `server/internal/api/bootstrap_test.go`
- main.go 适配构造函数

**Steps:**
- [ ] `Service` 加 `provider string`
- [ ] `Submit` 新签名：`Submit(ctx context.Context, userID, childID int64, nickname string, answers []Answer) (*Profile, error)`
- [ ] `renderDescription` 新签名：`renderDescription(ctx context.Context, nickname string, answers map[string]interface{}) string`；user message 改为 `fmt.Sprintf("孩子昵称：%s。其他档案信息（JSON）：%s", nickname, string(userPayload))`
- [ ] fail-open warn 后追加 `s.biz.LLMFailFallbackTotal.WithLabelValues(s.provider, s.model, "upstream_error").Inc()`
- [ ] `api/bootstrap.go`：在调用 `Submit` 之前用注入的 ChildReader（或直接复用 `childsvc.GetByID`，依现有所有权校验路径）取 child 的 Nickname；如果 not_found / not_owner，沿用既有错误映射
- [ ] 测试 A（service unit）：MockLLM 用 `RecordingMock` 捕获 `Messages[1].Content`，断言含传入的 nickname
- [ ] 测试 B（service unit）：MockLLM 返 error → counter +1，profile.Description=""，answers 仍写入
- [ ] 测试 C（api integration）：POST `/bootstrap/answers` happy path，验证 200

**Commit:** `fix(bootstrap): pass nickname into render prompt + add fail-open metric`

---

### Task 4：PostCheck duration-adaptive 主角阈值

**Files:**
- `server/internal/service/safety/postcheck.go`
- `server/internal/service/safety/postcheck_test.go`
- `server/internal/service/story/orchestrator.go`
- `server/internal/service/story/orchestrator_test.go`

**Steps:**
- [ ] `PostCheckInput` 加字段 `Duration int`
- [ ] 删除 `const minProtagonistOccurrences = 3`
- [ ] 新增 `func minProtagonistFor(duration int) int { switch duration { case 5: return 2; case 10: return 3; case 15: return 4; default: return 3 } }`
- [ ] `Check` 内替换 `nickCount < minProtagonistOccurrences` 为 `nickCount < minProtagonistFor(in.Duration)`
- [ ] `postcheck_test.go` 新增表驱动用例覆盖 5/10/15/0 四档
- [ ] `orchestrator.go` 在构造 `PostCheckInput` 处加 `Duration: p.Duration,`
- [ ] `orchestrator_test.go` 新增一条：duration=5、LLM 输出昵称只出现 2 次 → 期望 PostCheck Pass、走 LLM 路径（非 fallback）

**Commit:** `fix(safety): scale postcheck protagonist threshold by story duration`

---

### Task 5：system_prompt.tmpl 调整记忆段位置与措辞

**Files:**
- `server/safety/system_prompt.tmpl`
- `server/internal/service/story/prompt/builder_test.go`

**Steps:**
- [ ] 阅读 template 现状，确认 IDENTITY 段结尾位置
- [ ] 在 IDENTITY 与"【不可违反的 8 条强约束】"之间插入新块：
  ```
  {{- if .MemorySummary}}

  【故事记忆上下文】
  请在新故事中尝试借用以下记忆里的角色或场景，让孩子感受到熟悉的延续，但不要使用"还记得吗"这样的明确提示句。
  上次的故事记忆：{{.MemorySummary}}
  {{- end}}
  ```
- [ ] 删除模板末尾原有的 `{{- if .MemorySummary }} ... {{- end }}` 的"有记忆"分支（保留 `{{ else }}` "首次相遇"那一支，调整成独立的 `{{- if not .MemorySummary }} ... {{- end }}` 块留在原位）
- [ ] 新增测试 `TestBuilder_MemorySectionPosition`：构造 `BuildInput{MemorySummary: "测试摘要"}` → Build → 断言 `strings.Index(out.SystemPrompt, "【故事记忆上下文】")` 在 `strings.Index(out.SystemPrompt, "【不可违反的 8 条强约束】")` 之前；且 SystemPrompt 含子串 "尝试借用以下记忆里的角色或场景"
- [ ] 复跑既有 `prompt/builder_test.go` 全部用例确保未破坏首次相遇分支

**Commit:** `fix(prompt): elevate memory section + strengthen recall instruction`

---

### Task 6：child API UTF-8 校验

**Files:**
- `server/internal/api/child.go`
- `server/internal/api/child_test.go`

**Steps:**
- [ ] `import "unicode/utf8"`
- [ ] `create` handler：`ShouldBindJSON` 后立刻 `if !utf8.ValidString(req.Nickname) { c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason":"invalid_nickname","user_msg":"昵称包含非法字节，请确保为 UTF-8"}); return }`
- [ ] `update` handler：`if req.Nickname != nil && !utf8.ValidString(*req.Nickname) { ...同上... }`
- [ ] 测试构造非法 UTF-8：`badName := string([]byte{0xc8, 0xed, 0xa1, 0xa1})` —— 注意源文件本身仍是 UTF-8，但 `[]byte{...}` 字面量绕开了源文件编码约束；以这个字符串构造 JSON 请求体（`{"nickname":"<bad>","gender":"male","birthday":"2020-01-01"}` 直接拼字符串，不走 `json.Marshal`，因为 `json.Marshal` 会对非法字节做替换）
- [ ] 断言：状态码 400，响应 `reason == "invalid_nickname"`

**Commit:** `fix(child): reject non-UTF-8 nickname at create/update boundary`

---

### Task 7：手动回归冒烟

**Files:** 无代码改动

**Steps:**
- [ ] `make migrate-up` 确认无新迁移
- [ ] `make run-dev`
- [ ] 登录 + 创建/复用孩子（昵称"小宇"）
- [ ] 跑 BOOTSTRAP `POST /api/v1/bootstrap/answers` → 验证 description 中出现"小宇"，不出现"小明"
- [ ] `POST /stories/generate` duration=5 → 通过；duration=15 收紧到 4 次
- [ ] 连续生成两个故事，manual 阅读第二个故事是否有"借用"上一故事元素的迹象
- [ ] 用 raw curl/PowerShell 构造非法 UTF-8 nickname 的 POST `/children` → 期望 400 `invalid_nickname`
- [ ] `curl /metrics | findstr llm_fail_fallback_total` → 至少看到一次（mock 模式下故意触发 LLM 错误后）
- [ ] 把以上 5 项结果贴进 devlog

**Commit:** 无（手动验证步骤）

---

### Task 8：devlog + MEMORY + CLAUDE 同步

**Files:**
- `docs/devlog/2026-05-13.md`（新建）
- `MEMORY.md`（追加一句）
- `CLAUDE.md`（第 2 节翻页）

**Steps:**
- [ ] 写 devlog：5 个 fix 各自一段简要 + 冒烟结果数据 + 是否需要 Plan 6c
- [ ] MEMORY.md：在"Plan 6 关键技术教训"段后追加"Plan 6b: 把 fail-open 三处接入 llm_fail_fallback_total；PostCheck 主角阈值按时长分档；模板记忆段上移并改用强措辞；nickname UTF-8 守门"
- [ ] CLAUDE.md 第 2 节："Plan 6b 完成。下一步：写 Plan 7（BGM 混音 / 连续剧 / Flutter 客户端，按 spec 顺序由用户选）"
- [ ] 知识库：无新增词条（本 Plan 5 个修复都是对既有 5.16 / 9.14 / 11.10 三条词条的具体落地实践——只需在对应词条尾部追加一段"在 Plan 6b 中的应用"）

**Commit:** `docs: log plan 6b completion + sync MEMORY/CLAUDE`

---

## 不做什么（明确边界）

- 不引入新 LLM 调用、不引入新外部依赖、不动 Doubao endpoint 配置
- 不改 Outbox / Worker / TTS / Storage 任何逻辑（Plan 6 bugfix 已正确收尾）
- 不做"二次编辑 LLM"强制彩蛋——Issue 3 的修复止步于"软提示加权 + 位置上调"，验收只要求"较高概率"出现回调，不要求 100% 命中
- 不做历史脏数据批量修复（Plan 6 devlog 中 id=3 的乱码 nickname 已手动 UPDATE，后续靠 Task 6 拦截入口）
- 不动 CronCreate / 调度类需求
- 不引入新迁移文件——`memories.story_id` Plan 6 已建好
