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

## 3.2 三层架构（api / service / repository）
```
HTTP → api（翻译参数）→ service（业务规则）→ repository（数据 CRUD）→ DB
```
**纪律**：
- api 层**不写业务规则**，只翻译
- service 层**不依赖 HTTP 也不依赖具体 DB**——单测时 mock 下层即可
- repository 层**只 CRUD，不写业务判断**

好处：service 的单元测试不需要起服务器和数据库。

## 3.3 `go.mod` / `go.sum`
- `go.mod` —— "我依赖谁"
- `go.sum` —— "依赖的精确哈希"，防止依赖被偷偷篡改

**两个文件都必须 commit**。

## 3.4 `go mod tidy`
清理 go.mod / go.sum，只保留实际用到的依赖。加新 import 后跑一次。

## 3.5 `go install` 与 `-tags`
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```
拉源码 → 编译 → 二进制丢进 `$GOPATH/bin`（已在 PATH 里所以直接能用）。  
`-tags 'postgres'` = 条件编译——只编译标记了 postgres 的代码部分，减小体积。

## 3.6 Makefile
把团队要敲的命令封装成统一短指令：`make test` = `go test -race -count=1 ./...`。  
**铁律：必须 TAB 缩进**——1976 年定的设计错误，已经不能改。

## 3.7 Lint 与 golangci-lint
"编译能过"之外的另一道质量防线。能查出：
- 变量没用 / 错误没处理 / 命名不规范 / 格式不统一 / 错别字

`golangci-lint` 把 30+ 个 linter 集成到一个工具，配置文件 `.golangci.yml` 选择启用哪些。  
**v2 配置**和 v1 不兼容（schema 改了），我们用 v2。

## 3.8 测试覆盖率
`go test -cover ./...` 输出"测试碰到了多少行代码"的比例。  
不是越高越好——80% 是合理目标。**关键路径必须覆盖**（业务核心、安全、错误处理）。  
Plan 1 标准：service+pkg ≥ 70%。

## 3.9 godoc 注释
写在每个**导出**（首字母大写）符号上方的注释。**必须以符号名开头**：
```go
// New returns a new trace id ...
func New() string { ... }
```
linter `revive` 强制要求每个导出符号都有。

## 3.10 `_ "import"` 副作用导入
```go
import _ "github.com/lib/pq"   // 仅为了执行 init() 注册 driver
```
未来连 PG 时会用。
