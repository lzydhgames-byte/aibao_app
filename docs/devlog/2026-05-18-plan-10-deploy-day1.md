# 开发日志 — 2026-05-18（Plan 10 部署上线 · Day 1）

> ⚠️ 同日有多篇 devlog：本篇是 Plan 10 部署执行，另有 `2026-05-18-plan-09d-regression.md`（Plan 9d 回归）和 `2026-05-18-plan-09c-third.md`（Plan 9c 第三战）。

## 今日进展

按 [Plan 10 spec](../superpowers/specs/2026-05-18-plan-10-deployment.md) 执行部署上线。下午到晚上 ~7 小时，**完成度 92%**——后端、客户端、域名、HTTPS、签证全部就位，**唯一卡点是 COS 大文件上传从香港 → 上海 TLS 层卡死**。

## Phase 0：准备

- ✅ 域名定为 `aibao.dhgames.com`（复用用户主域名子域名，跳过备案）
- ✅ DNS A 记录 `aibao.dhgames.com → 43.128.30.183`（由用户公司 DNS 同事配置）
- ✅ MVP 期跳过真 SMS，纯 Mock SMS（123456 固定码）
- ✅ APK keystore 用 debug key（朋友试用够用）

## Phase 1-2：服务器初始化 + 基础设施

腾讯云**轻量应用服务器**（不是 CVM）香港 2C4G / 70GB / Ubuntu 24.04 LTS。

🎓 **意外发现**：服务器**不是干净状态**——之前的项目留下了：
- Docker + docker-compose 已装
- `/opt/aibao/docker-compose.yml` 跑着 `aibao-postgres` + `aibao-redis`（postgres:16-alpine + redis:7-alpine，健康跑了 3 周）
- 数据卷 `aibao_pgdata` + `aibao_redisdata` 持久化
- `lighthouse` + `ubuntu` 系统用户

**决策**：跳过 spec 里"apt 装 PG/Redis"，**复用现成 Docker 容器**。spec 里"YAGNI 不上 Docker" 调整为"已有 Docker 就复用"。

凭据复用：DB 用户 `aibao` / 密码 `aibao_prod_2026` / 库 `aibao`。

迁移 7 个 migration 全过：
```
1/u init / 2/u users_and_children / 3/u stories_and_outbox /
4/u audio_status / 5/u memories_story_id / 6/u bgm_assets / 7/u storylines
```
10 张表建好（bgm_assets / children / infra_check / memories / outbox_events / schema_migrations / stories / story_elements / storylines / users）。

## Phase 3：后端部署

- ✅ Go 1.23.4 装到 `/usr/local/go`
- ✅ golang-migrate 装到 `$GOPATH/bin`
- ✅ `bin/aibao-server` 编译出来 52MB
- ✅ `/etc/aibao/env` 12 个密钥（3 个我生成的 prod 密钥 + 9 个用户 setx 过的外部 API key）
- ✅ `/etc/systemd/system/aibao.service` 含 `EnvironmentFile + Restart=on-failure`
- ✅ `systemctl enable --now aibao` → active

🎓 中途发现 ffmpeg 没装，重启后 `apt install ffmpeg` 补上。

## Phase 4：Nginx + HTTPS

```
listen 80 / 443  →  https://aibao.dhgames.com
/api/* → 127.0.0.1:8080
/download/app-release.apk → static
/ → 落地页 (熊猫 emoji + 下载按钮)
```

Let's Encrypt 证书自动申请通过：
```
Successfully received certificate.
Certificate is saved at: /etc/letsencrypt/live/aibao.dhgames.com/fullchain.pem
Expires on 2026-08-16. Certbot has set up scheduled renewal.
```

## Phase 5：App release 打包 + 上传

🎓 **改动**：
- `ApiClient.baseUrl` 默认值改为 `String.fromEnvironment('API_BASE', defaultValue: 'http://127.0.0.1:8080')`
- `proguard-rules.pro` 加 ExoPlayer / Media3 / Flutter plugin / dio 的 keep 规则（防 R8 minify 把反射类剥光）
- `build.gradle.kts` 打开 release minify + shrink + proguard

`flutter build apk --release --dart-define=API_BASE=https://aibao.dhgames.com`
→ 50.4MB APK，通过 OrcaTerm 文件管理器上传到 `/var/www/aibao/download/`

加 nginx `application/vnd.android.package-archive` MIME（Chrome 等浏览器才能识别为安装包）。
生成 qrencode ASCII + PNG 二维码。

## Phase 6：真机端到端冒烟

