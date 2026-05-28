# 开发日志 — 2026-05-28（Plan 11 代码侧 30/32 完成）

> Plan 11 = AI 大纲预览（11A，B 模式）+ 成本可观测（11B，Thin Slice + Full Build）。本篇记录代码侧 30/32 task 全部 commit 落地。剩 2 个 task（真链路对账 + 收官追述）依赖上线后真实运营数据，单独补一篇 follow-up devlog。

## 一句话

把"用户填表手选风格主题"换成"AI 推断 + 大纲卡预览-确认"两阶段，同时上线**生产级成本可观测基础设施**——4 个 Sprint × 30 task × 31 commits × 7 个工作日，全程通过 Codex 三轮 review 和 subagent-driven 执行。

## Plan 11 起源

Plan 10 部署上线后真实用户反馈："每次提交需求后还要手填主题、风格——'跟奥特曼一起冒险'这种意图明显的输入，AI 应该自己听懂。" 这是产品定位问题：AI 强项（理解需求）被旁路，AI 弱项（让用户帮忙做决策）被强化。同时朋友试用规模 ~10 人，**商业化定价缺数据基础**——没有"人均月成本"这个数，定 9.9/19.9/29.9 都是拍脑袋。

两个独立子项目：

- **11A AI 大纲预览（B 模式）**：用户只填"需求 + 时长"；AI 推断主题/风格/教育意义；大纲卡用户确认后才生成正文
- **11B 成本可观测**：不加限流、不加付费 UI、不上 Grafana。先看见，再决定要不要拦

## Spec 阶段 — Codex 三轮 review

写完两份 spec 后拿给 Codex 评审：
- **一轮 review**：22 项意见（4 ✅ / 13 ⚠️ / 5 ❌），给 TOP 3 必改 + TOP 3 可选改进
- **二轮 review** ❌ Not ready：吸收 P3 契约冲突（outline_id ↔ storyline_id 互斥）+ 5 项新引入风险 N1-N5（双表写入顺序、PriceBook hot-reload、event_id 业务语义化、HMAC secret rotation、OutlineResolver 独立中立包）
- **三轮 review** ⚠️ Ready with caveats：清最后 3 处文字残留 + 明确 outline_events append-only 实施口径

成果：
- `docs/superpowers/specs/2026-05-25-plan-11a-ai-outline-preview.md`
- `docs/superpowers/specs/2026-05-25-plan-11b-cost-observability.md`
- 顶层 `docs/superpowers/specs/2026-04-28-aibao-design.md` / `2026-04-28-aibao-tech-architecture.md` 同步更新

🎓 **关键产出**：spec §7.5 N5 独立中立合约包 `outlinecontract` —— 解决 `service/story ↔ service/outline` 反向依赖的最干净方案。这种"在两个互依包之间加一个零实现包"的模式以后值得在其他地方复用。

## 实施阶段 — 4 个 Sprint × 30 task

### Sprint A — 11B Thin Slice 基础设施（Task 1-11）

> 必须先于 11A handler 上线。这一段做完后 cost_events 已经在记账，outline_events 表已就位。

| # | Task | Commit |
|---|---|---|
| 1 | PriceBook 单价校对 | `3783975` Minimax 实价 + 豆包/COS 估值 TODO |
| 2 | pkg/idhash HMAC | `30b625f` HMAC-SHA256 12hex + domain separation |
| 3 | pkg/cost PriceBook | `effb371` 接口 + viper YAML 加载 |
| 4 | pkg/cost Calc 纯函数 | `13c0f74` LLM/TTS/audio 三种单位 |
| 5 | outline_events migration | `0515fec` v8 append-only + 3 索引 + CHECK |
| 6 | cost_events migration | `b33e805` v9 event_id UNIQUE + price snapshot + 5 索引 |
| 7 | GORM models | `4ffa060` CostEvent + OutlineEvent |
| 8 | Recorder | `e073e5d` event_id 正则 + Prometheus + 异步队列 |
| 9 | Flusher | `66be2a3` ON CONFLICT 幂等 + 集成测 |
| 10 | Gateway Usage 暴露 | `33aa6c9` TTS CharCount + Storage bytesUploaded |
| 11 | Recorder wire 5 业务点 | `abbbcf9` story/audio/memory/chapter_hook 全打通 |

