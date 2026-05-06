# 测试相关知识

## 6.1 测试金字塔（单元 / 集成 / 端到端）
| 类型 | 范围 | 速度 | 例 |
|---|---|---|---|
| **单元** | 一个函数 | 毫秒级 | 测 Load() 解析 yaml |
| **集成** | 多组件协作 | 秒级 | 测 NewDB 真连上 PG |
| **端到端** | 整个系统 | 分钟级 | 注册→生成故事完整流程 |

**金字塔原则**：单元最多、集成适中、E2E 最少。  
项目目标：service+pkg ≥ 70%；gateway 做契约测试；部署前跑 smoke.sh。

## 6.2 TDD（测试驱动开发）
**先写测试，再写实现**。三步循环：
1. 🔴 **Red** 写测试 → 跑 → 应该失败
2. 🟢 **Green** 写最简实现 → 跑 → 通过
3. 🔵 **Refactor** 在测试保护下整理代码

**类比**：装修前先画图纸，工人照图纸做。先随便砌墙再说"看看像不像"会重做。  
项目体现：Plan 1 每个 Task 都明确写了 "Step N.1 写失败测试 → ..."。

## 6.3 `t.TempDir()` 测试隔离
```go
dir := t.TempDir()   // 临时目录，测试结束自动删
```
每个测试有独立目录，互不干扰。

## 6.4 `t.Setenv()` 环境变量隔离
```go
t.Setenv("KEY", "val")   // 测试结束自动还原
```
对比 `os.Setenv`：不会自动还原，测试间会污染 → flaky test。  
用 `t.Setenv` 后该测试**禁止 `t.Parallel()`**，Go 自动检测报错。

## 6.5 testify（assert / require）
- `require.NoError(t, err)` —— 失败**立即停**当前测试
- `assert.Equal(t, want, got)` —— 失败**继续跑**

典型组合：err 检查用 require（继续跑无意义），字段断言用 assert（一次跑完看全部错误）。

## 6.6 表驱动测试
把"输入→期望"做成表，循环测试：
```go
cases := map[Code]int{ CodeNotFound: 404, CodeInternal: 500 }
for c, want := range cases {
    assert.Equal(t, want, New(c, "x", "y").HTTPStatus())
}
```
加新 case 只是加一行，一目了然。

## 6.7 build tags `//go:build integration`
让某些 `.go` 文件**只在带特定 tag 时被编译**。
- `go test ./...` —— 跳过集成测试
- `go test -tags=integration ./...` —— 跑集成测试

分开是因为集成测试要起 Docker 容器，慢。

## 6.8 testcontainers-go
用代码动态启容器跑测试，结束自动清理：
```go
pg, _ := postgres.Run(ctx, "postgres:16-alpine", ...)
defer pg.Terminate(ctx)
```
每个测试自己的 PG 容器，端口随机分配——绝不会"昨天的数据污染今天"。

## 6.9 mock / fake / stub
| 类型 | 行为 |
|---|---|
| **stub** | 返回固定值 |
| **fake** | 简化但能用的实现（如内存版 Redis） |
| **mock** | 验证调用方式（被调几次、参数对不对） |

类比：stub=假人模特，fake=玩具厨房，mock=监视器。  
service 单测时 mock repository 和 gateway，不真起 DB / 不真调 LLM。
