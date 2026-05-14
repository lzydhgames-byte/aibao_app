# Plan 9b：Flutter 客户端 UI 补全（BOOTSTRAP / HEARTBEAT / 历史 / 续集 / 新孩子 / nickname 修）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (推荐) 或 superpowers:executing-plans 按 Task 顺序逐条执行。步骤使用 `- [ ]` checkbox 语法跟踪。Plan 9-A 已上线 4 屏最小闭环（login / home / generate / player），但 home 屏是占位卡片、新孩子档案要 PowerShell 建、BOOTSTRAP 问卷没接 UI、HEARTBEAT 完全没用、故事历史看不到、storyline 续集没入口。Plan 9b 把这些坑全部补齐。**实施者注意：本项目用户没有软件工程基础**——每个新出现的 Flutter / Dart / Riverpod / go_router 概念**必须**用 🎓 段落生活类比解释，并追加到 `docs/knowledge/12-flutter.md`（含"为什么需要"段）。详见 CLAUDE.md §7。

**Goal:** 在 Plan 9-A 基础上把 Flutter 客户端补到"中档可用"水平——补 6 件事 + 1 个后端小修。①首页改成"HEARTBEAT 问候卡 + 孩子卡 + 活跃连续剧 + 故事历史 + 浮动 CTA"五区段；②`child.profile.description` 为空时引导走 BOOTSTRAP 7 题问卷；③首次登录无孩子时强制走"新建孩子"表单；④"活跃连续剧"卡片可一键续集；⑤故事历史 5 条 ListTile，点击进 player 重放；⑥dio 401 拦截器自动登出回登录屏；⑦后端 `auth.go login_or_register` 补 UTF-8 nickname 校验（复用 Plan 6b §6.13 模式）+ 新增 `GET /api/v1/stories?child_id=N&limit=5` 故事历史接口。完成后，任何家长在 Android 手机上从首次启动到听完一集续集，全程无需开发者后台帮忙。

**Architecture:** 客户端继续沿用 Plan 9-A 三层结构 `api/` / `state/` / `screens/`，但 home 屏从单一卡片演化为"分区组合屏"——HeartbeatCard / ChildCard / StorylineCarousel / StoryHistoryList / BootstrapPromptCard 五个 widget 拼装。每个分区独立 `AsyncValue.when(...)` 处理 loading / error / empty，互不阻塞。新增 Riverpod providers：`heartbeatProvider`（`FutureProvider.family<HeartbeatResponse, int>`，按 childId 缓存）、`storyListProvider`（`FutureProvider.family<List<StoryListItem>, int>`）、`bootstrapProvider`（`StateNotifierProvider<BootstrapNotifier, BootstrapState>` 持 7 题答案 + submit）。路由层在 `router.dart` 新增 `/onboarding/create-child`、`/onboarding/bootstrap`，并扩展 `auth_state` redirect 逻辑——孩子档案不存在时强制跳 create-child；`profile.description` 为空时**软提示**（home 顶部黄条），不阻塞使用。后端只动 2 个文件：`server/internal/api/auth.go`（复用 `child.go` raw-body + `utf8.Valid` 模式）+ `server/internal/api/story.go` 新增 `list` handler（已有 `StoryRepo.ListByChild` 若缺则在 `repository/story_repo.go` 补一个方法 + 测试）。

**Tech Stack:**（沿用 Plan 9-A，无新依赖）
- Flutter 3.29.3 + Dart 3.7（Material 3 + seed `0xFF2E7D32`）
- `flutter_riverpod ^2.5` + `dio ^5.7` + `go_router ^14.6` + `just_audio ^0.9.40` + `flutter_secure_storage ^9.2`
- 测试：`http_mock_adapter ^0.6` + `mocktail ^1.0`（仅 ApiClient + BootstrapNotifier 单测；UI test 不强求）
- 目标平台：Android only（与 9-A 相同）；iOS / Web / release proguard / 暗色模式 OUT

**前置阅读：**
- Plan 9-A：[2026-05-15-plan-09a-flutter-mvp-demo.md](2026-05-15-plan-09a-flutter-mvp-demo.md) —— 客户端架构基础与 4 屏现状
- 2026-05-16 devlog：[../../devlog/2026-05-16.md](../../devlog/2026-05-16.md) —— 9-A 收官 known issues 与 9b 触发条件
- Plan 6：[2026-05-11-plan-06-bootstrap-and-memory.md](2026-05-11-plan-06-bootstrap-and-memory.md) —— BOOTSTRAP 后端 7 题问卷接口形态
- Plan 6b §6.13：[2026-05-12-plan-06b-known-issue-fixes.md](2026-05-12-plan-06b-known-issue-fixes.md) —— raw-body-then-unmarshal UTF-8 校验模式（auth nickname 修要复用）
- Plan 8：[2026-05-14-plan-08-storyline-and-heartbeat.md](2026-05-14-plan-08-storyline-and-heartbeat.md) —— HEARTBEAT + storyline 后端接口
- 后端源码：
  - `server/internal/service/bootstrap/questions.go` —— 7 题定义（ID / type / options 白名单）
  - `server/internal/api/bootstrap.go` —— `POST /api/v1/bootstrap/answers` 请求体
  - `server/internal/api/heartbeat.go` —— `GET /api/v1/heartbeat` 响应格式
  - `server/internal/api/story.go` —— `POST /stories/generate`（`start_storyline` / `storyline_id` 字段）+ 新增 `GET /stories` handler
  - `server/internal/api/auth.go` —— `login_or_register` 当前缺 UTF-8 校验
  - `server/internal/api/child.go` —— 已有 UTF-8 校验 raw-body 模式（Plan 6b Task 6），照抄
- 客户端源码：
  - `app/lib/api/api_client.dart` —— 9-A ApiClient（注意：用 `TokenStorage` 抽象 + `LoginResult` DTO）
  - `app/lib/state/auth_state.dart` —— 现状使用 sealed states `AuthInitial / AuthUnauthenticated / AuthAuthenticated / AuthError`
  - `app/lib/router.dart` —— go_router 配置 + `_AuthListenable` 桥接
- 协作规则：[../../../CLAUDE.md](../../../CLAUDE.md) §7"边做边学"——**硬要求**
- 知识库：
  - `docs/knowledge/12-flutter.md` —— Flutter 主题词条（form 设计、go_router 参数、AsyncValue.when 等本 Plan 新增词条要追加）
  - `docs/knowledge/06-testing.md` §6.13 —— 入口校验必须在 JSON unmarshal 之前

---

## 完成验收（Definition of Done）

### 后端（2 项）
1. **nickname UTF-8 修**：用 PowerShell 发 `Content-Type: application/json` + GBK 字节的 `nickname` → 返回 400 `{"reason":"invalid_nickname","user_msg":"昵称包含非法字节，请确保为 UTF-8"}`；合法 UTF-8 nickname → 200 正常登录。
2. **GET /stories list**：携带 JWT 调 `GET /api/v1/stories?child_id=3&limit=5` → 返回 `{"items":[...]}`，按 `created_at DESC` 排序；`child_id` 不属于当前 user → 403 `not_owner`；不带 child_id → 400；`limit` > 50 → 服务端强制夹到 50。

### 客户端（6 项）
3. **新孩子创建闭环**：清空 `children` 表后用 emulator 登录 → 自动跳 `/onboarding/create-child` → 填 nickname/gender/birthday 提交 → 回 `/home` 看到该孩子卡片。
4. **BOOTSTRAP 闭环**：新建孩子的 `profile.description` 为空 → home 顶部显黄色"完善小宇的画像"提示卡 → 点击进 `/onboarding/bootstrap` → 答完 7 题提交 → 后端日志可见 LLM 润色描述写入 `children.profile.description` → 回 home 黄条消失。
5. **HEARTBEAT 闭环**：home 顶部显时段问候（早/中/午/晚/夜深 5 段）"小宇早上好呀～"；有活跃 storyline 时附加"想继续之前的冒险吗？"。
6. **故事历史闭环**：生成 2 集后 home 中部显"最近听过" 5 条 ListTile（含 audio_status 徽章 pending/ready/failed）；点击 ready 的 → 跳 `/player/:id` 直接 setUrl 播放（不重新生成）。
7. **storyline 续集闭环**：home 显"活跃连续剧"水平卡片（标题 + 集数 + 下集预告）；点击"继续"→ `/generate?storylineId=N` 默认 prompt"继续上一集"+ 头部显 "📖 续集 #X" 徽章 → 提交后端 `POST /generate` 带 `storyline_id` → ep2 生成成功。
8. **401 自动登出**：用 PowerShell 手动到 redis / DB 把当前 access_token 失效（或改 JWT secret 重启后端）→ 在 home 触发任意请求 → toast"登录已过期"→ 自动跳 `/login`。

