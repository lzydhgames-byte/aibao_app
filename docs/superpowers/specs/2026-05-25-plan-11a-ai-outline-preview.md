# Plan 11A — AI 大纲预览（B 模式）spec

> 把"用户输入需求 → AI 直出整篇故事"的单趟流水，拆成"输入 → AI 大纲卡片 → 用户确认 → 生成正文"两阶段。

## 0. 一句话

用户在生成页只填**两件事**：一句话需求 + 时长（3/5/8 分钟）。AI 把主题、风格、教育意义、3-5 句梗概**当面"提案"**，用户点确认才进入正文生成；不满意可点"调整"修微调。

## 1. 背景与动机

### 1.1 当前痛点（来自 Plan 10 上线后真实用户反馈）

用户反馈："每次提交需求后还要手填主题、风格——'跟奥特曼一起冒险'这种意图明显的输入，AI 应该自己听懂。"

现状（Plan 9b 生成页）：用户必须手动选 **时长 + 风格 + 主题**三件套，"需求文本"反而是辅助。主次颠倒——AI 强项（理解需求）被旁路，AI 弱项（让用户帮忙做决策）被强化。

### 1.2 为什么 B 模式（预览-确认）而不是 A 模式（全自动）

| 维度 | A：全自动 | B：预览-确认 |
|---|---|---|
| 操作步骤 | 最少 | 多一步确认 |
| 用户掌控 | 看到结果才发现走偏 | 大纲阶段就能拦截 |
| 成本结构 | 烧错也是烧一整个故事 | 拒绝时只烧大纲一次小调用 |
| "AI 卖点时刻" | 无 | 大纲卡是产品差异化的"哇时刻" |

成本视角是决定性论据：B 模式让"拒绝率 X% 节省 Y 元/月"成为关键运营指标——和 Plan 11B（成本可观测）天然耦合。

## 2. 不在范围（YAGNI）

显式排除，避免范围漂移：

- ❌ **保留风格/主题手填字段作为"高级模式"**——MVP 阶段一刀切，砍掉。如果未来 5% 用户反馈"想强控风格"，再加二级路径
- ❌ **大纲入主库 PostgreSQL**——大纲是临时态，存 Redis 5 分钟 TTL 即可
- ❌ **多轮大纲对话**（"再换个角度"循环 N 次）——一次预览 + 一次调整就够，再不满意让用户改输入
- ❌ **大纲多语言/多文案模板**——一套中文模板足够
- ❌ **大纲 PostCheck 完整版**——跳过 PostCheck 的"主角占比"等正文专属校验。**但**：必须跑 §5.3 定义的 OutlineSafetyCheck（轻量）—— 大纲虽然给家长看，但 title/synopsis/educational_value 仍可能含恐惧词、IP 同人化错位、主角错位，不能裸过

## 3. 用户流程（新旧对比）

### 3.1 旧流程（Plan 9b）

```
[生成页]
  时长选择器 [3min / 5min / 8min]
  风格选择器 [温馨/冒险/搞笑/魔法/科普]    ← 用户必选
  主题选择器 [勇气/友谊/...6 选 1 + 换一批]  ← 用户必选
  需求输入框 [跟奥特曼一起冒险]
  [生成] ────────────────► 直接出整篇故事（30-90s）
```

### 3.2 新流程（Plan 11A）

```
[生成页 · 极简版]
  需求输入框 [跟奥特曼一起冒险]                 ← 主角字段
  时长 [3min / 5min / 8min]
  [让爱宝想想] ──► 大纲生成（~2s）

[大纲卡片页]
  📖 故事梗概
     小宇和奥特曼一起穿越到怪兽星球，
     他们要解救被困住的小光之子……
  🎯 教育意义：勇气 · 团队合作
  🎨 风格：冒险探索
  ⏱ 时长：5 分钟
  [开始生成]  [调整]  [换个角度]
```

**关键交互细节**：
- **[调整]** 打开抽屉式微调界面（不离开当前页），可改 `style` / `themes` / `educational_value`；**不能改需求文本 / 时长 / child_id**（这三项变化属于"新一轮预览"，要求点 [换个角度] 或回上一步重输）。调整后两种路径：
  - 点 **[直接确认]** → 不重新生成大纲，直接带 `outline_overrides` 进 generate
  - 点 **[重新预览]** → 用调整后的字段作为新一轮 hint 调 `/outlines/preview`（同 prompt+duration，新 outline_id）
- **[换个角度]** = 用同一份输入再生成一次大纲（同 prompt+duration+child_id，nonce 破缓存）。**不算 outline 被拒**——算 refresh。
- **[开始生成]** = 提交 `outline_id` 给 `/stories/generate` → 进 player 页等待

