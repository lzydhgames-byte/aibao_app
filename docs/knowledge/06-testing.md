# 测试相关知识

## 6.1 测试金字塔（单元 / 集成 / 端到端）
| 类型 | 范围 | 速度 | 例 |
|---|---|---|---|
| **单元** | 一个函数 | 毫秒级 | 测 Load() 解析 yaml |
| **集成** | 多组件协作 | 秒级 | 测 NewDB 真连上 PG |
| **端到端** | 整个系统 | 分钟级 | 注册→生成故事完整流程 |

**金字塔原则**：单元最多、集成适中、E2E 最少。  
项目目标：service+pkg ≥ 70%；gateway 做契约测试；部署前跑 smoke.sh。  
**为什么金字塔形**：单元测试快、稳、便宜——多写没关系；E2E 慢、脆（依赖网络/数据库状态）——多了拖慢 CI 还经常莫名挂掉。底层多上层少能在"测得全"和"跑得快"之间取得最佳平衡。

## 6.2 TDD（测试驱动开发）
**先写测试，再写实现**。三步循环：
1. 🔴 **Red** 写测试 → 跑 → 应该失败
2. 🟢 **Green** 写最简实现 → 跑 → 通过
3. 🔵 **Refactor** 在测试保护下整理代码

**类比**：装修前先画图纸，工人照图纸做。先随便砌墙再说"看看像不像"会重做。  
项目体现：Plan 1 每个 Task 都明确写了 "Step N.1 写失败测试 → ..."。  
**为什么需要**：先写实现容易"边写边偏离需求"——做完发现"对了哦还要测试"，往往为了凑测试改实现。TDD 反过来：测试先定义"成功长什么样"，实现只需"让测试过"——目标永远清晰。

## 6.3 `t.TempDir()` 测试隔离
```go
dir := t.TempDir()   // 临时目录，测试结束自动删
```
每个测试有独立目录，互不干扰。  
**为什么需要**：如果两个测试都往 `/tmp/foo` 写文件，跑顺序不同结果就不同——典型的 flaky test 来源。`t.TempDir` 给每个测试独立空间，测试间绝不污染。

## 6.4 `t.Setenv()` 环境变量隔离
```go
t.Setenv("KEY", "val")   // 测试结束自动还原
```
对比 `os.Setenv`：不会自动还原，测试间会污染 → flaky test。  
用 `t.Setenv` 后该测试**禁止 `t.Parallel()`**，Go 自动检测报错。  
**为什么需要**：环境变量是进程全局的——测试 A 设了不还原，测试 B 跑时就会"莫名其妙"看见这个值。`t.Setenv` 把"测试结束还原"变成自动行为。

## 6.5 testify（assert / require）
- `require.NoError(t, err)` —— 失败**立即停**当前测试
- `assert.Equal(t, want, got)` —— 失败**继续跑**

典型组合：err 检查用 require（继续跑无意义），字段断言用 assert（一次跑完看全部错误）。  
**为什么分两个**：err != nil 之后 cfg 是 nil，再跑 `cfg.Server.Port` 会 panic——必须 require 立刻停。但多个字段断言可以让所有失败一起暴露，开发者一次性修——assert 适合这种。

## 6.6 表驱动测试
把"输入→期望"做成表，循环测试：
```go
cases := map[Code]int{ CodeNotFound: 404, CodeInternal: 500 }
for c, want := range cases {
    assert.Equal(t, want, New(c, "x", "y").HTTPStatus())
}
```
加新 case 只是加一行，一目了然。  
**为什么需要**：测一个映射有 7 种枚举值——写 7 个测试函数太啰嗦且互相重复。表驱动一目了然看清楚"哪些情况都被覆盖了"，加新 case 只加一行。

## 6.7 build tags `//go:build integration`
让某些 `.go` 文件**只在带特定 tag 时被编译**。
- `go test ./...` —— 跳过集成测试
- `go test -tags=integration ./...` —— 跑集成测试