### 工程纪律
9. `cd app && flutter analyze` 输出 `No issues found!`；`flutter test` 全过（ApiClient + BootstrapNotifier + UTF-8 校验后端 Go 测试）。
10. 所有新出现的 Flutter / Dart / Riverpod / go_router 概念都已追加到 `docs/knowledge/12-flutter.md` 含"为什么需要"段（Task 15 验收）。
11. `docs/devlog/2026-05-16.md` 或新建 `2026-05-17.md` 记录 9b 收官 + known issues + 下一步建议。

---

## 范围决策记录（与用户对齐 —— 不要再争论）

| 维度 | 决策 |
|---|---|
| BOOTSTRAP 触发 | `child.profile['description']` 为空 → home 顶部 prompt 卡（软提示，不阻塞）→ 点进 7 题屏 |
| BOOTSTRAP UI 形式 | 一页滚动（不分页），每题独立卡片，按 `question.type` 分支渲染 |
| HEARTBEAT 缓存 | 进 home 时拉一次，pull-to-refresh 重拉；不做后台轮询 |
| 故事历史 | 5 条，按 `created_at DESC`；不分页；点 ready 直进 player；点 pending 也进 player（轮询 audio_url） |
| storyline 续集触发 | 复用 `/heartbeat` 的 `active_storylines`（不另起新接口） |
| 续集 prompt 默认 | "继续上一集"——用户可以改；UI 头部显"📖 续集 #X"徽章（X = 下一集集号） |
| 新孩子创建 | 登录后 child 列表空 → 强制跳 `/onboarding/create-child`（不可跳过）；只能建 1 个（后端 `UNIQUE(user_id)`）  |
| nickname UTF-8 修 | 仅修 `login_or_register` 入口；复用 `child.go` raw-body + `utf8.Valid` 模式 |
| 401 拦截 | dio onResponse 中检测 401 → 调注入的 `onUnauthorized` callback → main.dart 桥接到 `authProvider.notifier.logout()` → router redirect 自动跳 /login。`/auth/*` 路径**绕过**（登录中不触发） |
| iOS / release proguard / 暗色 / 录音 | OUT（9c+） |
| 错误处理 | toast + 内联条；不做错误屏；网络错也 toast |
| 故事列表分页 | OUT（`limit ≤ 50` 硬上限） |
| 多孩子档案 | OUT（一期 1 user 1 child 不变） |
| 启用/关闭 storyline 切换 UI | OUT（仅 BOOTSTRAP `enable_storyline` 一次性设置） |

---

## File Structure（增量；不动的不列）

```
app/
├── lib/
│   ├── api/
│   │   ├── api_client.dart                  ← 修改：+listStories +401 拦截器 +onUnauthorized callback
│   │   └── models/
│   │       ├── story.dart                   ← 修改：+StoryListItem 类（含 audio_status 用于徽章）
│   │       └── heartbeat.dart               ← 新建：HeartbeatResponse + ActiveStoryline
│   ├── state/
│   │   ├── child_state.dart                 ← 修改：+hasProfileDescription getter +createChild method
│   │   ├── heartbeat_state.dart             ← 新建：FutureProvider.family<HeartbeatResponse, int>
│   │   ├── story_list_state.dart            ← 新建：FutureProvider.family<List<StoryListItem>, int>
│   │   └── bootstrap_state.dart             ← 新建：BootstrapNotifier 持 7 题答案 + submit
│   ├── screens/
│   │   ├── home_screen.dart                 ← 重写：5 区段分区组合屏
│   │   ├── create_child_screen.dart         ← 新建：nickname/gender/birthday 表单
│   │   ├── bootstrap_screen.dart            ← 新建：7 题一页滚动
│   │   ├── generate_screen.dart             ← 修改：接收 storylineId query param + 续集徽章
│   │   └── player_screen.dart               ← 微调：从历史点入时如果 audio ready 直接 setUrl
│   ├── widgets/
│   │   ├── heartbeat_card.dart              ← 新建
│   │   ├── storyline_carousel.dart          ← 新建
│   │   ├── story_history_list.dart          ← 新建
│   │   ├── bootstrap_prompt_card.dart       ← 新建（home 顶部"完善画像"提示）
│   │   ├── bootstrap_question_card.dart     ← 新建（按 type 分支渲染）
│   │   └── audio_status_badge.dart          ← 新建（pending/ready/failed 三态徽章）
│   ├── router.dart                          ← 修改：+/onboarding/* 路由 + 扩展 redirect
│   └── main.dart                            ← 修改：桥接 onUnauthorized → authNotifier.logout
└── test/
    ├── api_client_test.dart                 ← 修改：+listStories test +401 拦截测试
    ├── bootstrap_state_test.dart            ← 新建
    └── heartbeat_state_test.dart            ← 新建

server/
└── internal/
    ├── api/
    │   ├── auth.go                          ← 修改：raw-body + utf8.Valid 校验
    │   ├── auth_test.go                     ← 修改：+TestLoginOrRegister_NonUtf8Nickname
    │   ├── story.go                         ← 修改：+list handler + RegisterRoutes 注册
    │   └── story_test.go                    ← 修改：+TestList_OK/+NotOwner/+MissingChildID/+LimitClamped
    └── repository/
        └── story_repo.go                    ← 可能修改：+ListByChild method（若已有则不动）
```

---

## API 形态

### 1 个新后端接口

#### `GET /api/v1/stories` (Bearer JWT)
**Query 参数:**
- `child_id` (int64, **必需**)
- `limit` (int, 可选, 默认 5, 最大 50；超过强制夹到 50)
- `order` (string, 可选, 默认 `created_at_desc`；MVP 只支持这个值，其他值忽略)

**Response 200:**
```json
{
  "items": [
    {
      "id": 42,
      "title": "小宇的森林小冒险",
      "duration_minutes": 5,
      "style": "温馨治愈",
      "audio_status": "ready",
      "storyline_id": 7,
      "episode_no": 2,
      "created_at": "2026-05-16T08:23:45Z"
    },
    { "id": 41, ... }
  ]
}
```

**错误：**
- 400 `invalid_argument`：缺 `child_id` 或不合法
- 403 `not_owner`：`child_id` 不属于当前 user
- 404 `child_not_found`
- 401 `unauthorized`：无 JWT

### 7 个已有接口客户端调用 pattern

| 接口 | 触发屏 / 触发点 | 客户端 method |
|---|---|---|
| `POST /auth/sms/send` | LoginScreen | `api.sendSmsCode(phone)` |
| `POST /auth/login_or_register` | LoginScreen（**修：UTF-8 校验**） | `api.loginOrRegister(...)` |
| `GET /me` | AuthNotifier `_bootstrap` | `api.getMe()` |
| `POST /children` | CreateChildScreen 提交 | `api.createChild(...)` |
| `GET /children` | childProvider | `api.listChildren()` |
| `POST /stories/generate` | GenerateScreen（含 storylineId 续集） | `api.generateStory(..., storylineId: 7)` |
| `GET /stories/:id` + `GET /stories/:id/audio_url` | PlayerScreen | 不变 |
| `GET /bootstrap/questions` | BootstrapScreen init | **新增** `api.getBootstrapQuestions()` |
| `POST /bootstrap/answers` | BootstrapScreen submit | **新增** `api.submitBootstrapAnswers(...)` |
| `GET /heartbeat?child_id=N` | HomeScreen 顶部 | **新增** `api.getHeartbeat(childId)` |
| `GET /stories?child_id=N&limit=5` | HomeScreen 中部 | **新增** `api.listStories(childId, limit)` |

---

## 数据模型（客户端 Dart）

### `StoryListItem`（精简版 Story；不含 text）
```dart
class StoryListItem {
  final int id;
  final String title;
  final int durationMinutes;
  final String style;
  final String audioStatus; // pending / ready / failed
  final int? storylineId;
  final int? episodeNo;
  final DateTime createdAt;

  const StoryListItem({
    required this.id, required this.title,
    required this.durationMinutes, required this.style,
    required this.audioStatus,
    this.storylineId, this.episodeNo,
    required this.createdAt,
  });

  factory StoryListItem.fromJson(Map<String, dynamic> j) => StoryListItem(
        id: j['id'] as int,
        title: (j['title'] ?? '') as String,
        durationMinutes: j['duration_minutes'] as int,
        style: (j['style'] ?? '') as String,
        audioStatus: (j['audio_status'] ?? 'pending') as String,
        storylineId: j['storyline_id'] as int?,
        episodeNo: j['episode_no'] as int?,
        createdAt: DateTime.parse(j['created_at'] as String),
      );
}
```

