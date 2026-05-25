# 爱宝（Aibao）技术架构设计文档（MVP / debug 版）

- 文档版本：v1.1（基于 Codex review 重构）
- 创建日期：2026-04-28
- 修订日期：2026-04-28
- 范围：一期 MVP debug 版技术架构
- 配套文档：[2026-04-28-aibao-design.md](2026-04-28-aibao-design.md)（产品设计）
- 受众：工程团队 / 运维 / 后续接手的开发者

> **阅读提示**：本文档面向"非软件工程背景"的产品负责人也能读懂，附带 🎓 概念讲解。涉及术语首次出现时都会简要解释。
> **v1.1 主要变更**：异步任务改 Outbox Pattern、音频私有化（object_key + 签名 URL）、安全链路双层（前置预审 + 后置审核）、新增音频编排章节、新增测试策略章节、新增 Metrics、新增 ADR 附录、children 表加 UNIQUE。

---

## 0. 架构必须守住的四条线

爱宝不是普通的故事生成器，而是**面向孩子的"有记忆的音频伙伴"**。本文档所有技术决策都必须服务于以下四条业务原则。任何设计变更若违反其中一条，必须先回到产品 spec 重新讨论。

| # | 原则 | 技术含义 |
|---|---|---|
| ① | **孩子永远是主角** | 每次 LLM 调用都注入"孩子是 C 位"约束；输入预审拦截"让爱宝当主角"类请求；后置审核校验主角身份 |
| ② | **爱宝人格一致** | SOUL/IDENTITY 作为不可变 system prompt；prompt 模板版本化；任何故事不得偏离爱宝核心人设 |
| ③ | **儿童数据默认敏感** | 音频私有化 + 短期签名 URL；日志脱敏；最小化原则采集；导出/删除接口从一开始就有 |
| ④ | **故事安全链路可验证** | 双层安全（输入预审 + 输出审核）；安全规则有单元测试；红线命中可追溯 |

> 🎓 **架构服务于业务原则**
> 软件架构本身没有"对错"，只有"是否服务于业务目标"。这四条线就是爱宝的"业务宪法"，所有后续技术细节都从这里推导。

---

## 1. 技术选型与原因

### 1.1 选型总览

| 层级 | 技术 | 主要原因 |
|---|---|---|
| 客户端 | Flutter（Dart 3.x） | 一份代码 iOS+Android 双发布，动画性能好 |
| 后端语言 | Go 1.22+ | 高并发 IO 场景天然契合，部署简单（单二进制） |
| HTTP 框架 | Gin | Go 生态最主流，文档/社区丰富 |
| 数据库 | PostgreSQL 16 | JSONB 适合九文件半结构化数据；事务保证 outbox 一致性；pgvector 预留 |
| 缓存 | Redis 7 | 会话/限流/缓存/轻量通知 |
| 对象存储 | 腾讯云 COS（**bucket 私有**） | 与服务器同生态；客户端通过短期签名 URL 访问 |
| LLM | 豆包 Pro（通过 Gateway 抽象） | 中文表达自然，国内合规，成本可控 |
| TTS | Minimax（通过 Gateway 抽象，一期单声音） | 中文童声/治愈系音色一线 |
| 音频混音 | ffmpeg（服务端） | TTS + BGM + 音效混合；BGM 素材库自建（CC0/商授素材） |
| 短信服务 | 腾讯云 SMS | 与服务器同生态 |
| Web 服务器 | Nginx | TLS 终结 + 反向代理 + 访问日志 |
| 进程管理 | systemd | Linux 原生，调试摩擦最小（不上 Docker） |
| 数据库迁移 | golang-migrate | 主流 Go 生态迁移工具 |
| 日志库 | Go 标准库 `log/slog` | 1.21+ 原生结构化日志 |
| 指标 | `prometheus/client_golang` 暴露 `/metrics` | MVP 端点存在；Prometheus server 上线前再起 |
| 服务器 | 腾讯云香港 2C4G / 70GB / 30Mbps | 已有资源，足够 1000 用户规模测试 |

### 1.2 不引入的技术（YAGNI）

| 技术 | 不引入原因 | 何时引入 |
|---|---|---|
| Docker / K8s | 单机部署 systemd 更快调试 | 多机部署或多环境管理时 |
| 微服务拆分 | 单体足以应对 MVP 复杂度 | 团队规模 > 5 人或某模块出现独立扩展需求 |
| 消息队列（Kafka/RabbitMQ/NATS） | PG Outbox + Redis 通知足够 | 任务量级 > 10万/日 或需要复杂路由 |
| ELK / Loki | 文件日志 + grep 足够 debug | 日志查询频繁或日志总量爆炸时 |
| Prometheus server / Grafana | MVP 仅暴露 /metrics 端点 | 上线前或需要可视化告警时 |
| gRPC | 客户端就 Flutter 一个，REST + JSON 更简单 | 引入更多客户端类型时 |
| WebSocket | 一期不做流式生成 | 做实时边生成边播放时 |

> 🎓 **YAGNI（You Aren't Gonna Need It）**
> 不要在没有实际需要时提前引入复杂度。每多一个组件就多一个故障点和学习成本。

---

## 2. 系统分层与组件总览

### 2.1 分层架构图

```
┌──────────────────────────────────────────────────────────┐
│ ① 客户端层（Flutter App）                                  │
│    iOS / Android；本地状态、播放器、录音                     │
└──────────────────────────────────────────────────────────┘
                          ↕  HTTPS / REST + JSON
┌──────────────────────────────────────────────────────────┐
│ ② 接入层（Nginx）                                          │
│    TLS 终结、反向代理、访问日志、入口限流                    │
└──────────────────────────────────────────────────────────┘
                          ↕  HTTP（127.0.0.1:8080）
┌──────────────────────────────────────────────────────────┐
│ ③ 业务层（Go 单体应用 aibao-server）                        │
│  ┌──────────┬──────────┬──────────┬──────────┬─────────┐│
│  │ 用户模块 │ 故事模块 │ 记忆模块 │ 安全模块 │ 音频编排 ││
│  └──────────┴──────────┴──────────┴──────────┴─────────┘│
│  ┌──────────────────────────────────────────────────────┐│
│  │ Gateway 抽象层：LLM / TTS / SMS / Storage / Audio   ││
│  └──────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────┐│
│  │ Outbox Worker（同进程，PG 取任务，Redis 通知唤醒）   ││
│  └──────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────┐│
│  │ /metrics 端点（仅 127.0.0.1）                         ││
│  └──────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────┘
        ↕              ↕              ↕            ↕
   ┌────────┐    ┌──────────┐   ┌──────────┐  ┌──────────┐
   │ ④ PG16 │    │ ⑤ Redis7 │   │ ⑥ COS    │  │ ⑦ 外部   │
   │用户/   │    │会话/限流/│   │（私有）  │  │豆包/     │
   │九文件/ │    │缓存/通知 │   │音频文件  │  │Minimax/  │
   │Outbox/ │    │          │   │签名 URL  │  │腾讯短信  │
   │MEMORY  │    │          │   │访问      │  │          │
   └────────┘    └──────────┘   └──────────┘  └──────────┘
```

### 2.2 组件职责

