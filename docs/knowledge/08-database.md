# 数据库相关

## 8.1 PostgreSQL（PG）
开源关系型数据库，业内主流之一。  
**为什么选 PG 不选 MySQL**：JSONB 字段、向量扩展（pgvector）、强一致事务都比 MySQL 强；MVP 单实例就能扛十万级用户，没必要为"将来分布式"提前付代价。

## 8.2 ORM（Object-Relational Mapping）
Go 结构体 ↔ 数据库表的自动映射工具。代替手写 SQL：
```go
db.Create(&user)            // 而不是 INSERT INTO users ...
db.Where("id = ?", 42).First(&user)
```
项目用 GORM。  
**为什么需要**：手写 SQL 容易拼错、容易 SQL 注入、字段改名了 SQL 字符串编译时不会报错——上线才发现。ORM 让"对象"和"表"对接，类型安全且查询参数自动转义。

## 8.3 连接池（connection pool）
不是每次查询都建一个 PG 连接（建立连接慢且消耗 PG 资源）——而是预先建一组连接、用完归还。GORM 默认就有连接池。  
关键参数：
- `SetMaxOpenConns(20)` — 最多 20 个并发连接
- `SetMaxIdleConns(5)` — 空闲时保留 5 个不释放
- `SetConnMaxLifetime(1h)` — 连接最长 1 小时强制重建（防止 NAT/防火墙断老连接）

**为什么需要**：建 PG 连接需要 TCP 握手 + 认证，几十毫秒；高并发下"每查一次建一个"延迟爆炸还会拖垮 PG 服务器。连接池把"建立成本"摊到所有请求上。

## 8.4 数据库迁移（database migration）
把数据库 schema 变化（建表、加字段、改索引）当成代码版本管：
```
migrations/
├── 000001_init.up.sql      ← 升级到 v1
├── 000001_init.down.sql    ← 回滚到 v0
├── 000002_users.up.sql     ← 升级到 v2
└── 000002_users.down.sql
```
工具（如 golang-migrate）按编号顺序执行 `.up.sql`，并维护一张 `schema_migrations` 表记录"现在到哪个版本了"。  
**为什么需要**：手动改生产数据库 schema = 灾难（忘了步骤、跨环境不一致、回滚不了）。迁移让 schema 变更**可重放、可版本控、可回滚**——和应用代码一样进 git。

### 8.4.1 golang-migrate 实操命令
```bash
# 升级到最新版本（执行所有未跑的 .up.sql）
migrate -path migrations -database "postgres://..." up

# 回滚 1 步
migrate -path migrations -database "..." down 1

# 查看当前版本
migrate -path migrations -database "..." version

# 强制设置当前版本（修复"卡在中间"的状态）
migrate -path migrations -database "..." force <N>
```
项目里 `Makefile` 封装了 `make migrate-up` / `make migrate-down`。  
`main.go` 启动时也调 `repository.RunMigrations(db, "migrations")`——服务每次启动自动跑未应用的迁移，避免"上线前忘了跑迁移"。  
**为什么 force 命令存在**：如果一个 .up.sql 跑到一半失败（比如某条 SQL 错），版本会卡住。修好 SQL 后用 `force <上一个版本号>` 重置。

### 8.4.2 GORM 基础操作
ORM = "对象-关系映射"。GORM 把 Go 结构体和 SQL 表对应起来。常见用法：
```go
db.Create(&user)                            // INSERT
db.First(&u, id)                            // SELECT * WHERE id=? LIMIT 1
db.Where("phone_hash = ?", h).First(&u)     // 带条件查询（参数化防注入）
db.Save(&user)                              // INSERT or UPDATE
db.Model(&u).Updates(map[string]any{...})   // 部分更新
db.Delete(&u)                               // DELETE
db.WithContext(ctx)                         // 带 context（超时/取消）
```
重要：**所有用户输入必须用 `?` 占位符 + 参数**，绝不拼字符串——否则 SQL 注入。  
**为什么用 GORM**：手写 SQL 容易写错列名、忘了加索引提示、字段改了 SQL 字符串没改也不报错。GORM 让"对象 ↔ 表"映射在编译期就能检查到字段错误。  
项目体现：`UserRepo` / `ChildRepo` 全部走 GORM；`isUniqueViolation(err)` 把 GORM 抛出的"重复键错误"翻译成 `ErrAlreadyExists`。

## 8.5 Redis 数据结构
| 类型 | 用途 |
|---|---|
| **String** | KV 缓存（最常用） |
| **List** | 队列 / 栈（LPUSH/RPOP） |
| **Hash** | 字段化对象（节省内存） |
| **Set** | 无序唯一集合（去重） |
| **Sorted Set** | 带分数排序集合（排行榜） |
| **Stream** | 消息流（带消费组） |
| **Pub/Sub** | 发布订阅（无持久化） |

项目用：String（缓存/限流/会话）、Pub/Sub（Outbox Worker 唤醒）。  
**为什么 Redis 这么强**：纯内存 + 单线程模型让所有操作微秒级完成；多种数据结构让你不用在客户端组装——服务端原子完成"原子计数器""列表 push"等需求。

## 8.6 TTL 与 LRU 淘汰
- **TTL（Time To Live）**：每个 key 设过期时间，到点自动消失
- **LRU 淘汰（Least Recently Used）**：内存满时优先删"最久没访问"的 key

我们 Redis 配置 `maxmemory 512mb` + `maxmemory-policy allkeys-lru`：内存到上限自动淘汰冷 key，绝不 OOM。  
**为什么需要**：缓存系统不能无限增长——Redis 只有内存这一种存储；TTL 让过期数据自然清理，LRU 在内存压力下保留"热"数据。

