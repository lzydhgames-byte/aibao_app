# Go 语言知识

按"语言基础 → 标准库 → 工程惯例"顺序。

---

## 2.1 Go module

**一句话**：Go 把"一个可以独立构建的代码集合"叫一个 module，简单说就是一个项目。

`go.mod` 文件是 module 的"身份证"：
- 第一行 `module xxx` —— module path
- 后面列出依赖了哪些第三方包

**何时引入**：Task 1 `go mod init`。

---

## 2.2 module path

**一句话**：这个 module 的全局唯一名字。其他代码 import 时用这个名字。

我们项目的 module path 是 `github.com/aibao/server`，所以内部 import 这样写：
```go
import "github.com/aibao/server/internal/pkg/logger"
```

**为什么长得像 GitHub URL**：历史习惯。Go 设计时假设"代码托管在网上，下载时直接从 URL 拉"。

**实际上**：
- 它就是个**字符串标识符**，Go 不会真的去访问那个 URL（除非别人 import 你的项目）
- 我们项目自己用，所以这个路径"是不是真实的 GitHub 地址"都无所谓
- 用 `github.com/aibao/server` 只是为了"将来想推 GitHub 时不用全局改"

**生活类比**：给孩子上户口本，户口本上的名字就是个标识符。不管别人能不能找到你家，名字本身就是合法标识。

**何时引入**：Task 1。

---

## 2.3 `internal/` 目录（私有包）

**一句话**：放在 `internal/` 下的代码**只允许同 module 自己用**，外人 import 会被 Go 编译器直接拒绝。

**为什么需要**：你写的代码可能有"对外公开的 API"和"内部实现细节"两类。后者不希望被别人随意依赖（你想改实现就改了，不应该顾及外部用户）。

**Go 的实现**：纯靠目录名约定。`internal/` 是 Go 编译器写死的关键字——任何路径里包含 `internal` 段的包，import 时会被检查包路径是否在 `internal` 的"姐姐目录"之内。不在就报错。

**我们项目**：所有真实业务代码都在 `internal/`。意味着即使将来别人 import `github.com/aibao/server`，也拿不到我们的内部包。这是"封装"的体现。

**何时引入**：Task 1 创建目录骨架。

---

## 2.4 `context.Context`（请求级上下文）

**一句话**：贯穿一次请求生命周期的"快递单"，可以装 traceId、超时、取消信号等。

**生活类比**：你下单买东西，快递单上有订单号、寄件人、收件人、超时时间。物流公司每个环节（仓库、卡车、配送员）都看这张单。如果你打电话取消订单，所有环节立刻知道并停手。

**核心方法**：
- `context.Background()` —— 创建空 context（一切之始）
- `context.WithValue(ctx, key, val)` —— 装一个键值进去（如 traceId）
- `ctx.Value(key)` —— 取出值
- `context.WithTimeout(ctx, 30*time.Second)` —— 加超时
- `ctx.Done()` —— 一个 channel，超时或取消时会关闭，下游可以监听并放弃工作

**为什么这么重要**：
- 没有它，全局变量满天飞，跨 goroutine 难以传递"请求级"信息
- 有了它，任何深层函数都能拿到请求 ID、用户 ID、超时设置——只要把 ctx 传下去
- 取消 / 超时是统一的——上游说停，整条链路都停

**何时引入**：Task 3 traceid 包用 context 存 trace id。

---

## 2.5 ctxKey 模式（避免 context key 冲突）

**一句话**：用一个**未导出的空类型**作为 context key，保证不同包不会因为用了同样的 key 字符串而互相覆盖。

```go
package traceid

type ctxKey struct{}        // 未导出 + 空类型

func WithID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, ctxKey{}, id)
}
```

**为什么不用字符串**：
- 如果 traceid 包用 `"trace_id"`、auth 包也用 `"trace_id"`，它们写进 context 的值会互相覆盖
- 用类型作为 key，每个包的 `type ctxKey struct{}` 是**完全不同的类型**——不会冲突