| # | 组件 | 职责 |
|---|---|---|
| ① | Flutter App | UI、本地状态、录音、音频播放、推送接收 |
| ② | Nginx | TLS 终结、反向代理、访问日志、入口限流 |
| ③ | Go 后端 | 全部业务逻辑，含 API、Outbox Worker、Metrics |
| ④ | PostgreSQL | 用户/孩子/九文件/故事/记忆/反馈/**Outbox** 持久化 |
| ⑤ | Redis | 登录态、限流、热点缓存、Outbox 唤醒通知、分布式锁 |
| ⑥ | 腾讯 COS | 音频文件**私有**存储；播放需后端签发短期签名 URL |
| ⑦ | 外部服务 | LLM（豆包）、TTS（Minimax）、短信（腾讯） |

### 2.3 关键设计决策
- **音频通过签名 URL 访问**：客户端不直接拿到永久 URL，每次播放从后端换取 15 分钟有效的签名 URL
- **异步任务走 PG Outbox**：业务写库与"待发事件"在同一事务，保证记忆更新不丢
- **Worker 与 API 同进程**：MVP 简化部署；用配置开关控制是否启用 Worker
- **PG / Redis / Metrics 仅监听 127.0.0.1**：不暴露公网

---

## 3. Go 后端项目结构

### 3.1 目录结构

```
aibao-server/
├── cmd/
│   └── server/main.go            程序入口
├── internal/
│   ├── api/                      第一层：HTTP Handler
│   │   ├── auth.go
│   │   ├── child.go
│   │   ├── story.go
│   │   └── middleware/           traceid / logger / auth / ratelimit / recover
│   ├── service/                  第二层：业务逻辑
│   │   ├── auth/
│   │   ├── child/
│   │   ├── story/                故事生成编排
│   │   │   ├── orchestrator.go   主编排（Plan 11A 起 Step 0 = HydrateFromOutline）
│   │   │   ├── prompt_builder.go Prompt 组装（System + User）
│   │   │   ├── ip_normalizer.go  真实 IP 同人化归一化
│   │   │   └── fallback.go       降级模板
│   │   ├── outlinecontract/      [Plan 11A 新增] 中立合约包（OutlineResolver 接口 + DTO + errors，无实现）
│   │   ├── outline/              [Plan 11A 新增] AI 大纲预览实现
│   │   │   ├── service.go        Preview(ctx, in) 主编排
│   │   │   ├── llm_prompt.go     大纲 LLM prompt 模板（doubao-lite）
│   │   │   ├── llm_parser.go     response_format=json 解析 + enum 校验
│   │   │   ├── safety_check.go   OutlineSafetyCheck（红线 / 害怕 / 主角 / IP）
│   │   │   ├── cache.go          Redis SET/GET（5min TTL）
│   │   │   └── resolver_impl.go  实现 outlinecontract.OutlineResolver（供 service/story 注入）
│   │   ├── memory/               九文件读写 + MEMORY 维护
│   │   ├── safety/               双层安全
│   │   │   ├── pre_check.go      输入预审（大纲调用复用，PostCheck 大纲跳过）
│   │   │   ├── post_check.go     输出审核
│   │   │   └── rules.go          红线规则
│   │   ├── audio/                音频编排（TTS + BGM + 混音）
│   │   │   ├── orchestrator.go
│   │   │   ├── cue_parser.go     从 LLM 输出解析音效标记
│   │   │   └── mixer.go          ffmpeg 混音
│   │   ├── cost/                 [Plan 11B 新增] 成本记账
│   │   │   ├── recorder.go       Record(ctx, CostEvent) 同步 metric + 异步入队 PG
│   │   │   ├── flusher.go        后台 goroutine 批量写 cost_events 表
│   │   │   ├── aggregator.go     按 user/day/purpose 聚合查询
│   │   │   └── report.go         CLI 报表渲染
│   │   └── budget/               预算熔断（Plan 11B 后从 cost_yuan_total 派生 budget gauge）
│   ├── gateway/                  抽象外部依赖
│   │   ├── llm/                  接口 + 豆包实现
│   │   ├── tts/                  接口 + Minimax 实现
│   │   ├── sms/                  接口 + 腾讯实现
│   │   └── storage/              接口 + COS 实现（含签名 URL）
│   ├── repository/               第三层：数据访问
│   │   ├── user_repo.go
│   │   ├── child_repo.go
│   │   ├── story_repo.go
│   │   ├── memory_repo.go
│   │   ├── outbox_repo.go        Outbox 读写
│   │   └── bgm_repo.go           BGM 素材库
│   ├── model/                    数据结构定义
│   ├── worker/                   Outbox 消费者
│   │   ├── worker.go             主循环（SELECT FOR UPDATE SKIP LOCKED）
│   │   └── handlers/             按 event_type 分文件
│   ├── metrics/                  Prometheus 指标定义
│   └── pkg/
│       ├── logger/               slog 封装
│       ├── config/
│       ├── errors/
│       ├── traceid/
│       ├── safehash/             敏感字段脱敏
│       ├── idhash/               [Plan 11B 新增] HMAC-SHA256 截断 12 hex + domain separation
│       └── cost/                 [Plan 11B 新增] PriceBook + Calculator（tokens/chars → 元）
├── cmd/
│   ├── aibao-server/             API + Worker 单体
│   ├── safetycheck/              安全 CLI（Plan 3）
│   ├── rules-lint/               词表 lint（Plan 9c）
│   └── cost-report/              [Plan 11B 新增] 成本汇总报表 CLI
├── config/
│   ├── config.dev.yaml
│   └── config.yaml.example       入 git；真实配置不入 git
├── migrations/                   schema 变更（golang-migrate）
├── scripts/
│   ├── install.sh
│   ├── backup.sh
│   └── deploy.sh
├── assets/bgm/                   BGM 素材清单（实际文件存 COS）
├── go.mod / go.sum
└── README.md
```

### 3.2 三层架构（api / service / repository）

> 🎓 **三层架构**：每层只做一件事，且不跨层。
> - **api 层**：HTTP 翻译，调 service。**绝不写业务规则**。
> - **service 层**：业务规则核心。
> - **repository 层**：只跟 PG/Redis 说话。

### 3.3 依赖倒置（Gateway 层）

业务代码只依赖 `gateway/{llm,tts,sms,storage}` 接口，不依赖具体 SDK。

```go
// internal/gateway/llm/llm.go（接口）
type Client interface {
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
    HealthCheck(ctx context.Context) error
}

// Plan 11B 起 GenerateResponse 必须返回 Usage（tokens/duration_ms 原始数据）
// 但 Gateway 自己不调用 service.cost.Recorder——分层约束（见 3.5）
```

> 🎓 **依赖倒置原则（DIP）**：高层依赖抽象不依赖具体。换 LLM 提供商只新增实现文件。

### 3.5 分层强约束（Plan 11B 起执行）

```
api      ─► service ─► repository / gateway / pkg
gateway  ─► pkg                              ← gateway 不依赖 service / repository
pkg      ─► （无）                           ← pkg 是最内层无 IO 核
```

**反向依赖编译期 enforce**：CI 跑 `go list -deps ./internal/gateway/...` 检查无 `internal/service` / `internal/repository` 出现。任何破坏分层的 PR 拒绝合入。

**Recorder 调用位置**：业务方（`service/story`, `service/outline`）拿到 Gateway 返回的 Usage → 显式调 `service/cost/Recorder.Record(evt)`。**不在 Gateway 拦截器/中间件里偷偷调**——破坏可读性 + 分层。

### 3.4 main.go 启动流程

1. 加载配置 → 初始化 logger
2. 初始化 PG 连接池 / Redis 客户端
3. 应用未跑过的 migrations（`migrate up`）
4. 初始化各 Gateway 实例
5. 依赖注入构造 service / repository
6. 注册 HTTP 路由（含 `/health` `/ready` `/metrics`）
7. 启动 Outbox Worker 协程
8. 启动 HTTP 服务器
9. 监听 SIGTERM 优雅关停（停接新请求 → 等在途完成 → 关 DB/Redis）

> 🎓 **优雅关停**：进程退出前完成在途请求，避免用户看到"故事生成到一半失败"。

---

## 4. 核心数据流：一次"生成故事"的完整链路

### 4.0 Plan 11A 起的两阶段流（大纲预览 → 正文生成）

> 详见 [Plan 11A spec](2026-05-25-plan-11a-ai-outline-preview.md)。本节是顶层概览。

```
阶段 1（同步，~2s）：POST /api/v1/outlines/preview
  Body: { child_id, prompt, duration_min }
  → service/outline/Preview
    → PreCheck（复用）
    → gateway/llm（doubao-lite, response_format=json, purpose=outline）
    → Redis SET outline:{id} TTL=5min
  ← 200 { outline_id, outline:{title,synopsis,themes,style,...}, expires_at }

阶段 2（异步音频管线，~10-20s 文本 + 后台 TTS）：POST /api/v1/stories/generate
  Body: { child_id, outline_id, outline_overrides? }   ← Plan 11A 新契约
  → 走下面 4.1 完整链路（Step 0 HydrateFromOutline 后接旧链路 Step a..i 不变）
```

兼容期内 `/stories/generate` 同时接受旧字段（duration_min/style/topic/prompt）走旧路径，不带 outline_id 时 fallback 旧流水线。详见 11A spec §6.4 弃用计划。

### 4.1 链路概览（含双层安全 + 音频编排 + Outbox）

```
1. Flutter App: 用户点"开始生成"（拿到 outline_id 后；或兼容期旧客户端直传字段）
   POST /api/v1/stories/generate
   Body (新):  { child_id, outline_id, outline_overrides?:{style?,themes?} }
   Body (旧, 兼容): { childId, prompt, duration: 5, style: "冒险", topic: "勇敢" }

2. Nginx → 终结 TLS → 转发到 :8080

3. middleware 链（顺序）：
   ① recover（panic 兜底）
   ② traceid（生成 traceId 注入 ctx）
   ③ logger（记录请求开始）
   ④ auth（JWT 解出 userId）
   ⑤ ratelimit（rate:gen:{userId} 5次/分钟）
   ⑥ budget_check（今日 LLM 预算未超额）

4. api/story.go.Generate handler：
   - 参数校验
   - 调 service.story.Orchestrator.Generate(ctx, params)

5. service.story.Orchestrator 编排：
   ★ 0. HydrateFromOutline（Plan 11A 新增）：
        - 同时含 outline_id + storyline_id：立即返 400 `conflicting_modes`（互斥，见 11A §6.6/§10.1）
        - 若请求带 outline_id：OutlineResolver 校验三元 ownership → 拿 outline，apply outline_overrides 白名单
        - 把 outline.style / themes / scene_seed / title / synopsis / educational_value 注入 BuildInput
        - 若 outline_id 过期或 Redis 缺失：返 **410 `outline_expired`**，不 fallback 旧路径
        - 若无 outline_id 也无 storyline_id：走旧字段直组装（兼容期）
   a. repository 查 child 档案（含害怕清单）
   b. repository 查 active_storyline + 最近 N 条 memory（彩蛋串联）

   ★ c. service.safety.PreCheck（输入预审，新增）
        - 意图分类（轻量规则 + 可选 LLM 兜底）：是否合理故事请求
        - 红线匹配：用户输入 vs 全局红线词库 + 个性化害怕清单
        - 真实 IP 归一化：奥特曼/喜羊羊等 → 同人化指令
        - 命中 → 返回友好拒绝（不进 LLM，省钱省风险）

   d. prompt_builder 组装：
        System Prompt = SOUL（不可变）+ IDENTITY（不可变）+ 强约束模板
                       （孩子是主角 / 爱宝是伙伴 / 害怕清单 / 风格 / 时长 / 教育主题）
                       + USER（孩子档案）+ MEMORY 摘要 + 串联钩子
        User Prompt   = 归一化后的需求描述
        Output Format = 故事文本 + 内嵌音效标记（如 [音效:开门] [BGM情绪:温馨]）

   e. gateway/llm.Generate(prompt) → 豆包 Pro 返回（约 8-15s）
        ├─ 超时/失败 → 重试 2 次（指数退避）
        └─ 仍失败 → fallback 模板 → 跳到步骤 g（无 BGM 标记）

   f. service.safety.PostCheck（输出审核，原有）
        - 红线匹配 + 个性化害怕清单
        - 主角身份校验（孩子名出现且为决策者）
        - 命中 → 重生成（最多 2 次） → 仍命中 → fallback 模板

   ★ g. service.audio.Orchestrator.Compose（新增）
        - cue_parser 解析音效/BGM 标记
        - bgm_repo 按情绪/时长选 BGM
        - gateway/tts.Synthesize 合成主语音
        - mixer ffmpeg 混音（TTS + BGM + SFX）
        - 失败 → 降级为纯 TTS；再失败 → 降级为预设安全模板音频

   ★ h. gateway/storage.Upload(audio_bytes) → 返回 object_key（不返回 URL）

   ★ i. PG 事务（原子性）：
        BEGIN;
          INSERT INTO stories(... audio_object_key=...);
          INSERT INTO story_elements(...);
          INSERT INTO outbox_events(event_type='memory_update', payload=...);
        COMMIT;

   j. 通知 Worker 立即唤醒：Redis PUBLISH outbox_signal "memory_update"
        （即使 PUBLISH 失败也无影响——Worker 会按周期轮询）

   k. 返回 { storyId, text }（不返回音频 URL）

6. Flutter App 拿到 storyId → 调 GET /api/v1/stories/{id}/audio_url
   后端鉴权（家长拥有该 storyId） → COS 签发 15 分钟签名 URL → 返回
   客户端用签名 URL 流式播放（CDN 仍生效）

7. 后台 Worker（同进程内）：
   - 收到唤醒信号 或 周期轮询（5 秒）
   - SELECT * FROM outbox_events
       WHERE status='pending' AND next_attempt_at <= now()
       ORDER BY id LIMIT 10 FOR UPDATE SKIP LOCKED
   - 处理每条事件（按 event_type 分发到 handler）
   - 成功 → UPDATE status='done'
   - 失败 → attempts++；next_attempt_at=now()+backoff；attempts>=5 → status='dead'
```

### 4.2 SLO（服务等级目标）

| 指标 | 目标 |
|---|---|
| 端到端生成时长（P95） | ≤ 25 秒（含混音） |
| LLM 调用耗时（P95） | ≤ 15 秒 |
| TTS 调用耗时（P95） | ≤ 5 秒 |
| 音频混音耗时（P95） | ≤ 5 秒 |
| API 错误率 | ≤ 1% |
| Outbox 堆积（pending） | ≤ 100 持续 5 分钟则告警 |
| Outbox 死信率（dead/total） | ≤ 0.1% |
| 输入预审拦截率 | 监控但不设阈值（数据用于调优） |

---

## 5. 数据存储设计

### 5.1 PostgreSQL 表设计

> 详细 DDL 见 `migrations/`，本节列结构概览。

#### 用户域

**users**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| phone_hash | varchar(64) UNIQUE NOT NULL | 手机号 SHA256（用于查询） |
| phone_encrypted | bytea NOT NULL | AES 加密原文（用于发短信解密） |
| nickname | varchar(50) | |
| subscription_tier | varchar(20) | free / pro |
| created_at, updated_at | timestamptz | |

**children**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| user_id | bigint NOT NULL FK | |
| nickname | varchar(50) | |
| gender | varchar(10) | |
| birthday | date | |
| profile | jsonb | 兴趣/害怕/家人 |
| created_at, updated_at | timestamptz | |

**约束（一期单孩子保护）：**
```sql
ALTER TABLE children ADD CONSTRAINT uniq_user_one_child UNIQUE(user_id);
```
> 二期开放多孩子：删除此约束 + 前端切换 UI；迁移路径见第 13 章演进路线。

#### 九文件域

**agent_files**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| child_id | bigint NOT NULL FK | |
| file_type | varchar(20) NOT NULL | SOUL / IDENTITY / AGENTS / USER / TOOLS / MEMORY / HEARTBEAT / BOOT / BOOTSTRAP |
| content | jsonb NOT NULL | |
| version | int NOT NULL DEFAULT 1 | 乐观锁 |
| updated_at | timestamptz | |

**约束**：`UNIQUE(child_id, file_type)`

#### 故事域

**stories**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| child_id | bigint NOT NULL FK | |
| title | varchar(200) | |
| text_content | text NOT NULL | |
| audio_object_key | varchar(500) NOT NULL | **COS object key，不存 URL** |
| audio_format | varchar(10) | mp3 / aac |
| audio_size_bytes | bigint | |
| audio_duration_seconds | int | |
| duration_minutes | int | 5/10/15 |
| style | varchar(20) | |
| topic | varchar(50) | |
| storyline_id | bigint NULLABLE | |
| episode_no | int NULLABLE | |
| has_bgm | bool NOT NULL DEFAULT true | 降级时为 false（仅 TTS） |
| created_at | timestamptz | |

**story_elements**（彩蛋串联用）
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| story_id | bigint NOT NULL FK | |
| element_type | varchar(20) | character / place / object / event |
| name | varchar(100) | |
| description | text | |
| recall_weight | float | |

#### 记忆域

**memories**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| child_id | bigint NOT NULL FK | |
| memory_type | varchar(30) | story_summary / interest / preference / feedback |
| payload | jsonb | |
| weight | float | |
| created_at | timestamptz | |

**active_storylines**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| child_id | bigint NOT NULL FK | |
| title | varchar(200) | |
| state | jsonb | |
| last_episode_no | int | |
| status | varchar(20) | active / completed / abandoned |
| updated_at | timestamptz | |

#### 反馈域

**story_feedbacks**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| story_id | bigint NOT NULL FK | |
| reaction | varchar(20) | like / dislike / mark_unlike |
| comment | text | |
| created_at | timestamptz | |

#### 异步事件域（Outbox Pattern，新增）

**outbox_events**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| event_type | varchar(50) NOT NULL | memory_update / storyline_update / interest_update / ... |
| aggregate_id | bigint | 关联的业务实体 ID（如 story_id），便于排查 |
| payload | jsonb NOT NULL | 任务参数 |
| status | varchar(20) NOT NULL | pending / processing / done / dead |
| attempts | int NOT NULL DEFAULT 0 | |
| last_error | text | 最后一次失败原因 |
| next_attempt_at | timestamptz NOT NULL | 下次可消费时间（指数退避） |
| created_at | timestamptz | |
| updated_at | timestamptz | |

**索引**：
- `(status, next_attempt_at)` —— Worker 拉取
- `(event_type, status)` —— 统计与排查
- `(aggregate_id)` —— 业务侧反查

#### BGM 素材域（新增）

**bgm_assets**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| name | varchar(100) | |
| mood | varchar(30) | warm / adventure / funny / magical / educational / calm |
| duration_seconds | int | |
| object_key | varchar(500) | COS 中的素材 key（私有）|
| license | varchar(50) | CC0 / commercial / self_made |
| license_doc_url | varchar(500) | 授权凭证（如有） |
| active | bool NOT NULL DEFAULT true | |

### 5.2 关键索引
- `users(phone_hash)` UNIQUE
- `children(user_id)` UNIQUE（即一期约束）
- `agent_files(child_id, file_type)` UNIQUE
- `stories(child_id, created_at DESC)`
- `story_elements(story_id, element_type)`
- `memories(child_id, memory_type, created_at DESC)`
- `active_storylines(child_id, status)`
- `outbox_events(status, next_attempt_at)`
- `bgm_assets(mood, active)`

### 5.3 Redis Key 规范

| 前缀 | 示例 | 用途 | TTL |
|---|---|---|---|
| `session:` | `session:{userId}` | 登录态 | 7 天 |
| `rate:gen:` | `rate:gen:{userId}` | 生成限流（5次/分） | 60s |
| `rate:sms:` | `rate:sms:{phone_hash}` | 短信限流（1次/60s） | 60s |
| `cache:topic_lib` | `cache:topic_lib:v1` | 教育主题库 | 1h |
| `cache:prompt:` | `cache:prompt:{name}:{version}` | Prompt 模板 | 永久（手动刷） |
| `cache:bgm_index` | `cache:bgm_index` | BGM 按 mood 的索引 | 1h |
| `lock:gen:` | `lock:gen:{childId}` | 防同孩子并发生成 | 30s |
| `dedup:gen:` | `dedup:gen:{hash(prompt+childId)}` | 短时重复请求去重 | 30s |
| `budget:llm:daily` | `budget:llm:daily:{YYYYMMDD}` | 每日 LLM token 累计 | 25h |
| `outbox_signal` | （Pub/Sub channel） | 唤醒 Worker | - |

> 注意：Redis 不再承载消息载体，仅作为通知/缓存/限流。任务的"真相"在 PG 的 outbox_events。

### 5.4 数据合规要点

- **手机号**：hash 用于查询，加密存储用于发送；日志仅显示 `138****5678`
- **孩子姓名**：profile JSONB 内可明文存（业务需要），**日志和错误上报中只能出现 `child_id_hash`**
- **音频文件**：COS bucket 私有；客户端通过短期签名 URL 访问；导出/删除以 `object_key` 为准
- **数据存储位置**：香港机房在合规上属境外；面向大陆儿童正式上线前必须迁境内
- **数据保留期**：用户注销后 30 天内删除全部数据（用户表 + 孩子表 + 故事 + COS 文件 + outbox 历史）
- **导出与删除接口**：`GET /api/v1/users/me/export`、`DELETE /api/v1/users/me`（异步任务，最终删除走 outbox）

---

## 6. Gateway 抽象层设计

### 6.1 LLM Gateway

```go
package llm

type GenerateRequest struct {
    SystemPrompt string
    UserPrompt   string
    MaxTokens    int
    Temperature  float64
    PromptVersion string // 用于追溯
}

type GenerateResponse struct {
    Text         string
    InputTokens  int
    OutputTokens int
    Provider     string
    Model        string
    Latency      time.Duration
}

type Client interface {
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
    HealthCheck(ctx context.Context) error
}
```

### 6.2 TTS / SMS / Storage 接口

- **TTS**：`Synthesize(ctx, text, voice) → audio_bytes`
- **SMS**：`Send(ctx, phone, template_id, params) → error`
- **Storage**：
  - `Upload(ctx, key, data, contentType) → error`
  - `Delete(ctx, key) → error`
  - `SignedURL(ctx, key, ttl) → url`（**新增；播放鉴权后调用**）

### 6.3 失败兜底策略

| 类型 | 策略 |
|---|---|
| 网络错误/5xx | 自动重试 2 次，指数退避（500ms / 1s） |
| 超时 | LLM 30s / TTS 15s / SMS 5s / Storage 10s |
| 故事生成熔断 | 连续失败 N 次切到降级模板（一组预生成的安全故事） |
| 预算熔断 | 每日 token 超阈值停服并告警，第二日 0 点恢复 |
| 签名 URL 失败 | 客户端重试 1 次，仍失败提示用户稍后再试 |

---

## 7. 双层安全链路（核心模块）

### 7.1 设计原则
**"前置预审省成本，后置审核保兜底，强约束注入定边界。"**

### 7.2 前置预审（service.safety.PreCheck）

**输入**：用户原始 prompt + 孩子档案（含害怕清单）

**步骤**：
1. **长度与字符过滤**：超长截断；过滤危险特殊符号
2. **规则匹配**：
   - 全局红线词库（暴力、血腥、恐怖、政治、性等）
   - 个性化害怕清单（孩子档案中明确禁忌的元素）
3. **意图分类**：
   - 默认走规则（关键词 + 启发式）
   - 模糊场景调用轻量分类调用（可复用 LLM 但用低成本模型 + 短 prompt，可选）
4. **真实 IP 归一化**（service.story.ip_normalizer）：
   - 检测出"奥特曼""超级飞侠"等真实 IP → 转化为"爱宝变身的同人形态"指令
   - 提供白名单 + 黑名单：白名单 IP 走同人化；黑名单（限制级、宗教等）直接拒绝
5. **结果**：
   - 通过 → 进入 prompt 组装
   - 拒绝 → 返回友好提示（不调 LLM，节省成本与风险）

### 7.3 强约束 System Prompt 模板

每次调 LLM 都注入以下不可变约束（来自 SOUL.md / IDENTITY.md / AGENTS.md）：

```
你是爱宝，一只温柔的熊猫小机器人。
不可违反的约束：
1. 孩子（{孩子昵称}，{年龄}岁）永远是故事主角和关键决策者，爱宝是伙伴
2. 你的本体始终是熊猫小机器人；故事中可"变身"为适配形态
3. 严禁出现：[全局红线列表]
4. 此孩子的个性化禁忌：[害怕清单]
5. 故事必须传递积极价值观；冲突必须有正向出路
6. 输出格式：用 [音效:xxx] [BGM情绪:xxx] 标记
7. 风格：{style}，时长目标 {duration} 分钟（约 {N} 字）
8. 教育主题：{topic}（如有）
```

模板版本化（v1 / v2 ...），存 `cache:prompt:story_system:{version}`，发版时刷新。

### 7.4 后置审核（service.safety.PostCheck）

**输入**：LLM 输出文本

**步骤**：
1. 红线词库匹配 + 个性化害怕清单匹配
2. **主角身份校验**：检查孩子昵称出现且为决策动作的发出者（启发式 + 可选 LLM 评分）
3. **命中处理**：
   - 触发重生成（保留原 user prompt，调高约束权重，最多 2 次）
   - 仍失败 → fallback 模板（一批预生成的安全故事 + 简单替换孩子昵称）

### 7.5 安全审计
- 每次预审/后审命中都写日志：`safety.precheck.fail` / `safety.postcheck.fail`，含命中规则名（不含原文）
- 命中数据进 metrics：`safety_fail_total{stage,reason}`
- 死信故事（连续重生成失败）记录到独立表 `safety_incidents`，便于人工复盘

---

## 8. 音频编排（Audio Orchestration）

### 8.1 设计目标
产品 spec 定义输出 = TTS + BGM + 音效。本模块负责把 LLM 输出的"带标记文本"转化为最终混音 mp3。

### 8.2 流程

```
LLM 输出（带标记）
  ↓
cue_parser 解析：
  - 抽出 [音效:开门] [音效:笑声] [BGM情绪:温馨] 等标记
  - 主语音文本（去除标记）
  ↓
gateway/tts.Synthesize(主语音文本) → tts.mp3
  ↓
bgm_repo 查询：
  - 按 BGM情绪 + 风格 + 时长 选 BGM
  - 按音效标签查 SFX 文件
  ↓
mixer.Compose（ffmpeg）：
  - tts.mp3 作为主轨
  - BGM 循环或拼接到目标时长，音量降至 -18dB
  - SFX 在标记时间点叠加（粗粒度对齐：按 TTS 文本位置估算时间）
  - 输出 final.mp3
  ↓
gateway/storage.Upload → object_key
```

### 8.3 降级路径

| 失败点 | 降级 |
|---|---|
| ffmpeg 混音失败 | 降级为纯 TTS 音频（`has_bgm=false`） |
| BGM 素材缺失 | 跳过 BGM，仅用 SFX |
| TTS 失败 | 重试 → 仍失败 → 整故事用 fallback 模板音频 |
| 标记解析失败 | 视作纯文本，走纯 TTS 路径 |

### 8.4 BGM 素材库管理

- 一期目标：每个 mood 至少 5 段 BGM、每个常用音效标签至少 3 个变体
- 来源：CC0（Pixabay / Freesound CC0 子集）+ 自制；商业素材另立流程
- **license 字段必填**；上线前法务复核
- 素材文件存 COS 私有 bucket；后端混音时下载到本地缓存
- 提供管理接口（仅运维）：上传 / 启用 / 停用

### 8.5 接口形态

```go
package audio

type ComposeRequest struct {
    StoryText  string  // 含标记
    Style      string
    DurationSec int
    StoryID    int64
}

type ComposeResponse struct {
    AudioBytes []byte
    Format     string
    HasBGM     bool
    Cues       []Cue   // 实际混入的音效/BGM，便于审计
}

type Orchestrator interface {
    Compose(ctx context.Context, req ComposeRequest) (*ComposeResponse, error)
}
```

---

## 9. 异步任务系统（Outbox Pattern）

### 9.1 为什么选 Outbox 而不是 Redis 队列

> 🎓 **业务一致性**：故事写库与"待发事件"必须同生共死。Redis 队列做不到——LPUSH 失败或 BRPOP 后崩溃都会丢消息。Outbox 把事件写在 PG 同一事务，任何崩溃都能恢复。

| 方案 | 优点 | 致命缺点 |
|---|---|---|
| Redis LPUSH+BRPOP | 简单 | BRPOP 后崩溃即丢消息 ❌ |
| Redis Streams + ack | 可靠 | 与业务库不在同事务，仍可能"故事写了消息没发"❌ |
| **PG Outbox + Redis 唤醒** | 业务库与事件同事务；崩溃后恢复；可重放 | 多一张表，Worker 需轮询 ✅ |

### 9.2 工作流程

**生产端**（业务事务）：
```sql
BEGIN;
INSERT INTO stories(...);
INSERT INTO story_elements(...);
INSERT INTO outbox_events(event_type, aggregate_id, payload, status, next_attempt_at)
  VALUES('memory_update', <story_id>, <payload>, 'pending', now());
COMMIT;
-- 事务后：Redis PUBLISH outbox_signal "memory_update"（best-effort 唤醒）
```

**消费端**（Worker 主循环）：
```
loop:
  msg = subscribe outbox_signal (timeout 5s)  // 信号唤醒 或 超时轮询
  events = SELECT * FROM outbox_events
             WHERE status='pending' AND next_attempt_at <= now()
             ORDER BY id LIMIT 10
             FOR UPDATE SKIP LOCKED       // 多 Worker 安全
  for each event:
    UPDATE status='processing'
    try:
      handler[event_type](event.payload)
      UPDATE status='done', updated_at=now()
    catch err:
      attempts++
      if attempts >= 5:
        UPDATE status='dead', last_error=...
        log + metric(safety_dead_total)
      else:
        backoff = 2^attempts seconds (cap 600)
        UPDATE status='pending', next_attempt_at=now()+backoff, last_error=...
```

### 9.3 幂等性约束
所有 handler 必须幂等：
- 写库用 UPSERT（`ON CONFLICT DO UPDATE`）
- 外部 API 调用前先查"是否已做过"
- payload 中带 `dedup_key`（一般等于 aggregate_id + event_type）

### 9.4 死信处理（DLQ）
- 状态为 `dead` 即死信
- 提供运维接口 `POST /admin/outbox/{id}/replay` 重置 `attempts=0, status='pending'`
- 提供清单接口 `GET /admin/outbox/dead` 查看死信
- 接口走独立鉴权（管理员账号），仅内网可访问

### 9.5 Outbox 历史清理
- `done` 状态保留 7 天后归档/删除（防止表无限增长）
- `dead` 状态人工复盘后再清理

---

## 10. 日志与可观测性

### 10.1 日志规范

**格式**：JSON 单行
```json
{
  "time": "2026-04-28T10:23:45.123+08:00",
  "level": "INFO",
  "trace_id": "tr-abc123",
  "user_id": 1042,
  "child_id_hash": "h_8b3a...",
  "module": "service.story",
  "msg": "story.generate.done",
  "duration_ms": 12300,
  "story_id": 5821
}
```

**级别**：DEBUG / INFO / WARN / ERROR；debug 版全开 DEBUG，上线后只保留 INFO+。

**存储**：`${LOG_DIR}/app.log` + `${LOG_DIR}/access.log`；按天切割；保留 14 天；单文件 ≤ 100MB。

**脱敏规则**：
- 手机号：仅 `138****5678`
- 孩子姓名：仅 `child_id_hash`
- prompt 内容：默认不入；DEBUG 级别仅记长度
- API Key：永不入日志

### 10.2 关键埋点（每次故事生成）

```
story.generate.start          child_id_hash duration style topic
safety.precheck.start
safety.precheck.done|fail     reason（如命中）
llm.call.start                provider tokens_in
llm.call.done|fail            duration_ms tokens_out
safety.postcheck.start
safety.postcheck.done|fail    reason
audio.compose.start
tts.call.done|fail            duration_ms audio_size_kb
audio.mix.done|fail           duration_ms has_bgm
storage.upload.done           object_key_hash
outbox.event.created          event_type aggregate_id
story.generate.done           total_ms
```

### 10.3 轻量 Metrics（新增）

集成 `prometheus/client_golang`，暴露 `/metrics`（仅 127.0.0.1）。

**MVP 必备指标**：

| 指标 | 类型 | 标签 | 说明 |
|---|---|---|---|
| `story_generate_total` | Counter | status (ok/fail/fallback) | 总数 |
| `story_generate_duration_seconds` | Histogram | - | 端到端时长 |
| `llm_call_duration_seconds` | Histogram | provider | LLM 时长 |
| `llm_call_total` | Counter | provider, status | LLM 调用次数 |
| `tts_call_duration_seconds` | Histogram | provider | TTS 时长 |
| `tts_call_total` | Counter | provider, status | |
| `audio_mix_duration_seconds` | Histogram | - | 混音时长 |
| `safety_fail_total` | Counter | stage (pre/post), reason | 安全命中 |
| `outbox_pending_count` | Gauge | - | 队列堆积（每 30s 采样） |
| `outbox_dead_total` | Counter | event_type | 死信累计 |
| `external_api_error_total` | Counter | provider | 外部错误 |
| `llm_budget_used_yuan` | Gauge | - | 每日预算消耗（Plan 11B 起从 cost_yuan_total 派生）|
| `http_request_duration_seconds` | Histogram | path, status | 通用 HTTP |
| **Plan 11A 大纲** ||||
| `outline_preview_total` | Counter | status | 大纲预览次数 |
| `outline_preview_duration_seconds` | Histogram | - | 大纲生成耗时 |
| `outline_confirmed_total` | Counter | - | 用户确认大纲次数 |
| `outline_refreshed_total` | Counter | - | "换个角度"次数 |
| `outline_expired_total` | Counter | - | 5min 未确认过期 |
| **Plan 11B 成本** ||||
| `cost_yuan_total` | Counter | provider, model, purpose, outcome | 累计成本（元）|
| `cost_yuan_per_user_total` | Counter | user_id_hash | 单用户累计 |
| `cost_event_record_failed_total` | Counter | reason | 记账失败 |
| `cost_flusher_batch_size` | Histogram | - | 每次 flush 批量 |
| `cost_flusher_lag_seconds` | Gauge | - | 队列中最老事件年龄 |

> 🎓 **指标 vs 日志**
> 日志是"发生了什么"（叙事，单条精确），指标是"统计是多少"（聚合数字，趋势）。SLO（如 P95、错误率）必须靠指标算，日志做不到聚合。

### 10.4 排查工作流

```bash
# 用户报"生成失败"，给时间戳
grep "ERROR" /var/log/aibao/app.log | grep "10:23:"

# 找到 trace_id 后看完整链路
grep "tr-abc123" /var/log/aibao/app.log

# 看当前指标
curl http://127.0.0.1:8080/metrics | grep story_generate

# 看死信
curl http://127.0.0.1:8080/admin/outbox/dead -H "Authorization: ..."
```

### 10.5 健康检查

- `GET /health` —— 进程存活，永远 200
- `GET /ready` —— 检查 PG / Redis / Storage 连通性，返回 200/503
- `GET /metrics` —— Prometheus 格式指标

---

## 11. 测试策略

### 11.1 测试分层

| 层级 | 范围 | 工具 | 覆盖率目标 |
|---|---|---|---|
| 单元测试 | 纯函数 / service 层（mock repository 与 gateway） | Go testing + testify | ≥ 70% |
| 集成测试 | gateway 实现的契约测试 | go test + 真实/模拟 SDK | 关键路径 100% |
| 端到端冒烟 | 部署前完整链路 | shell 脚本 / Postman | 主流程必跑 |

### 11.2 MVP 必测清单（service / pkg 层）

| 模块 | 必测点 |
|---|---|
| `prompt_builder` | 输出包含 SOUL/IDENTITY 完整内容；包含孩子档案；包含害怕清单；包含约束 1-8；版本号正确 |
| `safety.PreCheck` | 全局红线命中 → 拒绝；个性化害怕命中 → 拒绝；正常请求 → 通过；IP 归一化命中 → 转写 |
| `safety.PostCheck` | 红线命中 → 触发重生成；主角缺位 → 触发重生成；最大重试后 → fallback |
| `ip_normalizer` | 白名单 IP 转同人化；黑名单 IP 直接拒绝；未知 IP 默认放行 |
| `story.Orchestrator` | LLM 失败重试；连续失败走 fallback；事务原子（含 outbox） |
| `audio.Orchestrator` | 混音失败降级纯 TTS；TTS 失败降级模板；标记解析容错 |
| `cue_parser` | 标准标记解析；畸形标记容错；空标记空文本 |
| `memory_repo` | 写入幂等（UPSERT）；冲突时不破坏现有数据 |
| `worker` handlers | 同 task_id 多次消费结果一致；失败计数与退避 |
| `ratelimit` middleware | 5次/分窗口准确；不同用户独立 |
| `budget.Check` | 超阈值拒绝；跨日重置 |
| `storage.SignedURL` | 鉴权失败拒签；签名包含过期时间；过期拒绝 |
| `pkg/safehash` / 日志中间件 | 关键字段（手机号、姓名、prompt、API Key）不出现明文 |
| `service.user.Export` / `Delete` | 导出含全数据；删除清理 PG + COS + outbox 历史 |

### 11.3 集成测试（gateway 层）

仅做**契约测试**——Provider SDK 升级或换 Provider 时验证接口契约不变：
- LLM：能正常调用并返回非空 text；超时正确触发 ctx 取消
- TTS：能返回有效 audio bytes；编码正确
- Storage：上传后能下载；签名 URL 在 TTL 内可用，过期不可用
- SMS：mock 模式下行为正确（生产环境通过手动验证）

### 11.4 端到端冒烟（部署前必跑）

脚本 `scripts/smoke.sh`：
1. 注册新用户（手机号 + 验证码 mock）
2. 创建孩子档案
3. 触发 BOOTSTRAP
4. 生成一个故事（短时长，省钱）
5. 获取签名 URL 并下载音频，验证非空且时长合理
6. 查 outbox：相关事件已 `done`
7. 查 memory：故事已写入
8. 调用导出接口验证数据完整
9. 删除用户，验证数据清理（PG + COS）

### 11.5 测试数据
- 单元测试：固定 fixture（YAML 或 testdata 目录）
- gateway mock：每个 Provider 提供 `mock.go` 实现，开关来自环境变量

---

## 12. 部署形态（debug 版）

### 12.1 服务器规格
- 腾讯云香港 入门型：2 核 / 4GB / 70GB SSD / 30Mbps / 2048GB流量
- 操作系统：Ubuntu 22.04 LTS

### 12.2 进程清单

| 进程 | 内存上限 | 启动 |
|---|---|---|
| nginx | ~100 MB | systemd |
| postgresql-16 | 1 GB（`shared_buffers=256MB`, `work_mem=4MB`, `effective_cache_size=1GB`） | systemd |
| redis-server | 512 MB（`maxmemory 512mb`, `maxmemory-policy allkeys-lru`） | systemd |
| aibao-server | ~400 MB（`GOMEMLIMIT=400MiB`，含 ffmpeg 子进程） | systemd |

### 12.3 系统优化
- 创建 2GB swap
- PG / Redis / Metrics 仅监听 127.0.0.1
- 防火墙（ufw）：仅 22 / 80 / 443
- fail2ban：SSH 防爆破
- 安装 ffmpeg（apt）

### 12.4 配置管理

`/etc/aibao/config.yaml`，敏感字段走环境变量：

```yaml
server:
  port: 8080
  log_dir: /var/log/aibao
  log_level: debug
  metrics_addr: 127.0.0.1:8080  # /metrics 端点

postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
  # password: AIBAO_DB_PASSWORD（env）

redis:
  addr: 127.0.0.1:6379

llm:
  provider: doubao
  model: doubao-pro
  timeout_seconds: 30
  daily_budget_yuan: 200
  prompt_version: v1
  # api_key: AIBAO_DOUBAO_KEY（env）

tts:
  provider: minimax
  voice: aibao_default
  timeout_seconds: 15

storage:
  provider: cos
  bucket: aibao-debug-1300xxxxxx
  region: ap-hongkong
  cdn_domain: ""              # 留空走默认
  signed_url_ttl_seconds: 900 # 15 分钟

audio:
  ffmpeg_path: /usr/bin/ffmpeg
  bgm_volume_db: -18
  workdir: /var/lib/aibao/audio_tmp

sms:
  provider: tencent
  sign: 爱宝
  template_id: ""

worker:
  enabled: true
  poll_interval_seconds: 5
  batch_size: 10

safety:
  redline_dict_path: /etc/aibao/redlines.yaml
  ip_whitelist_path: /etc/aibao/ip_whitelist.yaml
  ip_blacklist_path: /etc/aibao/ip_blacklist.yaml
```

### 12.5 备份策略
- PG：`pg_dump` 每日 03:00 备份到 COS（`scripts/backup.sh`）
- Redis：AOF 持久化（`appendonly yes`，`appendfsync everysec`）
- 配置：`/etc/aibao/` 每周备份
- 保留：日备 30 天，周备 12 周

### 12.6 部署脚本
- `scripts/install.sh` —— 新服务器一键安装（含 ffmpeg、字体、systemd unit）
- `scripts/deploy.sh` —— 增量发布（拉代码 → build → migrate up → 优雅重启）

---

## 13. 安全规范

### 13.1 网络层
- HTTPS 强制（Let's Encrypt + Nginx）
- 仅 80/443/22 公网开放
- SSH 仅密钥
- PG / Redis / Metrics 仅 127.0.0.1

### 13.2 应用层
- JWT 鉴权（access 24h / refresh 7d，HS256，密钥走 env）
- 参数校验：结构体绑定 + validator
- SQL 注入防御：GORM 参数化
- 限流：见 5.3
- 敏感接口（admin/outbox 等）走独立鉴权

### 13.3 数据层
- 手机号加密 + hash 双存
- 日志脱敏（见 10.1）
- 音频私有化 + 签名 URL（见 6.2、5.4）
- 用户导出与删除接口

### 13.4 密钥
- 全部环境变量
- 不入 git（`.gitignore` 含 `config.prod.yaml`、`.env`）
- `/etc/aibao/config.yaml` 权限 0640，所属用户 aibao

---

## 14. 成本预估与控制

### 14.1 1000 测试用户量级估算

假设：1000 注册、30% 日活 = 300 DAU、人均 1.5 个故事/天 = **450 次生成/天**。

月度量级：
- LLM tokens：~6750 万 tokens/月
- TTS：~10.8 万分钟/月
- COS 存储：约 67.5GB/月新增（需要清理策略）
- COS 流量：约 100 GB/月（签名 URL 走 CDN）
- 短信：~2000 条/月

> 具体单价以 Provider 实时为准。**重点是订阅定价必须 ≥ 单用户成本**。
> **Plan 11B 起** `pkg/cost` 从 `config.yaml` 单价表读取实时单价，按 tokens/chars 实际计算入库 `cost_events` 表，不再依赖此处估算。

### 14.2 控制手段（写入代码）
- 每用户每分钟 5 次限流
- 免费用户每日 5 次额度（**MVP 阶段商业化推迟，Plan 11B 数据成熟后再启用**）
- 每日 LLM token 总额度熔断（`budget:llm:daily`，Plan 11B 后从实际成本派生）
- 30 秒重复请求去重（`dedup:gen:`）
- 90 天未访问的故事音频从 COS 清理（保留文本与 object_key 占位）
- **大纲预览-确认模式**（Plan 11A）：被拒大纲只烧小钱（doubao-lite 单次大纲调用），节省整篇正文 pipeline 成本（LLM 正文 + TTS + chapter_hook + memory_summary + COS）。具体单价以 Plan 11B PriceBook 当时快照为准（`cost_events.unit_price_snapshot`）
- **成本可观测**（Plan 11B）：`cost_events` 历史表 + `cost-report` CLI，为后期定价决策提供数据基础

---

## 15. 演进路线（什么信号触发什么升级）

### 15.1 升级触发器

| 信号 | 升级 |
|---|---|
| 同时生成并发 > 10 持续 5 分钟 | 服务器升 4C8G |
| PG 数据量 > 30GB | 扩硬盘到 200GB 或迁专用 PG 实例 |
| 出现付费用户 | PG 主从 + 异地每日备份；Prometheus + 告警必须就位 |
| 准备大陆正式上线 | 服务器迁境内 + ICP 备案；数据迁移；接入大陆 CDN |
| 多人开发 / 多环境 | 引入 Docker；CI/CD |
| Outbox 任务 > 10万/日 | Worker 拆独立进程；考虑分区表 |
| 日志查询频繁 | 引入 Loki + Grafana |
| 多机部署 | 拆 PG / Redis / Nginx 到独立机器；Go 后端水平扩展 |
| HEARTBEAT 真推送 | Worker 拆独立进程；增加预生成池表与推送服务 |
| 二期开放多孩子 | 删 `uniq_user_one_child` 约束；前端切换 UI；MEMORY/storyline 已是 child_id 维度无需变更 |
| 多 TTS 声音（C 方案） | tts.Synthesize 增 voice 参数（已预留）；UI 增声音选择 |

### 15.2 为迁移而设计（贯穿一期）

1. **配置外置**（IP/域名/密钥）
2. **路径变量**（日志路径走配置）
3. **服务地址抽象**（不假设 127.0.0.1）
4. **文件存储从一开始走 COS**（不存本地以后再迁）
5. **schema 走 golang-migrate**
6. **健康检查 `/health` `/ready`** 一开始就有
7. **备份脚本入 git**
8. **启动脚本一键化**

> 🎓 **Pets vs Cattle**：服务器当"牲口"，挂了 10 分钟拉新的。

---

## 16. 待决策项与开放问题

| # | 问题 | 提议默认 | 触发决策时间 |
|---|---|---|---|
| 1 | 域名 | 暂用 IP 测试 | 提交 App Store 阶段 |
| 2 | App Store 主体（个人/公司） | TBD | 提交审核前 |
| 3 | 教育主题库 50-100 主题清单 | 六大类骨架已定，需内容工作 | 实现规划阶段 |
| 4 | 真实 IP 法务策略 | 同人化 + 白/黑名单 | 内容审核模块开发前 |
| 5 | 订阅定价 | TBD | 上线前 |
| 6 | 儿童数据境内合规 | 香港 debug，发布前迁境内 | 大陆正式上线前 |
| 7 | BGM 素材采购预算 | 一期先用 CC0 + 自制 | 素材库不足时 |
| 8 | 输入预审是否调用 LLM | 默认仅规则；模糊场景再加 | 上线前依据数据 |

---

## 附录 A：术语速查表

- **Goroutine**：Go 的轻量协程
- **Context**：Go 请求级上下文，传递 traceId / userId / 超时
- **Middleware**：HTTP 处理链上的拦截器
- **JSONB**：PG 的二进制 JSON，可索引
- **Idempotent / 幂等**：执行 N 次结果与 1 次相同
- **Outbox Pattern**：业务库与事件同事务，保证一致性
- **Signed URL**：私有资源的临时访问链接，含过期时间
- **systemd**：Linux 进程管理
- **TraceID**：跨组件追踪一次请求的标识
- **CDN**：内容分发网络
- **JWT**：无状态身份令牌
- **DLQ**：死信队列
- **SLO**：服务等级目标
- **Graceful Shutdown**：优雅关停
- **DIP**：依赖倒置原则
- **YAGNI**：You Aren't Gonna Need It，不过早引入复杂度
- **ADR**：Architecture Decision Record，架构决策记录
- **SKIP LOCKED**：PG 行锁选项，跳过已被锁的行（多 Worker 并发安全）

## 附录 B：决策记录摘要

- 客户端：Flutter
- 后端：Go + Gin + GORM + slog
- 数据库：PostgreSQL 16
- 缓存：Redis 7（仅缓存/限流/通知）
- 异步：PG Outbox Pattern + Redis Pub/Sub 唤醒
- 对象存储：腾讯云 COS（私有 + 签名 URL）
- LLM：豆包 Pro（Gateway 抽象）
- TTS：Minimax（Gateway 抽象，一期单声音）
- 音频混音：服务端 ffmpeg；BGM 自建素材库
- 短信：腾讯云 SMS
- 服务器：腾讯云香港 2C4G
- 部署：systemd + Nginx，不上 Docker
- 部署形态：单体单机（API + Outbox Worker 同进程）
- 通讯协议：HTTPS + REST + JSON
- 鉴权：JWT；登录手机号 + 验证码
- 日志：slog JSON + 文件 + traceId 贯穿
- 指标：Prometheus 客户端，端点暴露但 server 暂不部署
- 安全：双层（前置预审 + 后置审核），强约束注入

---

## 附录 C：架构决策记录（ADR）

> 每条 ADR 包含：候选方案、决策、理由、风险、回滚条件。

### ADR-001：客户端选 Flutter
- **候选**：Flutter / React Native / iOS+Android 双原生
- **决策**：Flutter
- **理由**：动画/渲染性能强（爱宝形象、变身动画等高视觉权重场景）；单代码库降低小团队维护成本；中文社区与文档充分
- **风险**：iOS Kids Category 审核较严，需提前规划；部分原生能力需要自写 platform channel
- **回滚条件**：iOS 审核出现 Flutter 特定阻塞；性能不达预期

### ADR-002：后端选 Go
- **候选**：Go / Node.js / Python
- **决策**：Go
- **理由**：故事生成是 IO 密集（等 LLM/TTS）天然契合 goroutine；二进制部署无环境地狱；内存占用低契合 2C4G
- **风险**：团队学习曲线（若团队主语言非 Go）
- **回滚条件**：团队招聘困难；某些必需库 Go 生态缺失

### ADR-003：LLM 选豆包 Pro
- **候选**：豆包 Pro / DeepSeek / Qwen / Claude（海外）
- **决策**：豆包 Pro
- **理由**：中文表达自然、儿童语境理解佳；国内合规；价格可控
- **风险**：单一 Provider 依赖；API 限流与稳定性
- **缓解**：所有调用走 LLM Gateway 抽象，可一行配置切换；预留 DeepSeek/Qwen 实现
- **回滚条件**：质量大幅下降；价格不可承受；服务长期不稳定

### ADR-004：TTS 选 Minimax
- **候选**：Minimax / 火山引擎 / Azure / 腾讯 TTS
- **决策**：Minimax
- **理由**：中文童声/治愈系音色一线；情感表达自然
- **风险**：单一 Provider；定价波动
- **缓解**：TTS Gateway 抽象，可切换
- **回滚条件**：音色质量退化；价格不可承受

### ADR-005：对象存储选腾讯 COS
- **候选**：腾讯 COS / AWS S3 / Cloudflare R2 / 服务器本地
- **决策**：腾讯 COS（**私有 bucket + 签名 URL**）
- **理由**：与服务器同生态；国内访问快；CDN 集成
- **风险**：未来出海可能需迁 S3/R2
- **缓解**：Storage Gateway 抽象
- **回滚条件**：国际访问性能严重不足；价格不可承受

### ADR-006：异步任务选 PG Outbox + Redis 唤醒（不选 Redis 队列）
- **候选**：Redis List（LPUSH/BRPOP）/ Redis Streams / **PG Outbox** / Kafka/RabbitMQ
- **决策**：PG Outbox + Redis Pub/Sub 唤醒
- **理由**：**业务核心是"有记忆的 AI"，记忆更新不能丢**。Outbox 保证业务库与事件同事务；崩溃可恢复；无新组件
- **风险**：Outbox 表增长需定期清理；Worker 轮询有 5s 延迟（Pub/Sub 唤醒可缓解）
- **回滚条件**：任务量级 > 10万/日 或需复杂路由 → 引入 NATS/RabbitMQ

### ADR-007：选香港服务器（一期）
- **候选**：香港 / 国内（要备案） / 海外
- **决策**：香港
- **理由**：免备案，可快速启动 debug；现有资源
- **风险**：调豆包/Minimax 跨境多 30-100ms；大陆儿童数据合规属境外
- **缓解**：上线前迁境内（已纳入演进路线）
- **回滚条件**：性能不达 SLO；正式上线大陆市场

### ADR-008：单体应用（不拆微服务）
- **候选**：单体 / 微服务
- **决策**：单体（API + Worker 同进程，模块化代码）
- **理由**：MVP 复杂度低；运维简单；调试摩擦小
- **风险**：未来某模块独立扩展时需要拆分
- **缓解**：模块边界严守；Worker 启用通过配置开关，便于将来分离
- **回滚条件**：团队 > 5 人；某模块独立扩展需求出现

### ADR-009：不上 Docker
- **候选**：systemd / Docker / Docker Compose
- **决策**：systemd
- **理由**：单机调试摩擦最小；Go 二进制部署本身已无环境依赖
- **风险**：多机部署或多环境管理时不便
- **缓解**：所有配置外置 + 一键脚本，迁 Docker 仅是封装
- **回滚条件**：多机或多环境需求出现

### ADR-010：MVP 暴露 /metrics 但不部署 Prometheus server
- **候选**：仅日志 / 完整 Prometheus + Grafana / **/metrics 端点 only**
- **决策**：暴露 `/metrics`，server 暂不部署
- **理由**：客户端代码侵入小且早期植入；server 部署等到需要可视化告警时；P95 等可临时 `curl` 算
- **回滚条件**：上线 / 付费用户 / 告警需求出现 → 部署 Prometheus + Grafana

### ADR-011：双层安全（前置预审 + 后置审核）
- **候选**：仅后置 / 仅前置 / **双层**
- **决策**：双层
- **理由**：前置省 LLM 成本与风险（用户输入即拦截）；后置兜底防 LLM 越界；二者互补
- **风险**：增加链路复杂度；前置可能误伤合理请求
- **缓解**：前置规则可配置；命中数据进 metrics 用于调优
- **回滚条件**：误伤率高且无法调优 → 前置降级为仅红线匹配

### ADR-012：children 一期 UNIQUE(user_id)
- **候选**：仅业务约束 / DB 约束
- **决策**：DB UNIQUE 约束
- **理由**：MVP 单孩子是产品决策；DB 层防御并发/重试/脚本误用
- **回滚条件**：二期开放多孩子 → 删除约束，前端加切换 UI（其他表已是 child_id 维度，无需变更）
