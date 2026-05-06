# 测试相关知识

---

## 6.1 单元测试 vs 集成测试 vs 端到端测试

**一句话**：测试有不同粒度，从最小到最大依次是单元 → 集成 → 端到端。

| 类型 | 范围 | 速度 | 例子 |
|---|---|---|---|
| **单元测试**（unit） | 一个函数 / 一个类 | 极快（毫秒级） | 测 `Load(path)` 解析 yaml 是否正确 |
| **集成测试**（integration） | 多个组件协作 | 中等（秒级） | 测 `repository.NewDB` 能否真正连上 PG |
| **端到端测试**（E2E） | 整个系统 | 慢（分钟级） | 模拟用户注册 → 创建孩子 → 生成故事的完整流程 |

**金字塔原则**：单元测试要多（基础牢固）、集成测试适中、端到端测试少（贵且不稳）。

**生活类比**：
- 单元测试 = 检查每颗螺丝拧紧没
- 集成测试 = 检查门把手装上能转动
- 端到端测试 = 检查整辆车从启动到开走是否正常

**我们项目的体现**：
- service 层 + pkg 层主要写单元测试，覆盖率目标 ≥ 70%
- gateway 层做契约式集成测试（确保接口契约不变）
- 部署前用 `scripts/smoke.sh` 跑端到端冒烟

**何时引入**：技术架构第 11 章；Task 2 起每个 Task 都先写单元测试。

---

## 6.2 TDD（测试驱动开发）

**一句话**：**先写测试，再写实现**。测试是"规范的可执行版"。

**TDD 循环**（Red-Green-Refactor）：
1. 🔴 **Red**：写一个测试，描述你希望代码怎么表现 → 跑它 → 它**应该失败**（因为代码还不存在）
2. 🟢 **Green**：写最简单的实现让测试通过 → 跑它 → 通过
3. 🔵 **Refactor**：在测试保护下，整理代码（重命名、提取函数、消除重复），保证测试持续通过

**为什么这么做**：
- 文档里写"`Load` 应该读 yaml 并允许 env 覆盖"是不可机器验证的
- 测试代码 `assert.Equal(t, 9090, cfg.Server.Port)` 是机器可验证的
- 强制你**先想清楚要什么再动手实现**，避免越写越发散

**生活类比**：装修房子前先把"我希望住进去什么样"画成图纸，工人照图纸做。如果先随便砌墙再说"看看像不像我想要的"，多半要砸了重做。

**我们项目的体现**：Plan 1 每个 Task 都明确写了"Step N.1 写失败测试" → "Step N.2 跑测试确认 FAIL" → "Step N.3 实现" → "Step N.4 跑测试确认 PASS"。

**何时引入**：Task 2 第一次真实代码就严格走 TDD。

---

## 6.3 `t.TempDir()` 测试隔离

**一句话**：Go testing 包提供的"临时目录"，测试结束自动清理。

```go
func TestLoad_FromFile(t *testing.T) {
    dir := t.TempDir()                       // 获得一个临时目录
    path := filepath.Join(dir, "config.yaml")
    os.WriteFile(path, []byte("..."), 0o600) // 在里面创建测试用文件
    // 测试结束 → 整个目录自动删除
}
```

**为什么需要**：每个测试有自己独立的目录，互不干扰。**测试隔离**是高质量测试的基石——任何一个测试都不该依赖另一个测试留下的状态。

**何时引入**：Task 2 config 测试。

---

## 6.4 `t.Setenv()` 环境变量隔离

**一句话**：Go testing 包提供的"安全设置环境变量"——测试结束自动恢复原值。

```go
t.Setenv("AIBAO_POSTGRES_PASSWORD", "secret")
// 测试结束 → 自动恢复成测试前的值（或删除）
```

**对比 `os.Setenv`**：直接 `os.Setenv` 设了不会自动还原 → 后续测试也会"看见"这个值 → 测试间相互污染 → flaky test（一会儿过一会儿不过）。

**额外保护**：用 `t.Setenv` 后该测试**禁止 `t.Parallel()`**——Go 自动检测并报错。因为环境变量是进程全局，并行跑会乱套。

**何时引入**：Task 2 config 测试 env 覆盖。

---

## 6.5 testify（assert / require）

**一句话**：Go 流行的测试断言库，让"期待是什么 vs 实际是什么"一目了然。