**outline outcome 状态机**（11B 成本归集依据）：
- `pending` — 大纲已生成、未到终态
- `accepted` — 用户点 [开始生成]（或 [直接确认]）成功调起 `/stories/generate`
- `refreshed` — 用户点 [换个角度]，本 outline_id 作废、新 outline_id 替代（计入 `outline_refreshed_total`，不计入 rejected）
- `expired` — 5 分钟无操作，Redis TTL 过期（计入 `outline_expired_total`）
- `abandoned` — 用户离开页面或回退（无显式信号，由 expired 兜底）

## 4. 系统架构

### 4.1 模块拓扑（在 Plan 4 架构上的增量）

```
                 ┌─────────────────────────────────────────┐
                 │  POST /api/v1/outlines/preview          │ ← 新端点
                 │  POST /api/v1/outlines/:id/refresh      │ ← 换个角度
                 └────────────────┬────────────────────────┘
                                  │
                                  ▼
                 ┌─────────────────────────────────────────┐
                 │  service/outline/                       │ ← 新模块
                 │    Preview(ctx, child, prompt, dur)     │
                 │    └─ PreCheck → LLMOutlineGen          │
                 │       → Cache(Redis 5min TTL)           │
                 └────────────────┬────────────────────────┘
                                  │
                                  ▼
                       gateway/llm  (复用，Gateway 不变)
                       gateway/safety (复用 PreCheck)
                       gateway/cache (Redis)

                 ┌─────────────────────────────────────────┐
                 │  POST /api/v1/stories/generate          │ ← 微改（向后兼容）
                 │    新增可选 outline_id 字段              │
                 └────────────────┬────────────────────────┘
                                  │ outline_id 有 → 读 Redis 拿 outline
                                  ▼ outline_id 无 → 走旧逻辑（兼容期）
                       service/story/Orchestrator (现有，微改)
                         └─ Step 0: HydrateFromOutline (新)
                         └─ Step 1..5 不变 (Prompt/LLM/PostCheck/...)
```

**关键解耦点**：
- `service/outline/` 与 `service/story/` **完全独立**，通过 Redis cache 弱耦合
- Outline 失败 → 不影响现有故事生成路径（兼容期内）
- Story Orchestrator 拿不到 outline → fallback 到旧"用 prompt 字段直接组装 system prompt"路径

### 4.2 数据流（端到端时序）

```
家长                Flutter             Backend             LLM        Redis
  │                    │                   │                 │           │
  │─输入"跟奥特曼…"──►│                   │                 │           │
  │                    │─POST /outlines/preview ───────────► │           │
  │                    │                   │─PreCheck       │           │
  │                    │                   │─LLM 大纲调用──►│           │
  │                    │                   │◄───大纲 JSON──│           │
  │                    │                   │─SET outline ─────────────► │
  │                    │◄──{outline_id,..} │                 │           │
  │◄─大纲卡片展示─────│                   │                 │           │
  │                    │                   │                 │           │
  │─点 [开始生成] ────►│                   │                 │           │
  │                    │─POST /stories/generate {outline_id}─►          │
  │                    │                   │─GET outline ◄───────────── │
  │                    │                   │─Orchestrator 走完整流水线 │
  │                    │◄──{story_id,..}  │                 │           │
  │◄─进 player 等待──│                   │                 │           │
```

## 5. 大纲数据契约

### 5.1 LLM 输出 JSON Schema

大纲 LLM 调用 `response_format=json` 强制结构化输出：

```json
{
  "title": "小宇和奥特曼的怪兽星球冒险",
  "synopsis": "小宇和奥特曼穿越到怪兽星球，发现……（3-5 句梗概，控 80-120 字）",
  "themes": ["勇气", "团队合作"],
  "style": "冒险探索",
  "duration_min": 5,
  "educational_value": "学到遇到困难不退缩、和伙伴分工合作的重要性",
  "scene_seed": "S047"
}
```

**字段约束**：
- `style` 必须在 5 个预设值之一（温馨治愈/冒险探索/搞笑欢乐/神奇魔法/科普认知）——用 enum 校验
- `themes` 长度 1-3，从 50 主题词库选（与现有 prompt builder 共享词库）
- `duration_min` 必须等于用户传入的时长（防 LLM 自己改）
- `scene_seed` 复用 Plan 9c 的 80 种子池，由后端注入而非 LLM 选

### 5.2 Redis 缓存键

```
outline:{outline_id}     (UUID v4, crypto-random 128bit, 5min TTL)
value: JSON.stringify({
    outline_fields,
    user_id,                  ← 用于 ownership 校验
    child_id,                 ← 用于 ownership 校验
    prompt_text,
    outline_prompt_version,   ← 用于 A/B 与质量回放（见 §5.4）
    outline_group_id,         ← 同一意图的所有 outline 共享（refresh 后变体共组）
    variant_index,            ← 本组内序号（0 起，refresh 递增）
    parent_outline_id,        ← refresh 来源（首次预览为空）
    created_at
})
```

