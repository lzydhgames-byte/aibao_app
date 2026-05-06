# 可观测性（Observability）

> 待 Plan 1 / Task 6、Task 8、Task 11、Task 12 引入。

将涉及的概念预告：

## 已经接触的（暂记此处，正式条目在 Task 6 起补全）

### TraceID（贯穿全链路的请求 ID）
**一句话**：每个 HTTP 请求一个唯一 ID，从入口贯穿所有日志，便于排查"这个用户那次失败到底走到了哪一步"。

详见 [02.4 context.Context](02-go-language.md#24-contextcontext请求级上下文) 与 [02.5 ctxKey 模式](02-go-language.md#25-ctxkey-模式避免-context-key-冲突)。

完整解释将在 Task 11 middleware/logger 中正式建立。

## 即将引入

- **结构化日志（structured logging）** vs 纯文本日志
- `log/slog`（Go 1.21+ 标准库结构化日志）
- 日志级别（DEBUG / INFO / WARN / ERROR）
- 日志按天切割（lumberjack）
- 敏感字段脱敏（手机号、孩子姓名）
- **指标 vs 日志**：日志是"叙事"，指标是"统计数字"——SLO 必须靠指标
- Prometheus 客户端库
- Counter / Gauge / Histogram 三类指标
- SLO（Service Level Objective）