### `HeartbeatResponse` + `ActiveStoryline`
```dart
class ActiveStoryline {
  final int id;
  final String title;
  final int episodeCount;
  final String? nextHint;
  final DateTime? lastEpisodeAt;

  const ActiveStoryline({
    required this.id, required this.title, required this.episodeCount,
    this.nextHint, this.lastEpisodeAt,
  });

  factory ActiveStoryline.fromJson(Map<String, dynamic> j) => ActiveStoryline(
        id: j['id'] as int,
        title: (j['title'] ?? '') as String,
        episodeCount: (j['episode_count'] ?? 0) as int,
        nextHint: j['next_hint'] as String?,
        lastEpisodeAt: j['last_episode_at'] == null
            ? null
            : DateTime.parse(j['last_episode_at'] as String),
      );
}

class HeartbeatResponse {
  final String greeting;
  final List<ActiveStoryline> activeStorylines;
  const HeartbeatResponse({required this.greeting, required this.activeStorylines});
  factory HeartbeatResponse.fromJson(Map<String, dynamic> j) => HeartbeatResponse(
        greeting: (j['greeting'] ?? '') as String,
        activeStorylines: ((j['active_storylines'] ?? []) as List)
            .cast<Map<String, dynamic>>()
            .map(ActiveStoryline.fromJson)
            .toList(),
      );
}
```

### `BootstrapQuestion` + `BootstrapAnswer` + `BootstrapState`
```dart
enum BootstrapType { text, singleSelect, multiSelect, boolean }

class BootstrapQuestion {
  final String id;
  final String label;
  final BootstrapType type;
  final bool required;
  final List<String> options;
  final int maxLength;
  // fromJson: 把后端 string type 翻译成 enum
}

class BootstrapAnswer {
  final String questionId;
  /// text: String；single_select: String；multi_select: List<String>；boolean: bool
  final dynamic value;
}

class BootstrapState {
  final List<BootstrapQuestion> questions;
  final Map<String, BootstrapAnswer> answers; // key = questionId
  final bool submitting;
  final String? errorMsg;
  // copyWith ...
}
```

---

# Tasks

## Task 0：后端 `login_or_register` nickname UTF-8 校验

**Files:**
- Modify: `server/internal/api/auth.go`
- Modify: `server/internal/api/auth_test.go`

**前置教学（必读 + 写知识库）：**

🎓 **为什么需要 raw body + utf8.Valid 校验**：Go 的 `encoding/json` 在反序列化时，如果遇到非法 UTF-8 字节序列（比如 GBK 编码的中文），会**自动替换成 U+FFFD 替换字符**而不是报错——结果就是数据库里默默存了"乱码乱码"。如果不在 unmarshal 之前校验，一旦写入数据库，**回不去**。所以 Plan 6b §6.13 定的规矩是：**所有可能含中文的字符串字段，handler 入口必须先 `io.ReadAll(c.Request.Body)` + `utf8.Valid(raw)` 检查；通过后再 `c.Request.Body = io.NopCloser(bytes.NewReader(raw))` 让后续 `ShouldBindJSON` 能正常 unmarshal。** 追加到 `docs/knowledge/06-testing.md` §6.13 现有词条下补一个 "auth nickname 修补案例"。

- [ ] **Step 0.1：照 `child.go` create handler 重构 auth.go loginOrRegister**

将 auth.go 改造：
```go
import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "unicode/utf8"
    // ...
)

func (h *AuthHandler) loginOrRegister(c *gin.Context) {
    raw, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
        return
    }
    if !utf8.Valid(raw) {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_nickname", "user_msg": "昵称包含非法字节，请确保为 UTF-8"})
        return
    }
    var req loginOrRegisterReq
    if err := json.Unmarshal(raw, &req); err != nil {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
        return
    }
    c.Request.Body = io.NopCloser(bytes.NewReader(raw)) // 给后续 middleware 留口
    if req.Phone == "" || req.Code == "" {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
        return
    }
    // ... 后续 svc.LoginOrRegister 调用照旧
}
```

- [ ] **Step 0.2：sms/send 同步加 utf8 校验（防御性）**

`smsSend` handler 当前 phone 是数字，理论上不可能非 UTF-8，但为统一防御加一道——降低后续接口忘记加的概率。

- [ ] **Step 0.3：测试 `auth_test.go` 加 1 个 case**