**outline_id 设计**：crypto-random UUID v4，128 bit。短期票据可猜性近似为零。**outline_id 严禁出现在 access log / metric label / 错误响应正文里**（错误体只回 reason code，不回 ID）。

**Ownership 校验（强制）**：`/stories/generate` 拿到 outline_id 后必须校验 **user_id + child_id + outline_id 三元一致**。任一不一致返 403。

**过期处理**：5 分钟无操作 Redis TTL 失效；`/stories/generate` 拿不到 outline → 返 **410 Gone**（不是 400，下文 §6.5 错误码统一）。**绝不自动重新生成大纲冒充用户确认**——这违反"用户确认的是 A，实际生成的也必须是 A"的承诺。

**Redis 不可信假设**：Redis AOF/重启可能丢失大纲——视为"过期同等处理"，前端引导用户重新预览。**不依赖 Redis TTL 推断 outline outcome 事件**——outcome 由显式 API 调用驱动写入 `outline_events`（见 §5.5）。

### 5.3 OutlineSafetyCheck（轻量大纲安全校验）

**输入端**（PreCheck 之前 / 之内）：复用 `gateway/safety` PreCheck 跑用户 `prompt` 文本——红线词、害怕清单（含本 child profile 的个性化害怕词，从 [[bootstrap-fears]] 记忆拿）、IP 黑名单。

**输出端**（LLM 返回 JSON 之后，写 Redis 之前）：跑 `OutlineSafetyCheck(outline)`，覆盖：
1. `title` + `synopsis` + `educational_value` 三字段拼接后跑红线词扫描（reuse `gateway/safety` matcher）
2. `title` + `synopsis` 跑 child 个性化害怕词扫描
3. 主角校验：synopsis 必须含 child nickname（命中失败 → reject reason=`protagonist_missing`）
4. IP 同人化：synopsis 命中 IP 黑名单走转写规则；命中 IP 白名单（如奥特曼）走"陪伴角色而非主角"语义验证（启发式：主角 ≠ IP 名）

**安全失败处理**：
- 命中 → 不写 Redis，返 **422** + `reject_reason` + `category`
- 单次 LLM 返回的 outline 安全失败 → **允许 1 次自动 repair**：重发 LLM，附加 "请避免出现 X 类内容" hint。仍失败则用户侧报错。
- 重试次数计入 `outline_safety_repair_total` metric

### 5.4 大纲 prompt 版本化

`outline_prompt_version` 是 `vYYYYMMDD-N` 格式字符串（如 `v20260525-1`），由 prompt 模板代码内常量定义。每次模板变更 bump 版本号。

**用途**：
- Redis payload 记录
- 写入 `outline_events` + `cost_events`
- 日志结构化字段
- 未来 A/B 实验按 version 分桶

### 5.5 outline_events 轻量事件表

**实施口径声明**（T3）：`outline_events` 是 **append-only 事件流**，**不**对历史行做 UPDATE。每次状态变化追加一行新事件，`outline_id` 在表中允许多行；最新生命周期由"按 occurred_at 取最后一行"得出。

**为什么 append-only 而非 mutable-state**：
- 与现有 `outbox_events` 模式一致（Plan 4 已立的范式），团队心智成本低
- 历史轨迹可审计（refresh 几次、何时 expired 全在表里）
- 报表 JOIN 一律走 `SELECT DISTINCT ON (outline_id) ... ORDER BY outline_id, occurred_at DESC` 拿"最新生命周期"，避免 double count
- expired 也是追加一行（不是改 pending 行的 outcome），housekeeping 同理

**例外 — A2 expired 双路径的 SQL 形态**：
- ❌ **不要**用 `UPDATE outline_events SET outcome='expired' WHERE outcome='pending' AND ...`
- ✅ 用 `INSERT INTO outline_events (...outcome='expired'...) SELECT ... WHERE NOT EXISTS (SELECT 1 FROM outline_events WHERE outline_id=X AND outcome IN ('accepted','refreshed','expired'))`
- 即"该 outline_id 还没有终态行 → 追加一条 expired"；幂等且无锁竞争

不建完整 outline 主表，但需要轻量"生命周期事件"用于成本与运营分析：

```sql
CREATE TABLE outline_events (
    id              BIGSERIAL PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL,
    outline_id      VARCHAR(64) NOT NULL,
    outline_group_id VARCHAR(64) NOT NULL,
    user_id         BIGINT NOT NULL,
    child_id_hash   VARCHAR(64) NOT NULL,
    outcome         VARCHAR(16) NOT NULL,  -- pending|accepted|refreshed|expired|abandoned
    outline_prompt_version VARCHAR(32),
    duration_min    INTEGER,
    trace_id        VARCHAR(64)
);

CREATE INDEX idx_outline_events_group ON outline_events(outline_group_id);
CREATE INDEX idx_outline_events_user_day ON outline_events(user_id, occurred_at);
```

