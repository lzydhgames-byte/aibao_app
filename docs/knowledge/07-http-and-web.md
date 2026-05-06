# HTTP 与 Web 相关

## 7.1 HTTP 中间件（middleware）链
拦在"请求到 handler"前的一串处理函数。每个中间件做一件事，按注册顺序串成链：
```
请求 → [recover] → [traceid] → [logger] → [metrics] → handler
响应 ← [recover] ← [traceid] ← [logger] ← [metrics] ← 处理完毕
```
**为什么需要**：每个 handler 都要"打日志、量耗时、catch panic"——如果每个 handler 里都写一遍是地狱。中间件让这些通用逻辑统一抽出，注册一次全部享有。

## 7.2 Gin 中间件标准结构
```go
func Xxx() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 请求来时做的事
        c.Next()           // 把控制权交给链上下一个
        // 响应回来时做的事
    }
}
```
**为什么需要 `c.Next()`**：中间件经常需要"在请求和响应阶段都做事"。`c.Next()` 把"洋葱"剖开——前面是请求阶段，后面是响应阶段。

## 7.3 洋葱模型
中间件嵌套执行像剥洋葱：
```
A.before → B.before → C.before → handler → C.after → B.after → A.after
```
最外层中间件最早执行 before、最晚执行 after。  
**为什么需要**：监控类中间件（logger/metrics）需要在最外层——这样能测到全程耗时（包括内层中间件耗时）；安全类中间件（auth）需要在中间——拦截前先有 traceid 但要在业务前生效。

## 7.4 panic 恢复中间件
Go 的 panic 像心脏骤停——不 recover 整个进程崩。recover 中间件用 `defer + recover()` 兜底：
```go
defer func() {
    if rec := recover(); rec != nil {
        // 记日志 + 返 500
    }
}()
```
**为什么需要**：单个请求 panic（比如某 handler 有 bug）不该让整个服务挂——其他用户还在用。recover 把 panic 变成"这一个请求返 500"，进程继续。

## 7.5 `X-Trace-Id` Header 透传
入口生成 traceId 后**回写到响应 header**——客户端拿到下次请求可以带上同一个 ID，实现端到端串联。  
**为什么需要**：用户 App 报错时贴出一段日志，里面有 traceId——后端 grep 这个 ID 能立刻定位到具体那次请求。这是"用户报错支持流程"的关键基础。

## 7.6 RESTful API 与状态码
HTTP 状态码语义化分类：
| 类 | 含义 | 例 |
|---|---|---|
| 2xx | 成功 | 200 OK / 201 Created / 204 No Content |
| 4xx | 客户端错（你传错了） | 400 / 401 / 403 / 404 / 429 |
| 5xx | 服务器错（我们的锅） | 500 / 503 |

**为什么需要标准**：客户端拿到响应不需要解析 body 也能粗略判断"该重试还是放弃还是用户输入有误"。

## 7.7 Gin 框架基础
```go
r := gin.New()                    // 不带默认中间件（我们自己装）
r.Use(Recover(), TraceID(), ...)  // 注册中间件
r.GET("/health", handlerFunc)     // 注册路由
r.Run(":8080")                    // 启动 HTTP 服务
```
**为什么用 Gin**：Go 标准库 `net/http` 写 RESTful 偏底层（路由参数解析、中间件机制都要自己写）；Gin 提供主流的"路由 + 中间件 + 参数绑定"模式，社区最大、文档最全。

## 7.8 `gin.WrapH` —— 把 net/http handler 接入 Gin
```go
r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(reg, ...)))
```
prometheus client 提供的是标准 `http.Handler`，Gin 用 `WrapH` 适配。  
**为什么需要**：Go 生态里很多库（包括 promhttp）只实现 `http.Handler` 接口。`WrapH` 让这些标准 handler 也能挂在 Gin 路由上。

## 7.9 请求超时与 `ReadHeaderTimeout`
```go
srv := &http.Server{
    Addr: ":8080",
    Handler: router,
    ReadHeaderTimeout: 10 * time.Second,
}
```
**为什么需要**：恶意客户端可能"开了连接就不发数据"——耗光服务器连接数。`ReadHeaderTimeout` 给出"X 秒内必须发完 HTTP header"，否则断开。这是 Slowloris 攻击的基础防御。
