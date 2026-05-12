# MEMORY.md — 爱宝项目长期记忆

本文件记录跨会话的关键决策、产品共识、避免重复讨论的结论。新增条目按日期倒序排列。

---

## 产品定位

- **项目名**：爱宝（Aibao）
- **品类**：面向儿童（学龄前～小学低年级）的 AI 个性化故事 App
- **IP 主角**：爱宝 —— 一只可爱的 AI 熊猫小机器人，圆耳朵、黑眼圈、胸口有发光能量片
- **核心定位**：以孩子为主角的睡前故事 + 沉浸式音频（TTS + BGM/音效，无插图）
- **目标场景**：睡前哄睡（主）、路上、白天玩耍

## 核心设计决策（一期 MVP）

| 维度 | 决策 |
|---|---|
| 输出形态 | 语音朗读 + BGM/音效（D 方案） |
| 爱宝在故事中定位 | 百变伙伴（C 方案）：本体恒定，形态万变；孩子永远是主角 |
| 输入流程 | 一句话需求 + 结构化兜底旋钮（C 方案） |
| 输入主体 | 一期仅家长输入，孩子不直接交互 |
| 时长档位 | 5 / 10 / 15 分钟 |
| 故事风格 | 5 类：温馨治愈 / 冒险探索 / 搞笑欢乐 / 神奇魔法 / 科普认知 |
| 教育主题库 | 50-100 个主题，6 大分类，按年龄/挑战加权随机展示，可"换一批" |
| TTS 方案 | 一期单一爱宝声音；预留多声音架构，后期升级 |
| 生成延迟 | 10-20 秒整体生成后播放（非流式） |
| 故事串联 | 默认彩蛋 + 可选连续剧（D 方案，一期就做） |
| 主动推送 | 一期用"打开时呈现"伪推送替代；真·HEARTBEAT 推送二期做 |
| 多孩子支持 | 一期仅 1 个孩子档案 |
| 内容安全 | 全球未成年人保护合规基线（COPPA / GDPR-K / 中国未保条例 / UK AADC 等） |

## 九文件架构

采用 OpenClaude 风格九文件：SOUL / IDENTITY / AGENTS / USER / TOOLS / MEMORY / HEARTBEAT / BOOT / BOOTSTRAP。详细职责与初始内容见 spec 文档第 3 章。

特别强调的三个文件：
- **BOOTSTRAP**：首次相遇仪式 —— 情感锚点，关键体验
- **BOOT**：每次重逢的问候 + 伪推送展示
- **HEARTBEAT**：一期为轻量版（仅 App 内异步任务），二期承载主动推送

## 用户协作偏好

- MVP 优先，快速上线测试，功能不贪多
- 强调"界面要下功夫"，UI 设计将单独深化
- 决策风格明确迅速，喜欢直接的选项题
- 文档/沟通使用中文

## 技术架构（一期 MVP / debug 版）

详见 [docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md](docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md)。

| 维度 | 决策 |
|---|---|
| 客户端 | Flutter（iOS+Android） |
| 后端 | Go 1.22+ / Gin / GORM / slog |
| 数据库 | PostgreSQL 16（JSONB；预留 pgvector） |
| 缓存 | Redis 7（兼任轻量队列） |
| 对象存储 | 腾讯云 COS（音频客户端直连下载） |
| LLM | 豆包 Pro（接入 Gateway 抽象层） |
| TTS | Minimax（接入 Gateway 抽象层，一期单声音） |
| 短信 | 腾讯云 SMS |
| 服务器 | 腾讯云香港 2C4G / 70GB SSD / 30Mbps（够 ~1000 测试用户） |
| 部署 | systemd + Nginx，不上 Docker |
| 部署形态 | 单体单机（API + Worker 同进程） |
| 通讯协议 | HTTPS + REST + JSON |
| 鉴权 | JWT；登录用手机号+验证码 |
| 日志 | slog JSON 单行 + 文件 + traceId 贯穿；脱敏 |

### 架构必须守住的四条线（v1.1 新增，写入文档第 0 章）
- ① 孩子永远是主角
- ② 爱宝人格一致
- ③ 儿童数据默认敏感
- ④ 故事安全链路可验证