**写入时机**：
- `preview` 成功 → `outcome=pending`（同步 PG INSERT）
- `refresh` → 旧 outline 写 `outcome=refreshed`，新 outline 写 `outcome=pending`（同事务）
- `/stories/generate` 接受 outline_id → 旧 outline 写 `outcome=accepted`（在 Orchestrator step 0 校验通过后立即写，与故事生成在同一请求生命周期）
- **TTL 过期识别**（A2 修正——不裸依赖 Redis TTL 推断）：
  1. **主动驱动**（主路径）：用户每次进 `/stories` list 或 `/heartbeat` 时，后端顺手扫该 user 名下 `outcome=pending` 且 `occurred_at < now - 5min30s` 的行 → 改 `outcome=expired`
  2. **兜底 housekeeping**（每 10 分钟一次）：扫全表 `outcome=pending AND occurred_at < now - 10min` → 改 `expired`，处理无活跃用户的孤儿
  3. 两条路径用 SQL `UPDATE ... WHERE outcome='pending'` 保证幂等
- `abandoned` 无显式信号，由 `expired` 兜底

**与 cost_events 的关系**：
- `outline_events` 关心生命周期（outcome）
- `cost_events` 关心钱（per LLM call）
- 同一 outline_id 在两表都有记录；JOIN by outline_id

**双表写入顺序与一致性**（N1）：
- `outline_events` **同步**写 PG（小行级 INSERT，事务内随主业务一起提交；失败则 outline preview 整体失败）
- `cost_events` **异步**经 Recorder 队列 → flusher 批量写（详 11B §3.3）
- **JOIN 口径**：报表与对账以 `outline_events` 为权威生命周期数据；`cost_events` 缺行视为"待补"（最多滞后 1 分钟 batch + 5s 关停 flush）
- **不一致监控**：定时 job 每小时扫"过去 24h 内 outline_events 有 outcome=accepted 但 cost_events 找不到对应 outline LLM call"的孤儿 → 写 metric `cost_outline_join_miss_total`；持续 >0.5% 触发告警
- 双表**不共享事务**——cost_events 写失败不能回滚 outline 状态（钱已经花了）

## 6. API 契约

### 6.1 新增：`POST /api/v1/outlines/preview`

**请求**：
```json
{
  "child_id": 1,
  "prompt": "跟奥特曼一起冒险",
  "duration_min": 5
}
```

**响应（200）**：
```json
{
  "outline_id": "ol_8f3a2b...",
  "outline": { ... 5.1 中 JSON ... },
  "expires_at": "2026-05-25T15:35:00+08:00"
}
```

**错误**（统一信封见 §6.5）：
- **400** `bad_request`：duration_min 非 [3,5,8] / 必填字段缺失
- **403** `forbidden`：child_id 不属于当前 user
- **422** `safety_rejected`：PreCheck 或 OutlineSafetyCheck 拒绝，body 含 `category` + `reason_code`
- **429** `rate_limited`：限流（详见 §6.4），含 `retry_after` 秒数

### 6.2 新增：`POST /api/v1/outlines/:id/refresh`

"换个角度"。同 child_id + prompt + duration，但 nonce 不同破缓存。响应同 6.1。**`:id` 必须存在且属于当前 user**——不存在/不属于返 **404**（含义统一为"找不到资源"）。

新 outline 与原 outline **共享 `outline_group_id`**，`variant_index` 递增，`parent_outline_id` 指向原。原 outline 在 Redis 立即失效；`outline_events` 写一行 `outcome=refreshed`。

### 6.3 微改：`POST /api/v1/stories/generate`

向后兼容地新增可选字段：

```json
{
  "child_id": 1,
  "outline_id": "ol_8f3a2b...",       ← 新增
  "outline_overrides": {               ← 新增（可选；调整抽屉里改的字段）
    "style": "搞笑欢乐",
    "themes": ["友谊"],
    "educational_value": "学到分享的快乐"
  },
  // 旧字段保留过渡期（duration_min/style/topic/prompt）
}
```

**outline_overrides 白名单**（**安全要求**，后端强制校验）：
- ✅ 允许：`style` / `themes` / `educational_value`
- ❌ 拒绝：`duration_min` / `child_id` / `prompt` / `scene_seed`（这些字段必须等于服务端记录的 outline 原始值；不一致返 400）
- 后端实现：拿到 request → 校验 outline_id 归属 child_id → 校验 overrides 只含白名单字段 → 才进 Orchestrator

**优先级**：`outline_overrides` > `outline.字段` > 旧手填字段 > 默认值。这让前端可以**同时**走新路径，过渡期内旧客户端不受影响。

### 6.4 限流策略

全部 outline LLM 调用走**统一桶**（避免 preview + refresh 分桶用户实际能打 15 次/min）：

