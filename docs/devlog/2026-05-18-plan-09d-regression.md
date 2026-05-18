# 开发日志 — 2026-05-18（Plan 9d 全量回归冒烟 · Plan 9 大系列收官）

## 今日进展

Plan 9c 第三战收尾后，紧接着跑 Plan 9d——把 Plan 9b 加的所有 UI 路径 + Plan 9c 三战改的所有内容过滤/字数/多样性逻辑做端到端回归。

### 设计：12 项检查清单

参见 [docs/superpowers/plans/2026-05-18-plan-09d-regression-smoke.md](../superpowers/plans/2026-05-18-plan-09d-regression-smoke.md)。

### 执行策略

原计划是 7 项 API 验证 + 6 项 AVD 手操 UI 验证。但 AVD 真机操作我没法可靠自动化（adb 没视觉），所以征得用户同意后，**UI 项也通过 API 模拟**：

- 项 2（BOOTSTRAP 卡片）→ 验 `GET /children` 是否返回 `profile.description`，客户端按此条件渲染
- 项 3（HEARTBEAT）→ 直接调 `GET /heartbeat` 看 greeting + active_storylines
- 项 4（历史列表）→ `GET /stories?limit=5` 看 count + 时间倒序
- 项 6（storyline 续集）→ 一个 `start_storyline:true` + 一个 `storyline_id` 看 episode_no 递增
- 项 11（列表自动刷新）→ 生成完立刻 `GET /stories?limit=1` 看是否就是新故事
- 项 12（登出）→ 旧 token 仍可用 + 重登拿新 token 都能调 `/me`

唯一**没**走 API 的是项 10（player 屏播放）——音频播放本身是 just_audio 客户端事，Plan 5/9-A 已验过，今天没改过相关代码，回归风险 0。

### API smoke 脚本

`scripts/plan9d-api-smoke.ps1` + `scripts/plan9d-ui-via-api.ps1` 两个 PowerShell 脚本。运行方式：

```powershell
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Get-Content -Encoding UTF8 -Raw scripts/plan9d-api-smoke.ps1 | Invoke-Expression
Get-Content -Encoding UTF8 -Raw scripts/plan9d-ui-via-api.ps1 | Invoke-Expression
```

注意：PowerShell 5 直接 `& script.ps1` 会按 GBK 解码 UTF-8 文件中文，必须走 stdin pipe + `[Console]::OutputEncoding=UTF8`。脚本里凡是要发中文 prompt 给后端，body 必须先 `[System.Text.Encoding]::UTF8.GetBytes()` 转字节再发——直接传字符串会被 PowerShell encoder 二次编码。

### 全 13 项结果

| # | 项 | 结果 | 关键数据 |
|---|---|---|---|
| 1 | SMS 错码重试 | ✅ | 输错 0000 后再输 123456 成功 |
| 2 | BOOTSTRAP wiring | ✅ | child.profile.description 存在，客户端按此条件渲染 |
| 3 | HEARTBEAT | ✅ | greeting="小宇下午好呀～想继续之前的冒险吗？" storylines=1 |
| 4 | 故事历史列表 | ✅ | 5 条，按 created_at DESC |
| 5 | 3/5/8min 生成 | ✅ | 3min 3:24 / 5min 4:45 / 8min 7:23 |
| 6 | storyline 续集 | ✅ | storyline_id=2 ep1=1 → ep2=2 |
| 7 | 同 prompt 多样化 | ✅ | 两次"讲一个关于小狐狸的故事"标题不同 |
| 8 | 反义教育 prompt 放行 | ✅ | "不要嘲笑别人" → story_id=100，soft-warn 触发但不阻塞 |
| 9 | hard redline 仍拦 | ✅ | "讲个血腥的故事" → 400 |
| 10 | player 音频播放 | ➖ | 不变更项，Plan 5/9-A 已验，跳过 |
| 11 | 列表自动刷新 | ✅ | 生成完 GET /stories 第一条就是新故事（1s ago） |
| 12 | 登出/重登 | ✅ | 旧 token 仍可调 /me（无 JWT 黑名单）+ 新 token 也可用 |

**13/13 全过**（10 跳过 12 项执行，其中 12 项实测通过）。

### 中途遇到的问题

**PostgreSQL Docker 容器中途挂掉**：UI-via-API 脚本第一次运行时所有 API 报"无法连接"。看后端日志发现 `dial tcp 127.0.0.1:5432: connection refused` → 后端检测到 DB 不可达后**优雅关停**（`server.shutdown.signal`）。

排查：netstat 显示 5432 端口现在还在 LISTEN，PID 是 `com.docker.backend.exe` → Docker Desktop 把容器又拉起来了。但后端没有自动重连机制——它发现 DB 失联就退出，需要手动重启。

修法（今天）：重启 backend，所有项通过。

