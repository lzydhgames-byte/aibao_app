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

### 3.1 计算与采集解耦

成本计算**不在 Gateway 里**。Gateway 只暴露原始数据（tokens / chars）；`pkg/cost/` 这一独立单元负责"原始数据 → 元"的翻译。

```
gateway/llm        ─原始 tokens─►   pkg/cost/Calculator  ─元─►  metric/db
gateway/tts        ─原始 chars──►          ▲
                                           │ config.yaml 单价表
```

**为什么解耦**：豆包/Minimax 调价时只改 `config.yaml` 一行，零代码改动。计算逻辑可单独单测，不需要 mock Gateway。

### 3.2 双轨存储：Prometheus（实时）+ Postgres（历史）

| 用途 | 存储 |
|---|---|
| 实时观测（"现在每分钟多少钱"） | Prometheus Counter / Gauge |
| 历史汇总（"上月人均"） | PostgreSQL `cost_events` 表 |

**为什么双轨**：Prometheus 内存型，重启丢、保留期短，不适合月度汇总；PG 行级事件可任意聚合。两边数据来源是同一次记账——一次"call done"事件**同时**写 Prometheus 计数器和异步入队 PG。

### 3.3 异步入 PG，不阻塞主路径

主请求路径上**只写 Prometheus Counter**（内存原子操作，纳秒级）。`cost_events` 入 PG 走后台 flusher，每分钟批量写。即使 `cost_events` 写失败也不影响业务。

### 3.4 大纲被拒成本必须记录

这是 B 模式核心运营指标。`cost_events` 表 schema 必须能区分"被采纳"和"被丢弃"的调用：

```
purpose=outline, outcome=accepted    ← 用户点了"开始生成"（或"直接确认"）
purpose=outline, outcome=refreshed   ← 用户点了"换个角度"，本 outline 作废、新 outline 替代
purpose=outline, outcome=expired     ← 5 分钟没确认（兜底 abandoned）
purpose=story,   outcome=ok|fallback|fail
purpose=tts,     outcome=ok|fail
```

> outcome 命名与 11A §3.2 状态机对齐。`abandoned` 无显式信号（用户离开页面无 API 调用），由 `expired` 兜底。

这能直接回答："大纲机制每月帮我们省了多少钱（被拒大纲数 × 整篇故事成本）"。

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
   ├─ LLM 正文调用 ──► gateway/llm
   │                      │
   │                      ▼
   │                  service/cost/Recorder.Record({
   │                      user_id, child_id_hash,
   │                      purpose="story",
   │                      provider="doubao",
   │                      model="pro-32k",
   │                      tokens_in=4521, tokens_out=1842,
   │                      duration_ms=8932,
   │                      outcome="ok"
   │                  })
   │                      │
   │                      ├─►  Prometheus Counter (sync, fast)
   │                      └─►  in-memory queue → flusher (async)
   │
   ├─ TTS 调用 ──► gateway/tts
   │                 └─► service/cost/Recorder.Record({purpose="tts", chars=1418, ...})
   │
   └─ 完成

[Plan 11A: 大纲被拒]
service/outline/Service
   └─ 大纲 LLM 调用 ──► gateway/llm
                            └─► Recorder.Record({purpose="outline", outcome="rejected"})

[每分钟]
flusher → INSERT INTO cost_events VALUES (..., ..., ...)（batch 100 行）
```

## 5. 数据库 Schema

### 5.1 新表 `cost_events`

```sql
CREATE TABLE cost_events (
    id              BIGSERIAL PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL,
    user_id         BIGINT,                -- nullable（系统调用如 BGM 预下载）
    child_id_hash   VARCHAR(64),           -- 仅 hash 入库（隐私）
    purpose         VARCHAR(32) NOT NULL,  -- outline|story|chapter_hook|memory_summary|tts|bgm_download
    provider        VARCHAR(32) NOT NULL,  -- doubao|minimax|tencent_cos
    model           VARCHAR(64),
    tokens_in       INTEGER,
    tokens_out      INTEGER,
    chars           INTEGER,
    bytes           BIGINT,
    cost_yuan       NUMERIC(12, 6) NOT NULL,  -- 6 位小数够精
    outcome         VARCHAR(16) NOT NULL,  -- ok|rejected|expired|fallback|fail
    duration_ms     INTEGER,
    story_id        BIGINT,                -- 关联故事（nullable）
    outline_id      VARCHAR(64),           -- 关联大纲（nullable）
    trace_id        VARCHAR(64)
);