| 端点 | 限流 |
|---|---|
| `/outlines/preview` + `/outlines/:id/refresh` | 共用桶 **per-user 5/min**（首发） |
| `/stories/generate` | 保持原 5/min |

**调优计划**：上线后跑 1 周看 `outline_preview_total` + `outline_refreshed_total` 真实分布。如朋友试用普遍打不满 5/min 且 refresh 率 <30%，下期放宽到 8/min；如 refresh 率 >50% 暗示大纲质量差需治本，不靠限流。

### 6.5 统一错误信封

所有 outline 端点返回**统一 JSON 信封**，便于前端区分 UX 状态：

```json
{
  "error": {
    "code": "outline_expired",         // 机器可读，前端按此分支
    "message": "大纲已过期，请重新预览",   // 用户可见文案
    "category": "fears_personalized",  // 可选，安全拒绝时填
    "retry_after": 42                  // 可选，限流时填（秒）
  }
}
```

**状态码 ↔ code 对照**：

| HTTP | code | 含义 |
|---|---|---|
| 400 | `bad_request` | 参数缺失/格式错 |
| 400 | `conflicting_modes` | 同时传 outline_id + storyline_id（互斥，见 §6.6 + §10.1） |
| 403 | `forbidden` | child_id/outline_id ownership 不一致 |
| 404 | `not_found` | outline_id 不存在或不属于此 user |
| 410 | `outline_expired` | outline TTL 到期 / Redis 缺失 |
| 422 | `safety_rejected` | PreCheck 或 OutlineSafetyCheck 命中（含 `category`）|
| 429 | `rate_limited` | 限流（含 `retry_after`）|
| 500 | `llm_failed` | LLM 调用/JSON 解析失败 |
| 503 | `budget_exceeded` | 全局预算熔断（已有） |

### 6.6 新旧路径并存规则

为防"旧/新字段混传"导致语义模糊，**强制规则**：

| 请求形态 | 处理 |
|---|---|
| **同时**带 `outline_id` **和** `storyline_id` | **400 `conflicting_modes`** — 模式互斥，客户端二选一（见 §10.1） |
| 有 `outline_id`（无 storyline_id，无论是否带旧字段） | **必须**走新路径；旧字段（duration_min/style/topic/prompt）**全部忽略**，日志记录混传告警 |
| 有 `storyline_id`（无 outline_id） | 走续集路径（Plan 8），跳过大纲（见 §10.1） |
| 无 `outline_id` 也无 `storyline_id`，有旧字段全集 | 走旧路径（兼容期） |
| 无 `outline_id` 也无 `storyline_id`，旧字段不完整 | 400 `bad_request` |
| 有 `outline_id` 但 Redis 拿不到 | **410 `outline_expired`**，不 fallback 旧路径——若 fallback，"用户确认的 A 实际生成 B" 风险出现 |

### 6.7 弃用计划

旧路径（不带 `outline_id`）在 Plan 11A 上线后保留 **2 个 minor 版本**，第 3 个版本后端开始返回 `Deprecation` header，第 5 个版本下线。

## 7. 后端实施细节

### 7.1 新模块 `service/outline/`

```
internal/service/outline/
├── service.go         Preview(ctx, in) (*Outline, error)
├── llm_prompt.go      LLM 大纲 prompt 模板
├── llm_parser.go      response_format=json 解析 + 校验
├── cache.go           Redis SET/GET 包装
└── service_test.go
```

**Preview 流程**：
1. **输入安全**：PreCheck on user `prompt`（复用 `gateway/safety`，**完整跑**——含个性化害怕词、IP 黑白名单）
2. **LLM 调用**：调 `gateway/llm` Generate，`response_format=json`，附 `purpose=outline` + `outline_prompt_version` label（成本/质量归类用）
3. **JSON 解析**：严格 schema 校验（unknown field 拒绝） + enum/长度校验
4. **JSON 修复重试**：若 schema 失败 → 1 次 repair retry（提示模型"上次返回缺少字段 X / 字段 Y 越界"）；仍失败返 500 `llm_failed`
5. **输出安全**：跑 OutlineSafetyCheck（见 §5.3）on `title` + `synopsis` + `educational_value`；失败 → 1 次 repair retry；仍失败返 422 `safety_rejected`
6. **后端注入**：scene_seed（后端选，不让 LLM 自由发挥）+ outline_prompt_version + group/variant 元数据
7. **持久化**：写 Redis（含 ownership 字段，见 §5.2） + 写 `outline_events` outcome=pending → 返 outline_id

**Gateway 不调 Recorder**（架构分层修正）：`gateway/llm` 在响应中**只返 usage struct**（tokens_in/out/duration_ms）；由 `service/outline/` 拿到响应后调 `service/cost/Recorder.Record`。这保证 `gateway/*` 仅依赖 `pkg/*`，不依赖 `service/*`。