**核心两个子包**：
- `assert.Equal(t, expected, actual)` —— 检查；不等也**继续往下跑**
- `require.NoError(t, err)` —— 检查；不通过**立即停止**当前测试

**何时用 require**：失败后继续跑没意义的场景（如 err != nil 后 cfg 是 nil，往下还会 panic）。

**何时用 assert**：失败也想看后面其他断言结果的场景（一次跑出全部错误，便于一次性修复）。

**典型组合**：
```go
cfg, err := Load(path)
require.NoError(t, err)            // 失败立刻停（继续跑没意义）
assert.Equal(t, 8080, cfg.Server.Port)  // 失败继续，可能还想看下面字段
assert.Equal(t, "info", cfg.Server.LogLevel)
```

**何时引入**：Task 2 config 测试。

---

## 6.6 表驱动测试（table-driven test）

**一句话**：把一组"输入→期望输出"做成表格，循环测试每一行——避免重复代码。

```go
func TestHTTPStatus_AllCodes(t *testing.T) {
    cases := map[Code]int{
        CodeInvalidArgument: 400,
        CodeUnauthenticated: 401,
        CodeNotFound:        404,
        CodeInternal:        500,
    }
    for c, want := range cases {
        got := New(c, "x", "y").HTTPStatus()
        assert.Equal(t, want, got, "code=%v", c)
    }
}
```

**为什么好**：
- 加新 case 只是加一行
- 一目了然看清"哪些情况都被测了"
- 失败时附带的 `code=%v` 让你立刻知道是哪一行挂的

**何时引入**：Task 7 errors 包测试将用到。

---

## 6.7 build tags（构建标签）—— `//go:build integration`

**一句话**：让某些 `.go` 文件**只在某种构建条件下被编译**——常用来分离"快速单测"和"慢速集成测"。

```go
//go:build integration

package repository

func TestNewDB_Connects(t *testing.T) { ... }
```

**用法**：
- 默认 `go test ./...` —— **不**编译这文件，跳过集成测试
- `go test -tags=integration ./...` —— 编译这文件，跑集成测试

**为什么需要**：集成测试要起 Docker 容器（PG / Redis），耗时几秒到几十秒。日常开发只想跑单测要快——分开就好。

**Plan 1 的体现**：
- `make test` → 只跑单测
- `make test-integration` → 跑集成测试（需要 Docker）

**何时引入**：Task 13 DB 集成测试。

---

## 6.8 testcontainers-go

**一句话**：Go 库，用代码动态启动 Docker 容器跑测试，结束后自动清理。

```go
pg, err := postgres.Run(ctx, "postgres:16-alpine",
    postgres.WithDatabase("aibao"),
    postgres.WithUsername("aibao"),
    postgres.WithPassword("aibao"),
)
// 测试结束 → defer pg.Terminate(...) → 容器删除
```

**对比传统做法**：
- ❌ 测试假设"机器上已有 PG 在 5432"——CI 跑挂、新人电脑跑挂
- ✅ testcontainers：每个测试自己起一个 PG 容器，端口随机分配，跑完自删

**好处**：每次都是干净环境，绝不会"昨天的测试数据污染今天"。

**何时引入**：Task 13 DB 集成测试。

---

## 6.9 mock vs fake vs stub

**一句话**：替换真实依赖的"假货"，让单元测试不需要真依赖。

| 类型 | 行为 | 用例 |
|---|---|---|
| **stub** | 返回固定值 | "调 LLM 的话总返回这段文字" |
| **fake** | 简化但能用的实现 | "用内存 map 假装 Redis" |
| **mock** | 验证调用方式（被调几次、参数对不对） | "确认确实调了 storage.Upload 一次" |

**生活类比**：
- stub = 假人模特（永远摆同一个姿势）
- fake = 玩具厨房（能玩但煮不了真菜）
- mock = 监视器（看你是否按规范操作）

**我们项目的体现**：service 层单测时 mock repository 和 gateway——不真起 DB / 不真调 LLM。

**何时引入**：Plan 4 故事服务单测时大量使用。

---

## 6.10 测试覆盖率（再次）

详见 [03.8 测试覆盖率](03-go-engineering.md#38-测试覆盖率)。

**核心要点**：
- 不追求 100%
- 关键路径必须 100%（业务核心、安全相关、错误处理）
- 简单 getter / setter 不必强求
