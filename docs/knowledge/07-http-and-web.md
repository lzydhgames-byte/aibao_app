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
HTTP 状态码语义化分类（项目里实际用到的）：
| 码 | 含义 | 项目里何时用 |
|---|---|---|
| **200 OK** | 成功（已有结果返回） | GET / PATCH 成功 |
| **201 Created** | 成功创建了资源 | POST 创建孩子档案成功 |
| **204 No Content** | 成功但无返回体 | DELETE 成功（暂未用到） |
| **400 Bad Request** | 客户端传参错了 | JSON 格式错 / 手机号格式错 / 生日格式错 |
| **401 Unauthorized** | 没认证 / 认证失败 | 没带 token / token 过期 / 验证码错 |
| **403 Forbidden** | 已认证但没权限 | 用户 A 想改用户 B 的孩子（not_owner） |
| **404 Not Found** | 资源不存在 | child_not_found / 路由不存在 |
| **409 Conflict** | 资源冲突 | 一期单孩子约束：第二次 POST /children |
| **429 Too Many Requests** | 频率超限 | sms.send 60s 内重发 |
| **500 Internal Server Error** | 服务器崩了 | panic / 未预期错误 |
| **503 Service Unavailable** | 暂时不可用 | 预算熔断 / 依赖服务挂掉 |

**为什么需要标准化**：客户端拿到响应不需要解析 body 也能粗略判断"该重试还是放弃还是用户输入有误"。比如 4xx 重试无意义（参数得改），5xx 可以重试（可能瞬时故障）。  
项目体现：`apperr.AppError.HTTPStatus()` 把业务错误码（CodeNotFound 等）自动映射到 HTTP 状态。

## 7.7 Bearer Authentication
HTTP 标准的"携带令牌"格式：
```
Authorization: Bearer <token>
```
`Bearer`（持有人）的意思是"谁拿到这个 token 谁就是持有人"——**没有额外的身份证明**。所以 token 必须像现金一样保管。  
**类比**：演唱会票根——撕了一半给你，谁拿着谁能进，丢了就丢了。  
**为什么需要标准 scheme**：HTTP `Authorization` header 还有 `Basic`（用户名密码 base64）、`Digest`（旧的密码摘要）、`Bearer`（令牌）等多种 scheme，写明 `Bearer` 让 server 知道怎么解析后面的字符串。  
项目体现：`middleware.JWTAuth` 检查 `strings.HasPrefix(auth, "Bearer ")`，否则 401。

## 7.8 CRUD 与 HTTP 动词（特别讲 PATCH vs PUT）
REST 资源操作映射：
| 动词 | CRUD | 语义 | 项目里 |
|---|---|---|---|
| **POST** | Create | 新建（id 由服务端生成） | POST /children |
| **GET** | Read | 查询 | GET /children, GET /me |
| **PUT** | Replace | **整体替换**整个资源 | 暂未用 |
| **PATCH** | Update | **部分修改**资源字段 | PATCH /children/:id |
| **DELETE** | Delete | 删除 | 暂未用 |

**PATCH vs PUT 关键区别**：
- **PUT** 期望客户端发送**完整资源**——服务端整体替换。少传一个字段 = 那字段被清空
- **PATCH** 期望客户端只发**要改的字段**——服务端只动这些，其他保留

**类比**：PUT = 重写整本书；PATCH = 在书的某几页贴便签。  
**为什么 PATCH 更适合"修改部分字段"**：客户端不必先 GET 完整资源、再修改、再 PUT 回去，直接 PATCH 一个字段更省网络且并发安全。  
项目体现：PATCH /children/:id 的请求体所有字段都用 `*string` 指针类型，`nil` 表示"这个字段不改"。详见 [02.12 指针字段做 PATCH 部分更新](02-go-language.md#212-指针字段做-patch-部分更新)。

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