### 7.2 LLM 模型选型 + 成本估算

**大纲调用走 doubao-1.5-lite-32k**（已用于 memory summary 和 chapter hook）：
- 输入 ~600 tokens + 输出 ~400 tokens（**估算值**，实际由 cost_events 校准）
- 按 11B §5.2 单价（0.30/0.60 元/1M tokens）：单次大纲 ≈ `(600×0.30 + 400×0.60)/1_000_000 = ¥0.000420`
- 比 doubao-pro 便宜约一个数量级；大纲是结构化输出，对模型理解力要求中等，lite 够用

**口径声明**：上述数字仅为**预估**，单价以 Plan 11B Task 1（豆包/Minimax 官方计费单据校对）为准；实际成本以 `cost_events` 真实写入数据为准。spec 写死的任何元数字仅供方案论证，不作生产配置依据。

如实测 lite 主题推断错率 >15% 或 OutlineSafetyCheck repair 率 >10%，第二期升 pro——这是 **可配置项**（config.yaml `outline.llm.model`），不是硬编码。

### 7.3 Orchestrator 微改

在现有 5 步流水线**之前**新加一步，**仅在 outline_id 存在**时启用；无 outline_id 走旧路径：

```go
// step 0 (new, only when in.OutlineID != "")
outline, err := outlineResolver.Resolve(ctx, in.OutlineID, in.UserID, in.ChildID)  // 见下 §7.5
if err != nil {
    if errors.Is(err, ErrOutlineExpired) { return Err410 }
    if errors.Is(err, ErrOutlineForbidden) { return Err403 }
    return Err500
}
applyOverrides(outline, in.OutlineOverrides)  // 仅白名单字段，见 §6.3
in.Style         = outline.Style
in.Themes        = outline.Themes
in.SceneSeed     = outline.SceneSeed
in.TitleHint     = outline.Title       // 喂正文 prompt（B1）
in.SynopsisHint  = outline.Synopsis    // 喂正文 prompt（B1）
in.EducationalValueHint = outline.EducationalValue
// step 1..5: 原有流水线不动
```

**正文 prompt 模板新增段**（首尾呼应，防大纲/正文撕裂）：
```
## 本故事的预先设定（家长已确认）
- 标题：{{.TitleHint}}
- 梗概：{{.SynopsisHint}}
- 教育意义目标：{{.EducationalValueHint}}
请把以上设定作为故事骨架展开为完整故事，**不要偏离梗概的主要情节走向**。
```

### 7.4 限流

详见 §6.4：outline 端点共享 **per-user 5/min**；story generate 保持 5/min。

### 7.5 OutlineResolver 接口（包级解耦，N5 修正）

`service/story/Orchestrator` 不直接访问 Redis 或 `service/outline/cache`。引入小接口 `OutlineResolver`。

**接口位置**（N5 关键）：放在**独立中立包** `internal/service/outlinecontract/`，**不**放在 `service/outline/`——否则 `service/story` 仍要 `import "service/outline"`，包级双向依赖。

```
internal/service/
├── outlinecontract/         ← 中立合约包（只有接口 + DTO，无实现、无 IO）
│   ├── resolver.go          OutlineResolver interface + Outline DTO
│   └── errors.go            ErrOutlineExpired / ErrOutlineForbidden / ErrOutlineNotFound
│
├── outline/                 ← 实现方
│   └── resolver_impl.go     实现 OutlineResolver（内部读 Redis + 校验 ownership）
│
└── story/                   ← 消费方
    └── orchestrator.go      import "service/outlinecontract"（不 import "service/outline"）
```

```go
// service/outlinecontract/resolver.go
package outlinecontract

type Outline struct {
    OutlineID, Title, Synopsis, Style, EducationalValue string
    Themes []string
    SceneSeed string
    OutlineGroupID, ParentOutlineID string
    VariantIndex int
    OutlinePromptVersion string
}

type OutlineResolver interface {
    Resolve(ctx context.Context, outlineID string, userID, childID int64) (*Outline, error)
}
```

**main wire**（main.go）：
```go
outlineCache := outlinecache.New(redis)              // service/outline 子包
outlineResolver := outline.NewResolver(outlineCache) // 实现 outlinecontract.OutlineResolver
orchestrator := story.NewOrchestrator(... , outlineResolver)
```

**收益**：
- `service/story` 包**仅依赖** `service/outlinecontract`，不依赖 `service/outline`——编译期保证单向
- 测试 `service/story` 可用 mock OutlineResolver，零 outline 实现依赖
- 未来把 outline 迁到外部 KV（pebbledb / etcd / 自建 service），只换 `service/outline/resolver_impl.go` + main wire，`service/story` 不动