### 强制贯穿原则（为未来迁移而设计）
- 配置外置（不硬编码 IP/域名/密钥）
- 文件存储从第一天起走 COS（**私有 bucket + 签名 URL**）
- 数据库 schema 走 golang-migrate
- 健康检查 `/health` `/ready` 必备
- 启动 / 备份脚本入 git，不靠"祖传脚本"
- 服务器当"牲口"不当"宠物"

### 强制跨层规则
- api / service / repository 三层架构，业务逻辑只在 service
- 所有外部依赖（LLM/TTS/SMS/Storage）通过 Gateway 接口，业务依赖抽象
- 所有用户输入参数化，绝不拼 SQL
- 日志中绝不出现孩子姓名、API Key、明文手机号

### 关键模式（v1.1 经 Codex review 确立）
- **异步任务走 PG Outbox Pattern**：业务写库与事件同事务，Redis 仅做唤醒通知；崩溃可恢复，可重放，含 DLQ
- **音频私有化**：COS 私有 bucket；DB 存 `audio_object_key` 不存 URL；客户端通过后端鉴权后签发 15 分钟签名 URL 访问
- **双层安全链路**：前置预审（PreCheck，省 LLM 成本，规则 + 害怕清单 + IP 归一化）+ 后置审核（PostCheck + 主角身份校验）+ 强约束 System Prompt 模板
- **音频编排**：LLM 输出含 `[音效:xxx][BGM情绪:xxx]` 标记 → cue_parser 解析 → TTS 合成 → ffmpeg 混音 → 多级降级（混音失败→纯TTS→fallback模板）
- **轻量 Metrics**：Prometheus 客户端暴露 `/metrics` 端点（仅 127.0.0.1），覆盖 SLO 关键指标；server 暂不部署
- **children 一期 UNIQUE(user_id)**：DB 层强制单孩子约束，防御重试/并发/脚本误用

## 已实现的能力（不要重做）

### Plan 1（2026-05-07 完成，21 Task）后端基础设施
- Go 项目骨架（cmd / internal / migrations / config）
- 三层架构（api/service/repository）+ Gateway 抽象目录
- 7 个 pkg 工具包：config / traceid / safehash / logger / errors / metrics / api
- 4 个 HTTP 中间件：recover / traceid / logger / metrics
- DB（GORM + PG）+ Redis 客户端
- 端点：`/health` `/ready` `/metrics`
- 数据库迁移工具 + `infra_check` 占位表
- main.go 优雅关停
- 平均覆盖率 ~89%，0 lint issues

### Plan 2（2026-05-07 完成，20 Task）用户认证 + 孩子档案
- users / children 表（含 `UNIQUE(user_id)` 一期单孩子约束）
- 手机号验证码登录（Mock SMS，固定码 `123456`，60s 冷却，5min TTL）
- JWT（HS256，access 24h / refresh 7d，Type 字段防混用）
- 手机号双存：safehash（查询）+ AES-256-GCM（加密原文）
- 孩子档案 CRUD（含部分更新 PATCH 用指针字段）
- JWTAuth 中间件 + AppError → HTTP 状态码自动映射
- 端到端冒烟通过：sms.send → login → me → POST/GET/PATCH children → 401/409 全验证
- 平均覆盖率 ~85%，0 lint issues

### Plan 3（2026-05-08 完成，12 Task）双层安全 + Prompt 模板
- 红线词库 220 词、6 大类，YAML 启动加载到不可变 RuleSet
- IntentProvider 接口 + NoopProvider（Plan 4 替换为 LLMProvider）
- IP 同人化白名单 12 个 + 黑名单 30 个，YAML 管理
- PreCheck 6 类拦截（长度→危险字符→红线→害怕→IP黑→意图）
- PostCheck 3 类拦截（红线→害怕→主角身份启发式）
- System Prompt 模板（text/template + 8 条强约束 + 11 动态字段）
- `cmd/safetycheck` 3 子命令（precheck/postcheck/build-prompt）
- 覆盖率 90.9% / 81.2%；Matcher 11µs/op（远低于 1ms 验收）
- 0 lint issues