分开是因为集成测试要起 Docker 容器，慢。  
**为什么需要**：日常开发希望 `go test` 几秒跑完——但集成测试要起 PG/Redis 容器，几十秒到几分钟。用 build tag 把它们分开，单测随时跑、集成测试 CI 才跑。

## 6.8 testcontainers-go
用代码动态启容器跑测试，结束自动清理：
```go
pg, _ := postgres.Run(ctx, "postgres:16-alpine", ...)
defer pg.Terminate(ctx)
```
每个测试自己的 PG 容器，端口随机分配——绝不会"昨天的数据污染今天"。  
**为什么需要**：传统集成测试要么"假设机器上有 PG"——CI 跑挂、新人电脑跑挂；要么用 mock——测了个寂寞（mock 行为和真 PG 不一致）。testcontainers 让每个测试有独立真实 PG，环境干净且高保真。

## 6.9 测试替身：mock / fake / stub
单元测试需要"假装"外部依赖（数据库、LLM、Redis 等）——这些假实现统称**测试替身（Test Double）**，按"假到什么程度"分三种：

| 类型 | 行为 | 复杂度 | 例 |
|---|---|---|---|
| **stub** | 永远返回**固定值**，不验证怎么被调 | 最简单 | `stubBudget{allow: true}` 让 PreCheck 永远过 |
| **fake** | 用**简化但能跑**的实现（内存版） | 中等 | `fakeStoryRepo` 用 map 记 ID→Story |
| **mock** | 既假装实现，又**验证被调方式**（次数 / 参数） | 最复杂 | "断言 Generate 只被调了 1 次" |

**怎么选**：
- 只关心"返什么"，不关心"怎么被调" → **stub**（最便宜）
- 需要让测试做完整流程（写入后能读出来） → **fake**
- 关键是**调用次数/顺序/参数**本身就是要测的契约 → **mock**

**项目体现**：Plan 4 Orchestrator 测试用了三种全部——`fakeStoryRepo` 记录写入的 Story（fake），`stubBudget{allow:true}` 永远放行（stub），`mock.Calls == 0` 断言 PreCheck 拒绝时 LLM 没被调（mock）。

**类比**：拍电影里，stub=道具汽车（外形对就行），fake=能开但只能开 5 公里的轿车，mock=能开还能记录"司机踩了几次刹车"的车。

**为什么需要**：单元测试要在毫秒级跑完——不能真起 PG 或调豆包。但更深层的原因是：单测的目的是"测当前函数的逻辑"，不是"测依赖"。用替身锁死依赖行为，逻辑 bug 才能被精准定位。

**陷阱**：mock 滥用会导致"测试和实现长一样"——改实现就要改一堆 mock 断言，重构寸步难行。**优先 stub/fake，少用 mock**。

## 6.10 异步测试：require.Eventually 而不是 sleep
测试**异步流程**（Worker 处理事件、缓存过期、消息送达）时，最朴素的写法是：
```go
go w.Run(ctx)
time.Sleep(2 * time.Second)  // ❌ 等 worker 处理
assert.Equal(t, "done", reload(eventID).Status)
```
两个问题：**(1)** 太短会 flaky（CI 慢一拍就挂），**(2)** 太长拖测试速度。

正确写法用 testify 的 `require.Eventually`：
```go
require.Eventually(t, func() bool {
    var ev model.OutboxEvent
    db.First(&ev, eventID)
    return ev.Status == "done"
}, 2*time.Second, 50*time.Millisecond, "event should be done")
```
含义：**最多等 2 秒，每 50ms 重试一次**——条件成立立刻返回。机器快时几十毫秒就过，机器慢也最多等 2s。

**为什么需要**：异步系统时长不确定（网络抖动、调度延迟）。固定 sleep 是**用大 buffer 换稳定性**——浪费时间还不一定够。Eventually 是**轮询直到成立**——用最少时间换最强稳定性。