**反模式（避免）**：
- ❌ 把接口放在 `service/outline/contract.go`——`service/story` 仍要 import `outline` 包
- ❌ 把接口放在 `service/story/`——倒挂依赖，outline 实现要 import story
- ✅ 独立中立包是唯一干净解

## 8. Flutter UI 改动

### 8.1 generate_screen 重做

**当前**：duration picker + style picker + topic picker + prompt field + submit button。

**新版**：
- prompt field（占主视觉）
- duration segment control（3/5/8）
- "让爱宝想想" 主 CTA
- （隐藏字段：style/topic 默认 null，靠 outline 来填）

### 8.2 新增 outline_screen

- 故事标题 + 梗概 + 教育意义 + 风格 + 时长（卡片视觉）
- "开始生成" 主 CTA
- "调整" 抽屉按钮
- "换个角度" 次 CTA
- 5 分钟倒计时（提醒大纲会过期）

### 8.3 路由

```
home → generate（输入）→ outline（预览）→ player（正文等待）
                    │            │
                    └─────╳ 旧路径直接 player（兼容期）
```

### 8.4 状态管理

- `outlinePreviewProvider`（FutureProvider.family，key 由 prompt+duration 组成）
- `currentOutlineProvider`（StateProvider，存当前确认的 outline）
- 生成成功跳转前 `invalidate(outlinePreviewProvider)` 防陈旧（Plan 9b 教训）

## 9. 测试策略

| 层级 | 范围 |
|---|---|
| 单元 | service/outline/ 每函数 + JSON 解析容错 + cache TTL + Resolver ownership 校验 + Overrides 白名单 |
| 集成 | testcontainers Redis+PG + Mock LLM，跑 Preview→Generate 完整链路 |
| 端到端 | 真豆包 lite 跑 5 次 preview，人工验主题/风格/教育意义合理性；TitleHint/SynopsisHint 注入正文后大纲与正文骨架是否一致 |
| 回归 | Plan 9d smoke 脚本扩：outline preview→accepted→generate；refresh 切换；expired 410 路径 |

### 9.1 安全负例（必跑，每条都必须有 golden case）

| 场景 | 期望结果 |
|---|---|
| LLM 返回 title 含红线词 | OutlineSafetyCheck 拒，repair 1 次仍失败返 422 + category |
| LLM 返回 synopsis 含 child fears（个性化害怕词） | 拒 + category=fears_personalized |
| LLM 返回 synopsis 主角是奥特曼而非孩子 | 拒 + category=protagonist_missing |
| LLM 返回 JSON schema 不合规（缺 themes 字段） | 1 次 repair；仍失败 500 llm_failed |
| LLM 返回 duration_min 偷改成 10 | 服务端覆写为请求时 duration（or 拒，但更严格） |
| LLM 返回 unknown 字段 | schema 拒 + repair |
| outline_overrides 含 `child_id` 字段 | 400 bad_request（白名单外） |
| 跨用户 outline 越权（user_A 的 outline 用 user_B 的 token confirm） | 403 forbidden |
| 跨 child 越权（同 user 不同 child） | 403 forbidden |
| outline_id 篡改/伪造 | 404 not_found（找不到） |
| Redis 重启后 outline 丢失 | /stories/generate 返 410 outline_expired |
| TTL 自然过期 | 410 outline_expired |
| 限流并发触发 | 429 rate_limited + retry_after |

### 9.2 兼容测试

- 旧 Flutter 客户端（不带 outline_id）继续工作 → smoke 通过
- 混传（带 outline_id 同时带 duration_min）→ 服务端忽略旧字段、走新路径、告警日志
- 11A 与 Plan 8 storyline 续集互斥见 §10
- 11A 与 BOOTSTRAP 害怕清单注入 PreCheck 见 §10

### 9.3 主观验收

找你和你朋友各跑 5 个真实输入（"想听个温柔点的"/"明天考试想给孩子打气"/"小宇喜欢恐龙"），看大纲是否"懂用户"。这条没自动化指标，靠人判。验收标准：5/5 中至少 4 个大纲让你"愿意点开始生成"。

## 10. 现有功能兼容性

### 10.1 与 Plan 8 storyline（连续剧）

**规则**：续集（`storyline_id` 非空）与大纲（`outline_id` 非空）**严格互斥**。理由：续集已通过 `chapter_hook` + 上集 summary 隐式承接，再加大纲会双重设定且容易冲突；语义二义性会让 spec/客户端/测试同时变复杂。

**契约**（与 §6.6 一致）：
- Flutter 端：home 页"续集卡片"点进去直接进 generate 接口（仅传 `storyline_id`），不经 outline 流程
- Flutter 端：generate 输入页"让爱宝想想"路径仅传 `outline_id`（无 storyline_id）
- 后端：`/stories/generate` 收到**同时**含 `outline_id` 和 `storyline_id` → 立即返 **400 `conflicting_modes`**（**不**做"哪个优先"猜测，让客户端显式二选一）
- 续集路径（仅 storyline_id）：`cost_events` purpose 记 `story` + storyline_id 关联，**不**写 `outline_events`
- 大纲路径（仅 outline_id）：正常走 §5/§6 + outline_events 完整生命周期