### Plan 5（2026-05-11 完成，17 Task）异步音频管线 + TTS Gateway + COS Storage
- TTS Gateway 抽象（Minimax REST t2a_v2 + Mock）
- Storage Gateway 抽象（Tencent COS + Mock；私有 bucket + 15 分钟签名 URL）
- 异步音频管线（Orchestrator 同事务写 `tts_synthesis` outbox 事件 → Worker handler → `audio_status: pending → ready/failed`）
- `GET /api/v1/stories/:id/audio_url`（3 态分支：202 pending / 200 ready+url / 503 failed；所有权校验）
- 端到端真链路冒烟通过：POST 16.3s（audio_status=pending）→ T+6s ready → 880KB / 54s mp3
- 业务 metrics 真埋上：tts_call_duration_seconds / storage_upload_duration_seconds / audio_ready_total

### Plan 6（2026-05-12 完成，16 Task）BOOTSTRAP + 记忆深化 + 彩蛋管线
- BOOTSTRAP onboarding：`GET /bootstrap/questions`（7 题 + version）+ `POST /bootstrap/answers`（验证 → LLM 润色 → 写 `children.profile.description`）
- Memory Summarizer（doubao-1.5-lite-32k 极短 30 字摘要，与长版并写两行 memory：weight=1.0 长版 + weight=1.2 短版）
- Memory Selector（查询 child 近 3 条 `[story_summary, interest]` memory，拼接注入 `BuildInput.MemorySummary`）
- System Prompt 首次/回调双分支模板（含"可以自然回调"软提示）
- 端到端真链路冒烟通过：BOOTSTRAP 润色 2.3s / 故事真生成 15s / Selector 注入 675 字 context 验证
- **修复 2 个 latent bug**：(1) outbox event payload 事务后 patch 不持久化（Plan 4 埋, Plan 6 显形, fix=优先 e.AggregateID）；(2) intent_model endpoint 绑了文生图 Seedream 模型（Plan 4 埋, Plan 4/5 fail-open 兜底掩盖, Plan 6 让 IntentModel 真干活时暴露）
- **Known issues 留给 Plan 6b**：BOOTSTRAP 润色没传 nickname / PostCheck minProtagonistOccurrences=3 太严 / 软提示彩蛋实测 LLM 不听需升级 prompt

### Plan 4（2026-05-09 完成，22 Task）故事生成 + LLM Gateway + Outbox Worker
- LLM Gateway 抽象（Doubao OpenAI 兼容 + Mock + BudgetGate 预算熔断）
- Story Orchestrator（PreCheck → Prompt → LLM → PostCheck → Fallback → Persist 同事务）
- Outbox Pattern（4 表 + Worker + `FOR UPDATE SKIP LOCKED` + 指数退避 + DLQ）
- 故事生成 API（POST /stories/generate, GET /stories/:id）
- 限流 + 预算 middleware（per-user 5/min；超日预算 503）
- 5 个 fallback 故事模板 + 启发式 element extractor
- 业务 metrics 定义（9 个；埋点 Plan 5/6 完善）
- 端到端真豆包冒烟通过：21s / 568 字 / 0 红线 / Outbox 8/8 done / Memories 6 行

## 待决策项

- 域名（发布前再注册，debug 阶段用 IP）
- App Store 上架主体（个人/公司）
- 教育主题库 50-100 主题具体清单
- 真实 IP（如奥特曼）法务策略（白名单同人化方案已起步，法务复核待做）
- 订阅定价
- 儿童数据境内合规方案（大陆正式上线前必须迁境内）
- 商业模式：免费/订阅/付费档位

## 决策时间线

- **2026-04-28** — 完成产品 brainstorming，输出一期 MVP 产品 spec
- **2026-04-28** — 完成技术架构 brainstorming，输出技术架构文档（v1.1 含 Codex review）
- **2026-04-28** — 完成 Plan 1 实现规划（后端基础设施）
- **2026-05-07** — Plan 1 全部 21 Task 完成，端到端冒烟通过
- **2026-05-07** — 完成 Plan 2 实现规划 + 全部 20 Task 实施，端到端冒烟通过
- **2026-05-07** — 知识库补全 Plan 2 涉及的 12 个新概念（10 主题 100+ 词条）
- **2026-05-07** — 完成 Plan 3 实现规划（双层安全 + Prompt 模板，待执行）
- **2026-05-08** — Plan 3 全部 12 Task 完成，CLI demo 通过；覆盖率 90.9%/81.2%
- **2026-05-09** — Plan 4 完成：LLM Gateway + Story Orchestrator + Outbox Worker 全栈实现，端到端真豆包生成验证通过
- **2026-05-11** — Plan 5 完成：TTS (Minimax) + 异步音频管线 + COS 存储，端到端 6 秒出音频（POST 文本立即返回，后台 TTS+上传 ~6s 完成）
- **2026-05-12** — Plan 6 完成：BOOTSTRAP 首次相遇仪式 + 记忆深化（Summarizer + Selector）+ 彩蛋串联管线（含 2 个 latent bugfix：outbox payload 后改不生效 / intent_model endpoint 配错文生图模型）
- **2026-05-13** — Plan 6b 完成：5 项 known-issue 修复 + 第一次真实证明彩蛋回调有效（Story 23 → Story 22 小精灵）
- **2026-05-14** — Plan 7 完成：音频混音管线就位但 MVP 暂不启用 BGM；纯 TTS smoke 验证降级链路完整工作（`bgm_not_found → audio.mix.degraded → pure TTS`），未来收 BGM 上传 COS + `make seed-bgm` 零代码启用