🎓 **Sprint A 关键决策**：
- **PriceBook 校对采用混合策略**：Minimax 拉到实价（platform.minimaxi.com/document/Price 静态站可 WebFetch），豆包+COS 因登录墙/JS 渲染拉不到，用社区估值 + 字段级 TODO 占位。上线前必须从控制台拉实价 + bump v20260525-2 重启服务（spec §5.2 不支持 hot-reload）
- **event_id 业务语义化**：从 `{trace_id + monotonic counter}` 改为 `{trace_id}:{purpose}:{stage}:{attempt}` —— 跨进程/重启可幂等
- **分层强约束**：Gateway 不调 Recorder，业务方拿到 `Usage` struct 后显式调；Task 24 CI 守门

### Sprint B — 11A 大纲后端（Task 12-23）

| # | Task | Commit |
|---|---|---|
| 12 | outlinecontract 中立包 | `80174a9` 接口 + DTO + errors，零 IO |
| 13 | outline Cache (Redis) | `44c2433` 5min TTL + ownership 字段 |
| 14 | EventStore (PG) | `814cdeb` append-only + INSERT WHERE NOT EXISTS 幂等 |
| 15 | LLM prompt + parser | `35d8c69` response_format + 严格 schema + 10 negative tests |
| 16 | OutlineSafetyCheck | `9f25b86` 红线/害怕/主角/IP 四类校验 |
| 17 | Service.Preview | `95de319` 2 阶段 repair retry + cost recording |
| 18 | ResolverImpl | `da6ec6a` 三元 ownership + replay defense |
| 19 | preview handler + apperr 410/422 | `d64aaa3` 共享桶 5/min |
| 20 | refresh handler | `ae362f8` invalidate + same group + variant++ |
| 21 | Orchestrator step 0 | `3e67d1c` HydrateFromOutline + 互斥两层 |
| 22 | prompt outline hints | `1390bd6` Title/Synopsis/EduValue 注入正文 |
| 23 | housekeeping 双路径 | `1a26fae` SweepUser + 10min 全表兜底 |

🎓 **Sprint B 关键决策**：
- **OutlineResolver 接口在中立包 `outlinecontract/`**：`service/story` 仅依赖合约不依赖实现；编译期断言 `var _ outlinecontract.OutlineResolver = (*ResolverImpl)(nil)` 守门
- **append-only 严守**：所有 outline_events 写入都是 `INSERT`，无 UPDATE。`MarkExpiredIfPending` 用 `INSERT ... WHERE NOT EXISTS` 幂等
- **story 包零反向 import**：用 `"accepted"` 字符串字面量而非 `outline.OutcomeAccepted` 常量；`OutlineEventAppender` narrow interface 在 consumer 侧（story）定义

### Sprint C — CI + Flutter UI（Task 24-28）

| # | Task | Commit |
|---|---|---|
| 24 | deps-lint CI | `7a511ae` check_layering.sh + Makefile target |
| 25 | Flutter API client | `e7e906a` previewOutline / refreshOutline / generateStoryFromOutline + feature_flag |
| 26 | Riverpod providers | `12a2f80` outlinePreviewProvider (family) + currentOutlineProvider |
| 27 | generate_screen 重做 | `feb4bc8` 三分支：续集 / flag-off / outline；LegacyGenerateScreen 完整保留 |
| 28 | outline_screen | `e72b79c` 倒计时 + 3 action + invalidate storyListProvider |

🎓 **Sprint C 关键决策**：
- **续集模式（storylineId）必须保留**：spec §10.1 续集与 outline 互斥；GenerateScreen 三分支路由——续集 / flag-off / outline——把 Plan 9b 续集 UI 完整委托给 LegacyGenerateScreen
- **Flutter feature flag 编译时控**：`FeatureFlags.outlineEnabled = bool.fromEnvironment('OUTLINE_ENABLED', defaultValue: true)`。紧急回滚 `flutter build apk --dart-define=OUTLINE_ENABLED=false`
- **Riverpod 文件命名**：`state/outline_state.dart` 而非 plan 写的 `providers/outline_provider.dart` —— 沿用现有 `heartbeat_state.dart` / `child_state.dart` 惯例