**未来扩展**：续集启用大纲需另起子 plan（11C？），需先解决 "上集承接 + 大纲设定" 双约束的产品体验冲突，超本期。

### 10.2 与 BOOTSTRAP 害怕清单

`OutlineSafetyCheck` 输入端 PreCheck 跑用户 prompt 时，**必须**注入当前 child 的个性化害怕词（从 child profile / memory `[[bootstrap-fears]]` 取）。

输出端 OutlineSafetyCheck on `title` + `synopsis` 同样要查个性化害怕清单——这是 §5.3 第 2 项的明文要求。

### 10.3 与 HEARTBEAT 时段问候

无冲突。HEARTBEAT 只读、不进大纲流程；home 页时段问候独立于 outline。

### 10.4 与 Plan 9c 红线词/SceneSeed

- 红线词：OutlineSafetyCheck 复用 `rules.yaml`（共享数据源，与 PostCheck 同一份）
- SceneSeed：大纲 prompt **不让 LLM 选**，由后端注入；正文 prompt 沿用大纲选定的 seed（不重新抽）——这避免大纲是"海边"正文却"森林"的撕裂

## 11. 上线策略 + 回滚

### 10.1 灰度

- Backend 部署后 outline 端点立即可用
- Flutter 端用 `feature_flag.outline_enabled` 控制走新/旧路径
- 上线第 1 天 flag=false（后端先跑通），第 2 天朋友试用打开

### 10.2 回滚

- Flutter 端：flag=false 立即退回旧 UI
- Backend：outline 端点保留但前端不调用，零影响

### 10.3 监控

新增 metric：
- `outline_preview_total{status, child_id_hash}`
- `outline_preview_duration_seconds`
- `outline_outcome_total{outcome}`（accepted / refreshed / expired，对齐 §3.2 状态机）
- `outline_safety_repair_total`（安全 repair 重试次数，超阈值告警）

这些 metric 直接喂 Plan 11B 的成本视图。

## 12. 涉及的现有文档更新

- `docs/superpowers/specs/2026-04-28-aibao-design.md`：3.3.4 节"故事生成流程"改写为新流程；2.1 节"操作者：家长"加一句"AI 协助理解需求"
- `docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md`：4.1 服务目录加 `service/outline/`；时序图加大纲阶段；7.2 PreCheck 段标注"大纲调用复用 PreCheck + 增加 OutlineSafetyCheck 输出端校验"
- `MEMORY.md` / `CLAUDE.md`：Plan 11A 落地后追加

## 13. 风险与待定

### 13.1 上线前 must-fix 风险

| 风险 | 缓解 | 状态 |
|---|---|---|
| 大纲跳过完整 PostCheck 导致儿童害怕/IP 错位内容流向家长视野 | OutlineSafetyCheck（§5.3）+ 8 类安全负例 golden cases（§9.1） | **must-fix** |
| outline_id 越权（跨 user / 跨 child） | Redis payload 含 user_id + child_id + 三元校验（§5.2） | **must-fix** |
| outline_overrides 携带 child_id/prompt/duration 等敏感字段越权 | 白名单只 3 字段（§6.3）+ 后端强制校验 | **must-fix** |
| outline_id 落入日志/metric label 被横向移动 | log/metric 严禁出现 outline_id 明文（§5.2） | **must-fix** |
| 大纲 vs 正文调性脱节（家长看到 A，孩子听到 B） | Title/SynopsisHint 强制喂正文 prompt（§7.3） + 主观验收 | **must-fix** |
| /stories/generate 旧/新字段混传引发歧义 | §6.6 强制规则：有 outline_id 则旧字段全部忽略 | **must-fix** |

### 13.2 上线后观察 + 调优

| 风险 | 缓解 |
|---|---|
| doubao-lite 主题推断错率高 | scene_seed 注入 + 错率 >15% 升 pro（可配置） |
| 用户疯狂点"换个角度"烧钱 | 统一桶限流 5/min（§6.4）+ 11B 监控触发 |
| Redis AOF/重启导致 outline 大量丢失 | 视为正常 expired；前端引导重新预览 |
| outline_prompt 改版后无法对比历史 | outline_prompt_version 全链路记录（§5.4） |
| 单价漂移（豆包/Minimax 调价）导致历史成本不可比 | cost_events 记 price_version（11B §5）+ 历史只读不回放 |

### 13.3 待定（非阻塞）

- 调整抽屉 UI 细节（9b 已有的 picker 复用 vs 新做）
- "换个角度"是否需要展示历史候选——倾向不做，YAGNI
- 续集模式启用大纲（见 §10.1）——超本期
