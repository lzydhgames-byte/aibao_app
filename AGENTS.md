# AGENTS.md — 爱宝（Aibao）项目协作指南

本文件用于指导 AI 助手（Codex 等）在本项目中的协作方式。每次会话开始时会被自动加载。

---

## 1. 项目一句话

爱宝是一款面向儿童的 AI 故事 App。家长设置孩子档案后用文字或语音描述需求，AI 以孩子为主角生成个性化故事，TTS+BGM 朗读。爱宝是一只百变的熊猫小机器人 IP，拥有跨会话记忆，让 AI 像有生命的伙伴。

## 2. 当前阶段

**brainstorming → spec 已完成（一期 MVP）**
当前下一步：写实现规划（writing-plans），技术架构尚待 brainstorm。

权威设计文档：[docs/superpowers/specs/2026-04-28-aibao-design.md](docs/superpowers/specs/2026-04-28-aibao-design.md)

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
