# 开发日志 — 2026-05-21（Plan 10 部署上线 · Day 2）

> 接 [Day 1](2026-05-18-plan-10-deploy-day1.md)。Day 1 完成度 92%，唯一卡点是 COS 音频上传。本篇把它修完——**Plan 10 部署 100% 完成**。

## 一句话

困了两天的 COS 上传，是一条"三层洋葱"bug：**网络层 → 权限层 → 操作层**，每剥一层冒出下一层。全部修完后 3 个故事音频一次性全部 `ready`。

## 故障链：三层剥洋葱

### 第一层：网络层 — 跨地域 TLS 卡死

Day 1 已诊断：香港主机 → `cos.ap-shanghai` 大文件 PUT 在 TLS handshake 卡死（5MB 20s timeout，小文件 300ms 秒过）。

**修法**：换 COS bucket 到同地域。新建 `aibao-audio-hk-1356733768`，region `ap-hongkong`。
改 `/etc/aibao/env` 的 `AIBAO_STORAGE_COS_BUCKET` + `AIBAO_STORAGE_COS_REGION`，重启 aibao。

**结果**：TLS 卡死**消失**——`audio.compose.ok` 后上传请求一路打到 COS 服务器，握手秒过。✅ 网络层修复。

### 第二层：权限层 — 403 AccessDenied

新桶冒出新错误：`HTTP 403 <Code>AccessDenied</Code>`。

🎓 **诊断关键**：错误码是 `AccessDenied`，**不是** `InvalidAccessKeyId`、**不是** `SignatureDoesNotMatch`。这个组合精确排除了"密钥配错"和"签名算错"——COS 认得这把密钥、也认可签名，**纯粹是这把密钥没有这个新桶的访问权限**。

写了一段裸 Python（HMAC-SHA1 手算 COS V5 签名）直接对新桶发 GET，确认 403 不是后端代码问题，是密钥本身的归属问题。

**根因**：env 里的密钥 `AKIDGc...` 属于**另一个账号**；新桶 `aibao-audio-hk` 建在主账号「Sun God」（APPID `1356733768`）名下。两个账号，所以 403。旧桶 `aibao-audio-dev` 当初能用，是因为旧桶在那个 `AKIDGc` 账号下或被显式授权过。

**修法**：换成新桶持有者主账号的密钥。

### 第三层：操作层 — SecretKey 不可二次查看

去主账号「访问管理 → API 密钥管理」拿密钥时撞墙：

🎓 **腾讯云 SecretKey 只在「新建密钥」那一刻显示一次，之后永久不可查看**（安全设计，防反复偷看）。主账号那把 `AKIDz1...` 是两个月前建的，早过窗口期。

**修法**：新建一把密钥 `AKIDxuhG...`，弹窗当场拿到 SecretKey，立即用掉。
（注：一个腾讯云主账号最多 2 把密钥，新建后到上限；老的 `AKIDz1...` SecretKey 已丢失，可日后禁用。）

## 收尾命令

```bash
sed -i 's#^AIBAO_STORAGE_COS_SECRET_ID=.*#...AKIDxuhG...#' /etc/aibao/env
sed -i 's#^AIBAO_STORAGE_COS_SECRET_KEY=.*#...#' /etc/aibao/env
systemctl restart aibao
# 复活 3 个失败的 outbox 事件
UPDATE outbox_events SET status='pending', attempts=0, next_attempt_at=NOW()
  WHERE event_type='tts_synthesis' AND status IN ('processing','dead','failed');
```

## 验证结果 ✅

三个故事全部翻盘：

| story id | audio_status | 时长 |
|---|---|---|
| 3 | **ready** | 391s（≈6:31） |
| 2 | **ready** | 203s（≈3:23） |
| 1 | **ready** | 169s（≈2:49） |

日志里 `audio.compose.ok` 后**紧跟** `storage.upload.ok`，3 个文件全部上传成功，再无 `handle_failed`。

**Plan 10 部署 100% 完成。** 端到端闭环全通：手机浏览器下载 APK → 安装 → 登录 → 文本生成 → TTS 合成 → COS 上传 → 签名 URL → 播放。

## 关键教训

1. **多层故障要按"改一刀→看上游原始回复→再改一刀"的节奏走**。这条 bug 链三层，每层修对了才暴露下一层；指望一次改完只会瞎猜。
2. **云存储 4xx 错误码要分清**：`InvalidAccessKeyId`（密钥不存在/错）≠ `SignatureDoesNotMatch`（密钥对但签名算错）≠ `AccessDenied`（密钥对、签名对、但无此资源权限）。三者指向完全不同的修法。看错误码比看 Message 更可靠。
3. **跨账号资源陷阱**：COS 桶按 APPID 归属。新桶名尾号 = 持有账号 APPID，这是免费的归属校验线索（新桶 `...-1356733768` ↔ 主账号 APPID `1356733768`）。换桶时密钥也要跟着换到同一个账号。
4. **云厂商 SecretKey 普遍只显示一次**（腾讯云、AWS、阿里云同理）。新建时务必当场存好（下载 CSV / 立即写进配置），关掉弹窗就再也拿不回来——只能作废重建。
5. **对象存储和计算资源尽量同地域**：跨地域不只是慢，大流量 PUT 还可能在 TLS 层直接卡死。部署前就该让 COS region = 服务器 region。

## 涉及到的知识点（同步至 docs/knowledge/）

- 10-security-and-compliance.md：云存储 4xx 错误码三分法（AccessDenied vs InvalidAccessKeyId vs SignatureDoesNotMatch）；SecretKey 一次性显示
- 13-deployment.md：对象存储与计算资源同地域原则

## Plan 10 总结（Day 1 + Day 2）

腾讯云香港轻量服务器，从零到端到端可演示：
- 基础设施：复用前项目 Docker PG/Redis + Go 1.23 + golang-migrate + systemd + Nginx + Let's Encrypt
- 域名 `aibao.dhgames.com`（子域名跳过备案）
- release APK 50MB 在线下载（proguard ExoPlayer keep 规则 + R8 minify）
- COS 音频私有桶 `aibao-audio-hk`（ap-hongkong，与服务器同地域）
- 完整闭环：下载 → 安装 → 登录 → 生成 → TTS → COS → 播放，真机验证通过

朋友现在可以拿手机直接试用爱宝了。