**项目体现**：Plan 4 worker_test 验证 outbox 事件被消费用的就是 Eventually 模式——预期几十毫秒就成立但 CI 慢时给 2s 容忍。

**类比**：等外卖。固定 sleep = "我每次都坐 30 分钟然后才下楼"——快了浪费时间慢了错过。Eventually = "每 5 分钟下楼看一眼，看到就拿"——既不亏又不漏。

## 6.11 Git Bash on Windows 中文编码陷阱
Git Bash 默认 locale 是 **GBK / CP936**。当用 `curl -d '{"prompt":"中文"}'` 发请求时，bash 会把命令行参数中的 UTF-8 字符串按 GBK 重编码为字节，发到服务端被当 UTF-8 解码 → 乱码。

**项目体现**：Plan 4 smoke 测试时红线词"血腥"未被拦截，根因就是这个——服务端 PreCheck 收到的根本不是"血腥"两字的 UTF-8 字节，而是 GBK 重编码后的乱码字节，匹配不到红线词库里的"血"。

**怎么绕**：用 PowerShell 的 `[System.Text.Encoding]::UTF8.GetBytes($body)` 显式拿到 UTF-8 字节，再走 `Invoke-WebRequest -Body $bytes`。

**为什么需要这条**：定位这个 bug 浪费了一小时——因为 server 端代码看着像有 bug（"红线词库不全？"），实际是 client 编码错。生产环境完全不会出现：Flutter / iOS HTTP 库永远 UTF-8 序列化 JSON，与 client locale 无关。但本地 smoke 测试是真会踩。

**类比**：寄国际快递时收件人地址写成中文又不贴翻译标签——快递员（服务端）不认得，包裹送不到。问题不在快递公司，在你寄之前没用对编码。

## 6.12 基础设施连通性预测试（infra smoke before code smoke）

接入新的外部依赖（云存储、第三方 API、第三方 SDK）时，**在写应用代码前**先用一次性脚本独立验证：云凭证、bucket / API 端点、网络连通。脚本特点：
- **脱离项目代码**——单独的 `smoke-xxx/main.go` 或 `.sh`，不 import 任何业务模块
- **裸 SDK 调用**——直接 `cos.NewClient(...)` → `client.Object.Put(...)`，不走 service / repo
- **最小可行操作**——能上传一个 1KB 文件 / 能 TTS 一句"你好"就够了

**项目体现**（Plan 5）：接 COS 和 Minimax 时分别先跑：
```
smoke-cos/main.go   —— 上传 hello.txt → 拿签名 URL → 下载验证
smoke-tts/main.go   —— 调 t2a_v2 合成"你好爱宝" → 写本地 mp3 播放
```
**提前发现的问题**：
1. COS region 配错（北京 / 广州 endpoint 不一致）
2. 子账号未绑 COS:PutObject 策略
3. **bucket 双 APPID 陷阱**（见知识库 10.11）—— 如果在应用层 smoke 发现，错误信息会被业务包装成"audio_status=failed"，根本看不到 `NoSuchBucket` 原始报错

**为什么需要这条**：应用层 smoke 失败时，错误信息往往**层层包装**——"故事生成失败" → "outbox tts_synthesis 失败" → "COS upload error" → 最里面才是 `NoSuchBucket`。如果先做过 infra smoke 确认裸 SDK 能通，应用 smoke 失败时**根本不需要怀疑基础设施**——直接看业务代码即可。定位时间至少差 3×。

**类比**：装修前先**单独**测试水电——通水通电了再做防水做地板。如果直接铺地板再发现水管漏水，要砸地板重做。基础设施 smoke 就是"先测水电"。

**操作惯例**：smoke 脚本**不入仓**（写完即删，或加 `.gitignore` 规则）。知识库这里记录用法即可。Plan 5 提交时已通过 `.gitignore` 排除 `smoke-*` 目录（见 commit 9be2c0b）。