**生活类比**：两人都叫"老王"，没法分清；但"工程部老王"和"销售部老王"就分清了。"工程部"对应的就是 `internal/traceid` 这个包路径。

**何时引入**：Task 3。

---

## 2.6 struct tag（结构体标签）

**一句话**：附加在结构体字段上的元数据，反引号包起来，运行时用反射读取。

```go
type ServerConfig struct {
    Port int `mapstructure:"port"`
}
```

反引号里的 `mapstructure:"port"` 就是 struct tag。**它和 Go 编译无关**——只是给某些库用的"指令"。

**常见 tag**：
| tag | 谁用 | 用途 |
|---|---|---|
| `json:"foo"` | encoding/json | JSON 编解码时字段叫 `foo` |
| `yaml:"foo"` | yaml 库 | YAML 编解码 |
| `mapstructure:"foo"` | mapstructure | map → struct 的字段映射 |
| `gorm:"column:foo"` | GORM | 数据库列名 |
| `validate:"required"` | validator | 校验规则 |

**一个字段可以挂多个 tag**，空格分隔：
```go
Port int `mapstructure:"port" validate:"min=1,max=65535"`
```

**何时引入**：Task 2 config 结构体。

---

## 2.7 三层"配置解码"链路

**一句话**：YAML 文件 → `map[string]any` → struct，每层各司其职。

```
config.dev.yaml (硬盘)          ← 文本
    ↓ viper 读 + 解析
map[string]any (内存)          ← 通用键值结构
    ↓ mapstructure 解码（看 `mapstructure:"xxx"` 标签）
Config 结构体 (内存)           ← 强类型
```

**这种分层的意义**：viper 处理"从哪读"（文件/环境变量/远程配置中心），mapstructure 处理"怎么填进结构体"。**关注点分离**——换源（比如改用 etcd）只换 viper 那层。

**何时引入**：Task 2。

---

## 2.8 错误处理：`fmt.Errorf` 与 `%w`

**一句话**：Go 的错误是值，可以一层层包装；`%w` 让外层错误能"追溯"到根因。

```go
if err := v.ReadInConfig(); err != nil {
    return nil, fmt.Errorf("read config %s: %w", path, err)
}
```

- `%w`（**w**rap）：把原始 err 包进新错误里
- 调用方可以用 `errors.Is(err, target)` 判断根因
- 也可以用 `errors.As(err, &myType)` 解出特定类型

**生活类比**：快递包裹被一层层套盒子。最外层标着"易碎品"（外层 message），打开后里面还有一张"原始发货单"（被 wrap 的 err）。用 `errors.Is` 就是"打开所有盒子看根源"。

**为什么要 wrap 而不是只返回原 err**：调用栈深时，单纯的 `"connection refused"` 没用——加上 `"read config /etc/aibao.yaml: connection refused"` 就一目了然。

**何时引入**：Task 2 config 包错误处理。

---

## 2.9 指针 vs 值接收者 / 指针 vs 值返回

**一句话**：返回 `*Config`（指针）让数据只有一份；返回 `Config`（值）每次拷贝一份。

```go
func Load(path string) (*Config, error)   // 我们的选择
```

**为什么用指针**：
- 配置可能被多个组件持有，传递时不希望每次都复制（浪费内存）
- 将来想加方法（如 `cfg.Validate()`），用指针接收者更灵活
- Go 惯例：稍大的结构体或预期被修改的结构体，都用指针

**值返回更适合**：
- 极小的结构体（如 `time.Time`）
- 你**希望**调用者拿到独立副本不影响原物的场景

**何时引入**：Task 2 `Load` 返回 `*Config`。

---

## 2.10 `_ = something` 显式忽略

**一句话**：声明"我故意不用这个返回值，别警告我"。

```go
_ = v.BindEnv(key)   // BindEnv 只在 key 为空时报错，这里 key 非空所以忽略
```

Go 启用了 errcheck lint 后，**忽略错误返回值会报错**。`_ = ...` 是显式声明"我知道有错误，但不关心"。要附**简短注释**说明为什么不关心。

**何时引入**：Task 2 config 包 BindEnv 调用。
