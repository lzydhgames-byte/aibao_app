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
