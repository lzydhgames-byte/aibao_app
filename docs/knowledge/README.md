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
| [02-go-language.md](02-go-language.md) | Go 模块、module path、`internal/` 目录、struct tag、context、ctxKey 技巧 |
| [03-go-engineering.md](03-go-engineering.md) | Go 项目结构、三层架构、Makefile、go.mod / go.sum、go install / -tags |
| [04-docker.md](04-docker.md) | Docker、容器 vs 虚拟机、docker-compose、绑定 127.0.0.1 |
| [05-software-design.md](05-software-design.md) | 关注点分离、依赖倒置 (DIP)、YAGNI、Outbox Pattern、12-Factor App、Pets vs Cattle |
| [06-testing.md](06-testing.md) | TDD 循环、单元测试 vs 集成测试、`t.TempDir`、`t.Setenv`、覆盖率 |
| [07-http-and-web.md](07-http-and-web.md) | （后续 Task 引入）|
| [08-database.md](08-database.md) | （后续 Task 引入） |
| [09-observability.md](09-observability.md) | 结构化日志、TraceID、指标 vs 日志（后续完善）|
| [10-security-and-compliance.md](10-security-and-compliance.md) | 密钥管理、敏感字段脱敏、签名 URL（后续完善）|

---

## 何时更新

每当 Claude 在协作中讲解一个新概念，**会同步追加到对应文件**。如果你发现讲过但没记录的知识点，提醒 Claude 补上即可。
