# 可观测性（Observability）

## 9.1 TraceID（贯穿全链路的请求 ID）
每个 HTTP 请求一个唯一 ID，从入口贯穿所有日志，便于排查"这个用户那次失败到底走到了哪一步"。  
项目体现：traceid 包通过 context 传递 + Logger 中间件自动注入每条日志的 `trace_id` 字段。

## 9.2 结构化日志（structured logging）
**传统文本日志**：`[INFO] user 42 generated story 5821 in 12.3s` —— 机器要正则解析。  
**结构化日志（JSON）**：每条一行 JSON，键值清晰：
```json
{"time":"...","level":"INFO","msg":"story.generated","user_id":42,"duration_ms":12300}
```
**类比**：手写订单 vs 标准表单。后者机器能直接录入。  
SLO 监控、告警、ELK/Loki 检索全靠这种格式。

## 9.3 `log/slog`
Go 1.21+ 标准库自带的结构化日志。  
新项目直接用，不需要再引入 zap / zerolog。

## 9.4 日志级别（DEBUG / INFO / WARN / ERROR）
| 级别 | 用途 |
|---|---|
| DEBUG | 细节调试信息（生产环境通常关闭） |
| INFO | 关键业务节点（启动、请求处理完成） |
| WARN | 可恢复异常 |
| ERROR | 业务失败（需要关注） |

项目策略：debug 阶段全开，上线后只保留 INFO+。

## 9.5 日志切割（lumberjack）
日志会无限增长。`lumberjack` 自动：
- 单文件超过 100MB → 拍快照新建文件
- 保留 14 天，老文件压缩
- 太老的自动删除

防止撑爆磁盘。

## 9.6 `io.MultiWriter`
Go 技巧：一份内容同时写多个目的地。
```go
mw := io.MultiWriter(rot, os.Stderr)
```
项目体现：日志同时写文件（持久化）+ stderr（开发时眼见）。

## 9.7 排查工作流
```bash
# 1. 找时间段的 ERROR
grep "ERROR" logs/app.log | grep "10:23:"

# 2. 拿到 trace_id 后看完整链路
grep "tr-abc123" logs/app.log
```

## 9.8 指标 vs 日志
| | 日志 | 指标 |
|---|---|---|
| 性质 | 叙事（一条一条） | 统计数字（聚合） |
| 用途 | 排查具体问题 | SLO、告警、趋势 |
| 例 | "tr-abc123 调 LLM 12.3s" | "P95 LLM 时长 = 14.5s" |

**SLO 必须靠指标算，日志做不到聚合**。

## 9.9 Prometheus 客户端 / `/metrics` 端点
项目内 `prometheus/client_golang` 注册指标，HTTP 暴露 `/metrics` 端点。  
MVP 阶段不部署 Prometheus server——需要时 `curl 127.0.0.1:8080/metrics` 临时看。

## 9.10 三类指标（Counter / Gauge / Histogram）
| 类型 | 用途 | 例 |
|---|---|---|
| **Counter** | 只增不减的计数器 | `http_requests_total` |
| **Gauge** | 可增可减的瞬时值 | `outbox_pending_count` |
| **Histogram** | 分布统计（自动算 P50/P95/P99） | `http_request_duration_seconds` |

## 9.11 SLO（Service Level Objective）
"我们承诺什么质量"的可衡量目标。例：
- "故事生成 P95 ≤ 25 秒"
- "API 错误率 ≤ 1%"

必须靠 metrics 算，不靠"感觉今天还行吧"。
