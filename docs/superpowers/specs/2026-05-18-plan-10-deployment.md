# Plan 10 spec — 部署上线

**日期**：2026-05-18 brainstorm，预计 2026-05-19 执行
**目标**：从"本地 MVP 跑通"到"朋友/家长扫码下载 APK 试用"
**预算时间**：明天下午 4 小时 全走完

## 1. 决策概览

| # | 决策 | 选定 | 理由 |
|---|---|---|---|
| 1 | 服务器 | 腾讯云香港 2C4G / 70GB / 30Mbps（已购未初始化） | spec 1.1 已定 |
| 2 | 进程管理 | systemd（`Restart=always`） | spec 1.1 已定，跟 spec 一致 |
| 3 | 反向代理 | Nginx | spec 1.1 已定 |
| 4 | HTTPS 证书 | Let's Encrypt + certbot 自动续期 | 免费 + 自动 |
| 5 | 域名 | **`aibao.dhgames.com`**（复用用户主域名的子域名） | 0 成本，立即生效；香港 IP 不需备案 |
| 6 | 数据库 | PostgreSQL 16（apt install 服务器原生） | 不上 Docker（spec 1.2 已说 YAGNI） |
| 7 | Redis | apt install 服务器原生 | 同上 |
| 8 | 数据迁移 | golang-migrate（已就位）启动前手动跑 | 单实例无并发，简单稳 |
| 9 | App 分发 | APK 直传（二维码 + 链接），跳过应用商店 | 朋友试用够用 |
| 10 | App release 模式 | Plan 9-A 踩过 ExoPlayer NPE，必须加 proguard rules | 已记录到 12-flutter.md |
| 11 | App 后端地址 | 编译时通过 `--dart-define=API_BASE=https://aibao.dhgames.com` 注入 | 一个 build 多用 |
| 12 | 真 SMS | 腾讯云 SMS（签名+模板审核 ~3 工作日） | **明天先用 Mock SMS 上线**，等审核通过后热切真 SMS |
| 13 | 备份 | cron + pg_dump → 本地 70GB 内（暂不传 COS） | 简单，磁盘够 |
| 14 | 监控告警 | Prometheus `/metrics` 仅 127.0.0.1；本地 ssh tunnel 查 | spec 已定，单机够用 |
| 15 | 日志收集 | slog 写文件 + lumberjack 轮转 | 已就位 |
| 16 | 密钥管理 | systemd `EnvironmentFile=/etc/aibao/env` (root only, 0600) | 不进 git，重启保留 |
| 17 | 灰度 / 回滚 | 单实例热替换，有计划停机窗口（夜间）。重启 < 5s 影响轻 | MVP 期单实例够 |
| 18 | 故障恢复 | systemd `Restart=on-failure RestartSec=5s` | 兜底 Plan 9d 暴露的 PG-挂-后端死 问题 |

## 2. 范围内 vs 范围外

### 范围内（明天必须完成）

1. 服务器初始化（用户 / 防火墙 / fail2ban）
2. PostgreSQL 16 + Redis 7 安装 + 配置（仅 127）
3. golang-migrate 迁移 schema
4. 后端二进制构建 + 部署 + systemd service
5. Nginx + Let's Encrypt HTTPS
6. 域名 DNS A 记录指向服务器 IP
7. Flutter app 改可配后端地址 + 编 release APK + proguard rules
8. APK 上传到 COS public bucket 或服务器 nginx，生成二维码
9. 端到端冒烟（手机扫码下载 → 真服务器登录 → 生成故事 → 听）

### 范围外（后续 Plan 处理）

- 真 SMS 切换（审核期间走 Mock）
- App Store 上架（一期不做）
- pg_dump → COS 备份链路（一期本地够）
- Prometheus / Grafana 远程 dashboard
- 多实例 / 负载均衡（用户量未起）
- iOS 客户端

## 3. 任务拆解（顺序 + 预估工时）

### Phase 0：准备（0.5h，今天可以做）

| Task | 工时 | 备注 |
|---|---|---|
| ~~0.1 注册域名~~ | ✅ 跳过 | 已用 `aibao.dhgames.com`（用户主域名子域名） |
| 0.2 在 dhgames.com 控制台加 A 记录 `aibao.dhgames.com → <server_ip>` | 5min | 用户操作 |
| 0.3 准备腾讯云 SMS 签名 + 模板审核申请 | 20min | 等待期 3 工作日 |
| 0.4 准备 APK 签名 keystore | 10min | `keytool -genkey` 生成（部署时一起做） |

### Phase 1：服务器初始化（0.5h）

| Task | 工时 | 备注 |
|---|---|---|
| 1.1 SSH 进服务器，改 root 密码 / 加 SSH key | 5min |  |
| 1.2 创建 `aibao` 应用用户（非 root） | 5min |  |
| 1.3 ufw 防火墙：仅开 22 / 80 / 443 | 5min |  |
| 1.4 安装 fail2ban + 配置 sshd jail | 10min | 防 SSH 爆破 |
| 1.5 配置时区 / NTP / hostname | 5min |  |

### Phase 2：基础设施（1h）

