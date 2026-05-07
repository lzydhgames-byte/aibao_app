# 爱宝项目知识库

本目录是项目开发过程中积累的"软件工程知识手册"。每个文件按主题分类，每条知识点都用接地气的方式讲解，便于回头复盘。

---

## 阅读建议

- 不必从头读到尾。**遇到不懂的术语时，先来 README 找在哪个文件，再去查**。
- 每条都标了"何时引入"——可以反过来看，复习"那次开发用到了什么"。
- 同一个概念第一次出现时讲透；后面提到只引用编号，不重复。

---

## 主题索引

| 文件 | 内容 |
|---|---|
| [01-git.md](01-git.md) | Git 仓库、commit、分支、`.gitignore`、commit message 惯例、`.gitkeep` |
| [02-go-language.md](02-go-language.md) | module path、`internal/`、context、struct tag、错误包装、iota、errors.Is/As、Mutex、regex、指针 PATCH |
| [03-go-engineering.md](03-go-engineering.md) | Go 项目结构、三层架构、Makefile、go.mod / go.sum、go install / -tags |
| [04-docker.md](04-docker.md) | Docker、容器 vs 虚拟机、docker-compose、绑定 127.0.0.1 |
| [05-software-design.md](05-software-design.md) | 关注点分离、依赖倒置 (DIP)、YAGNI、Outbox Pattern、12-Factor App、Pets vs Cattle、AppError 模式 |
| [06-testing.md](06-testing.md) | TDD 循环、单元/集成/E2E、`t.TempDir`、`t.Setenv`、testify、表驱动、testcontainers |
| [07-http-and-web.md](07-http-and-web.md) | 中间件、洋葱模型、Gin、状态码全集、Bearer 认证、CRUD（PATCH vs PUT） |
| [08-database.md](08-database.md) | PG/ORM/连接池、migration（含 golang-migrate 实操）、GORM、Redis 数据结构、TTL/LRU、SETNX/GETDEL |
| [09-observability.md](09-observability.md) | 结构化日志、TraceID、lumberjack、Prometheus 三类指标、SLO |
| [10-security-and-compliance.md](10-security-and-compliance.md) | hash/salt、三种脱敏、JWT、HMAC vs RSA、Access/Refresh Token、AES-256-GCM/Nonce |

---

## 何时更新

每当 Claude 在协作中讲解一个新概念，**会同步追加到对应文件**。如果你发现讲过但没记录的知识点，提醒 Claude 补上即可。
