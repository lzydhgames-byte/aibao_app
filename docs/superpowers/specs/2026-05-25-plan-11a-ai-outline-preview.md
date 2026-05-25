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
- ❌ **大纲 PostCheck**（恐惧词/主角校验等）——大纲是给家长看的预览，不是给孩子的内容；PreCheck 保留即可

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
outline:{outline_id}     (UUID, 5min TTL)
value: JSON.stringify({outline 字段 + child_id + prompt 文本 + created_at})
```

`outline_id` 是仅用于一次性确认的票据；过期后 `/stories/generate` 拿不到 outline，**返回 400 让用户重新预览**（这是故意设计——逼用户在 5 分钟内决定，否则大纲信息可能已和当时心情脱节）。

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

**错误**：
- 400：PreCheck 拒绝（红线词/恐惧词命中），返回 `reject_reason` + `category`
- 400：duration_min 非 [3,5,8]
- 429：限流（per-user 10/min，比 generate 宽，因为是只读式探索）

### 6.2 新增：`POST /api/v1/outlines/:id/refresh`

"换个角度"。同 child_id + prompt + duration，但 nonce 不同破缓存。响应同 6.1。

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

### 6.4 弃用计划

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
1. PreCheck（复用 `gateway/safety`，**完整跑**——不能跳过）
2. 调 `gateway/llm` Generate，传 `purpose=outline` label（给 Plan 11B 成本归类用）
3. 解析 JSON → 校验 enum/长度
4. 注入 scene_seed（后端选，不让 LLM 自由发挥）
5. 写 Redis，返 outline_id

### 7.2 LLM 模型选型

**大纲调用走 doubao-1.5-lite-32k**（已用于 memory summary 和 chapter hook）：
- 输入 ~600 token + 输出 ~400 token
- 单次 ~0.005 元，**比 doubao-pro 便宜 8-10 倍**
- 大纲是结构化输出，对模型理解力要求中等，lite 够用

如果实测 lite 推断主题错率 >15%，第二期升 pro——这是个**可配置项**，不是硬编码。

### 7.3 Orchestrator 微改

在现有 5 步流水线**之前**新加一步：

```go
// step 0 (new)
if in.OutlineID != "" {
    outline := outlineCache.Get(in.OutlineID)
    if outline == nil { return ErrOutlineExpired }
    applyOverrides(outline, in.OutlineOverrides)
    in.Style = outline.Style
    in.Theme = outline.Themes[0]
    in.SceneSeed = outline.SceneSeed
    in.SynopsisHint = outline.Synopsis   // 喂给正文 prompt 做"承接"
}
// step 1..5: 原有流水线不动
```

**关键设计**：正文 prompt 拿到 `SynopsisHint` 后，模板加一段"请把这个梗概展开为完整故事"——让大纲和正文**首尾呼应**，避免大纲说一套、正文写一套的撕裂感。

### 7.4 限流

- Outline preview：per-user 10/min（探索性，放宽）
- Outline refresh：per-user 5/min（防止用户疯狂换角度烧钱）
- Story generate：保持原 5/min

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
| 单元 | service/outline/ 每个函数 + JSON 解析容错 + cache TTL |
| 集成 | testcontainers Redis + Mock LLM，跑 Preview 流水线 |
| 端到端 | 真豆包 lite 跑 5 次 preview，人工验主题/风格/教育意义合理性 |
| 回归 | Plan 9d smoke 脚本扩 1 项：outline → generate 链路通 |

**主观验收**：找你和你朋友各跑 5 个真实输入（"想听个温柔点的"/"明天考试想给孩子打气"/"小宇喜欢恐龙"），看大纲生成是否"懂用户"。这条没有自动化指标，靠人判。

## 10. 上线策略 + 回滚

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
- `outline_confirmed_total` / `outline_refreshed_total` / `outline_expired_total`（确认率 / 换角度率 / 过期率）

这些 metric 直接喂 Plan 11B 的成本视图。

## 11. 涉及的现有文档更新

- `docs/superpowers/specs/2026-04-28-aibao-design.md`：3.3.4 节"故事生成流程"改写为新流程；2.1 节"操作者：家长"加一句"AI 协助理解需求"
- `docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md`：4.1 服务目录加 `service/outline/`；时序图加大纲阶段；7.2 PreCheck 段标注"大纲调用复用 PreCheck，不跑 PostCheck"
- `MEMORY.md` / `CLAUDE.md`：Plan 11A 落地后追加

## 12. 风险与待定

| 风险 | 缓解 |
|---|---|
| doubao-lite 主题推断不准（"奥特曼"→暴力？） | scene_seed 注入 + PreCheck + 实测后可升 pro |
| 用户疯狂点"换个角度"烧钱 | per-user 5/min 限流 + Plan 11B 监控触发 |
| 大纲生成失败影响主路径 | outline 失败时前端可"直接生成"（fallback 到旧路径） |
| 大纲和正文调性脱节 | SynopsisHint 喂正文 prompt + 主观验收 5 次 |

待定（非阻塞）：
- 调整抽屉 UI 细节（这是 9b 已有的 picker 复用 vs 新做）
- "换个角度"是否需要展示历史候选（"刚才那版/这版选哪个"）—— 倾向不做，YAGNI