| Task | 工时 | 备注 |
|---|---|---|
| 2.1 `apt install postgresql-16 postgresql-contrib redis-server nginx certbot` | 10min |  |
| 2.2 PostgreSQL：创建 `aibao` 用户和 `aibao` 数据库 / pg_hba.conf 限 127 | 10min |  |
| 2.3 Redis：bind 127.0.0.1 / requirepass / persistence | 5min |  |
| 2.4 跑 golang-migrate 建表 | 5min | 直接 `migrate -path migrations -database "..." up` |
| 2.5 Nginx：写 server block 反代 :8080 / 配置访问日志 | 15min |  |
| 2.6 certbot：申请 Let's Encrypt 证书 + 自动续期 | 10min | `certbot --nginx -d aibao.dhgames.com` |
| 2.7 验证 HTTPS：`curl https://aibao.dhgames.com/health` | 5min |  |

### Phase 3：后端部署（1h）

| Task | 工时 | 备注 |
|---|---|---|
| 3.1 本地 `GOOS=linux GOARCH=amd64 go build` 出 Linux 二进制 | 5min |  |
| 3.2 scp 二进制 + 配置 / safety/* 模板文件到服务器 | 10min |  |
| 3.3 写 `config/config.prod.yaml`（生产配置：log level / 端口 / DB host 127 等） | 10min |  |
| 3.4 写 `/etc/aibao/env`（敏感凭据：JWT secret / AES key / Doubao key / Minimax key / COS key） | 10min | root:root 0600 |
| 3.5 写 systemd unit `/etc/systemd/system/aibao.service` | 15min | EnvironmentFile + Restart=on-failure |
| 3.6 `systemctl daemon-reload && systemctl enable --now aibao` | 5min |  |
| 3.7 验证：`journalctl -u aibao -f` + `curl https://aibao.dhgames.com/health` | 5min |  |

### Phase 4：App release 打包（1h）

| Task | 工时 | 备注 |
|---|---|---|
| 4.1 在 ApiClient 默认 baseUrl 加 `String.fromEnvironment('API_BASE', defaultValue: 'http://127.0.0.1:8080')` | 10min |  |
| 4.2 配 `android/app/build.gradle.kts` signing config（用 4.4 的 keystore） | 10min |  |
| 4.3 写 `android/app/proguard-rules.pro` 解 ExoPlayer NPE（12-flutter.md §12.7 已记） | 15min |  |
| 4.4 `flutter build apk --release --dart-define=API_BASE=https://aibao.dhgames.com` | 5min |  |
| 4.5 真机（OPPO 或 AVD）安装 release APK，端到端冒烟 | 15min |  |
| 4.6 APK 上传到 nginx static 目录 / 生成下载链接 + 二维码 | 5min | qrencode CLI |

### Phase 5：端到端冒烟（0.5h）

| Task | 工时 | 备注 |
|---|---|---|
| 5.1 用真手机/AVD 扫码下载 APK 安装 | 5min |  |
| 5.2 走 Plan 9d 13 项流程的子集（用户视角）：登录 / 生成 / 听 / 续集 / 历史 / 登出 | 20min |  |
| 5.3 反查 systemd 日志确认无 panic / unexpected | 5min |  |

## 4. 风险清单

| 风险 | 缓解 |
|---|---|
| Let's Encrypt 拒证书（DNS 没解析全球生效） | 等 30min DNS 传播 / 用 `--http-01` 而非 `--dns-01` |
| ExoPlayer NPE 在 release apk 重现 | 12-flutter.md §12.7 有 proguard 模板照抄；不行就回 debug apk 临时给朋友试 |
| 服务器内存吃紧（2GB） | PG `shared_buffers=256MB` / Redis `maxmemory=128mb` / Go server max heap 监控；超了换 4G 机型 |
| 香港服务器到大陆访问慢 | 朋友试用阶段可接受；正式上线前考虑 CDN 或迁内陆 |
| 真 SMS 审核被拒 | 第二次提交 / 临时手动加白名单用户（Mock 路径） |
| 域名当天买未生效 | 备用方案：用 IP + 自签证书顶一两天 |

## 5. 验收标准

部署完毕后必须满足：

1. ✅ `https://aibao.dhgames.com/health` 返回 `{"status":"ok"}`
2. ✅ 二维码扫码后能下载安装 release APK
3. ✅ 真机走完 Plan 9d 13 项里的 user-flow 子集（登录到听完一个故事）
4. ✅ `systemctl status aibao` 显示 active running
5. ✅ 故意 `systemctl stop postgresql` 后等 30s 再 start，aibao 服务自动恢复（systemd restart 兜底）
6. ✅ DB / Redis / Metrics 端口（5432 / 6379 / 9100）从公网 SCAN 拒绝（ufw 防火墙生效）
7. ✅ certbot 自动续期定时任务已添加（`certbot renew --dry-run` 通过）

## 6. 接力卡（明天开战）

**一句话给新会话**：
> Plan 10 部署上线开干。看 `docs/superpowers/specs/2026-05-18-plan-10-deployment.md` 任务拆解，从 Phase 0 开始。今天用户可能已经完成 Phase 0.1-0.4。

**前置确认**：
- 域名 `aibao.dhgames.com` 已确认（复用用户主域名子域名）
- DNS A 记录指向服务器 IP 是否已配置？`dig +short aibao.dhgames.com` 验证
- SMS 签名/模板申请是否已在腾讯云提交审核？（3 工作日等待期，期间走 Mock SMS）

**主战场命令**：spec 文档每行 task 都给出了具体 shell 命令片段。新会话直接照着跑就行。