| 步骤 | 结果 |
|---|---|
| 微信扫码 | ❌ 腾讯生态拦截非自家域名 |
| 手机自带浏览器（华为/OPPO/小米 ROM） | ❌ 落地页 HTML 被当二进制保存（未备案 .com 域名风控） |
| **直接输入 `aibao.dhgames.com/download/app-release.apk`** | ✅ 真下载 50MB APK |
| Android 安装 + 打开 | ✅ |
| 登录（13900000001 / 123456） | ✅ 200 OK |
| 创建孩子 / 看首页 / 点生成 | ✅ |
| **文本生成** | ✅ **97 秒**（97s 一开始有点慢，但 dio 180s 撑得住）|
| **TTS 合成** | ✅ 168 秒 mp3，2.6MB |
| **COS 上传** | ❌ **TLS handshake timeout** |

## 关键故障：COS 上传从香港→上海 TLS 卡死

详细诊断链路（耗时 ~3 小时）：

### 第一轮假设：网络问题
- `curl -v https://aibao-audio-dev-1356733768.cos.ap-shanghai.myqcloud.com/`
- TLS 握手 OK，3.6 秒返 403 AccessDenied（无 auth 是预期）
- **网络层通**

### 第二轮假设：cos-go-sdk-v5 TLSHandshakeTimeout 默认 10s 太短
- 改 cos.go 注入自定义 `http.Transport` with `TLSHandshakeTimeout: 30s` + keep-alive
- commit `76f702c`
- 部署后**还是 TLS handshake timeout**

### 第三轮假设：上面那次没真生效
- 检查 binary 时间戳 + `signV5` 函数 grep
- 发现 git pull 拉到了最新 commit，但 binary **没重新 build**
- 重新 `go build` + restart
- **还是 TLS handshake timeout**

### 第四轮假设：cos-go-sdk-v5 内部有兼容性问题
- 写 `cos_probe.go` 用 **Go 标准库 net/http + 手工 COS V5 签名**，绕过 SDK
- **20 字节文本上传 → 300ms HTTP 200** ✅
- commit `93a09e8`：把生产 `Upload()` 改成裸 net/http
- 重新部署后仍然 timeout

### 第五轮决定性测试：大小决定一切
- 写 `big_probe.go`，**5MB 随机数据**走同一签名代码
- 结果：**20.8 秒 → TLS handshake timeout**
- 同一份代码、同一个签名、同一个 endpoint，**只差文件大小**

### 真正根因（猜测）

香港主机 → 腾讯云 ap-shanghai COS 这条链路，**对大流量 PUT 在网络层有特殊处理**——可能是：
- MTU 不匹配导致 TLS handshake 阶段丢包
- 防火墙对大 HTTPS 上传做了 SNI 检查 + rate limit
- TCP window 协商不畅

这不是代码层能修的问题。

## 中途意外事件

1. **Minimax 余额耗尽**：今天反复重试烧光了昨天充值的 100 块，又充了 100 块。
2. **outbox event 状态机卡住**：`processing` 状态没被 worker 正确回写为 `pending`/`dead`。手工 SQL 复活了 3 次。
3. **手机浏览器 vs 微信扫码**：国产 ROM 自带浏览器对未备案域名直接当下载，微信对非腾讯域名拦截扫码。最稳的下载路径是手机浏览器**直接输 URL**。

## 已上线服务

| 资源 | URL | 状态 |
|---|---|---|
| 落地页 | https://aibao.dhgames.com | ✅ |
| API 健康检查 | https://aibao.dhgames.com/api/v1/health（其实在 /health） | ✅ |
| APK 下载 | https://aibao.dhgames.com/download/app-release.apk | ✅ |
| 二维码 | https://aibao.dhgames.com/qrcode.png | ✅ |
| HTTPS 证书 | Let's Encrypt，2026-08-16 过期，自动续期 | ✅ |

## 今日 commit 链

```
cbc1a3c spec(plan10): deployment go-live brainstorm + 18-decision matrix + 5-phase plan
a3334b7 spec(plan10): lock domain to aibao.dhgames.com (reuse user's main domain)
8a32582 spec(plan10): skip real SMS, ship with Mock — defer to Plan 11/12
76f702c fix(storage): custom HTTP transport for COS upload, 30s TLS handshake timeout
93a09e8 fix(storage): bypass cos-go-sdk for Upload, use bare net/http + V5 signature
```

## 关键教训

