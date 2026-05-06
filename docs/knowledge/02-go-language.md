# Go 语言知识

## 2.1 Go module
"一个可以独立构建的代码集合" = 一个 module。`go.mod` 是它的身份证（写明 module path 和依赖）。

## 2.2 module path
Module 的全局唯一名字。我们的：`github.com/aibao/server`，import 时这样写：
```go
import "github.com/aibao/server/internal/pkg/logger"
```
长得像 GitHub URL 是历史习惯——**就是个字符串标识符**，不必真存在那个仓库。

## 2.3 `internal/` 私有包
放在 `internal/` 下的代码 **只允许同 module 自己用**，外人 import 编译器直接拒绝。  
**类比**：私人卧室——外人不能进。我们所有业务代码都在这里，等于强制封装。

## 2.4 `context.Context`
贯穿一次请求生命周期的"快递单"，装 traceId、超时、取消信号。  
**类比**：快递单。物流每个环节都看它。客户取消订单，所有环节立刻停手。  
核心方法：`context.WithValue(ctx, key, val)` 放、`ctx.Value(key)` 取、`context.WithTimeout` 加超时。

## 2.5 ctxKey 模式
用**未导出的空类型**做 context key，避免不同包用相同字符串 key 冲突。
```go
type ctxKey struct{}   // 每个包定义自己的，类型不同绝不撞
```
**类比**：两个"老王"分不清；"工程部老王"和"销售部老王"就分清了。

## 2.6 struct tag
反引号里附在字段上的元数据，给某些库当"指令"用。
```go
Port int `mapstructure:"port"`
```
和 Go 编译无关。常见：`json:` `yaml:` `mapstructure:` `gorm:` `validate:`。一个字段可挂多个，空格分隔。

## 2.7 配置三层解码
```
YAML 文件 → map[string]any → struct
        viper 读              mapstructure 填
```
**关注点分离**：换源（如 etcd）只换 viper 那层。

## 2.8 错误包装：`fmt.Errorf` 与 `%w`
```go
return fmt.Errorf("read config %s: %w", path, err)
```
`%w` 把原 err 包进来，外层加上下文。调用方 `errors.Is/As` 可追溯根因。  
**类比**：包裹套盒子，最外层标"易碎品"，里面还有原始发货单。

## 2.9 指针 vs 值返回
返回 `*Config`（指针）= 数据一份；返回 `Config`（值）= 每次拷贝。  
配置、可能被多组件持有的对象 → 用指针。极小结构体（`time.Time`）→ 用值。

## 2.10 `_ = something` 显式忽略
告诉 errcheck linter "我故意不用这个返回值"。要附简短注释说明为什么不关心。