CREATE INDEX idx_cost_events_occurred ON cost_events(occurred_at);
CREATE INDEX idx_cost_events_user_day ON cost_events(user_id, occurred_at);
CREATE INDEX idx_cost_events_purpose ON cost_events(purpose, occurred_at);
```

**保留策略**：本期不实现 TTL（数据量小，几个月内不上压力）。下期视情况加 partition by month。

### 5.2 单价配置（config.yaml）

```yaml
cost:
  llm:
    doubao-pro-32k:
      input_yuan_per_1m_tokens:  4.00
      output_yuan_per_1m_tokens: 8.00
    doubao-1.5-lite-32k:
      input_yuan_per_1m_tokens:  0.30
      output_yuan_per_1m_tokens: 0.60
  tts:
    minimax-t2a-v2:
      yuan_per_1k_chars: 0.85   # 占位价，需要根据 Minimax 实际计费方式校对
  storage:
    tencent-cos-hk:
      put_yuan_per_10k_requests: 0.10
      bandwidth_yuan_per_gb:     0.50
```

**注**：单价数字是占位，实施时需校对豆包/Minimax 实际定价单据（不是简单"听说价"）。校对是 Plan 11B 第一个 Task。

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

### 6.3 用户 ID hash 处理

`user_id_hash` 是 user_id 的 SHA256 取前 8 位 hex。理由：
- Prometheus 高基数 label 是性能杀手，限制 8 位 = 4 字节空间足够区分本月活跃 ~200 用户
- 隐私层面 hash 比明文 ID 安全（虽然内部 metric 端点，多一层无害）
- 撞库无所谓——这是 metric 不是认证

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

预估 12-16 个 task：

1. 校对豆包/Minimax/COS 实际单价（**先做**，不然后面全是估算）
2. `pkg/cost/` 新单元 + 单测
3. `cost_events` migration + GORM model
4. `service/cost/Recorder` + 内存队列 + 单测
5. `service/cost/Flusher` 后台 goroutine + 集成测
6. Gateway 层埋点改造（llm/tts/storage 各加 Record 调用）
7. Orchestrator 改造（传 purpose label）
8. 大纲拒绝/过期 outcome 上报（依赖 Plan 11A，编排顺序灵活）
9. Prometheus metrics 注册 + label 限基数策略
10. `cost-report` CLI
11. 文档：单价表来源 + 报表口径定义
12. 验证：人造数据跑一遍报表 + 真链路对账
13. 上线 monitor：观察 flusher_lag 一周

## 9. 测试策略

| 层级 | 范围 |
|---|---|
| 单元 | Calculator 单价/边界（0 tokens, 极大 tokens）；Recorder 队列满了的行为 |
| 集成 | testcontainers PG + Mock LLM/TTS，跑一次完整 story 生成，对账 cost_events |
| 端到端 | 真豆包+Minimax 跑 1 个故事，CLI 报告输出值与 Provider 后台账单对账（误差 <5%）|
| 回归 | Plan 9d smoke 加一项：`bin/cost-report --since=now-1h` 不崩 |

**对账验收**：本期上线后**第 1 天**手工拿 Minimax 后台账单 + 豆包后台账单 + `cost-report` 输出三方对照。误差 >10% 视为单价配错或埋点漏，必修。

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
  - 4.1 服务目录加 `pkg/cost/` + `service/cost/`
  - 8.x（监控章节）加成本 metric 清单
  - 14.1 决策矩阵补"商业化定价依据：Plan 11B"
- `MEMORY.md` / `CLAUDE.md`：Plan 11B 落地后追加

## 12. 风险与待定

| 风险 | 缓解 |
|---|---|
| Minimax 单价计费模型不透明（按字 vs 按字符 vs 按秒） | Task 1 单价校对必须看官方计费文档；不行就问客服 |
| cost_events 写入失败丢数据 | Prometheus 是真实账，PG 只是历史；丢一条不影响熔断 |
| user_id_hash label 高基数撑爆 Prometheus | 限本月活跃前 200，其他归一化到 "_other_" |
| 单价漂移导致历史数据失真 | `cost_events` 记 `cost_yuan` 是当时计算结果（不可回放）；后期改价不动历史 |
| 异步 flusher 服务挂导致排队丢失 | 服务关停信号触发最后一次 flush；下次启动从 0 开始（可接受） |

待定（非阻塞）：
- 是否给运营接口加"导出 CSV"——不做（直接 psql 即可）
- COS 存储成本（GB·月）的计量周期——本期只算请求/带宽，存储费下期
