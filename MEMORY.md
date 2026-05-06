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

## 待决策项

- 域名（发布前再注册，debug 阶段用 IP）
- App Store 上架主体（个人/公司）
- 教育主题库 50-100 主题具体清单
- 真实 IP（如奥特曼）法务策略
- 订阅定价
- 儿童数据境内合规方案（大陆正式上线前必须迁境内）
- 商业模式：免费/订阅/付费档位

## 决策时间线

- **2026-04-28** — 完成产品 brainstorming，输出一期 MVP 产品 spec
- **2026-04-28** — 完成技术架构 brainstorming，输出技术架构文档
