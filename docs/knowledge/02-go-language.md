# Go 语言知识

## 2.1 Go module
"一个可以独立构建的代码集合" = 一个 module。`go.mod` 是它的身份证（写明 module path 和依赖）。  
**为什么需要**：早期 Go 没有 module，所有人代码堆在一个 `$GOPATH` 下，依赖版本一片混乱。module 让每个项目独立管自己的依赖版本，避免 A 项目升级某个库影响 B 项目。

## 2.2 module path
Module 的全局唯一名字。我们的：`github.com/aibao/server`，import 时这样写：
```go
import "github.com/aibao/server/internal/pkg/logger"
```
长得像 GitHub URL 是历史习惯——**就是个字符串标识符**，不必真存在那个仓库。  
**为什么需要**：全球这么多 Go 项目，得有办法保证"叫 logger 的包到底是哪个 logger"。module path 给每个包唯一坐标。

## 2.3 `internal/` 私有包
放在 `internal/` 下的代码 **只允许同 module 自己用**，外人 import 编译器直接拒绝。  
**类比**：私人卧室——外人不能进。我们所有业务代码都在这里，等于强制封装。  
**为什么需要**：你写的代码有"对外稳定 API"和"内部实现"两类。后者你想改就改，不希望被外人依赖锁死。Go 用 `internal/` 在编译器层强制这个边界。

## 2.4 `context.Context`
贯穿一次请求生命周期的"快递单"，装 traceId、超时、取消信号。  
**类比**：快递单。物流每个环节都看它。客户取消订单，所有环节立刻停手。  
核心方法：`context.WithValue(ctx, key, val)` 放、`ctx.Value(key)` 取、`context.WithTimeout` 加超时。  
**为什么需要**：没有它的话，"这个请求是哪个用户的""超时还有多久""用户取消了没"——这些信息只能用全局变量传，多请求并发立刻乱套。context 让请求级信息**显式贯穿**整条调用链。

## 2.5 ctxKey 模式
用**未导出的空类型**做 context key，避免不同包用相同字符串 key 冲突。
```go
type ctxKey struct{}   // 每个包定义自己的，类型不同绝不撞
```
**类比**：两个"老王"分不清；"工程部老王"和"销售部老王"就分清了。  
**为什么需要**：如果 traceid 包用 `"trace_id"` 字符串、auth 包也用 `"trace_id"` 字符串，它们写进同一个 context 的值会互相覆盖。每个包的 ctxKey 类型不同，从根上不可能撞。

## 2.6 struct tag
反引号里附在字段上的元数据，给某些库当"指令"用。
```go
Port int `mapstructure:"port"`
```
和 Go 编译无关。常见：`json:` `yaml:` `mapstructure:` `gorm:` `validate:`。一个字段可挂多个，空格分隔。  
**为什么需要**：同一个结构体可能要被多个库处理（解析 yaml、写数据库、做校验）。每个库认自己的 tag，互不干扰。

## 2.7 配置三层解码
```
YAML 文件 → map[string]any → struct
        viper 读              mapstructure 填
```
**关注点分离**：换源（如 etcd）只换 viper 那层。  
**为什么需要**：把"从哪读"和"怎么填进 struct"解耦——以后如果要从 etcd / Consul / 命令行 flags 读配置，只换 viper 那层，mapstructure 那层不动。

## 2.8 错误包装：`fmt.Errorf` 与 `%w`
```go
return fmt.Errorf("read config %s: %w", path, err)
```
`%w` 把原 err 包进来，外层加上下文。调用方 `errors.Is/As` 可追溯根因。  
**类比**：包裹套盒子，最外层标"易碎品"，里面还有原始发货单。  
**为什么需要**：单纯返回 `"connection refused"` 没用——加上 `"read config /etc/aibao.yaml: connection refused"` 才知道是"读哪个文件失败"。`%w` 同时保留底层错误对象，让调用方还能用 `errors.Is(err, sql.ErrNoRows)` 之类做判断。

## 2.9 指针 vs 值返回
返回 `*Config`（指针）= 数据一份；返回 `Config`（值）= 每次拷贝。  
配置、可能被多组件持有的对象 → 用指针。极小结构体（`time.Time`）→ 用值。  
**为什么需要权衡**：值类型每次拷贝有开销但绝对安全（互不影响）；指针避免拷贝但多个持有者改同一份要小心并发。**配置这种"全局只该一份、被到处读"的对象用指针自然**。

## 2.10 `_ = something` 显式忽略
告诉 errcheck linter "我故意不用这个返回值"。要附简短注释说明为什么不关心。  
**为什么需要**：errcheck 启用后，没处理的错误会被当 bug 报错。但有些场景错误真的可以忽略（比如关闭已关闭的资源）。`_ = ...` 是"我知道有错误，故意忽略"的明确表达，比悄悄不写好得多。

## 2.11 `sync.Mutex` / `sync.RWMutex` 锁
多 goroutine 同时访问共享变量时防止竞态。  
- `sync.Mutex` —— 互斥锁，**任何时候只能一个**进
- `sync.RWMutex` —— 读写锁，**多个读并行 OK，写独占**

**类比 RWMutex**：图书馆——多人同时看一本书 OK，但有人借走（写）时其他人得等。  
**为什么需要**：Go 鼓励并发（goroutine 极便宜），但并发读写同一变量会出 race condition（值乱了甚至程序崩）。锁是最基础的保护手段。RWMutex 比 Mutex 性能好——读多写少的场景（比如全局 logger）允许并发读。  
项目体现：logger 包用 RWMutex 保护全局 default logger 引用——启动时写一次，运行中所有 goroutine 高并发读。
