# Go 工程实践

按"项目骨架 → 工具链 → 测试与质量"顺序。

---

## 3.1 标准项目布局（cmd / internal / pkg）

**一句话**：Go 社区有一套常见目录约定，跟着用能让任何 Go 程序员一眼看懂结构。

```
server/
├── cmd/server/         程序入口（main.go 在这里）
├── internal/           内部代码（外部不能 import）
│   ├── api/            HTTP handler
│   ├── service/        业务逻辑
│   ├── repository/     数据访问
│   ├── gateway/        外部依赖抽象
│   └── pkg/            内部共用工具
├── migrations/         数据库迁移 SQL
├── config/             配置文件
└── go.mod
```

**关键点**：
- `cmd/<binary-name>/main.go` —— 程序入口。如果有多个二进制（比如 `cmd/server`、`cmd/cli`），各自一个目录
- `internal/` —— 见 [02.3 internal 目录](02-go-language.md#23-internal-目录私有包)
- `pkg/` —— 在 `internal/` 之内的 `pkg` 是"内部共用工具"。`internal/pkg/` 的代码可以被同 module 的所有部分用，但外部不行

**生活类比**：
- `cmd/` 是大门
- `internal/` 是私人卧室（外人不能进）
- `internal/pkg/` 是公共工具间（家人都能用）

**何时引入**：Task 1 创建目录骨架。

---

## 3.2 三层架构（api / service / repository）

**一句话**：把代码分成三层，每层只做一件事，且不跨层调用。

```
HTTP 请求
    ↓
api 层（handler）：    把 HTTP 请求"翻译"成业务参数
    ↓
service 层：         业务规则核心。"生成故事时要先查档案 → 注入记忆 → 调 LLM"
    ↓
repository 层：       只跟 PG/Redis 说话
    ↓
PostgreSQL / Redis
```

**每层的纪律**：
- **api 层**：**绝不写业务规则**。只解析参数 / 校验格式 / 调 service / 把返回值转 JSON
- **service 层**：核心业务逻辑都在这。**不依赖 HTTP，不依赖具体数据库**——单元测试时 mock 掉 repository 和 gateway 即可，无需真实 DB
- **repository 层**：只 CRUD 数据库，不写业务判断

**好处**：
- 业务逻辑独立于 HTTP 和数据库
- 单测只测 service 层，不需要起服务器和数据库
- 将来要做 CLI 工具或 cron 任务，直接调 service，不用绕 HTTP

**何时引入**：技术架构 spec 第 3 章；Task 1 创建对应目录。

---

## 3.3 `go.mod` / `go.sum`

**一句话**：`go.mod` 列出"我依赖谁"，`go.sum` 锁住"具体哪个版本的哈希"，确保不同人构建结果一致。

**`go.mod` 长这样**：
```
module github.com/aibao/server

go 1.22

require (
    github.com/spf13/viper v1.19.0
    github.com/google/uuid v1.6.0
)
```

**`go.sum`**：每个依赖的 SHA256 哈希。Go 在下载依赖时校验哈希，发现不匹配立刻报错——**防止有人偷偷篡改你依赖的代码**。

**两个文件都必须 commit**——不 commit `go.sum`，新机器上构建出来的二进制可能用了被篡改的依赖。

**何时引入**：Task 1 `go mod init`；Task 2 添加 viper 依赖后自动生成。

---

## 3.4 `go mod tidy`

**一句话**：清理 `go.mod` 和 `go.sum`，只保留实际用到的依赖。

**使用场景**：
- 加了新 import → 跑 `go mod tidy` 把它写入 go.mod
- 删了某个 import → 跑 `go mod tidy` 把它从 go.mod 移除

**何时引入**：Task 2 添加 viper / testify 依赖。

---

## 3.5 `go install` 与 `-tags`

**一句话**：从源码下载并编译一个 Go 写的命令行工具，放进 `$GOPATH/bin`。

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

- 拉源码 → 编译 → 二进制放进 `$GOPATH/bin`
- 该目录通常已在 `PATH` 里，所以装完就能直接 `migrate -version`

**`-tags 'postgres'` 是什么**：Go 的"构建标签"。某些库用条件编译——`migrate` 工具支持很多数据库（MySQL/SQLite/MongoDB...），全编进去太大。`-tags 'postgres'` 告诉编译器"只编译标记了 postgres 的代码部分"。

**生活类比**：买菜清单上写"只要绿色蔬菜"，水果摊老板就只给你拿绿叶菜。

**何时引入**：Task 0 安装 migrate / golangci-lint。

---

## 3.6 Makefile 与 `make`

**一句话**：把"团队成员要敲的命令"封装成统一短指令，避免每个人记法不同。

```makefile
test:
	go test -race -count=1 -cover ./...
```

之后 `make test` = `go test -race -count=1 -cover ./...`。

**为什么需要**：
- 团队所有人 `make test`，参数永远统一
- CI/CD 流水线也直接 `make test`，发版前测试和本地一致
- 复杂命令不用记参数

**注意**：Makefile **必须用 TAB 缩进 recipe**，不能用空格——这是 1976 年 Stuart Feldman 设计 Make 时定的，他后来公开承认是设计错误，但已无法修改。

**何时引入**：Task 1 创建 `server/Makefile`。

---

## 3.7 Lint（代码检查）与 golangci-lint

**一句话**：除了"编译能过"之外的另一道质量防线，自动找出"虽能跑但有问题"的代码。

**典型 lint 能发现**：
- 声明了变量没用（`unused`）
- 错误返回值忘了处理（`errcheck`）
- 函数命名不规范（`revive`）
- 缩进 / 格式不统一（`gofmt`）
- 字面错别字（`misspell`）

**`golangci-lint`**：把 30+ 种 linter 集成到一个工具里，配置文件 `.golangci.yml` 选择启用哪些。

**为什么要在 MVP 就启用**：lint 是"低成本高收益"——配置一次，自动持续帮你拦截低级错误。等代码量大了再补补不上。

**v1 vs v2 配置**：v2 改了 schema（formatters 单独分组、删了 `gosimple` 合并到 `staticcheck`），如果用 v1 syntax 在 v2 上跑会报错。Task 1 的实现里我们用了 v2 syntax。

**何时引入**：Task 0 安装；Task 1 配置 `.golangci.yml`；以后每个 Task 都跑一遍。

---

## 3.8 测试覆盖率

**一句话**：测试运行时"碰到了多少行代码"的比例。

```bash
go test -cover ./...
```

输出类似 `coverage: 87.3% of statements`。

**怎么用**：
- 不是越高越好——80% 是合理目标，100% 通常代价过大且鼓励写无意义的测试
- **关键路径必须覆盖**（业务核心、安全相关、错误处理）
- 简单 getter / setter 不必强求覆盖

**Plan 1 的标准**：service+pkg 层 ≥ 70%。

**何时引入**：Task 2 第一个有真实代码的包，自动启用覆盖率统计。

---

## 3.9 godoc 注释

**一句话**：写在每个**导出**（首字母大写）类型、函数、常量上方的注释，工具会用它生成文档。

```go
// New returns a new trace id like "tr-<8-char>" (truncated UUID for log brevity).
func New() string { ... }
```

**规则**：
- 注释**必须以函数/类型名开头**（`New returns ...` 而不是 `Returns ...`）
- 一句话总结即可，复杂的可以多写几行
- linter `revive` 的 `exported` 规则会强制要求每个导出符号都有注释

**为什么强制**：Go 的标准库/优秀第三方库都遵循这个习惯，所以 IDE 鼠标悬停就能看到 API 用法。强制注释 = 强制可读性。

**何时引入**：Task 1 启用 revive；Task 2 起每个文件都用。

---

## 3.10 `_ "import"` 副作用导入

（暂未在项目用到，但常见，先备）

**一句话**：只为了执行包的 `init()` 函数而 import，不直接用包内符号。

```go
import _ "github.com/lib/pq"   // 注册 PG driver 到 database/sql 全局表
```

**何时引入**：未来某个 Task 用 database/sql 接 PG 时。
