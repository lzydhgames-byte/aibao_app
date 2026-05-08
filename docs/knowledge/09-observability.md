# 可观测性（Observability）

## 9.1 TraceID（贯穿全链路的请求 ID）
每个 HTTP 请求一个唯一 ID，从入口贯穿所有日志，便于排查"这个用户那次失败到底走到了哪一步"。  
项目体现：traceid 包通过 context 传递 + Logger 中间件自动注入每条日志的 `trace_id` 字段。  
**为什么需要**：一个请求会触发好几条日志（参数校验、调 LLM、调 TTS、写库……）。没 traceId 时这些日志混在一起没法关联——用户报错你只能瞎找。有了 traceId，`grep tr-abc123 app.log` 一秒还原整条调用链。

## 9.2 结构化日志（structured logging）
**传统文本日志**：`[INFO] user 42 generated story 5821 in 12.3s` —— 机器要正则解析。  
**结构化日志（JSON）**：每条一行 JSON，键值清晰：
```json
{"time":"...","level":"INFO","msg":"story.generated","user_id":42,"duration_ms":12300}
```
**类比**：手写订单 vs 标准表单。后者机器能直接录入。  
SLO 监控、告警、ELK/Loki 检索全靠这种格式。  
**为什么需要**：传统文本日志不同人写法不一致，机器要正则才能提取字段——脆弱低效。JSON 格式让机器直接 `jq '.user_id'`、ELK/Loki 直接索引——监控告警都建立在结构化基础上。

## 9.3 `log/slog`
Go 1.21+ 标准库自带的结构化日志。  
新项目直接用，不需要再引入 zap / zerolog。  
**为什么用 slog 不用 zap**：之前社区有 zap、zerolog、logrus 等流派，每个项目选不同的会让代码迁移困难。Go 标准库 1.21 引入 slog 后，社区有了**统一标准**——新项目直接用，向前兼容性最好。

## 9.4 日志级别（DEBUG / INFO / WARN / ERROR）
| 级别 | 用途 |
|---|---|
| DEBUG | 细节调试信息（生产环境通常关闭） |
| INFO | 关键业务节点（启动、请求处理完成） |
| WARN | 可恢复异常 |
| ERROR | 业务失败（需要关注） |

项目策略：debug 阶段全开，上线后只保留 INFO+。  
**为什么分级**：DEBUG 全开生产环境会写满磁盘且关键信息淹没在噪音里；只开 ERROR 又错过早期信号。分级让你按场景调"信噪比"——开发时全开看细节，生产保留关键节点。

## 9.5 日志切割（lumberjack）
日志会无限增长。`lumberjack` 自动：
- 单文件超过 100MB → 拍快照新建文件
- 保留 14 天，老文件压缩
- 太老的自动删除

防止撑爆磁盘。  
**为什么需要**：服务跑久了日志会无限大，撑爆磁盘 = 服务挂掉。手动删日志容易忘还可能误删。lumberjack 自动滚动 + 自动清理，一劳永逸。

## 9.6 `io.MultiWriter`
Go 技巧：一份内容同时写多个目的地。
```go
mw := io.MultiWriter(rot, os.Stderr)
```
项目体现：日志同时写文件（持久化）+ stderr（开发时眼见）。  
**为什么需要**：开发时希望日志在终端直接看到，生产环境希望写文件持久化——两个需求其实可以同时满足。MultiWriter 把"写到哪里"和"写什么"解耦——业务代码只 `lg.Info(...)`，写到几个地方由 MultiWriter 配置。

## 9.7 排查工作流
```bash
# 1. 找时间段的 ERROR
grep "ERROR" logs/app.log | grep "10:23:"

# 2. 拿到 trace_id 后看完整链路
grep "tr-abc123" logs/app.log
```
**为什么这个流程**：用户反馈通常是"刚才下单失败了"——你只有大致时间。先用时间过滤拿到几条 ERROR，从中拿到 trace_id，再用 trace_id 过滤拿全部相关日志。这是结构化日志 + traceId 设计的回报。