## 关键技术教训（来自实施过程）

- **Windows HTTP 全局代理坑**：本机 `http_proxy=127.0.0.1:18081` 会拦截 curl 并改 body。任何本地 smoke test 必须 `curl --noproxy "*"`
- **Go 二进制必须重新 build**：每次实施完 Task 改了 main/router/handler 后，跑 smoke 前必须 `go build` 一次。`bin/aibao-server` 没刷新会导致"代码改了但路由还是旧的"
- **testcontainers v0.42 API 变化**：`WithWaitStrategyAndDeadline` 弃用，改 `wait.ForLog` / `wait.ForListeningPort`
- **Windows 下无法验证 graceful shutdown**：`taskkill -F` 是 SIGKILL，验不了 SIGTERM 流程；生产环境（Linux+systemd）才能真验
- **viper Unmarshal 不读 env-only 字段**：必须显式 `BindEnv` 列表
- **golangci-lint v2 schema 与 v1 不兼容**：formatters 单独分组，`gosimple` 合到 `staticcheck`
- **火山引擎 Endpoint ID 系统**：豆包 OpenAI 兼容入口不接受模型名称（如 `doubao-1.5-pro-32k`），必须在控制台创建"推理接入点" → 拿到 `ep-m-...` ID → 该 ID 作为 `model` 字段调用。每次接入新模型都要走这个流程。已记录到知识库 11.8。
- **Git Bash on Windows 中文编码污染**：Git Bash 默认 GBK locale 把命令行参数中的 UTF-8 字符串重编码为 GBK 字节再 POST。Smoke 测试中文 prompt 必须用 PowerShell + `[System.Text.Encoding]::UTF8.GetBytes()` 显式发 UTF-8。生产 Flutter 客户端不受影响（HTTP 库永远 UTF-8 序列化）。已记录到知识库 6.10。
- **fail-open vs fail-closed 取舍**：意图分类 LLM 失败时 fail-open 到 safe（不拦用户），红线词匹配 fail-closed 必须拦。原则：影响用户体验的非关键检查 fail-open，安全硬要求 fail-closed。已记录到知识库 11.9 / 10-security-and-compliance.md。
- **viper Bind Env 自动映射规则**：配置字段路径 `a.b.c` 自动绑定到 env `AIBAO_A_B_C`。Plan 4 中 `llm.api_key` 自动绑到 `AIBAO_LLM_API_KEY`，但用户习惯用 `AIBAO_LLM_DOUBAO_API_KEY` → 在 main.go 加 fallback shim 把后者拷贝到前者。
- **COS Bucket 命名陷阱**（Plan 5）：腾讯云 COS 控制台显示完整名 `<short>-<appid>`，但 cos-go-sdk-v5 期望传入**短名**——SDK 内部自动拼 `-<appid>`。运维者很容易把完整名贴进配置 → SDK 又拼一次 → 404 NoSuchBucket。生产代码必须容错"完整名/短名"二义性输入：`if !strings.HasSuffix(Bucket, "-"+AppID) { Bucket = Bucket + "-" + AppID }`。同类陷阱出现在 S3、阿里 OSS。已记录知识库 10。
- **异步音频路径的体验价值**（Plan 5）：实测 TTS + COS 上传仅 ~6 秒——比 LLM 文本生成 16 秒还短。即便如此，**走异步**让用户在 16s 拿到文本立即可读，比同步路径等满 22s 体验好得多。感知延迟 = 总延迟 × 用户专注度——把"不必专注等待"的工作甩到后台是 UX 设计的核心手段。已记录知识库 5.15。
- **smoke 前先做基础设施连通性预测试**（Plan 5）：Plan 4 教训沉淀，Plan 5 接 COS 和 Minimax 时分别先用一次性 `smoke-cos/main.go` / `smoke-tts/main.go`（脱离项目代码、裸 SDK 调用）验证凭证 + bucket + voice_id，提前发现 region 不匹配 + 子账号策略未绑 + bucket 双拼问题。这类问题在应用层 smoke 才暴露会被业务逻辑包装错误信息淹没，定位至少 ×3 时间。已记录知识库 6.12。
- **outbox event 的 payload 不可后改**（Plan 6 暴露的 Plan 4 旧坑）：一旦 outbox row INSERT 进表，Go 内存里那个 `*OutboxEvent` 对象的修改不会同步到 DB。常见陷阱是事务里"先 INSERT 占位、后填补真值"——Orchestrator 在事务里写 outbox 时 story.ID 还没生成，事务后 patch payload.story_id 只改了内存。Handler 拉到的 payload 永远是占位值 0。**正确做法**：用 outbox row 的 `aggregate_id` 字段（CreateWithOutbox 内部填）承载这类"事务后才知道"的 id，或 handler 进来时再 query 一次最新 entity。Plan 4 埋下、Plan 5 tts_synthesis 已绕过、Plan 6 memory FK 才真正暴露。已记录知识库 5.16。
- **fail-open 链路让 bug 推迟到下一个 plan 才显形**（Plan 6 暴露的 Plan 4 旧坑）：Plan 4 时 intent_model endpoint 绑成 Doubao-Seedream-5.0-lite（文生图模型，不接受 chat completion）——意图分类 LLM 全部 404，但 fail-open 兜底到 IntentSafe，**业务功能无感**。Plan 6 让 IntentModel 真干活（Bootstrap polish + Memory summary），bug 立刻浮出。**教训**：fail-open 链路必须配指标告警（`rate(*_fail_fallback_total) / rate(*_call_total) > 30% over 5min` 触发），否则 latent bug 累积到下游集中爆发。修复需要 Volcengine 控制台新建 endpoint。已记录知识库 9.14。
- **软提示 prompt 工程的局限**（Plan 6 实测）：把"上次故事的 30 字总结"塞进 system prompt 尾部 + 写"可以自然回调"，LLM 仍倾向沿着 user prompt 自由发挥，对 memory context **几乎不响应**——Plan 6 smoke 海洋故事记忆 + 森林新主题 → 故事 3 完全没出现海洋元素。"软提示"哲学优雅但效果弱。后续要么把 memory 段抬到更高优先级位置、要么改用更明确"请尝试借用..."措辞、要么加一轮"故事编辑 LLM call"检查回调是否出现并 force regenerate。**先验证、后优化、再上线**是 prompt 工程的标准节奏。已记录知识库 11.10。
- **基础设施先于内容**（Plan 7 设计选择）：Plan 7 选择把整条 BGM 管线（migration + repo + cache + mixer + orchestrator + CLI）写完上线，但**不收一首 BGM**。理由：基础设施编写 + 测试是确定性工作可外包给 subagent；BGM 素材筛选是人类品味判断的 1-2 小时事，把它压在 critical path 上是错配。**等用户反馈想要音乐时再花那 1-2 小时**——零代码工作启用，Plan 6b 投资的 `bgm_not_found_total` 指标今天就在 dashboard 提供"启用前后差多少"的可见信号。这条原则可推广：所有"内容驱动 + 代码驱动"双轨任务，**代码先行，内容后补**。已记录知识库（devlog 2026-05-14 教训第 3 条）。
- **prompt 工程的措辞 + 位置联动**（Plan 6b 实测修正 Plan 6 结论）：Plan 6 时 memory 段在 system prompt 末尾 + 措辞"可以自然回调"，LLM 完全无视。Plan 6b 同时把段落移到 IDENTITY 之后 + 改成"请尝试借用以下记忆里的角色或场景"，Story 23 真回调了 Story 22 的小精灵——**单独改一项可能没效果，组合改才质变**。这条印证了知识库 11.10「软提示 vs 硬提示」的二元论过于简化，真实工程是"软提示 + 位置加权 + 措辞强度"的三维调节。已追加到知识库 11.10 末尾。