### Sprint D — Full Build 代码部分（Task 29-30）

| # | Task | Commit |
|---|---|---|
| 29 | Aggregator | `1b60c58` Overall/ByPurpose/TopUsers/OutlineSaving full pipeline 公式 + 2 integration tests |
| 30 | cost-report CLI | `0070faf` cobra 3 subcommands + Makefile target |

🎓 **Sprint D 关键决策**：
- **OutlineSaving = N × avg(full_pipeline_cost) - actual_outline_spend**：full_pipeline 含 story LLM + TTS + chapter_hook + memory_summary + storage_put 全量
- **DISTINCT ON 拿最新生命周期**：`DISTINCT ON (outline_id) ... ORDER BY outline_id, occurred_at DESC, id DESC` —— append-only 表的标准查询模式
- **CLI 用 cobra 而非 stdlib flag**：与 `safetycheck` / `rules-lint` 风格对齐

## 待补：Task 31 + 32（依赖上线）

| Task | 性质 | 触发条件 |
|---|---|---|
| 31 真链路对账 | 运营 + 数据分析 | 上线 ≥ 24 小时 + 真实 cost_events 累积 + 拉豆包/Minimax/COS 后台账单三方对照（误差 <10% 验收） |
| 32 收官追述 | 写 follow-up devlog | Task 31 完成后 |

**为什么不在本篇一起写**：Task 31 需要真实数据 + 厂商账单（厂商账单通常 T+1 才同步）。代码侧已经完结的事实先固定记入档案，运营动作单独跟进。

## 实施阶段亮点

### Subagent-Driven 执行模式实测

22 task 由 subagent 完成 + 6 task inline（Anthropic 529 拥塞时回退 + 风险极低 task）+ 2 task 需要二次派遣（subagent 正确停下来报告冲突 → 给决策再下派）。**结论**：32-task 规模下主会话上下文压力可控，质量没下降。Subagent-driven 模式下次类似 plan 沿用。

### Plan 文档 vs 实际偏差 ~15 处

实施期发现 plan 写作时凭印象假设的"项目 API 长什么样"多处不一致。已在 plan 文件追加 **附录 A 实施期偏差日志**（commit `4f8d414`），分类记录后端 Go（metrics 注入模式、logger.FromCtx、apperr 构造、safety API、RuleSet 字段类型等）+ Flutter Dart（state/ 而非 providers/、_dio + _ensureSuccess、valueOrNull、storylineId 续集保留等）+ Infra（无 .github/workflows） + Subagent 实测对比。**未来 Plan 11C+ 或类似 plan 应先扫附录 A，避免重踩**。

### 关键质量信号

- **Layering 强约束保住**：`go list -deps ./internal/gateway/...` 全程零 service 反向依赖；CI 守门（Task 24）
- **Append-only 严守**：所有 outline_events 写入都是 INSERT，无 UPDATE
- **Integration tests 覆盖关键路径**：cost flusher 幂等、outline cache TTL、events append-only、resolver 三元 ownership + replay defense、housekeeping 双路径、aggregator full pipeline 公式
- **Flutter analyze 全绿** + **story orchestrator 不 import outline 包**（仅 outlinecontract）

## Plan 11 当前状态

- ✅ 30/32 代码 + 文档 commit 落地
- ⏳ 后端待打包部署到香港服务器（systemd + Nginx + Let's Encrypt + COS 流程都还在）
- ⏳ Flutter 待 `--dart-define=OUTLINE_ENABLED=true` 打 release apk
- ⏳ 朋友试用 ≥ 1-2 天后跑 Task 31 真链路对账
- ⏳ Task 32 follow-up devlog 补完整

## 涉及到的知识点（同步至 docs/knowledge/）

- 05-software-design.md：中立合约包打破双向依赖 / append-only 事件流 / narrow interface 在 consumer 侧定义
- 09-observability.md：双轨数据源（PG 事实 + Prometheus 近似）+ event_id 业务语义化幂等
- 10-security-and-compliance.md：HMAC + domain separation + 12hex 截断 / 三元 ownership + replay defense
- 11-llm-engineering.md：response_format=json + schema repair retry + safety output 端校验 + repair 重试
