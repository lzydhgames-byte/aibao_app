# CLAUDE.md — 爱宝（Aibao）项目协作指南

本文件用于指导 AI 助手（Claude Code 等）在本项目中的协作方式。每次会话开始时会被自动加载。

---

## 1. 项目一句话

爱宝是一款面向儿童的 AI 故事 App。家长设置孩子档案后用文字或语音描述需求，AI 以孩子为主角生成个性化故事，TTS+BGM 朗读。爱宝是一只百变的熊猫小机器人 IP，拥有跨会话记忆，让 AI 像有生命的伙伴。

## 2. 当前阶段

**Plan 1 + 2 + 3 + 4 + 5 + 6 + 6b + 7 全部实现并通过冒烟。**
Plan 7 完成（2026-05-14）：音频混音管线全部代码就位；MVP 暂不收 BGM 文件，纯 TTS 路径运行 + 降级链路实测有效。未来收 BGM 上传 COS + `make seed-bgm` 即可启用。
当前下一步：Plan 8（连续剧 + HEARTBEAT 伪推送） / Plan 9（Flutter 客户端） / Plan 10（部署上线），按用户选。

权威文档：
- 产品设计 spec：[docs/superpowers/specs/2026-04-28-aibao-design.md](docs/superpowers/specs/2026-04-28-aibao-design.md)
- 技术架构 spec：[docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md](docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md)
- 已完成的 Plan 1：[docs/superpowers/plans/2026-04-28-plan-01-backend-infrastructure.md](docs/superpowers/plans/2026-04-28-plan-01-backend-infrastructure.md)
- 已完成的 Plan 2：[docs/superpowers/plans/2026-05-07-plan-02-auth-and-child-profile.md](docs/superpowers/plans/2026-05-07-plan-02-auth-and-child-profile.md)
- 已完成的 Plan 3：[docs/superpowers/plans/2026-05-07-plan-03-safety-and-prompt-builder.md](docs/superpowers/plans/2026-05-07-plan-03-safety-and-prompt-builder.md)
- 已完成的 Plan 4：[docs/superpowers/plans/2026-05-08-plan-04-story-generation.md](docs/superpowers/plans/2026-05-08-plan-04-story-generation.md)
- 已完成的 Plan 5：[docs/superpowers/plans/2026-05-11-plan-05-tts-and-storage.md](docs/superpowers/plans/2026-05-11-plan-05-tts-and-storage.md)
- 已完成的 Plan 6：[docs/superpowers/plans/2026-05-11-plan-06-bootstrap-and-memory.md](docs/superpowers/plans/2026-05-11-plan-06-bootstrap-and-memory.md)
- 已完成的 Plan 7：[docs/superpowers/plans/2026-05-13-plan-07-audio-mixing-bgm.md](docs/superpowers/plans/2026-05-13-plan-07-audio-mixing-bgm.md)（MVP 暂不入 BGM 库）