## 9.8 指标 vs 日志
| | 日志 | 指标 |
|---|---|---|
| 性质 | 叙事（一条一条） | 统计数字（聚合） |
| 用途 | 排查具体问题 | SLO、告警、趋势 |
| 例 | "tr-abc123 调 LLM 12.3s" | "P95 LLM 时长 = 14.5s" |

**SLO 必须靠指标算，日志做不到聚合**。  
**为什么不能只用日志**：日志一条条记录"发生了什么"——但你想知道"过去 5 分钟 P95 时长是多少"——日志要把所有条目读出来排序计算。流量大时这是不可承受之重。指标是"已经聚合好的统计数字"——查询毫秒级返回。

## 9.9 Prometheus 客户端 / `/metrics` 端点
项目内 `prometheus/client_golang` 注册指标，HTTP 暴露 `/metrics` 端点。  
MVP 阶段不部署 Prometheus server——需要时 `curl 127.0.0.1:8080/metrics` 临时看。  
**为什么先暴露端点不部署 server**：Prometheus + Grafana 部署本身有维护成本，MVP 阶段流量小用不上可视化告警——只暴露端点几乎零成本，等需要可视化时再起 server。这是 [05.3 YAGNI](05-software-design.md#53-yagni-you-arent-gonna-need-it) 的体现。

## 9.10 三类指标（Counter / Gauge / Histogram）
| 类型 | 用途 | 例 |
|---|---|---|
| **Counter** | 只增不减的计数器 | `http_requests_total` |
| **Gauge** | 可增可减的瞬时值 | `outbox_pending_count` |
| **Histogram** | 分布统计（自动算 P50/P95/P99） | `http_request_duration_seconds` |

**为什么分这三类**：监控关心的"问题形态"不同——
- **趋势/速率**（每秒多少请求）→ Counter（除以时间得速率）
- **当前状态**（队列堆积多少）→ Gauge（直接看当前值）
- **延迟分布**（P95 多少）→ Histogram（自动桶化算分位）

类型选错了想算的算不出来，所以一定要按场景选对。

## 9.11 SLO（Service Level Objective）
"我们承诺什么质量"的可衡量目标。例：
- "故事生成 P95 ≤ 25 秒"
- "API 错误率 ≤ 1%"

必须靠 metrics 算，不靠"感觉今天还行吧"。  
**为什么需要**：没 SLO 时优化是凭感觉——"好像变慢了？""错误是不是多了？"。有 SLO 后变成具体目标——"P95 突破了 25s，需要排查"——决策有据可依。

## 9.12 业务指标（Business Metrics）
区别于"基础设施指标"（HTTP 请求数 / Go 协程数等技术性数据），业务指标关注**业务发生了什么**：
```
story_generate_total{status}        # 故事生成总数（按成功/失败）
story_generate_duration_seconds     # 端到端耗时
llm_call_duration_seconds{provider} # LLM 调用耗时
safety_fail_total{stage,reason}     # 安全拦截统计
outbox_pending_count                # 队列堆积
llm_budget_used_yuan                # 今日预算消耗
```
**为什么需要**：监控不只是"服务挂没挂"。业务指标让你能问"过去 5 分钟 P95 是多少？""安全规则今天拦了多少？""今日 LLM 花了多少钱？"——这些都不是基础指标能回答的。

## 9.13 预算熔断（Budget Circuit Breaker）
每天给外部 API 调用设个上限，超过自动停服防止半夜烧钱。  
**类比**：信用卡设了"每日消费限额 1000 元"——超过自动拒付。  
**实现**：Redis key `budget:llm:daily:YYYYMMDD` 累加每次调用的费用估算；每次调外部 API 前检查；超阈值返 503。次日 0 点 key 过期自动重置。  
**为什么需要**：LLM 按 token 计费，bug 或恶意刷接口能一夜烧光月预算。预算熔断是"宁可拒服务也不破产"的工程化止损。  
项目策略：100 元/天，足够 1500 次故事生成；超额停服等于"今天测试够了，明天接着来"。
