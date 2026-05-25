# Plan 11B — 成本可观测 spec

> 让"花了多少钱、谁花的、花在哪"看得见。**不加限流、不加付费 UI、不上 Grafana**。先看见，再决定要不要拦。

## 0. 一句话

补齐 LLM/TTS 实际成本 metric + 人均成本归集 + 一个 `make cost-report` CLI，让你能回答三个问题：上个月一共花了多少钱？人均一个故事多少钱？钱主要花在 LLM 还是 TTS？

## 1. 背景与动机

### 1.1 当前状态

- ✅ `llm_budget_used_yuan` Gauge（Plan 4 加的，但是**估值**，按"豆包均价 × token"硬编码估）
- ✅ `tts_chars_total` Counter（Plan 9c 加的，按时长档位分桶）
- ❌ TTS 字数 → 实际元的换算缺失
- ❌ 没有"按用户"维度
- ❌ 没有"按调用目的"分类（一次故事生成包含 outline / story / chapter_hook / memory_summary 多个 LLM 调用，混在一个桶里）
- ❌ 没有一个"看一眼就懂"的报表入口

### 1.2 为什么要补

Plan 10 上线后朋友试用规模约 10 人。MVP 阶段商业化推迟，但**定价决策需要数据基础**——没有"人均月成本"这个数，定 9.9/19.9/29.9 都是拍脑袋。Plan 11B 不解决商业化，但**为下一期商业化做数据准备**。

### 1.3 为什么现在做（而不是等真要定价时再做）

定价那一刻才补 metric 会面对一个困境：**历史数据无法补齐**。Prometheus Counter 只能从加埋点那一刻开始累计，过去的钱已经烧掉但记录不下来。所以这个工作**只能往前补**，不能往后补——越早补，定价依据越坚实。

## 2. 不在范围（YAGNI）

- ❌ **Grafana / Prometheus server / ELK**——10 用户量级，CLI 报表足够，上 dashboard 是过度工程
- ❌ **实时告警**（"今日成本超 X 元"）——已有 `budget:llm:daily` 熔断兜底，告警是定价后的事
- ❌ **付费 UI / 订阅档位**——本期不动 App，纯后端
- ❌ **多租户成本隔离**（按企业/团队归集）——MVP 个人用户场景，不需要
- ❌ **按 prompt 内容相似度聚合成本**（"奥特曼类故事比恐龙类贵 X%"）——有意思但超本期
- ❌ **预测性成本曲线**——数据量不够支撑预测

## 3. 关键设计决策

### 3.1 计算与采集分层（Gateway 不依赖 service）

**分层强约束**：`gateway/*` 仅依赖 `pkg/*`，**不能**调用 `service/*`——这是 Plan 1 立的三层架构基线，11B 不能破。

```
gateway/llm   ─Generate() 返回 LLMResp{Content, Usage{tokens_in/out, ms}}─►  调用方
gateway/tts   ─Synthesize() 返回 TTSResp{Audio, Usage{chars, ms}}────────►  调用方
                                                                              │
                                                                              ▼
                                                     service/outline,story  调用
                                                              │
                                                              ▼
                                              service/cost/Recorder.Record(evt)
                                                              │
                                                  ┌───────────┼────────────┐
                                                  ▼           ▼            ▼
                                          pkg/cost.Calc   Prometheus   in-mem queue
                                          (tokens→¥)      Counter      → flusher → PG
```