## 8.7 `SELECT ... FOR UPDATE SKIP LOCKED`
PG 行级锁的"跳过已锁行"模式。Outbox Worker 用它做并发安全：多个 Worker 同时拉任务，每个 Worker 拿到的是独占的不同行。  
```sql
SELECT * FROM outbox_events WHERE status='pending'
ORDER BY id LIMIT 10 FOR UPDATE SKIP LOCKED;
```
**为什么需要**：传统做法是 SELECT + UPDATE 两步——并发时两个 Worker 可能拿到同一行。`FOR UPDATE` 锁住选中行；`SKIP LOCKED` 让其他 Worker 自动跳过被锁的行——天然并发安全。

## 8.8 Redis 原子操作：SETNX / GETDEL
Redis 单线程模型让单个命令天然原子，但**两个命令之间**有竞态。下面两个组合命令把"两步合一"做成原子：

- **SETNX**（SET if Not eXists）：仅当 key 不存在时才设值，并返回是否成功。  
  项目用法：发短信冷却——`SETNX cooldown:138... 1 EX 60`，返 `false` 就意味着 60s 内已发过，拒绝重发
- **GETDEL**：原子地"读出值并立即删除"。  
  项目用法：验证码"一次性消费"——`GETDEL code:138...`，读到值就同时删掉，避免被重放使用

**类比**：
- SETNX = 抢车位——只允许第一个抢到的人占
- GETDEL = 取快递+签收——一气呵成，不会被别人半路抢走

**为什么不用 SET + DEL 两步**：高并发下两步之间会有窗口期。SETNX/GETDEL 是 Redis 服务端**单条命令**完成，**任何并发都不可能拆开**。  
项目体现：`auth/codestore_redis.go` 的 `Save`（SETNX cooldown + SET code）和 `Take`（GETDEL code）。

## 8.9 数据库事务（Transaction） + ACID
把多条 SQL 包成"全成或全不成"的原子单位。
```sql
BEGIN;
INSERT ...; INSERT ...; UPDATE ...;
COMMIT;            -- 全部生效（或 ROLLBACK 全部撤销）
```
**ACID 四特性**：
- **A** 原子性：要么全成要么全不成
- **C** 一致性：事务前后数据库满足约束
- **I** 隔离性：并发事务互不干扰
- **D** 持久性：commit 后断电也不丢

**为什么需要**：Outbox Pattern 完全靠事务原子性——业务写入和"待发任务"在同一事务里，要么都成要么都失败。这是分布式系统一致性的基石。

## 8.10 数据库索引（Index）
给某列做"目录"——查询时不用扫全表。  
**类比**：书的目录——找"第 5 章"直接翻 78 页，不用从第 1 页翻起。  
**为什么不全列加索引**：索引占空间，写入时要维护。**读多写少的列**才加；写多读少（如日志表）通常只主键。  
项目体现：`outbox_events(status, next_attempt_at)` 让 Worker 拉任务秒回；`stories(child_id, created_at DESC)` 查孩子近期故事秒回。


## 8.11 GORM AutoMigrate vs 生产 migrate：schema 必须双向等价

项目用 `golang-migrate` 跑 `.up.sql` 文件作为生产 schema 源；但集成测试用 `testcontainers + GORM AutoMigrate` 从 Go struct tag 反射建表。**两者必须保持等价**——否则测试通过的代码上生产可能就因 schema 差异而炸，或反过来生产正常而测试莫名报错。

**真实案例**（Plan 7 Task 2 fix，commit `a2d5b25`）：`bgm_assets` 表生产 migration 写了 `filename VARCHAR(200) NOT NULL UNIQUE`，但 GORM struct tag 漏写 `uniqueIndex` → AutoMigrate 建出来的表没 UNIQUE 约束 → `Upsert` 的 `ON CONFLICT(filename)` 在测试里报"no matching constraint"，生产却 OK。**修法**：给 struct 加 `gorm:"uniqueIndex"` tag 让两边等价。

**为什么需要**：测试环境 schema = 生产环境 schema 是"集成测试的可信度"基础。一旦两边发散：
- 测试通过 ≠ 生产可行——隐藏的差异让 bug 漏到生产
- 测试失败可能只是"测试环境的 schema 错"——开发会浪费时间调代码逻辑而真凶是 struct tag
- 这种"信号污染"会逐步腐蚀对测试的信任——人会开始绕过测试

**实施清单**（每加一张表都过一遍）：

| schema 约束 | SQL migration | GORM struct tag |
|---|---|---|
| 主键 | `PRIMARY KEY` | `primaryKey` |
| 外键 | `REFERENCES x(y)` | （通常 GORM 关系或手动加） |
| UNIQUE 约束 | `UNIQUE` | `uniqueIndex` |
| NOT NULL | `NOT NULL` | `not null`（gorm 默认 nullable！） |
| 默认值 | `DEFAULT xxx` | `default:xxx` |
| 复合索引 | `CREATE INDEX ... ON t(a,b)` | `index:idx_name,composite:xxx` |
| 列名映射 | 列名 | `column:xxx` |

**典型坑**：GORM 默认所有字段都是 nullable（即使 Go 里是非指针类型）——必须显式 `not null`。生产 SQL 没遗漏，但测试环境因 GORM 默认行为漏了约束。

**项目体现**：Plan 7 Task 2 fix（subagent 在跑集成测试时自报，commit `a2d5b25`）。

**类比**：合同——中文版和英文版必须逐条对应。漏一条约束就像"中文写了违约金 10 万但英文写 1 万"——签的时候没人察觉，出事时才发现两边对不上。

