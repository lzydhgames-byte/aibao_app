# 软件设计原则与模式

按"通用原则 → 设计模式"顺序。

---

## 5.1 关注点分离（Separation of Concerns, SoC）

**一句话**：每段代码只做一件事，不掺和别的。

**生活类比**：餐厅里切菜的、炒菜的、上菜的、收银的各管一摊——分工明确，谁请假都不会让整家店瘫痪。如果切菜的同时管收银，今天他病了店里就停摆。

**在我们项目的体现**：
- 三层架构（[03.2](03-go-engineering.md#32-三层架构-api--service--repository)）—— api / service / repository 各管一摊
- viper + mapstructure（[02.7](02-go-language.md#27-三层配置解码链路)）—— 一个管"从哪读"，一个管"怎么填"

**违反它会怎样**：代码纠缠成一团，改一处带来连锁问题；测试要同时 mock 一堆东西。

**何时引入**：技术架构 spec；Task 1-2 体现。

---

## 5.2 依赖倒置原则（Dependency Inversion Principle, DIP）

**一句话**：高层业务依赖**抽象接口**，不依赖**具体实现**。

**对比**：
- ❌ **直接依赖具体**：`service.story.Generate()` 里直接 `import doubaoSDK`，调用 `doubaoSDK.Call(...)`。换 LLM 提供商 → 业务代码全改。
- ✅ **依赖抽象**：`service.story` 只依赖 `gateway/llm.Client` 接口，不知道也不关心底层是豆包还是 Claude。换提供商 → 只新增一个实现文件，业务零改动。

```go
// gateway/llm/llm.go（接口）
type Client interface {
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}

// gateway/llm/doubao.go（实现）
type doubaoClient struct { /* ... */ }
func (c *doubaoClient) Generate(...) { /* 调豆包 */ }
```

**生活类比**：插座是抽象（统一标准），灯泡是具体实现。换灯泡随便换，插座不变。如果灯泡焊死在墙里，换灯泡要拆墙。

**我们项目的体现**：技术架构第 6 章——LLM / TTS / SMS / Storage 全部走 Gateway 抽象层。

**何时引入**：技术架构 spec；Plan 4-5 实现。

---

## 5.3 YAGNI（You Aren't Gonna Need It）

**一句话**：在没有实际需要时，**不要**提前引入复杂度。

**反面教材**：
- "我**可能**未来要支持多语言" → 现在就加 i18n 框架
- "万一**以后**要换数据库" → 现在就抽象 ORM 层
- "**说不定**会上 K8s" → 现在就写 Helm chart

**结果**：代码堆满"为未来准备"的废墟，真到那一天需求又变了，那些预备工作要么没用要么过时。

**正确做法**：等到痛点真的出现，再加复杂度。

**生活类比**：刚搬进新家就买"以备不时之需"的备用沙发、备用床、备用洗衣机——结果客厅塞不下，还都没用过。

**我们项目的体现**：MVP 不上 Docker（systemd 够用）、不拆微服务、不上 Kafka（Outbox 够用）、不部署 Prometheus server（暴露端点即可）。

**何时引入**：技术架构 spec 第 1.2 节"不引入的技术"明确列了一堆。

---

## 5.4 12-Factor App（十二要素）

**一句话**：构建可云原生部署 SaaS 应用的 12 条原则，业内事实标准。

完整 12 条 [12factor.net](https://12factor.net/)。我们目前用到的核心几条：

### 配置（Config）—— 严格分离环境
**原则**：代码里**绝不**写死配置（数据库 URL、密钥）。配置通过**环境变量**注入。

**为什么**：
- 同一份代码可以跑在 dev / staging / prod，行为不同只看 env
- 密钥永远不上 git
- 上线流水线只改 env，不改代码

**我们项目的体现**：
- 配置走 yaml + 环境变量覆盖（`AIBAO_POSTGRES_PASSWORD` 等）
- `config.prod.yaml` 在 `.gitignore` 里
- 任何 IP / 域名 / 密钥都不写死在 Go 源码

### 进程（Processes）—— 无状态
**原则**：每个进程不在内存里保存"会话状态"，状态全放外部存储（DB / Redis）。

**为什么**：进程随时可重启，重启后能立刻提供服务；可以水平扩展（启 N 个进程负载均衡）。

**何时引入**：Task 2 配置；后续 Task 持续遵守。

---

## 5.5 Pets vs Cattle（宠物 vs 牲口）

**一句话**：现代运维把服务器当"牲口"——挂了换新的，10 分钟拉起来；不当"宠物"。

**对比**：
- **宠物服务器**：精心配置、有名字（"web01"）、出问题就抢救；上面装了无数手动配置；坏了团队哭着抢救
- **牲口服务器**：随时可以杀掉换新的，因为所有配置都在代码里；坏了？买一台新的，跑一个脚本，10 分钟恢复

**怎么做到**：
1. 配置外置（[5.4 12-Factor 配置](#54-12-factor-app十二要素)）
2. 文件存对象存储而不是本地硬盘
3. 数据库 schema 走迁移工具（migrate）
4. 启动脚本入 git（`scripts/install.sh`）
5. 备份脚本入 git（`scripts/backup.sh`）

**我们项目的体现**：技术架构第 15.2 节"为迁移而设计"——从第一天起就把服务器当牲口。

**何时引入**：技术架构 spec；Plan 9 部署阶段彻底落地。

---

## 5.6 幂等（Idempotent）

**一句话**：执行 N 次和执行 1 次结果完全一样。

**例子**：
- ✅ 幂等：`UPDATE users SET status='active' WHERE id=42` —— 跑 100 次都是 active
- ❌ 不幂等：`UPDATE users SET balance = balance + 100 WHERE id=42` —— 跑 100 次扣 100 块还是 1 万块？差别巨大

**为什么重要**：
- 网络抖动、超时重试、Worker 重启都可能导致同一操作执行多次
- 幂等设计让"多次执行"变安全
- 这是分布式系统能稳定运行的基础

**生活类比**：电梯按钮——按一次和按十次都是"叫电梯到这一层"，结果一样。如果按一次叫一趟，按十次叫十趟，那就乱套了。

**我们项目的体现**：
- traceid 包的 `Ensure(ctx)` 函数：有 traceId 就用，没有才生成
- Outbox handler：`UPSERT` 写库（冲突时覆盖，结果与首次写一致）
- 任务 payload 带 `task_id`，处理前先查"是否已做过"

**何时引入**：Task 3 traceid 的 `Ensure` 函数；Plan 6 Outbox Worker 强制要求。

---

## 5.7 Outbox Pattern（事务出箱模式）

**一句话**：业务库写入 + "待发事件"写入**在同一个数据库事务**里，保证事件不丢。

**问题**：
传统做法是"写完业务库 → 发消息到队列"。两步之间崩溃 → 业务写了但消息没发 → 下游永远不知道。

例如：故事保存了但 MEMORY 没更新——产品核心卖点"有记忆的 AI"被破坏。

**Outbox 解法**：
```sql
BEGIN;
INSERT INTO stories(...);                              -- 业务写入
INSERT INTO outbox_events(event_type, payload, ...);   -- 事件写入（同一事务）
COMMIT;
```

由于 PG 事务的原子性，**要么两个都成，要么两个都不成**——绝不会"业务写了但事件没记"。

然后 Worker 异步从 `outbox_events` 表里读未处理的事件来做后续。Worker 崩溃也不要紧——事件还在表里，下次重启继续处理。

**生活类比**：餐厅厨房收单——服务员把"客人点了什么"和"送到几号桌"**写在同一张订单纸上**，绝不会出现"做了菜但不知道送哪儿"。

**我们项目的体现**：技术架构第 9 章；将由 Plan 6 实现。

**何时引入**：技术架构 spec。

---

## 5.8 优雅关停（Graceful Shutdown）

**一句话**：进程退出前先完成在途请求，再关闭。

**对比**：
- ❌ 强杀：`kill -9` —— 进程立即死，正在处理的请求中断，用户看到"故事生成到一半失败"
- ✅ 优雅：`kill -TERM`（默认 `kill`）—— 进程收到 SIGTERM 信号 → 停止接受新请求 → 等正在处理的请求完成 → 关闭数据库连接 → 退出

**实现要点**：
1. 监听 SIGTERM / SIGINT 信号
2. 收到信号后调 `srv.Shutdown(ctx)`，给 30 秒让在途请求完成
3. 关闭 DB / Redis 连接

**何时引入**：Task 18 main.go。

---

## 5.9 软件分层与依赖方向

**一句话**：高层依赖低层，低层不依赖高层。

```
api 层
    ↓ 依赖
service 层
    ↓ 依赖
repository 层 + gateway 层
    ↓ 依赖
pkg（工具包）
```

**为什么这样**：
- pkg 没有任何依赖 → 任何包都能用，永远不会循环依赖
- repository 不依赖 service → 换 service 不影响 repository
- 测试时 mock 起来很容易（mock 下层即可）

**Go 编译器会拦截"循环依赖"**——A 包 import B，B 包 import A，编译报错。这强制你把分层想清楚。

**何时引入**：Task 1 创建目录结构时已遵守。