```go
func TestLoginOrRegister_NonUtf8Nickname(t *testing.T) {
    // 构造 nickname 字段为 GBK "你好" = bytes {0xc4, 0xe3, 0xba, 0xc3}
    body := []byte(`{"phone":"13900000001","code":"123456","nickname":"`)
    body = append(body, 0xc4, 0xe3, 0xba, 0xc3)
    body = append(body, []byte(`"}`)...)
    req := httptest.NewRequest("POST", "/api/v1/auth/login_or_register", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    // ... assert 400 reason=invalid_nickname
}
```

- [ ] **Step 0.4：跑测试**

```powershell
cd f:\claud\aibao_app\server
go test ./internal/api/... -run TestLoginOrRegister -count=1
```

全过。

- [ ] **Step 0.5：commit**

```powershell
git add server/internal/api/auth.go server/internal/api/auth_test.go
git commit -m "fix(auth): nickname utf8 validation in login_or_register (reuse plan 6b 6.13 pattern)"
```

---

## Task 1：后端 `GET /stories` 故事列表接口

**Files:**
- Modify: `server/internal/api/story.go`
- Modify: `server/internal/api/story_test.go`
- Modify (or extend): `server/internal/repository/story_repo.go`（如缺 `ListByChild` 方法）

**前置教学：**

🎓 **ownership 校验为什么不能省**：URL 上的 `child_id` 是客户端传的——**任何登录用户都能改这个数字**。如果不校验"这个 child_id 真的属于当前 JWT 的 user"，就等于让 A 用户能拉 B 用户家娃的故事列表（横向越权 IDOR 漏洞）。规矩：**所有带 child_id query / path param 的 handler 入口都必须先查 child + assert `child.UserID == uid`**。追加到 `docs/knowledge/10-security-and-compliance.md`。

🎓 **limit 必须服务端夹**：客户端可能传 `limit=99999`——如果服务端不夹，攻击者一个请求就能把单 DB 查询拖到几秒。规矩：**任何分页接口服务端必须有 hard cap**，不能信客户端。

- [ ] **Step 1.1：检查/补 `StoryRepo.ListByChild`**

打开 `server/internal/repository/story_repo.go`，确认是否有 `ListByChild(ctx, childID, limit)` 方法。如缺：

```go
func (r *gormStoryRepo) ListByChild(ctx context.Context, childID int64, limit int) ([]*model.Story, error) {
    if limit <= 0 || limit > 50 { limit = 50 }
    var items []*model.Story
    err := r.db.WithContext(ctx).
        Where("child_id = ?", childID).
        Order("created_at DESC").
        Limit(limit).
        Find(&items).Error
    if err != nil { return nil, err }
    return items, nil
}
```

接口 `StoryRepo` interface 同步加这条签名。

- [ ] **Step 1.2：handler `list` 写入**

`server/internal/api/story.go` 加：

```go
func (h *StoryHandler) RegisterRoutes(g *gin.RouterGroup) {
    g.POST("/stories/generate", h.generate)
    g.GET("/stories", h.list)            // ← 新增（注意：必须在 /stories/:id 之前注册，否则 gin 路由可能误命中 :id）
    g.GET("/stories/:id", h.get)
}

func (h *StoryHandler) list(c *gin.Context) {
    uid, ok := userctx.FromContext(c.Request.Context())
    if !ok { RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录")); return }
    childIDStr := c.Query("child_id")
    if childIDStr == "" {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "缺少 child_id"}); return
    }
    childID, err := strconv.ParseInt(childIDStr, 10, 64)
    if err != nil {
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "child_id 不合法"}); return
    }
    limit := 5
    if s := c.Query("limit"); s != "" {
        if n, e := strconv.Atoi(s); e == nil { limit = n }
    }
    if limit <= 0 { limit = 5 }
    if limit > 50 { limit = 50 }

    // ownership 校验：必须先查 child
    ch, err := h.childRepo.FindByID(c.Request.Context(), childID) // 注入一个 child reader
    if err != nil { RespondError(c, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案")); return }
    if ch.UserID != uid { RespondError(c, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权访问")); return }

    items, err := h.repo.ListByChild(c.Request.Context(), childID, limit)
    if err != nil { RespondError(c, apperr.Wrap(err, apperr.CodeInternal, "list_failed", "服务暂时不可用")); return }

    out := make([]gin.H, 0, len(items))
    for _, s := range items {
        out = append(out, gin.H{
            "id":               s.ID,
            "title":            s.Title,
            "duration_minutes": s.DurationMinutes,
            "style":            s.Style,
            "audio_status":     s.AudioStatus,
            "storyline_id":     s.StorylineID,
            "episode_no":       s.EpisodeNo,
            "created_at":       s.CreatedAt,
        })
    }
    c.JSON(http.StatusOK, gin.H{"items": out})
}
```

⚠️ **结构调整**：`StoryHandler` 当前只持 `orch + repo`，需加 `childRepo` 字段——读 `heartbeat.go` 的 `HeartbeatChildReader` interface 模式，新建 `StoryChildReader` interface（只含 `FindByID`），构造函数加一个参数。`cmd/server/main.go` wire 处补一行注入。

- [ ] **Step 1.3：测试 `story_test.go` 加 4 个 case**

- `TestList_OK`：正常返回 N 条
- `TestList_MissingChildID`：400
- `TestList_NotOwner`：403
- `TestList_LimitClamped`：传 `limit=999` 实际返回 ≤ 50（用 mock repo 断言传入的 limit ≤ 50）

- [ ] **Step 1.4：跑测试 + 手动 PowerShell 冒烟**

```powershell
cd f:\claud\aibao_app\server
go test ./internal/api/... -run TestList -count=1
go run ./cmd/server
# 另开窗口：
$token = "<拿真的 JWT>"
Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/stories?child_id=3&limit=5" `
  -Headers @{Authorization="Bearer $token"}
```

- [ ] **Step 1.5：commit**

```powershell
git add server/internal/api/story.go server/internal/api/story_test.go server/internal/repository/story_repo.go server/cmd/server/main.go
git commit -m "feat(api): GET /stories list-by-child with ownership check + limit clamp"
```

---

## Task 2：客户端 DTO 新增

**Files:**
- Modify: `app/lib/api/models/story.dart`（+ `StoryListItem`）
- Create: `app/lib/api/models/heartbeat.dart`
- Create: `app/lib/api/models/bootstrap.dart`（`BootstrapQuestion` + `BootstrapAnswer` enum）

- [ ] **Step 2.1：story.dart 加 `StoryListItem`**

参见前文"数据模型"段，追加到 `app/lib/api/models/story.dart` 已有 `Story` 类下面。**不复用 Story 类**——Story 含 `text` 全文（长），列表场景 5 条 × 5KB 文本就是 25KB 流量浪费，分开两个 DTO。

🎓 教学（**为什么 list DTO 不复用详情 DTO**）：API 设计常见模式——list 返回精简字段，detail 返回完整字段。客户端也对应两个 model。理由：流量、移动端电量、列表渲染速度。追加到 `docs/knowledge/07-http-and-web.md`。

- [ ] **Step 2.2：heartbeat.dart 新建**

参见前文 `HeartbeatResponse` + `ActiveStoryline`。

- [ ] **Step 2.3：bootstrap.dart 新建**

```dart
enum BootstrapType { text, singleSelect, multiSelect, boolean }

BootstrapType _typeFromString(String s) => switch (s) {
      'text' => BootstrapType.text,
      'single_select' => BootstrapType.singleSelect,
      'multi_select' => BootstrapType.multiSelect,
      'boolean' => BootstrapType.boolean,
      _ => BootstrapType.text,
    };

class BootstrapQuestion {
  final String id;
  final String label;
  final BootstrapType type;
  final bool required;
  final List<String> options;
  final int maxLength;

  const BootstrapQuestion({
    required this.id, required this.label, required this.type,
    required this.required, required this.options, required this.maxLength,
  });

  factory BootstrapQuestion.fromJson(Map<String, dynamic> j) => BootstrapQuestion(
        id: j['id'] as String,
        label: j['label'] as String,
        type: _typeFromString(j['type'] as String),
        required: (j['required'] ?? false) as bool,
        options: ((j['options'] ?? []) as List).cast<String>(),
        maxLength: (j['max_length'] ?? 0) as int,
      );
}

class BootstrapAnswer {
  final String questionId;
  final dynamic value; // String / List<String> / bool
  const BootstrapAnswer({required this.questionId, required this.value});
  Map<String, dynamic> toJson() => {'question_id': questionId, 'value': value};
}
```

- [ ] **Step 2.4：跑 `flutter analyze`**

零 issue。

- [ ] **Step 2.5：commit**

```powershell
git add app/lib/api/models/
git commit -m "feat(app): StoryListItem + HeartbeatResponse + BootstrapQuestion DTOs"
```

---

## Task 3：ApiClient 补 method + 401 拦截器

**Files:**
- Modify: `app/lib/api/api_client.dart`
- Modify: `app/lib/main.dart`（桥接 `onUnauthorized`）
- Modify: `app/test/api_client_test.dart`（+ listStories + 401 拦截测试）

- [ ] **Step 3.1：构造函数加 `onUnauthorized` callback 参数**

```dart
class ApiClient {
  final Dio _dio;
  final TokenStorage _storage;
  final void Function()? _onUnauthorized;

  ApiClient({
    required TokenStorage storage,
    String baseUrl = 'http://127.0.0.1:8080',
    Dio? dio,
    void Function()? onUnauthorized,
  })  : _onUnauthorized = onUnauthorized,
        // ...

  // 在已有 interceptor 后追加 401 拦截：
  _dio.interceptors.add(InterceptorsWrapper(
    onResponse: (response, handler) {
      if (response.statusCode == 401) {
        final path = response.requestOptions.path;
        // /auth/* 跳过——登录中触发 401 不应反弹回登录屏
        if (!path.startsWith('/auth/')) {
          _onUnauthorized?.call();
        }
      }
      handler.next(response);
    },
  ));
}
```

🎓 教学（**dio interceptor 顺序**）：interceptor 按 add 顺序执行；request 阶段先进先出，response 阶段后进先出。把 JWT 注入拦截器放前面，401 拦截放后面——保证已带 token 的请求触发 401 时还能识别。"为什么需要"：避免 401 拦截器把没带 token 的登录请求误判为登出场景。追加到 `12-flutter.md`。

🎓 教学（**`/auth/*` 路径白名单**）：登录页本身就在调 `/auth/login_or_register`——如果验证码错了后端返 401（如果服务端的 codeError 错误码映射成了 401），不能让客户端"自动登出"——用户根本还没登上去。常见错配：业务"验证码错误"应是 400，但有些团队全写 401，所以客户端做防御性白名单。

- [ ] **Step 3.2：加 method**

```dart
Future<List<StoryListItem>> listStories(int childId, {int limit = 5}) async {
  final r = await _dio.get('/stories',
      queryParameters: {'child_id': childId, 'limit': limit});
  _ensureSuccess(r);
  return ((r.data['items'] ?? []) as List)
      .cast<Map<String, dynamic>>()
      .map(StoryListItem.fromJson)
      .toList();
}

Future<HeartbeatResponse> getHeartbeat(int childId) async {
  final r = await _dio.get('/heartbeat', queryParameters: {'child_id': childId});
  _ensureSuccess(r);
  return HeartbeatResponse.fromJson(r.data as Map<String, dynamic>);
}

Future<List<BootstrapQuestion>> getBootstrapQuestions() async {
  final r = await _dio.get('/bootstrap/questions');
  _ensureSuccess(r);
  return ((r.data['questions'] ?? []) as List)
      .cast<Map<String, dynamic>>()
      .map(BootstrapQuestion.fromJson)
      .toList();
}

Future<String> submitBootstrapAnswers({
  required int childId,
  required List<BootstrapAnswer> answers,
}) async {
  final r = await _dio.post('/bootstrap/answers', data: {
    'child_id': childId,
    'answers': answers.map((a) => a.toJson()).toList(),
  });
  _ensureSuccess(r);
  return (r.data['description'] ?? '') as String;
}
```

`generateStory` 加 `storylineId` 可选参数：

```dart
Future<Story> generateStory({
  required int childId, required String prompt,
  required int duration, required String style,
  String topic = '',
  int? storylineId,
  bool startStoryline = false,
}) async {
  final r = await _dio.post('/stories/generate', data: {
    'child_id': childId, 'prompt': prompt,
    'duration': duration, 'style': style, 'topic': topic,
    if (storylineId != null) 'storyline_id': storylineId,
    if (startStoryline) 'start_storyline': true,
  });
  _ensureSuccess(r);
  return Story.fromJson(r.data as Map<String, dynamic>);
}
```

- [ ] **Step 3.3：main.dart 桥接 onUnauthorized**

```dart
void main() {
  runApp(const ProviderScope(child: AibaoApp()));
}

class AibaoApp extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return MaterialApp.router(
      title: '爱宝',
      theme: aibaoTheme(),
      routerConfig: router,
    );
  }
}
```

在 `apiClientProvider` 处把 callback 接进来：

```dart
final apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    storage: SecureTokenStorage(),
    onUnauthorized: () {
      // ⚠️ 注意：不能直接 ref.read(authProvider.notifier).logout()——
      // provider 之间互调需小心循环初始化。用 Future.microtask 推到下一帧：
      Future.microtask(() => ref.read(authProvider.notifier).logout());
    },
  );
});
```

🎓 教学（**Future.microtask 推帧**）：Riverpod provider 在初始化时如果同步触发 `ref.read(其他 provider.notifier)`，可能导致循环依赖死锁或 build-time exception。`Future.microtask` 把回调推到事件循环下一轮，让所有 provider 先完成初始化。"为什么需要"：避免"我还没盖完房就在屋里造楼梯"。

- [ ] **Step 3.4：测试**

新增 test：
- `listStories 成功解析 5 条 items`
- `401 触发 onUnauthorized callback`（用 `mocktail` mock callback，断言被调用 1 次）
- `401 在 /auth/* 路径不触发 onUnauthorized`

- [ ] **Step 3.5：commit**

```powershell
git add app/lib/api/api_client.dart app/lib/main.dart app/test/api_client_test.dart
git commit -m "feat(api): listStories/getHeartbeat/bootstrap methods + 401 auto-logout"
```

---

## Task 4：state/heartbeat_state.dart

**Files:** Create `app/lib/state/heartbeat_state.dart`

- [ ] **Step 4.1：写 provider**

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/models/heartbeat.dart';
import 'auth_state.dart';

final heartbeatProvider =
    FutureProvider.family<HeartbeatResponse, int>((ref, childId) async {
  final api = ref.watch(apiClientProvider);
  return api.getHeartbeat(childId);
});
```

🎓 教学（**`FutureProvider.family<T, ARG>`**）：带参数的 FutureProvider——同一个 provider 工厂可以根据 ARG 产出 N 份独立缓存的 Future。本项目 `family<HeartbeatResponse, int>` 表示"每个 childId 一个独立 future"。"为什么需要"：多孩子时不会串数据。追加到 `12-flutter.md`。

- [ ] **Step 4.2：commit**

```powershell
git add app/lib/state/heartbeat_state.dart
git commit -m "feat(state): heartbeatProvider (FutureProvider.family by childId)"
```

---

## Task 5：state/story_list_state.dart

**Files:** Create `app/lib/state/story_list_state.dart`

- [ ] **Step 5.1：写 provider**

```dart
final storyListProvider =
    FutureProvider.family<List<StoryListItem>, int>((ref, childId) async {
  final api = ref.watch(apiClientProvider);
  return api.listStories(childId, limit: 5);
});
```

- [ ] **Step 5.2：在 generate / bootstrap submit 后 invalidate**

生成成功或孩子档案变更后，invalidate 这个 provider：`ref.invalidate(storyListProvider(childId))`——保证 home 刷新时拿到最新列表。

🎓 教学（**`ref.invalidate` vs `ref.refresh`**）：
- `invalidate`：丢弃缓存，**下次有 watcher 时**才重跑
- `refresh`：丢弃缓存并**立即重跑**，返回新 future

list 这种"用户回 home 才看"的场景用 invalidate 更省。详情屏要立刻刷的用 refresh。追加到 `12-flutter.md`。

- [ ] **Step 5.3：commit**

```powershell
git add app/lib/state/story_list_state.dart
git commit -m "feat(state): storyListProvider (5-item recent stories per child)"
```

---

## Task 6：state/bootstrap_state.dart

**Files:** Create `app/lib/state/bootstrap_state.dart` + 单测 `app/test/bootstrap_state_test.dart`

- [ ] **Step 6.1：BootstrapNotifier**

```dart
class BootstrapNotifier extends StateNotifier<BootstrapState> {
  BootstrapNotifier(this._api) : super(const BootstrapState.initial()) {
    _loadQuestions();
  }
  final ApiClient _api;

  Future<void> _loadQuestions() async {
    state = state.copyWith(loading: true);
    try {
      final qs = await _api.getBootstrapQuestions();
      state = state.copyWith(loading: false, questions: qs);
    } on ApiException catch (e) {
      state = state.copyWith(loading: false, errorMsg: e.userMsg);
    }
  }

  void setAnswer(String questionId, dynamic value) {
    final next = Map<String, BootstrapAnswer>.from(state.answers);
    next[questionId] = BootstrapAnswer(questionId: questionId, value: value);
    state = state.copyWith(answers: next, errorMsg: null);
  }

  /// 提交前校验：必填题必须有值；text 题必须非空白；multi_select 1-3 项
  String? validate() {
    for (final q in state.questions) {
      final a = state.answers[q.id];
      if (q.required && a == null) return '"${q.label}" 未填写';
      if (a == null) continue;
      switch (q.type) {
        case BootstrapType.text:
          final s = (a.value as String).trim();
          if (q.required && s.isEmpty) return '"${q.label}" 未填写';
          if (q.maxLength > 0 && s.length > q.maxLength) return '"${q.label}" 超过 ${q.maxLength} 字';
        case BootstrapType.multiSelect:
          final list = (a.value as List).cast<String>();
          if (q.required && list.isEmpty) return '"${q.label}" 至少选 1 项';
        // single_select / boolean 无额外校验
        default: break;
      }
    }
    return null;
  }

  Future<String?> submit(int childId) async {
    final err = validate();
    if (err != null) { state = state.copyWith(errorMsg: err); return null; }
    state = state.copyWith(submitting: true, errorMsg: null);
    try {
      final description = await _api.submitBootstrapAnswers(
        childId: childId,
        answers: state.answers.values.toList(),
      );
      state = state.copyWith(submitting: false, submitted: true);
      return description;
    } on ApiException catch (e) {
      state = state.copyWith(submitting: false, errorMsg: e.userMsg);
      return null;
    }
  }
}

final bootstrapProvider =
    StateNotifierProvider.autoDispose<BootstrapNotifier, BootstrapState>(
  (ref) => BootstrapNotifier(ref.watch(apiClientProvider)),
);
```

🎓 教学（**autoDispose**）：默认 provider 一旦创建就常驻内存。`autoDispose` 表示"没人 watch 时自动销毁"——bootstrap 7 题答案不需要在用户离开问卷屏后保留，加上更省。"为什么需要"：避免内存泄漏 + 下次进问卷时拿到全新初始态。追加到 `12-flutter.md`。

- [ ] **Step 6.2：单测**

`bootstrap_state_test.dart`：
- setAnswer 正常更新
- validate 必填漏填返回错误
- validate text 超长返回错误
- submit 成功后 state.submitted == true

跑 `flutter test test/bootstrap_state_test.dart`。

- [ ] **Step 6.3：commit**

```powershell
git add app/lib/state/bootstrap_state.dart app/test/bootstrap_state_test.dart
git commit -m "feat(state): BootstrapNotifier with 7-question answer collection + validation"
```

---

## Task 7：widgets/* 五个新组件

**Files:**
- Create: `app/lib/widgets/heartbeat_card.dart`
- Create: `app/lib/widgets/storyline_carousel.dart`
- Create: `app/lib/widgets/story_history_list.dart`
- Create: `app/lib/widgets/bootstrap_prompt_card.dart`
- Create: `app/lib/widgets/bootstrap_question_card.dart`
- Create: `app/lib/widgets/audio_status_badge.dart`

约束：**每个文件 < 200 行**。复杂的拆子组件。

- [ ] **Step 7.1：audio_status_badge.dart**

```dart
class AudioStatusBadge extends StatelessWidget {
  final String status; // pending/ready/failed
  const AudioStatusBadge(this.status, {super.key});
  @override
  Widget build(BuildContext context) {
    final (label, color) = switch (status) {
      'ready'   => ('已就绪', Colors.green),
      'pending' => ('生成中', Colors.orange),
      'failed'  => ('失败', Colors.red),
      _         => ('—', Colors.grey),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(label, style: TextStyle(fontSize: 12, color: color)),
    );
  }
}
```

- [ ] **Step 7.2：heartbeat_card.dart**

接收 `HeartbeatResponse`，渲染大字问候 + 🐼 emoji 头像。

```dart
class HeartbeatCard extends StatelessWidget {
  final HeartbeatResponse data;
  const HeartbeatCard(this.data, {super.key});
  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(children: [
          const Text('🐼', style: TextStyle(fontSize: 40)),
          const SizedBox(width: 12),
          Expanded(child: Text(data.greeting,
              style: Theme.of(context).textTheme.titleMedium)),
        ]),
      ),
    );
  }
}
```

- [ ] **Step 7.3：storyline_carousel.dart**

水平 ListView 显示活跃 storyline。点击 "继续" 调 `onContinue(storyline)` callback。

```dart
class StorylineCarousel extends StatelessWidget {
  final List<ActiveStoryline> items;
  final void Function(ActiveStoryline) onContinue;
  const StorylineCarousel({super.key, required this.items, required this.onContinue});
  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) return const SizedBox.shrink();
    return SizedBox(
      height: 160,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: items.length,
        separatorBuilder: (_, __) => const SizedBox(width: 12),
        itemBuilder: (_, i) => _StorylineTile(items[i], onContinue),
      ),
    );
  }
}
// _StorylineTile 是私有子组件，单独的 _ 前缀 stateless widget
```

- [ ] **Step 7.4：story_history_list.dart**

垂直 ListView 5 条 ListTile，每条显标题 + 风格 + 时长 + audio_status badge + 续集 #X 标注。点击调 `onTap(storyId)`。

- [ ] **Step 7.5：bootstrap_prompt_card.dart**

home 顶部下方的黄色提示卡：「💡 完善小宇的画像，让爱宝把孩子写进故事里」+ 右侧"开始"按钮 → 路由 `/onboarding/bootstrap`。

- [ ] **Step 7.6：bootstrap_question_card.dart**

按 `question.type` 分支渲染：

```dart
class BootstrapQuestionCard extends ConsumerWidget {
  final BootstrapQuestion q;
  const BootstrapQuestionCard(this.q, {super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(bootstrapProvider);
    final answer = state.answers[q.id];
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 8),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(q.label + (q.required ? ' *' : ''), style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 12),
            _buildInput(context, ref, answer),
          ],
        ),
      ),
    );
  }

  Widget _buildInput(BuildContext c, WidgetRef ref, BootstrapAnswer? a) {
    switch (q.type) {
      case BootstrapType.text:
        return TextField(
          controller: TextEditingController(text: (a?.value as String?) ?? ''),
          maxLength: q.maxLength > 0 ? q.maxLength : null,
          decoration: const InputDecoration(border: OutlineInputBorder()),
          onChanged: (v) => ref.read(bootstrapProvider.notifier).setAnswer(q.id, v),
        );
      case BootstrapType.singleSelect:
        return Wrap(spacing: 8, children: q.options.map((o) => ChoiceChip(
          label: Text(o),
          selected: a?.value == o,
          onSelected: (_) => ref.read(bootstrapProvider.notifier).setAnswer(q.id, o),
        )).toList());
      case BootstrapType.multiSelect:
        final selected = (a?.value as List?)?.cast<String>() ?? [];
        return Wrap(spacing: 8, children: q.options.map((o) {
          final on = selected.contains(o);
          return ChoiceChip(
            label: Text(o), selected: on,
            onSelected: (_) {
              final next = [...selected];
              on ? next.remove(o) : next.add(o);
              ref.read(bootstrapProvider.notifier).setAnswer(q.id, next);
            },
          );
        }).toList());
      case BootstrapType.boolean:
        return SwitchListTile(
          value: (a?.value as bool?) ?? false,
          title: const Text('开启'),
          onChanged: (v) => ref.read(bootstrapProvider.notifier).setAnswer(q.id, v),
        );
    }
  }
}
```

🎓 教学（**ChoiceChip vs Checkbox**）：Material 3 推荐用 ChoiceChip 替代传统 Checkbox 来做单选/多选——触摸目标更大（儿童 App 友好）、可视面积大、可一行排多个。"为什么需要"：儿童 App 家长一手抱娃一手操作，按钮越大越好。追加到 `12-flutter.md`。

🎓 教学（**TextEditingController 在 onChanged 模式下不要在 build 里 new**）：上面的代码 `TextEditingController(text: ...)` 写在 build 里**每次 rebuild 都会新建一个 controller**——光标会跳到末尾。正确做法是用 `ConsumerStatefulWidget` 把 controller 放 state 里，或者用 `initialValue` 参数 + `TextFormField`。**实施时注意改成 stateful 或换 TextFormField**。这是 Plan 9-A 已踩过的坑——见 `docs/devlog/2026-05-16.md` 附录 B。追加到 `12-flutter.md`"踩坑录"。

- [ ] **Step 7.7：commit**

```powershell
git add app/lib/widgets/heartbeat_card.dart app/lib/widgets/storyline_carousel.dart app/lib/widgets/story_history_list.dart app/lib/widgets/bootstrap_prompt_card.dart app/lib/widgets/bootstrap_question_card.dart app/lib/widgets/audio_status_badge.dart
git commit -m "feat(widgets): heartbeat/storyline/history/bootstrap cards"
```

---

## Task 8：screens/create_child_screen.dart

**Files:** Create `app/lib/screens/create_child_screen.dart`

- [ ] **Step 8.1：写表单屏**

```dart
class CreateChildScreen extends ConsumerStatefulWidget {
  const CreateChildScreen({super.key});
  @override ConsumerState createState() => _CreateChildScreenState();
}

class _CreateChildScreenState extends ConsumerState<CreateChildScreen> {
  final _formKey = GlobalKey<FormState>();
  final _nickname = TextEditingController();
  String _gender = 'boy';
  DateTime _birthday = DateTime(DateTime.now().year - 4, 1, 1);
  bool _submitting = false;

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _submitting = true);
    try {
      final api = ref.read(apiClientProvider);
      await api.createChild(
        nickname: _nickname.text.trim(),
        gender: _gender,
        birthday: '${_birthday.year.toString().padLeft(4, '0')}-'
                  '${_birthday.month.toString().padLeft(2, '0')}-'
                  '${_birthday.day.toString().padLeft(2, '0')}',
      );
      ref.invalidate(currentChildProvider); // 触发 home 拉新 child
      if (mounted) context.go('/home');
    } on ApiException catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.userMsg)));
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('给孩子建个档案')),
      body: SafeArea(child: Padding(
        padding: const EdgeInsets.all(24),
        child: Form(key: _formKey, child: ListView(children: [
          TextFormField(
            controller: _nickname,
            decoration: const InputDecoration(labelText: '昵称（如：小宇）', border: OutlineInputBorder()),
            validator: (v) => (v ?? '').trim().isEmpty ? '请输入昵称' : null,
            maxLength: 20,
          ),
          const SizedBox(height: 16),
          DropdownButtonFormField<String>(
            value: _gender,
            decoration: const InputDecoration(labelText: '性别', border: OutlineInputBorder()),
            items: const [
              DropdownMenuItem(value: 'boy', child: Text('男孩')),
              DropdownMenuItem(value: 'girl', child: Text('女孩')),
            ],
            onChanged: (v) => setState(() => _gender = v ?? 'boy'),
          ),
          const SizedBox(height: 16),
          OutlinedButton.icon(
            icon: const Icon(Icons.cake),
            label: Text('生日：${_birthday.year}-${_birthday.month}-${_birthday.day}'),
            onPressed: () async {
              final picked = await showDatePicker(
                context: context, initialDate: _birthday,
                firstDate: DateTime(2010, 1, 1),
                lastDate: DateTime.now(),
              );
              if (picked != null) setState(() => _birthday = picked);
            },
          ),
          const SizedBox(height: 32),
          FilledButton(
            onPressed: _submitting ? null : _submit,
            child: Text(_submitting ? '提交中...' : '保存'),
          ),
        ])),
      )),
    );
  }
}
```

🎓 教学（**Form + FormKey + validator**）：Flutter 表单标准模式——Form 包裹一组 TextFormField；每个 field 写 `validator: (v) => ... ? null : '错误消息'`；调 `_formKey.currentState!.validate()` 触发所有 validator，全过才返回 true。"为什么需要"：避免手写一堆 setState 校验 + 错误位置不一致。追加到 `12-flutter.md`。

- [ ] **Step 8.2：commit**

```powershell
git add app/lib/screens/create_child_screen.dart
git commit -m "feat(screen): create-child onboarding form"
```

---

## Task 9：screens/bootstrap_screen.dart

**Files:** Create `app/lib/screens/bootstrap_screen.dart`

- [ ] **Step 9.1：一页滚动 7 题**

```dart
class BootstrapScreen extends ConsumerWidget {
  const BootstrapScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(bootstrapProvider);
    final child = ref.watch(currentChildProvider).valueOrNull;

    if (child == null) {
      return const Scaffold(body: Center(child: Text('请先创建孩子档案')));
    }
    if (state.loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (state.errorMsg != null && state.questions.isEmpty) {
      return Scaffold(body: Center(child: Text('加载失败：${state.errorMsg}')));
    }

    return Scaffold(
      appBar: AppBar(title: const Text('完善画像（7 题）')),
      body: SafeArea(child: Column(children: [
        Expanded(child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text('回答这 7 个问题，爱宝会更懂${child.nickname}。',
              style: Theme.of(context).textTheme.bodyMedium),
            const SizedBox(height: 12),
            ...state.questions.map((q) => BootstrapQuestionCard(q)),
          ],
        )),
        if (state.errorMsg != null)
          Padding(
            padding: const EdgeInsets.all(8),
            child: Text(state.errorMsg!, style: const TextStyle(color: Colors.red)),
          ),
        Padding(
          padding: const EdgeInsets.all(16),
          child: FilledButton(
            onPressed: state.submitting ? null : () async {
              final desc = await ref.read(bootstrapProvider.notifier).submit(child.id);
              if (desc != null && context.mounted) {
                ref.invalidate(currentChildProvider);
                context.go('/home');
              }
            },
            child: Text(state.submitting ? '提交中...' : '提交'),
          ),
        ),
      ])),
    );
  }
}
```

- [ ] **Step 9.2：手动冒烟一次**

到 home 点 "完善画像" → 7 题屏 → 7 题答完 → 提交 → 后端日志看到 LLM 润色 → 回 home 黄条消失。

- [ ] **Step 9.3：commit**

```powershell
git add app/lib/screens/bootstrap_screen.dart
git commit -m "feat(screen): bootstrap 7-question form (single-page scroll)"
```

---

## Task 10：screens/home_screen.dart 重写

**Files:** Modify `app/lib/screens/home_screen.dart`

- [ ] **Step 10.1：5 区段组合屏**

```dart
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final childAsync = ref.watch(currentChildProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('爱宝'),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout),
            onPressed: () => ref.read(authProvider.notifier).logout(),
          ),
        ],
      ),
      floatingActionButton: childAsync.valueOrNull != null
          ? FloatingActionButton.extended(
              icon: const Icon(Icons.auto_stories),
              label: const Text('今天讲什么'),
              onPressed: () => context.go('/generate'),
            )
          : null,
      body: childAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('加载孩子档案失败：$e')),
        data: (child) {
          if (child == null) {
            // ⚠️ 配合 router redirect 强制跳 create-child；此处兜底
            WidgetsBinding.instance.addPostFrameCallback((_) => context.go('/onboarding/create-child'));
            return const Center(child: CircularProgressIndicator());
          }
          return _HomeBody(child: child);
        },
      ),
    );
  }
}

class _HomeBody extends ConsumerWidget {
  final Child child;
  const _HomeBody({required this.child});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final hb = ref.watch(heartbeatProvider(child.id));
    final stories = ref.watch(storyListProvider(child.id));
    final hasDesc = child.hasProfileDescription;

    return RefreshIndicator(
      onRefresh: () async {
        ref.invalidate(heartbeatProvider(child.id));
        ref.invalidate(storyListProvider(child.id));
        await ref.read(heartbeatProvider(child.id).future);
      },
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          // 1. HEARTBEAT 问候
          hb.when(
            loading: () => const Card(child: Padding(padding: EdgeInsets.all(16), child: LinearProgressIndicator())),
            error: (e, _) => Card(child: Padding(padding: const EdgeInsets.all(16), child: Text('问候加载失败: $e'))),
            data: (data) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              HeartbeatCard(data),
              if (data.activeStorylines.isNotEmpty) ...[
                const SizedBox(height: 12),
                Text('活跃连续剧', style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 8),
                StorylineCarousel(
                  items: data.activeStorylines,
                  onContinue: (sl) => context.go('/generate?storylineId=${sl.id}'),
                ),
              ],
            ]),
          ),

          const SizedBox(height: 16),

          // 2. BOOTSTRAP 提示卡（条件显）
          if (!hasDesc) const BootstrapPromptCard(),

          const SizedBox(height: 16),

          // 3. 孩子卡
          Card(
            child: ListTile(
              leading: const Text('🐼', style: TextStyle(fontSize: 32)),
              title: Text(child.nickname),
              subtitle: Text('${child.ageYears} 岁 · ${child.gender == "boy" ? "男孩" : "女孩"}'),
            ),
          ),

          const SizedBox(height: 16),

          // 4. 故事历史
          Text('最近听过', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          stories.when(
            loading: () => const Card(child: Padding(padding: EdgeInsets.all(16), child: LinearProgressIndicator())),
            error: (e, _) => Card(child: Padding(padding: const EdgeInsets.all(16), child: Text('加载失败: $e'))),
            data: (items) => items.isEmpty
                ? const Card(child: Padding(padding: EdgeInsets.all(16),
                    child: Text('还没有故事——点右下角"今天讲什么"开始吧～', textAlign: TextAlign.center)))
                : StoryHistoryList(
                    items: items,
                    onTap: (id) => context.go('/player/$id'),
                  ),
          ),

          const SizedBox(height: 80), // 给 FAB 留空间
        ],
      ),
    );
  }
}
```

🎓 教学（**`AsyncValue.when`**）：Riverpod 把 FutureProvider 包成 `AsyncValue`，自动有 loading / error / data 三态。用 `.when()` 一次性写完三个分支，避免到处写 `if (snap.connectionState == ConnectionState.waiting)`。"为什么需要"：UI 三态优雅切换、避免漏写 loading 导致白屏。追加到 `12-flutter.md`。

🎓 教学（**`RefreshIndicator`**）：下拉刷新——Material 风格。本项目家长可能想刷新 heartbeat 看新的活跃 storyline。"为什么需要"：移动端用户期望的标准手势，不做就显得"不像 App"。

- [ ] **Step 10.2：child.dart 加 `hasProfileDescription` getter**

```dart
// app/lib/api/models/child.dart
bool get hasProfileDescription {
  if (profileJson.isEmpty) return false;
  try {
    final m = jsonDecode(profileJson) as Map<String, dynamic>;
    final desc = (m['description'] ?? '') as String;
    return desc.trim().isNotEmpty;
  } catch (_) { return false; }
}
```

⚠️ 注意：当前 9-A `Child` model 用 `profileJson` 字段（raw string）。**保留这个字段名**——getter 内部 jsonDecode。这样不破坏现有反序列化路径。

- [ ] **Step 10.3：commit**

```powershell
git add app/lib/screens/home_screen.dart app/lib/api/models/child.dart
git commit -m "feat(screen): home five-section layout (heartbeat/bootstrap/child/storyline/history)"
```

---

## Task 11：generate_screen.dart 续集支持

**Files:** Modify `app/lib/screens/generate_screen.dart`

- [ ] **Step 11.1：接收 storylineId query param**

```dart
class GenerateScreen extends ConsumerStatefulWidget {
  final int? storylineId;
  const GenerateScreen({super.key, this.storylineId});
  // ...
}
```

router.dart 解析 query：
```dart
GoRoute(
  path: '/generate',
  builder: (_, st) {
    final s = st.uri.queryParameters['storylineId'];
    return GenerateScreen(storylineId: s == null ? null : int.tryParse(s));
  },
),
```

- [ ] **Step 11.2：UI 续集徽章**

```dart
if (widget.storylineId != null)
  Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
    decoration: BoxDecoration(
      color: aibaoGreen.withValues(alpha: 0.15),
      borderRadius: BorderRadius.circular(20),
    ),
    child: Text('📖 续集 (storyline #${widget.storylineId})'),
  ),
```

prompt TextField 的初始值改为 "继续上一集"。

- [ ] **Step 11.3：提交时带 storylineId**

```dart
final story = await api.generateStory(
  childId: child.id, prompt: prompt,
  duration: duration, style: style,
  storylineId: widget.storylineId,
);
ref.invalidate(storyListProvider(child.id));
ref.invalidate(heartbeatProvider(child.id));
if (mounted) context.go('/player/${story.id}');
```

🎓 教学（**go_router query params 与 path params 选哪个**）：
- path param `/player/:id`：资源主键，URL 一部分，分享/书签都靠它
- query param `?storylineId=7`：可选**修饰符**，控制行为而非定位资源

本项目 generate 屏不区分"列表上的某一项"，storylineId 只是个上下文修饰——用 query 合适。追加到 `12-flutter.md`。

- [ ] **Step 11.4：commit**

```powershell
git add app/lib/screens/generate_screen.dart app/lib/router.dart
git commit -m "feat(screen): generate accepts storylineId query param + sequel badge"
```

---

## Task 12：player_screen.dart 微调（从历史点入）

**Files:** Modify `app/lib/screens/player_screen.dart`

- [ ] **Step 12.1：确认 player 已用 storyByIdProvider**

如已用，无需大改。补一段：进入时如果 `audio_status == 'ready'`，**立即**调一次 `getAudioUrl` 拿 signed URL setUrl，不要等 3s 轮询第一帧。

```dart
@override
void initState() {
  super.initState();
  // 立即 fetch 一次 audio_url（即使 story.audioStatus 还是 pending 也无害——response 会告知真实状态）
  _initialFetch();
}

Future<void> _initialFetch() async {
  try {
    final r = await ref.read(apiClientProvider).getAudioUrl(widget.storyId);
    if (r is AudioReady && mounted) {
      await _player.setUrl(r.url);
    }
  } catch (_) {/* 轮询会兜底 */}
}
```

- [ ] **Step 12.2：commit**

```powershell
git add app/lib/screens/player_screen.dart
git commit -m "feat(screen): player eager-fetch audio_url for ready stories from history"
```

---

## Task 13：router.dart 新路由 + redirect 扩展

**Files:** Modify `app/lib/router.dart`

- [ ] **Step 13.1：新路由**

```dart
GoRoute(path: '/onboarding/create-child', builder: (_, __) => const CreateChildScreen()),
GoRoute(path: '/onboarding/bootstrap',    builder: (_, __) => const BootstrapScreen()),
```

- [ ] **Step 13.2：redirect 扩展（强制 create-child）**

```dart
redirect: (ctx, st) {
  final auth = ref.read(authProvider);
  if (auth is AuthInitial) {
    return st.matchedLocation == '/' ? null : '/';
  }
  final loggingIn = st.matchedLocation == '/login';
  if (auth is AuthUnauthenticated || auth is AuthError) {
    return loggingIn ? null : '/login';
  }
  if (auth is AuthAuthenticated) {
    if (st.matchedLocation == '/' || loggingIn) return '/home';

    // ⚠️ 新逻辑：已登录但无孩子 → 强制 create-child（除非已经在路上）
    final childAsync = ref.read(currentChildProvider);
    final child = childAsync.valueOrNull;
    final inOnboarding = st.matchedLocation.startsWith('/onboarding/');
    if (childAsync.hasValue && child == null && !inOnboarding) {
      return '/onboarding/create-child';
    }
    // 已有孩子但停在 create-child → 跳回 home
    if (child != null && st.matchedLocation == '/onboarding/create-child') {
      return '/home';
    }
    // ⚠️ 注意：profile.description 为空不强制跳 bootstrap——仅在 home 上显软提示卡
  }
  return null;
},
```

⚠️ **`_AuthListenable` 同步要监听 `currentChildProvider`**——否则 createChild 完成后 router 不会重评估 redirect：

```dart
class _AuthListenable extends ChangeNotifier {
  _AuthListenable(this._ref) {
    _ref.listen<AuthState>(authProvider, (_, __) => notifyListeners());
    _ref.listen<AsyncValue<Child?>>(currentChildProvider, (_, __) => notifyListeners());
  }
  final Ref _ref;
}
```

- [ ] **Step 13.3：commit**

```powershell
git add app/lib/router.dart
git commit -m "feat(router): onboarding routes + force-create-child redirect"
```

---

## Task 14：端到端冒烟（手动 6 步 happy-path）

**目标：每个 6 大补全各 1 步 happy-path 验证（最后一遍）。**

- [ ] **Step 14.1：环境**

```powershell
# 启动后端
cd f:\claud\aibao_app\server
go run ./cmd/server

# 另一窗口跑客户端
cd f:\claud\aibao_app\app
flutter run -d <android-emulator-or-real-device>
```

清空相关用户数据：
```powershell
# 进 DB（用你的连接方式）DELETE FROM children WHERE user_id = (SELECT id FROM users WHERE phone_hash = ...);
# DELETE FROM stories WHERE child_id = ...; DELETE FROM storylines WHERE child_id = ...;
```

- [ ] **Step 14.2：六步**

1. **新孩子创建**：emulator 登录 `13900000001 / 123456` → 自动跳 create-child → 填 "小宇 / 男孩 / 2020-03-15" → 回 home 看到小宇卡
2. **BOOTSTRAP**：home 顶部有黄色提示卡 → 点 → 7 题屏 → 答完 → 提交 → 回 home 黄条没了
3. **HEARTBEAT**：home 顶部"小宇早上好呀～"（或时段对应）显示
4. **生成首集**：FAB → "讲个森林小冒险" / 5min / 温馨治愈 → 提交 → 等待 → player 显示
5. **故事历史**：返回 home → "最近听过"显 1 条 ready 状态 → 点击 → 直接进 player 不再生成
6. **续集**：再生成 1 次（标记为连续剧或现有 storyline）→ 回 home 看 "活跃连续剧"卡 → 点"继续" → generate 屏头部显"📖 续集" → 提交 → 等待 → player 显示 ep2 → 看 ep2 文本能看到对 ep1 的呼应

外加 7（401 拦截）：在后端进程暂停 → 改 JWT secret → 重启 → 模拟器点 generate → toast"登录已过期"+ 自动回 /login。

- [ ] **Step 14.3：所有步骤通过 → 截图存到 `docs/devlog/2026-05-17.md`**

---

## Task 15：知识库 + 文档收口

**Files:**
- Modify: `docs/knowledge/12-flutter.md`（追加本 Plan 所有 🎓 词条）
- Modify: `docs/knowledge/06-testing.md` §6.13（auth 修补案例追加）
- Modify: `docs/knowledge/07-http-and-web.md`（list/detail DTO 区分）
- Modify: `docs/knowledge/10-security-and-compliance.md`（IDOR ownership 校验）
- Modify: `CLAUDE.md` §2"已落地的能力"
- Modify: `MEMORY.md`
- Create: `docs/devlog/2026-05-17.md`

- [ ] **Step 15.1：12-flutter.md 追加词条**

至少 8 条新条目（按本 Plan 出现顺序）：
1. `FutureProvider.family<T, ARG>` —— 带参数的异步缓存
2. `ref.invalidate` vs `ref.refresh` —— 主动失效缓存的两种模式
3. `autoDispose` —— provider 自动销毁机制
4. `AsyncValue.when` —— 三态优雅渲染
5. `RefreshIndicator` —— 下拉刷新标准手势
6. `Form + GlobalKey<FormState> + validator` —— 表单标准模式
7. `ChoiceChip` —— Material 3 单选/多选首选
8. `go_router` query param vs path param —— 路由参数选择原则
9. `TextEditingController 不要在 build 里 new`（踩坑录）
10. `Future.microtask 推帧避免循环初始化`

每条按知识库格式："是什么 + 生活类比 + 为什么需要 + 在本项目中怎么用 + 何时引入（commit）"

- [ ] **Step 15.2：CLAUDE.md §2"已落地的能力"追加**

```markdown
- **Flutter 客户端中档完整版**（Plan 9b）：BOOTSTRAP 7 题问卷屏 + HEARTBEAT 时段问候卡 + 故事历史列表 + 活跃 storyline 续集入口 + 新孩子创建表单屏 + dio 401 自动登出拦截器
- **后端补：login_or_register UTF-8 nickname 校验**（auth.go 复用 Plan 6b §6.13 模式）
- **后端补：GET /api/v1/stories 故事列表接口**（按 child_id + limit ≤ 50 + ownership 校验）
```

- [ ] **Step 15.3：MEMORY.md 决策记录**

记录：
- "Plan 9b 收官 = Flutter 客户端中档完整版"
- 已上接口清单（含新 GET /stories）
- known issues：iOS / release proguard / 录音输入 / 暗色模式 / 错误屏全部留给 9c+
- 用户偏好：BOOTSTRAP 一页滚动（不分页）/ 续集 prompt 默认"继续上一集" / 软提示不强制跳

- [ ] **Step 15.4：devlog 2026-05-17.md**

模板：
- 6 大补全各 1 段
- 1 个后端小修
- 6 步冒烟截图
- 下一步建议：Plan 9c（iOS / release proguard / 错误屏） / Plan 10（部署上线 / 应用商店）

- [ ] **Step 15.5：commit**

```powershell
git add docs/
git commit -m "docs(plan-9b): knowledge updates + devlog + memory"
```

---

## 附录 A：Plan 9b 之后的 known issues / 留给 9c+

| 项 | 计划归属 | 说明 |
|---|---|---|
| iOS 支持 | Plan 9c | flutter create 重跑 + ios/ 目录补 + just_audio 平台权限 + 真机 codesign |
| release proguard | Plan 9c | `flutter build apk --release` 时 just_audio / secure_storage 反混淆规则 |
| 暗色模式 | Plan 9c | `darkTheme` + child friendly 配色 |
| 录音输入（spec §4.3） | Plan 11 | speech_to_text + 音频上传 |
| 错误屏 | Plan 9c | 全屏错误页（非 toast） |
| 多孩子档案 UI | Plan 12 | 一期保留 1:1，未来扩展时一并改后端 UNIQUE 约束 |
| storyline 单独管理屏 | Plan 12 | 列表 + 关闭 + 强制开新一季 |
| 应用商店上架 | Plan 10 | Android 签名 + 用户协议 + 隐私政策 + 备案 |

## 附录 B：Plan 9b 触发的 Riverpod cache invalidate 时机表

| 操作 | invalidate 列表 |
|---|---|
| createChild 成功 | `currentChildProvider` |
| submitBootstrapAnswers 成功 | `currentChildProvider` |
| generateStory 成功 | `storyListProvider(childId)` + `heartbeatProvider(childId)` |
| logout | （Riverpod 整个 ProviderScope 不重建——靠 router redirect 跳 /login，state 在下次登录时由 _bootstrap 重置） |
| pull-to-refresh on home | `heartbeatProvider(childId)` + `storyListProvider(childId)` |

## 附录 C：手机端 vs 模拟器 baseUrl 切换

Plan 9-A 已通过 `adb reverse tcp:8080 tcp:8080` 用 127.0.0.1。9b 沿用，不改。如需 AVD-only 调试，临时把 `ApiClient(baseUrl: 'http://10.0.2.2:8080')`。

---

**Plan 9b 完。下一步走 Plan 9c（iOS + release + 错误屏 + 暗色）还是 Plan 10（部署上线）由用户拍板。**
