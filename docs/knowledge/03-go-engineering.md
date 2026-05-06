# Go 工程实践

## 3.1 标准项目布局
```
server/
├── cmd/server/main.go    程序入口
├── internal/             业务代码（外部不能 import）
│   ├── api/              HTTP handler
│   ├── service/          业务逻辑
│   ├── repository/       数据访问
│   ├── gateway/          外部依赖抽象
│   └── pkg/              内部共用工具
├── migrations/           数据库迁移 SQL
├── config/               配置
└── go.mod
```
**类比**：cmd 是大门、internal 是私人卧室、internal/pkg 是公共工具间。  
**为什么需要约定**：任何 Go 工程师打开陌生项目，看到这个结构 30 秒内就能定位"主入口在哪、业务逻辑在哪、外部依赖怎么抽象的"。如果每个项目都自创布局，团队上手成本爆炸。

## 3.2 三层架构（api / service / repository）
```
HTTP → api（翻译参数）→ service（业务规则）→ repository（数据 CRUD）→ DB
```
**纪律**：
- api 层**不写业务规则**，只翻译
- service 层**不依赖 HTTP 也不依赖具体 DB**——单测时 mock 下层即可
- repository 层**只 CRUD，不写业务判断**

**为什么需要**：业务逻辑独立于 HTTP 和数据库——单元测试不需要起服务器和数据库；将来要加 CLI 工具或 cron 任务复用业务，直接调 service 不用绕 HTTP；换数据库只改 repository 层。**纠缠在一起的代码改一处带来连锁问题**，分层就是为了切断这种连锁。

## 3.3 `go.mod` / `go.sum`
- `go.mod` —— "我依赖谁"
- `go.sum` —— "依赖的精确哈希"，防止依赖被偷偷篡改

**两个文件都必须 commit**。  
**为什么需要 sum**：你 `go.mod` 写了 `viper v1.19.0`，但 v1.19.0 的源码内容是什么？如果有人把镜像服务器上的 v1.19.0 偷换成带后门的版本怎么办？`go.sum` 记录了你**第一次下载时的哈希**，之后每次构建都对比——发现哈希不匹配立刻报错。

## 3.4 `go mod tidy`
清理 go.mod / go.sum，只保留实际用到的依赖。加新 import 后跑一次。  
**为什么需要**：手动维护 go.mod 容易忘——加了 import 没写依赖、删了 import 没清理。`tidy` 自动同步两者。

## 3.5 `go install` 与 `-tags`
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```
拉源码 → 编译 → 二进制丢进 `$GOPATH/bin`（已在 PATH 里所以直接能用）。  
`-tags 'postgres'` = 条件编译——只编译标记了 postgres 的代码部分，减小体积。  
**为什么需要 tags**：migrate 工具支持 MySQL/SQLite/MongoDB/PG 等十几种数据库，全编进去几十 MB；`-tags 'postgres'` 告诉编译器"只要 PG 那部分"，二进制小很多。

## 3.6 Makefile
把团队要敲的命令封装成统一短指令：`make test` = `go test -race -count=1 ./...`。  
**铁律：必须 TAB 缩进**——1976 年定的设计错误，已经不能改。  
**为什么需要**：团队 5 个人 5 种习惯敲命令——"我习惯加 -v"、"我忘了加 -race"——bug 时间不一致；`make test` 强制统一。CI/CD 流水线也直接 `make test`，发版前测试和本地完全一致。

## 3.7 Lint 与 golangci-lint
"编译能过"之外的另一道质量防线。能查出：
- 变量没用 / 错误没处理 / 命名不规范 / 格式不统一 / 错别字

`golangci-lint` 把 30+ 个 linter 集成到一个工具，配置文件 `.golangci.yml` 选择启用哪些。  
**v2 配置**和 v1 不兼容（schema 改了），我们用 v2。  
**为什么需要**：编译器只管"语法对不对、类型对不对"。但代码里很多坏味道编译能过却埋雷——比如错误返回值忘了处理（最经典的 Go bug 来源）。lint 是低成本高收益——配置一次自动持续帮你拦截。

## 3.8 测试覆盖率
`go test -cover ./...` 输出"测试碰到了多少行代码"的比例。  
不是越高越好——80% 是合理目标。**关键路径必须覆盖**（业务核心、安全、错误处理）。  
Plan 1 标准：service+pkg ≥ 70%。  
**为什么需要 + 为什么不追求 100%**：覆盖率是"测试到位的指标"，但不是终极目标。强追 100% 会鼓励写无意义测试（测 getter / setter）反而稀释关键测试的关注度。

## 3.9 godoc 注释
写在每个**导出**（首字母大写）符号上方的注释。**必须以符号名开头**：
```go
// New returns a new trace id ...
func New() string { ... }
```
linter `revive` 强制要求每个导出符号都有。  
**为什么必须以符号名开头**：godoc 工具会自动把这些注释生成 API 文档；"以符号名开头"是 godoc 的解析规则。IDE 鼠标悬停看到的也是这个注释——强制写让代码自带文档。

## 3.10 `_ "import"` 副作用导入
```go
import _ "github.com/lib/pq"   // 仅为了执行 init() 注册 driver
```
未来连 PG 时会用。  
**为什么需要**：有些库的 `init()` 函数有副作用（比如 PG driver 注册到 database/sql 全局表），但你代码里不直接用它的符号——`_` 表示"我只要副作用，不用包内任何东西"。
