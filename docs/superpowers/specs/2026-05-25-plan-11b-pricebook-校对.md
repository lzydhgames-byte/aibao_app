# Plan 11B PriceBook 单价校对记录（v20260525-1）

## 一句话

本期 thin slice 采用**混合策略**——Minimax 拉到官方实价、豆包 + COS 因控制台需登录或前端 JS 渲染未能拉到，先用社区公示估值占位 + TODO 标注，**上线前必须补齐并 bump 版本号到 v20260525-2**。

## 为什么不一次性拉完

WebFetch 对火山引擎控制台和腾讯云 COS 价格页失败——这两个页面要么需要登录、要么是前端动态渲染。Minimax 文档站是静态站点所以拿到了。**这不是 spec 漏洞，是 Plan 11B §5.2 早已预案的"hot-reload 不支持，改价必重启"流程的第一次演练**：估值跑通流水线，cost_events.unit_price_snapshot JSONB 忠实记录"这条行用 v20260525-1 估值算的"，事后实价进来时按 version 区分回放。

## 当前版本 v20260525-1 条目

| provider | model | 字段 | 值 | 来源 |
|---|---|---|---|---|
| doubao | doubao-1.5-pro-32k | input | 4.00 元/1M tokens | 社区公示估值，TODO 上线前替换为火山引擎控制台实价 |
| doubao | doubao-1.5-pro-32k | output | 8.00 元/1M tokens | 同上 |
| doubao | doubao-1.5-lite-32k | input | 0.30 元/1M tokens | 同上 |
| doubao | doubao-1.5-lite-32k | output | 0.60 元/1M tokens | 同上 |
| minimax | t2a-v2 | chars | 0.315 元/1K chars | **官网实价**：platform.minimaxi.com/document/Price 2026-05-25 拉取，HD 系列套餐一 ¥630 / 2M chars 换算 |
| tencent_cos | hk-standard | put_yuan_per_10k_requests | 0.10 元/万次 | 社区公示估值，TODO 上线前替换为腾讯云控制台实价 |
| tencent_cos | hk-standard | bandwidth_yuan_per_gb | 0.50 元/GB | 同上 |

总计 **字段级 TODO 6 处**（4 个豆包字段 + 2 个 COS 字段），均以 `# TODO 待校对` 行内注释标注在 `server/config/config.yaml.example`；另 2 段说明性 TODO 在 `pricing_source` 段 + 2 个分组 banner，明示为何混合策略。

## 何时升级到 v20260525-2

**触发条件**（任一即触发）：
1. 用户从火山引擎控制台拉到豆包 pro/lite 真实单价
2. 用户从腾讯云 buy.cloud.tencent.com/price/cos 拉到 ap-hongkong 标准存储真实单价
3. 在 Plan 11B Full Build Task 31「真链路对账」执行前

**升级步骤**（spec 11B §5.2 不支持 hot-reload）：
1. 修 `server/config/config.yaml.example` 对应字段 + 删 `# TODO 待校对` 注释
2. bump `price_book_version: v20260525-2`
3. 更新本文档（追加新版本表 + 来源链接/截图引用）
4. 同步修服务器 `/etc/aibao/env` 关联的 config.yaml + `systemctl restart aibao`
5. 部署运维 checklist 写入 13-deployment.md 知识库（Plan 10 Day 2 教训沉淀）

## 审计 trail

每条 cost_events 行的 `price_version` 字段记录"用了哪个版本算的"。如未来发现 v20260525-1 估值偏离实价超过 10%，**不在线回放历史**（spec §5.1 历史只读），但可离线 audit 模式手工 `UPDATE` 历史 cost_yuan 用新 unit_price_snapshot 重算（仅审计场景，非日常）。