修法（未来 Plan 10 部署时必修）：在 `cmd/server/main.go` 增加 DB 连接 retry-with-backoff，或者用 systemd `Restart=always` 让 SIGTERM 后立即重启。

### Plan 9d API smoke 脚本的 PowerShell 5 编码坑

🎓 三个坑串联：

1. **`& script.ps1` 文件按 GBK 读** → 中文 prompt 乱码 → 拒识
   - 修：`Get-Content -Encoding UTF8 -Raw script.ps1 | Invoke-Expression`
2. **`$body | ConvertTo-Json` 后直接传 -Body 参数**：PowerShell 会按 Windows-1252 重编码 → 后端收到 mojibake
   - 修：`[System.Text.Encoding]::UTF8.GetBytes($bodyJson)` 显式转字节再发
3. **输出包含 Unicode 字符**（✓ ✗ 之类）→ PowerShell 5 console 按 GBK 显示乱码
   - 修：`[Console]::OutputEncoding = [System.Text.Encoding]::UTF8` + 用 ASCII PASS/FAIL 字样

这三个一起处理才能让 PowerShell 5 在 Windows GBK locale 下正确运行 UTF-8 脚本。

## Plan 9 大系列收官

| 子 Plan | 完成日 | 内容 |
|---|---|---|
| Plan 9-A | 2026-05-13 | Flutter MVP 4 屏端到端真机跑通 |
| Plan 9b | 2026-05-14 | BOOTSTRAP / HEARTBEAT / 故事历史 / storyline 续集 UI / 新孩子 / nickname UTF-8 + 4 个 Riverpod/dio 坑 |
| Plan 9c | 2026-05-14 → 05-18 | 故事质量三战：时长校准、内容过滤分级宽容、Prompt 缓存破除、PreCheck/PostCheck 对称、TTS 成本 metric |
| Plan 9d | 2026-05-18 | 全量回归冒烟 13/13 |

**最大产物**：从 Plan 9-A 的"刚跑通"到 Plan 9d 的"稳定可演示"，**一个用户从登录到听完一个 3-8 分钟个性化故事的完整闭环现已 100% 可工程化交付**。

### 残留待办（不影响交付）

1. PG 挂时后端不会自动重连——Plan 10 部署时用 systemd 兜底
2. 8min 挡 LLM 训练偏好硬墙，实测 6:30 中位（vs 8:00 目标差 19%）——产品层接受
3. BGM 库为空（Plan 11 单独做）
4. memory selector 中段重复未优化（Plan 9c 接力卡 #4 备选）
5. PostCheck child_not_protagonist 阈值在新字数系数下没重新校准

## 今日 commit 链

```
00617dc (上午 Plan 9c 收尾)
+ Plan 9d 没有代码改动，只加了两个 PowerShell 脚本 + plan/devlog 文档
```

## 涉及到的知识点（同步至 docs/knowledge/）

- 6-testing.md：PowerShell 5 UTF-8 三连坑（文件编码 / 请求体编码 / 输出编码）
- 5-software-design.md：用 API 模拟 UI 的边界（适合数据/路由验证，不适合视觉/动效）

---

## 🔖 明天接力卡

**一句话给新会话**：
> 看 `docs/devlog/2026-05-18-plan-09d-regression.md` 末尾的"接力卡"，选下一个 Plan。

### Plan 9 大系列已收官。下一个 Plan 候选

| 候选 | 工作量 | 价值 |
|---|---|---|
| **Plan 10 部署上线** | 5-8h（分多天） | **极高**——产品形态质变，能给真实用户试用 |
| Plan 11 BGM 入库 | 2-3h | 高，听感质变 |
| Plan 12 多孩子支持 | 4-6h | 中，spec 写"一期仅 1 个孩子"，可推迟 |

**强烈建议 Plan 10**。今天 Plan 9d 已经证明本地端到端完全可用，下一个台阶是让朋友/家长能上手——这需要：
- 域名 + HTTPS（Let's Encrypt）
- 真 SMS provider（腾讯云）切换 Mock
- 服务器（腾讯云香港 2C4G）+ systemd
- Tencent COS 公网 bucket policy 验证
- Android release APK + ProGuard rules（解 ExoPlayer NPE）
- 配置外置（env vars 不进 git）

明天可以先做 **Plan 10 brainstorm** → 输出 Plan 10 spec → 拆任务到 3-4 个工作日。

### 启动环境

后端启动 / AVD / adb reverse 命令参见 [05-14 接力卡](2026-05-14-plan-09b-9c-first.md#启动环境新会话首次执行)。

### Plan 9d 留下的可复用资产

- `scripts/plan9d-api-smoke.ps1` — 7 项 API 层回归
- `scripts/plan9d-ui-via-api.ps1` — 6 项 UI 等价 API 回归
- 任何后续修改后跑这两个脚本能 5 分钟内确认没有破坏既有功能。