### 已落地的能力（不要重做）
- 后端骨架（Go + Gin + GORM + slog + Prometheus + 健康检查 + 优雅关停）
- 三层架构（api/service/repository）+ Gateway 抽象层目录就位
- 用户认证：手机号 + 验证码 + JWT（access 24h / refresh 7d）
- 孩子档案：CRUD + 一期 `UNIQUE(user_id)` 约束
- 手机号双存（safehash + AES-256-GCM）
- **双层安全护栏**：PreCheck 6 类拦截 + PostCheck 3 类拦截 + 220 红线词 6 大类 + 12 IP 白名单 + 30 IP 黑名单
- **System Prompt 模板**：text/template + 8 条强约束 + 11 个动态字段
- **cmd/safetycheck CLI**：3 个子命令演示安全 + Prompt 装配
- **LLM Gateway 抽象**（Doubao + Mock + BudgetGate 预算熔断）
- **Story Orchestrator**（PreCheck → Prompt → LLM → PostCheck → Fallback → Persist 同事务）
- **Outbox Pattern**（4 表：stories / story_elements / memories / outbox_events + Worker + SKIP LOCKED + 指数退避 + DLQ）
- **故事生成 API**（POST /stories/generate, GET /stories/:id）
- **限流 + 预算 middleware**（per-user 5/min；超日预算 503）
- **5 个 fallback 故事模板** + 启发式 element extractor
- **业务 metrics 定义**（9 个；TTS/Storage/audio 部分已在 Plan 5 真正埋点）
- **TTS Gateway 抽象**（Minimax REST t2a_v2 + Mock）
- **Storage Gateway 抽象**（Tencent COS + Mock；私有 bucket + 15 分钟签名 URL）
- **异步音频管线**（POST /generate 立即返回 + `tts_synthesis` Worker handler + `audio_status` 3 态 `pending/ready/failed`）
- **GET /api/v1/stories/:id/audio_url**（3 态分支 + 所有权校验 + 15 分钟签名 URL）
- **BOOTSTRAP 首次相遇仪式**（7 题表单 + LLM 润色 `children.profile.description`）
- **Memory Summarizer**（doubao-1.5-lite 生成 30 字极短摘要，与长版并写 weight=1.2 高优先级 row）
- **Memory Selector**（注入近 3 条 memory 到 `BuildInput.MemorySummary`，故事生成前自动调用）
- **System prompt 首次/回调双分支模板**（首次相遇 vs 含"上次故事记忆"软提示）
- **音频混音管线**（Plan 7）：`cue_parser` 解析 `[音效:xxx][BGM情绪:yyy]` → `audio.Orchestrator` 串 TTS + BGM lazy 缓存 + ffmpeg mixer → 失败降级到纯 TTS（`audio.mix.degraded` 埋点）
- **BGM 资产管理**（Plan 7）：`bgm_assets` 表 + `bgm_repo.PickByMood` 加权随机 + COS lazy-download per-filename `sync.Once` 缓存 + `make seed-bgm` CLI（MVP 暂不入库 BGM 文件，纯 TTS 路径运行）
- 知识库 11 主题 130+ 词条（用户复盘用）

### 端到端可演示接口（已通过冒烟）
- `GET /health` `GET /ready` `GET /metrics`
- `POST /api/v1/auth/sms/send`
- `POST /api/v1/auth/login_or_register`
- `GET /api/v1/me`（需 Bearer JWT）
- `POST/GET/PATCH /api/v1/children`（需 Bearer JWT）
- `POST /api/v1/stories/generate`（需 Bearer JWT）
- `GET /api/v1/stories/:id`（需 Bearer JWT）
- `GET /api/v1/stories/:id/audio_url`（需 Bearer JWT；15 分钟 COS 签名 URL）
- `GET /api/v1/bootstrap/questions`（需 Bearer JWT；7 题问卷 + version）
- `POST /api/v1/bootstrap/answers`（需 Bearer JWT；LLM 润色后写 children.profile.description）

### CLI 可演示
- `safetycheck precheck "..."` —— 前置预审
- `safetycheck postcheck --child=... "..."` —— 后置审核
- `safetycheck build-prompt --child=... ...` —— 完整 System Prompt 装配

## 3. 产品架构核心约定

- **九文件人格架构**：SOUL / IDENTITY / AGENTS / USER / TOOLS / MEMORY / HEARTBEAT / BOOT / BOOTSTRAP（OpenClaude 风格）
- **爱宝定位**：百变伙伴（本体熊猫，故事中可变形）—— 永远不能让爱宝抢走孩子的主角位置
- **孩子是主角**：所有故事生成必须确保孩子是 C 位、是做关键决定的人
- **MVP 范围**：参见 spec 文档第 4 章；未列入的功能默认不做

## 4. 协作规则

### 4.1 流程纪律
- 重大功能/设计决策走 brainstorming 流程，先 spec 后 plan 后实现
- 每个开发任务前先看 spec 是否已覆盖；spec 没说的回来问，不要自由发挥
- 实现进展记录到 [docs/devlog/](docs/devlog/) —— 每天/每个里程碑一篇

