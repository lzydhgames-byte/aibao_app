# 软件设计原则与模式

## 5.1 关注点分离（SoC）
每段代码只做一件事。**类比**：餐厅切菜的、炒菜的、收银的各管一摊。  
项目体现：三层架构、viper（"从哪读"）+ mapstructure（"怎么填"）。

## 5.2 依赖倒置（DIP）
高层依赖**抽象接口**，不依赖**具体实现**。  
**类比**：插座（抽象）+ 灯泡（具体）——换灯泡随便换，墙不动。  
项目体现：LLM/TTS/SMS/Storage 全走 Gateway 抽象层；换豆包→Claude 业务零改动。

## 5.3 YAGNI（You Aren't Gonna Need It）
没有真实需要时，**不要**提前引入复杂度。  
**类比**：刚搬家就买"以备不时之需"的备用沙发——客厅塞不下还都没用过。  
项目体现：MVP 不上 Docker、不拆微服务、不上 Kafka、Prometheus 暂不部署。

## 5.4 12-Factor App（重点：配置外置）
**铁律**：代码里**绝不**写死配置。配置走环境变量。  
好处：同一份代码可跑 dev/staging/prod；密钥永不上 git；上线只改 env。  
项目体现：yaml + `AIBAO_*` env 覆盖；`config.prod.yaml` 在 `.gitignore`。

## 5.5 Pets vs Cattle
现代运维：服务器当**牲口**——挂了换新的，10 分钟拉起来。不当宠物精心抢救。  
做到这点的关键：配置外置、文件存对象存储、schema 走迁移工具、备份脚本入 git。

## 5.6 幂等（Idempotent）
执行 N 次和 1 次结果完全一样。  
✅ `UPDATE users SET status='active' WHERE id=42` —— 跑 100 次都是 active  
❌ `UPDATE users SET balance = balance + 100 WHERE id=42` —— 跑 100 次扣 1 万 vs 100 块  
**类比**：电梯按钮——按一次和按十次都是叫电梯到这层。  
项目体现：traceid 的 `Ensure`、Outbox handler 用 UPSERT、payload 带 dedup_key。

## 5.7 Outbox Pattern（事务出箱模式）
业务库写入 + "待发事件"写入**在同一个数据库事务**里。  
解决"业务写了但消息没发"的丢失问题——保证事件不丢。
```sql
BEGIN;
INSERT INTO stories(...);
INSERT INTO outbox_events(event_type, payload, ...);
COMMIT;
```
**类比**：餐厅订单纸——"客人点了什么"和"送到几号桌"写在同一张纸上，绝不会做了菜不知道送哪儿。  
我们的核心卖点"有记忆的 AI"靠它兜底。

## 5.8 优雅关停（Graceful Shutdown）
进程退出前先完成在途请求，再关闭。  
- ❌ `kill -9` 强杀：用户看到"故事生成到一半失败"
- ✅ `kill -TERM`（默认）：进程接到信号 → 拒绝新请求 → 等在途完成 → 退出  
项目体现：Task 18 main.go 监听 SIGTERM。

## 5.9 软件分层与依赖方向
```
api → service → repository + gateway → pkg
```
高层依赖低层，低层不依赖高层。Go 编译器**自动拦截循环依赖**——强制你想清楚分层。