**职责**：
- **gateway/\***：API 调用 + 返回 `Usage` struct 原始数据（tokens、chars、ms）。**不算钱、不调 Recorder。**
- **pkg/cost/**：纯函数 `Calc(provider, model, usage) → ¥`。无 IO、无依赖、易单测。喂入 PriceBook 即可。
- **service/cost/Recorder**：编排——拿 Usage → 调 pkg/cost 算钱 → 写 Prometheus（sync） + 入队（async） → flusher 批量 INSERT。
- **service/outline, service/story 等业务方**：业务调 gateway 拿到 Usage → 调 Recorder.Record(evt)。

**为什么这样切**：豆包/Minimax 调价改 config，不动 Gateway；新增 LLM provider 加 `pkg/cost/PriceBook` 表项，不动业务；mock Gateway 单测无需 mock 成本系统。

### 3.2 数据源真伪：PG 是事实源，Prometheus 是近似观测

**spec 立场**（更正之前的"Prometheus 是真实账"表述）：

| 维度 | PostgreSQL `cost_events` | Prometheus |
|---|---|---|
| 性质 | **事实源（source of truth）** | 近似实时观测 |
| 持久化 | 是 | 否（进程内存，重启归零） |
| 部署 | 已有 PG（Docker 复用） | 当前**未部署 Prometheus server**——仅暴露 `/metrics` 端点供未来抓取 |
| 用于 | 历史汇总 / 定价决策 / 审计 / 对账 | 实时监控仪表盘（未来）+ 健康检查 |
| 漂移处理 | 任何 PG/Prom 差异以 PG 为准 | 仅记录 `cost_event_record_failed_total` 用于排查 |

定价决策、月度对账、用户成本归集**只看 PG**。Prometheus 数字仅供"看活着没"的健康观测。

### 3.3 异步入 PG，不阻塞主路径

主路径调 `Recorder.Record(evt)`：
1. 同步：算钱 + Prometheus Counter inc（纳秒级原子）
2. 同步：写一个**幂等 event_id**到内存队列（trace_id + monotonic counter）
3. 异步：后台 flusher 每分钟 batch INSERT cost_events（`ON CONFLICT(event_id) DO NOTHING` 保证幂等）

**失败行为**：
- 队列满（默认 10000 容量）→ drop + `cost_event_record_failed_total{reason=queue_full}`
- PG 写失败 → flusher 指数退避重试 3 次 → 仍败则丢弃 + metric `reason=db_write`
- 业务路径**永不**因成本记账失败而中断

**关停信号**：`SIGTERM` 触发最后一次 flush（最多等 5 秒）；之后未刷出数据丢弃。下次启动从空队列开始（可接受——损失上限为 1 分钟批量）。

### 3.4 大纲生命周期成本归集

这是 B 模式核心运营指标。**两表协作**（11A §5.5 outline_events + 11B cost_events）：

- `outline_events`：记**生命周期**（pending → accepted/refreshed/expired），每个 outline 有 1+ 条事件
- `cost_events`：记**每次 LLM/TTS/storage 调用的钱**，purpose=outline 时关联 outline_id

`cost_events.outcome` 命名与 11A §3.2 状态机对齐：

```
purpose=outline,  outcome=ok        ← LLM 调用本身成功（不论 outline 后续是否被采纳）
purpose=outline,  outcome=fail      ← LLM 调用失败（不计入"被拒省钱"逻辑）
purpose=story,    outcome=ok|fallback|fail
purpose=tts,      outcome=ok|fail
purpose=chapter_hook,    outcome=ok|fail
purpose=memory_summary,  outcome=ok|fail
purpose=storage_put,     outcome=ok|fail
```

**"被拒大纲省钱"计算口径**（D2 修正）：

```
未进入正文的 outline = outline_events.outcome IN (refreshed, expired)
                       AND 不存在后续 outcome=accepted 的关联记录
                       （refresh 可能多次，最终未 accepted 才算"未进入正文"）

每个被拒 outline 实际烧钱  = 该 outline_id 对应的 cost_events.SUM(cost_yuan)（含 LLM 调用）
"如果继续生成会烧多少"     = avg(full_pipeline_cost_per_accepted_story)
                              其中 full_pipeline_cost = story LLM + TTS
                                                      + chapter_hook + memory_summary
                                                      + storage_put（COS 上传）

每个被拒 outline 净省  = avg(full_pipeline_cost) - 该 outline 实际花费

月度省钱总额 = SUM(净省 for each 未进入正文的 outline)
```

`cmd/cost-report` 的报表口径必须按上述公式计算，不能用"被拒数 × story cost"裸算。

## 4. 系统架构

### 4.1 模块拓扑（增量）

```
internal/
├── pkg/
│   └── cost/                      ← 新单元（无依赖核）
│       ├── calculator.go          LLM/TTS 成本计算
│       ├── prices.go              从 config 加载单价
│       └── calculator_test.go
│
├── gateway/
│   ├── llm/                       (已有，仅加 label/usage 暴露)
│   └── tts/                       (已有，仅加 chars 上报)
│
├── service/
│   └── cost/                      ← 新模块
│       ├── recorder.go            Record(ctx, evt CostEvent)
│       ├── flusher.go             定时 batch 写 PG
│       ├── aggregator.go          按 user / day / purpose 聚合
│       └── report.go              CLI 报表逻辑
│
└── api/admin/
    └── cost_handler.go            (新增) 仅 internal 端口暴露的 admin 端点

cmd/
└── cost-report/                   ← 新 CLI
    └── main.go
```

### 4.2 数据流（一次故事生成的成本记录）

```
[用户点"开始生成"]
   │
   ▼
service/story/Orchestrator
   │
   ├─ resp, _ := gateway/llm.Generate(...)
   │   resp.Usage = {tokens_in=4521, tokens_out=1842, duration_ms=8932}
   │
   ├─► Recorder.Record(CostEvent{
   │       user_id, child_id_hash, purpose="story",
   │       provider="doubao", model="pro-32k",
   │       usage: resp.Usage, outcome="ok",
   │       story_id, trace_id, event_id (idempotent),
   │       outline_id (if any),
   │   })
   │     │
   │     ├─ pkg/cost.Calc(provider, model, usage) → cost_yuan
   │     │   ├─ 查 PriceBook(provider, model, billing_mode)
   │     │   └─ snapshot price_version + unit_price
   │     ├─ Prometheus Counter (sync, fast)
   │     └─ in-memory queue (async)
   │
   ├─ resp, _ := gateway/tts.Synthesize(...)
   │   resp.Usage = {chars=1418, duration_ms=4200}
   │
   └─► Recorder.Record(CostEvent{purpose="tts", usage: resp.Usage, ...})

[Plan 11A: 大纲被拒后续清算 — outline_events 记 outcome，cost_events 仅记调用本身]
service/outline/Service
   └─ resp, _ := gateway/llm.Generate(...)
   └─► Recorder.Record(CostEvent{purpose="outline", outcome="ok", outline_id, ...})
       (无论用户后续是否 accept，本 LLM 调用 cost 都已发生)

[每分钟]
flusher → INSERT INTO cost_events VALUES (...) ON CONFLICT(event_id) DO NOTHING
```

**关键不变量**：`gateway/*` 包**不 import** `service/*`。代码层面用 go module 依赖检查 enforce。

## 5. 数据库 Schema

### 5.1 新表 `cost_events`

```sql
CREATE TABLE cost_events (
    id              BIGSERIAL PRIMARY KEY,
    event_id        VARCHAR(64) NOT NULL UNIQUE,  -- 幂等键（trace_id + monotonic counter）
    occurred_at     TIMESTAMPTZ NOT NULL,
    user_id         BIGINT,                -- nullable（系统调用如 BGM 预下载）
    child_id_hash   VARCHAR(32),           -- HMAC-SHA256 截断 12 hex（见 §6.3）
    purpose         VARCHAR(32) NOT NULL,  -- outline|story|chapter_hook|memory_summary|tts|storage_put|bgm_download
    provider        VARCHAR(32) NOT NULL,  -- doubao|minimax|tencent_cos
    model           VARCHAR(64),
    billing_mode    VARCHAR(32),           -- standard|cached|batch|reasoning（为 Claude/GPT 多模式预留）
    -- usage 字段：按 provider 不同填不同子集
    tokens_in       INTEGER,
    tokens_out      INTEGER,
    tokens_cached   INTEGER,               -- 缓存 hit token（为未来 prompt caching 预留）
    chars           INTEGER,
    bytes           BIGINT,
    audio_seconds   NUMERIC(8, 2),         -- 为按时长计费的 TTS 预留
    -- 价格快照（审计 + 调价回放）
    cost_yuan       NUMERIC(12, 6) NOT NULL,  -- 当时计算结果
    currency        VARCHAR(8) DEFAULT 'CNY',
    price_version   VARCHAR(32) NOT NULL,     -- PriceBook 版本号（vYYYYMMDD-N）
    unit_price_snapshot JSONB,                -- 当时具体单价（事后调价不影响历史）
    -- 业务关联
    outcome         VARCHAR(16) NOT NULL,  -- ok|fallback|fail（不含 outline 生命周期状态）
    duration_ms     INTEGER,
    story_id        BIGINT,                -- 关联故事（nullable）
    outline_id      VARCHAR(64),           -- 关联大纲（nullable）
    outline_group_id VARCHAR(64),          -- 同一意图的所有 outline 共享
    outline_prompt_version VARCHAR(32),    -- A/B 实验追踪
    trace_id        VARCHAR(64)
);

CREATE UNIQUE INDEX idx_cost_events_event_id ON cost_events(event_id);
CREATE INDEX idx_cost_events_occurred ON cost_events(occurred_at);
CREATE INDEX idx_cost_events_user_day ON cost_events(user_id, occurred_at);
CREATE INDEX idx_cost_events_purpose ON cost_events(purpose, occurred_at);
CREATE INDEX idx_cost_events_outline ON cost_events(outline_id) WHERE outline_id IS NOT NULL;
CREATE INDEX idx_cost_events_outline_group ON cost_events(outline_group_id) WHERE outline_group_id IS NOT NULL;
```

**保留策略**：本期不实现 TTL（数据量小，几个月内不上压力）。1000 万行或月报变慢再加 partition by month / materialized daily aggregate。

**audit & replay**：`unit_price_snapshot` JSONB 形如 `{"input": 0.30, "output": 0.60, "unit": "yuan_per_1m_tokens"}`。当时填错单价后期发现时可对历史重算（`UPDATE ... SET cost_yuan = ...` 仅在 audit 模式下手工跑），但默认**历史只读不回放**。

### 5.2 PriceBook 配置（config.yaml）

PriceBook 用 `provider + model + billing_mode + version` 四元 key（E1 修正），为未来多 provider 多模式预留：

```yaml
cost:
  price_book_version: v20260525-1   # 本次 PriceBook 版本号
  pricing_source: "豆包官方计费 / Minimax 官网公示 / 腾讯云 COS 计费"
  entries:
    - provider: doubao
      model: doubao-1.5-pro-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 4.00
      output: 8.00
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
    - provider: minimax
      model: t2a-v2
      billing_mode: standard
      unit: yuan_per_1k_chars
      chars: 0.85          # 占位，Task 1 校对
    - provider: tencent_cos
      model: hk-standard
      billing_mode: standard
      put_yuan_per_10k_requests: 0.10
      bandwidth_yuan_per_gb:     0.50
  # 未来扩展示例（不在本期实现）：
  # - provider: anthropic, model: claude-opus-4-7, billing_mode: cached
  #   unit: usd_per_1m_tokens (会触发 USD→CNY 换算 + cached input 折扣)
```

**版本化规则**：
- 改任何条目 → `price_book_version` bump（如 `v20260601-1`）
- 所有 `cost_events` 记录 `price_version = price_book_version`
- 历史数据不回放，但 `unit_price_snapshot` 字段提供按版本审计能力

**注**：单价数字是占位，**Plan 11B 第一个 Task** 是校对豆包/Minimax/COS 官方计费单据（不依赖"听说价"）。

### 5.3 PriceBook 接口

```go
// pkg/cost/pricebook.go
type Usage struct {
    TokensIn      int
    TokensOut     int
    TokensCached  int       // 缓存命中（可选）
    Chars         int
    Bytes         int64
    AudioSeconds  float64   // 按时长计费 TTS（可选）
}

type PriceBookKey struct {
    Provider     string
    Model        string
    BillingMode  string  // standard / cached / batch / reasoning
}

type PriceBook interface {
    Lookup(key PriceBookKey) (PriceEntry, error)
    Version() string
}

type Calculator interface {
    Calc(key PriceBookKey, u Usage) (yuan float64, snapshot PriceEntry, err error)
}
```

MVP 阶段 PriceBookKey 几乎只用 `provider + model`，但接口已为多 mode 预留——未来加 Claude `cached` mode 不动业务代码。

## 6. Metrics

### 6.1 新增

| Metric | 类型 | Labels | 含义 |
|---|---|---|---|
| `cost_yuan_total` | Counter | provider, model, purpose, outcome | 累计成本（元） |
| `cost_yuan_per_user_total` | Counter | user_id_hash (低基数：限本月活跃前 200) | 单用户累计 |
| `cost_event_record_failed_total` | Counter | reason | 入队/写库失败 |
| `cost_flusher_batch_size` | Histogram | - | 每次 flush 批量 |
| `cost_flusher_lag_seconds` | Gauge | - | 队列中最老事件年龄 |

### 6.2 保留 / 调整

- `llm_budget_used_yuan`（已有）→ 改为从 `cost_yuan_total` 派生（同一数据源，避免双写漂移）
- `tts_chars_total`（已有）→ 保留作为底层数据；上层成本视图统一查 `cost_yuan_total`

### 6.3 用户/孩子 ID hash 处理（升级为 HMAC）

**算法**：`HMAC-SHA256(secret, "<domain>:" + id)` 截断为 12 hex（48 bit）

- `user_id_hash`：domain = `"user"`
- `child_id_hash`：domain = `"child"`
- `secret`：从环境变量 `AIBAO_ID_HASH_SECRET` 读取（与 `safehash` 已用的 secret 同源或独立——实施时统一）

**为什么 HMAC 而非裸 SHA256**：裸 hash 对自增 ID 可被**枚举反推**（`SHA256(1)` `SHA256(2)`... 全部预计算就行）；HMAC 加 secret 后无法枚举。

**为什么 12 hex（48 bit）**：
- 200 用户撞库概率 ≈ `200²/2 / 2⁴⁸` ≈ `1.4×10⁻¹⁰`（生日悖论）
- 1 万用户 ≈ `1.8×10⁻⁷`，仍极低
- 16 hex（64 bit）更稳但 label 长度翻倍，Prometheus 内存占用增加，48 bit 是甜点

**Prometheus label 基数控制**：label 形如 `user_id_hash="3f8a2b9c1d04"`。本月活跃前 200 用户独立 label；超出归到 `_other_`。规则在 Recorder 层实现，每月初重新计算 top200。

**Domain separation**：`HMAC(secret, "user:42")` ≠ `HMAC(secret, "child:42")`，防同 ID 跨表关联出隐私。

## 7. CLI 与 Admin 端点

### 7.1 `cmd/cost-report/main.go`

调用方式：
```
# 上个月汇总
bin/cost-report --since=2026-04-01 --until=2026-04-30

# 按用户排行（top 10 烧钱用户）
bin/cost-report --by=user --since=last_month --limit=10

# 按目的拆分
bin/cost-report --by=purpose --since=last_7d

# 大纲拒绝率
bin/cost-report --outline-stats --since=last_30d
```

输出：纯文本表格（terminal 友好），不出图，不出 JSON（除非加 `--json`）。

样例输出：
```
Period: 2026-04-01 to 2026-04-30

=== Overall ===
Total cost:      ¥143.27
Stories:         287  (avg ¥0.50)
Outlines:        412  (acceptance: 287/412 = 69.7%)
Outline saving:  125 outlines rejected × ¥0.42 (avg story cost) = ¥52.5 saved

=== By Purpose ===
story         ¥104.20  (72.7%)
tts           ¥ 32.15  (22.4%)
outline       ¥  4.18  ( 2.9%)
memory_sum    ¥  1.84  ( 1.3%)
chapter_hook  ¥  0.90  ( 0.6%)

=== Top 5 Users ===
user_hash  stories  total
3f8a2b...     42   ¥ 21.30
7c1d9e...     38   ¥ 18.95
...
```

### 7.2 Admin HTTP 端点（可选，暂不做）

明确不做。理由：CLI + SQL 直查覆盖所有用例；Admin HTTP 会引入认证/授权新边界，YAGNI。

## 8. 实施增量（Task 概览，writing-plans 阶段细化）

**关键编排**（D4 修正）：11B 拆为 **Thin Slice**（与 11A 同 sprint）+ **Full Build**（后跟）。理由：11A 上线第一天起就需要 outline 成本数据，否则永远补不回；CLI/report 可以晚一两周。

### 8.1 Thin Slice（必须与 11A 同 sprint 上线）

1. ★ 校对豆包/Minimax/COS 实际单价（先做）
2. ★ `pkg/cost/` Calculator + PriceBook 接口 + config 加载 + 单测
3. ★ `cost_events` migration（含 event_id 幂等 + price_version 字段）
4. ★ `service/cost/Recorder` 同步 API + 内存队列 + Prometheus Counter
5. ★ `service/cost/Flusher` 后台 goroutine + 关停 flush + 集成测
6. ★ Gateway 层暴露 Usage（不调 Recorder）—— `gateway/llm`、`gateway/tts`、`gateway/storage` 返回值各加 Usage 字段
7. ★ Orchestrator / outline service 改造（拿 Usage → 调 Recorder）—— 与 11A Task 同步落地
8. ★ ID hash 工具升级到 HMAC（`pkg/idhash/`）—— user_id + child_id 共用
9. ★ `outline_events` 表 migration + 写入埋点（11A §5.5）

### 8.2 Full Build（11A 上线后 1-2 周内补齐）

10. `service/cost/Aggregator` 按 user/day/purpose 聚合
11. `cmd/cost-report` CLI + 输出格式 + 子命令
12. 文档：单价表来源 + 报表口径定义 + outline 省钱口径
13. 真链路对账：豆包后台 + Minimax 后台 + COS 后台 + `cost-report` 输出三方对照，误差 <10% 验收
14. 上线 monitor：观察 `cost_flusher_lag_seconds` 一周；若 lag 频繁超 60s 调整 batch 节奏

## 9. 测试策略

| 层级 | 范围 |
|---|---|
| 单元 | Calculator 单价/边界（0 tokens, 极大 tokens, unknown provider）；PriceBook lookup miss；Recorder 队列满/关停 flush；ID hash domain separation；event_id 幂等性 |
| 集成 | testcontainers PG + Mock Gateway，跑一次完整 story 生成 → 对账 cost_events；Flusher 批量 INSERT + ON CONFLICT 幂等；进程异常退出 → 重启 → 不重复插 |
| 端到端 | 真豆包+Minimax 跑 1 个故事，CLI 报告输出值与 Provider 后台账单对账（误差 <10%）|
| 回归 | Plan 9d smoke 加一项：`bin/cost-report --since=now-1h` 不崩 |

### 9.1 必跑负例（golden cases）

| 场景 | 期望结果 |
|---|---|
| Gateway 返回 nil Usage（provider 错） | Recorder log warning + metric record_failed{reason=nil_usage}，业务继续 |
| PriceBook 没有对应 (provider, model) | Calc 返 0 + metric record_failed{reason=price_miss}，业务继续 |
| 队列满（注入 10000 排队） | drop 旧事件 + metric record_failed{reason=queue_full} |
| Flusher 写 PG 失败 3 次 | 批次丢弃 + metric reason=db_write |
| event_id 重复（同 trace_id + counter 重发） | ON CONFLICT DO NOTHING；不双计费 |
| SIGTERM 触发关停 | 最后一次 flush 在 5s 内完成；超时丢弃但不阻塞退出 |
| Gateway 不调 service 反向依赖（编译期保证） | go module 依赖检查脚本 fail（CI 跑） |
| PriceBook 调价后 + 历史不变 | 旧 cost_events.cost_yuan 不动；新事件用新 version |
| ID hash secret 漂移（重启换 secret） | 已写入的 hash 保持不变；新 hash 用新 secret；不强制一致（接受） |
| outline_id 关联 cost_events vs outline_events JOIN | 任一 outline 必能 JOIN 到完整生命周期 |

### 9.2 对账验收（上线第 1 天 must-do）

手工拿 **Minimax 后台账单 + 豆包后台账单 + COS 后台用量 + `cost-report` 输出** 四方对照，每个 provider 误差 <10%。

误差 >10% 的处理顺序：
1. 看 PriceBook 单价是否填错（最常见）
2. 看 Usage 字段是否漏埋点（如 `tokens_cached` 没记导致少算）
3. 看 record_failed metric 是否有显著 drop（队列/写库失败）
4. 仍找不到 → 临时把对应 purpose 标 `audit_pending`，先记账"约等于"

## 10. 上线策略 + 回滚

### 10.1 灰度

- migration 上线后 `cost_events` 表立即可写
- Recorder 默认开启（埋点 noop 时低成本，开关意义不大）
- 上线第 1 天观察 `cost_event_record_failed_total` + `cost_flusher_lag_seconds`

### 10.2 回滚

- 全部新增，无现有契约改动
- Recorder 异常不影响主业务（异步 + try/catch）
- 极端回滚：feature flag `cost.recorder.enabled=false` 完全关 Recorder，业务无感

### 10.3 监控

新 metric 加 alert rule（**仅日志告警，不上 pagerduty**）：
- `cost_flusher_lag_seconds > 300`：flusher 卡住，事件可能丢
- `rate(cost_event_record_failed_total[5m]) > 0.1`：队列满或写库失败

## 11. 涉及的现有文档更新

- `docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md`：
  - 4.1 服务目录加 `pkg/cost/` + `pkg/idhash/` + `service/cost/` + `cmd/cost-report/`
  - 7.x 加成本数据流图（Gateway → service → Recorder）
  - 8.x 监控章节加 cost_yuan_total / cost_event_record_failed_total / cost_flusher_lag_seconds metric
  - 14.1 决策矩阵补"商业化定价依据：Plan 11B Thin Slice 与 11A 同步上线"
  - 0 章"强制贯穿原则"加："Gateway 不依赖 service；分层用 go module 依赖检查 enforce"
- `MEMORY.md` / `CLAUDE.md`：Plan 11B 落地后追加

## 12. 风险与待定

### 12.1 上线前 must-fix

| 风险 | 缓解 | 状态 |
|---|---|---|
| Minimax 计费模型不透明（按字/字符/秒）→ 单价配错 | Task 1 强制看官方文档/客服；上线第 1 天对账验收 | **must-fix** |
| Gateway → service 反向依赖破坏分层 | go module 依赖检查脚本 + CI 拦截 | **must-fix** |
| ID hash 使用裸 SHA256 可被枚举 → 隐私 | 升级 HMAC-SHA256 + 12 hex + domain separation（§6.3） | **must-fix** |
| event_id 不幂等 → 双计费 | event_id UNIQUE + ON CONFLICT DO NOTHING（§5.1） | **must-fix** |
| price_version 缺失 → 历史数据不可审计 | cost_events 强制 price_version + unit_price_snapshot 字段（§5.1） | **must-fix** |

### 12.2 上线后观察

| 风险 | 缓解 |
|---|---|
| user_id_hash label 高基数撑爆 Prometheus | 限活跃前 200 +`_other_`兜底 |
| 单价漂移（豆包/Minimax 调价）历史失真 | cost_events 记当时快照；不回放历史；调价时 bump price_version |
| 异步 flusher 服务挂导致排队丢失 | SIGTERM 触发最后 flush；下次从 0 开始（损失上限 1 分钟） |
| 11A outline 上线但 11B Recorder 漏埋点 → outline 省钱口径失真 | Thin Slice 强制与 11A 同 sprint（§8.1）；不允许 11A 早 11B 一周 |
| PriceBook config 改完忘记重启 | viper hot-reload 或运维 checklist；改价时手工 reload metrics |

### 12.3 待定（非阻塞）

- 运营接口"导出 CSV"——不做（直接 psql 即可）
- COS 存储成本（GB·月）的计量周期——本期只算请求/带宽，存储费下期
- Prometheus server 部署——MVP 阶段仅暴露 `/metrics`；正式商业化前部署 + Grafana
- 多 LLM provider 接入（Claude / GPT）—— PriceBook 接口已预留 `billing_mode`，业务侧解耦完成；接入时只需加 PriceBook 条目 + Gateway 实现