### 4.2 内容安全是硬要求
- 涉及故事生成 prompt、内容过滤、儿童数据存储的代码改动，**必须**对照 spec 第 7 章红线
- 任何可能让孩子害怕、模仿危险行为、接触不当内容的设计都要 stop & flag

### 4.3 技术决策待定
以下决策尚未做出，遇到时不要默认假设，先问用户：
- 客户端形态（iOS / Android / 小程序 / Web）
- 后端语言与架构
- LLM / TTS / BGM 选型
- 数据存储与儿童数据合规方案

### 4.4 文档与代码风格
- 文档语言：中文为主，技术术语保留英文
- 用户偏好简洁回答；不要为已经显而易见的代码写注释
- 编辑现有文件优先于创建新文件

## 5. 关键文件索引

| 文件 | 用途 |
|---|---|
| [docs/superpowers/specs/2026-04-28-aibao-design.md](docs/superpowers/specs/2026-04-28-aibao-design.md) | 一期 MVP 完整设计文档（权威） |
| [MEMORY.md](MEMORY.md) | 项目级长期记忆与决策摘要 |
| [docs/devlog/](docs/devlog/) | 开发日志，按日期组织 |

## 6. 用户协作偏好

- 倾向 MVP 优先、快速上线测试
- 强调"界面要下功夫"
- 决策风格：明确、迅速；不喜欢模糊

## 7. 边做边学（重要 — 必须遵守）

用户**没有软件工程基础**，本项目兼任他的学习场。每次协作时遵守：

### 7.1 遇到知识点必须解释
凡是出现以下情况，**必须停下来用接地气、生活化、可类比的方式解释**：
- 专有名词（TDD、CI/CD、依赖注入、幂等、协程……）
- 命令行工具或参数（`go install -tags`、`docker compose up -d`、`git rebase`……）
- 编程概念（指针、接口、context、并发……）
- 行业惯例（Conventional Commits、12-Factor App、SemVer……）
- 设计模式与原则（YAGNI、DIP、Outbox Pattern……）

**讲解要求：**
- 优先用日常生活类比（"git commit 像存档点"，"context 像快递单"）
- 一次只讲一个概念，避免堆砌
- 标志：用 🎓 emoji 起头，让用户一眼能识别"这是教学段落"

**每条知识库词条必须包含"为什么需要"段（重要）：**
- 仅"是什么"不够——容易记不住、用不上
- "为什么需要"讲清楚**动机和痛点**——通常是"如果没这东西会怎样"
- 这一段是用户复盘时的最大收获点，不能省

### 7.2 必须同步落地到知识库
**每次解释一个知识点，必须把它追加到 [docs/knowledge/](docs/knowledge/) 目录下的对应主题文件**（用户日后复盘用）。

知识库结构：
```
docs/knowledge/
├── README.md                    索引（按主题分类，列出所有词条）
├── 01-git.md                    Git 相关概念
├── 02-go-language.md            Go 语言基础
├── 03-go-engineering.md         Go 工程实践（项目结构、模块、测试、lint）
├── 04-docker.md                 Docker / 容器
├── 05-software-design.md        软件设计原则与模式
├── 06-testing.md                测试相关概念
├── 07-http-and-web.md           HTTP / Web 相关
├── 08-database.md               数据库相关
├── 09-observability.md          日志、指标、追踪
└── 10-security-and-compliance.md 安全与合规
```

每个主题文件按"词条"组织，每个词条包含：
- 术语名（中英对照）
- 一句话定义
- 生活类比 / 通俗讲解
- 在本项目中怎么用
- 何时引入的（首次出现的 commit 或 task）

**缺哪个文件就建哪个**；不强求一开始就齐全。

### 7.3 复盘原则
用户会定期回头看知识库。所以：
- 词条不重复（同一个概念多处提到时，第二次起只引用既有词条编号）
- 同一文件内按"先简单后复杂"或"先基础后进阶"排序
- 老旧词条若理解加深，可补充而不是新建