1. **服务器不是空的就当二手机处理**：先 `docker ps -a / ls /opt / netstat / dpkg -l` 探一遍再动手。否则可能跟前任项目的容器/服务/规则打架。
2. **重新 build 永远是部署的第一步**：`git pull` 不会自动重新编译。binary 时间戳是诊断"代码改了但行为没变"的第一信号。
3. **TLS handshake timeout 跟 payload 大小相关**：之前以为 TLS 握手在 body 发送前完成，错。某些场景下 TCP/TLS 是 streaming 的，握手阶段对吞吐量敏感。
4. **国产 ROM 浏览器对未备案域名风控严**：除了备案，别无他法。MVP 期可以让朋友"直输 URL"绕过。
5. **微信不要试图分发 APK**：腾讯生态拦截外域 APK 是写在规则里的。直接 URL + 手机浏览器是最稳路径。
6. **outbox state machine 卡 processing**：现有实现没处理"worker 拿了 event 但失败前服务挂了"的情况。Plan 11 可以补一个 stale-processing reaper。

## 涉及到的知识点（同步至 docs/knowledge/）

- 06-testing.md：probe.go 单文件诊断模式
- 09-observability.md：嵌套 timeout 链路诊断
- 10-security-and-compliance.md：HTTPS 证书自动续期 + Let's Encrypt
- 04-docker.md：Docker 复用既有项目容器
- 13-deployment.md（新建）：Plan 10 部署完整流水线

---

## 🔖 明天接力卡

**一句话给新会话**：
> Plan 10 部署还差最后一公里 —— COS ap-shanghai 大文件上传从香港 TLS 卡死。看 `docs/devlog/2026-05-18-plan-10-deploy-day1.md` "明天怎么走"。

### 必修：解决 COS 大文件上传

3 个候选方案，按优先级：

#### 方案 A（推荐）：换 COS bucket 到广州或香港 region

腾讯云控制台 → COS → 新建 bucket `aibao-audio-prod-{appid}`：
- region 选 **ap-guangzhou**（广州，距香港 ~150ms）或 **ap-hongkong**（香港，0 跨地域）
- 复制 dev bucket 的访问策略
- 更新 `/etc/aibao/env` 的 `AIBAO_STORAGE_COS_BUCKET` + `AIBAO_STORAGE_COS_REGION`
- 重启 aibao
- 复活 outbox 3 个 events 看是否 ready
- 工作量 30 分钟

#### 方案 B：开启 COS 全球加速

腾讯云 COS 控制台 → 当前 bucket → "全球加速" → 开启。
访问域名变成 `aibao-audio-dev-{appid}.cos.accelerate.myqcloud.com`，绕开跨地域慢链路。
但加速域名按流量额外计费 ~0.2 元/GB。
更新 cos.go 的 host 拼接逻辑。

#### 方案 C：换部署服务器到内地

放弃香港，买腾讯云上海/北京/广州轻量服务器 + 重新部署。
但你那台 OPPO 测试机和你电脑都在内地，香港服务器其实更接近"全球可访问"目标。**不推荐**。

**我建议方案 A**——最快、最便宜、跟现有架构一致。

### 验证步骤（A 方案做完后）

1. SSH 到服务器：`/var/www/aibao` 现有 APK 不动（APK 静态文件不走 COS）
2. 改 env：`/etc/aibao/env` 的 BUCKET / REGION
3. `systemctl restart aibao`
4. 复活 outbox：`UPDATE outbox_events SET status='pending', attempts=0 WHERE event_type='tts_synthesis'`
5. 等 60s
6. 查 stories audio_status 是否 ready
7. 手机 App 测听一个故事

如果方案 A 通——Plan 10 100% 完成。
如果还不通——继续诊断网络层（traceroute、mtr、ping packet size）。

### 启动环境（明天 OrcaTerm 进服务器后）

```bash
# 1. 进项目目录
cd /opt/aibao_app

# 2. 拉最新代码（如果今天本机有改动）
git pull

# 3. 看服务状态
systemctl status aibao
docker ps

# 4. 实时日志
journalctl -u aibao -f
```

### Plan 10 关键文件锚点

- `/etc/aibao/env` — 12 个密钥（root only 0600）
- `/etc/systemd/system/aibao.service` — systemd unit
- `/etc/nginx/sites-enabled/aibao` — nginx 反代配置
- `/opt/aibao_app/server/config/config.prod.yaml` — 后端生产配置
- `/var/www/aibao/index.html` — 落地页
- `/var/www/aibao/download/app-release.apk` — APK 50MB
- `/var/www/aibao/qrcode.png` — 二维码

### 备份 / 回滚

如果方案 A 把生产搞挂，回滚：
```bash
# /etc/aibao/env 把 BUCKET / REGION 改回原值
systemctl restart aibao
```
（除了 env 没改其他东西，回滚成本极低）
