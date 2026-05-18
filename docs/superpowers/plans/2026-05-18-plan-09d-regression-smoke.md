# Plan 9d — 端到端回归冒烟（Plan 9b/9c 全量验收）

**目标**：在 Pixel 7 AVD 上把 Plan 9b 加的 UI + Plan 9c 三战改的所有内容过滤 / 字数 / 多样性逻辑跑一遍，确认没有回归。

**前置**：Plan 9c 第三战已经完成（commit `00617dc`）。今天的代码 + 配置应该已经全部生效。

## 12 项检查清单

| # | 功能 | 走法 | 验证点 | 关联 |
|---|---|---|---|---|
| 1 | 登录（SMS Peek+Consume） | 输错验证码 1 次 + 再输对 | 不再"已过期" | Plan 9b commit `2488d85` |
| 2 | BOOTSTRAP 卡片 | 删 child.profile.description 重登 | 首页提示卡片出现 | Plan 9b |
| 3 | HEARTBEAT 问候 | 进首页 | 时段问候 + 活跃 storyline 列表 | Plan 9b |
| 4 | 故事历史列表 | 首页"最近听过" | 5 条按时间倒序 | Plan 9b |
| 5 | 生成 3/5/8 分钟 | 各生成 1 个 | 挡位正确 + 实际时长在 ±20% | Plan 9c 第三战 (5min 4:30-5:30 / 8min 6:00-7:30 接受) |
| 6 | 生成 storyline 续集 | 点续集卡 → 生成 | episode_no +1，续集 #N 标识 | Plan 9c 第二战 (warn-only) |
| 7 | 同 prompt 不重复 | 同 prompt 生成 2 次 | 故事不同 | Plan 9c 第二战 commit `3231d8f` |
| 8 | 反义教育 prompt | "不要嘲笑别人" | 不报 400 | Plan 9c 第三战 commit `f26257a` |
| 9 | 红线词正确拦截 | "讲个血腥的故事" | PreCheck 拒绝 400 | 验证 hard category 仍然 hard-fail |
| 10 | player 屏播放 | 进 player | 文本 + 音频可播 + 续集标识 | Plan 9b |
| 11 | 列表自动刷新 | 生成完返回首页 | 新故事立刻在最近听过 | Plan 9b commit `1c41cf6` |
| 12 | 登出 | 点登出按钮 | 回到登录页，孩子档案清空 | Plan 9b commit `bc13d2c` |

## 执行方式

**API 层验证**（PowerShell 批量，快）：项 1、5、7、8、9
**UI 层验证**（在 AVD 手动操作）：项 2、3、4、6、10、11、12

## 通过标准

- 12/12 项实测通过
- 后端日志无 panic / error / unexpected fallback
- 没有发现需要修复的 bug

## 不通过时

记录到当天 devlog，按问题修，必要时分裂为 Plan 9d.x 子任务。

## 完成后

devlog 写 batch 总结表（每项耗时 + 备注），关闭 Plan 9c 全战。
下一步进入 Plan 10（部署上线）或 Plan 11（BGM 入库）。
