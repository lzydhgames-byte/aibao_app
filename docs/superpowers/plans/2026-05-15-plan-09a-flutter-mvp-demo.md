# Plan 9-A：Flutter 最小 MVP 客户端（4 屏验证）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 实施者请注意：本项目用户**没有软件工程基础**，每个新概念（Flutter / Dart / widget / Riverpod / dio / go_router / just_audio）首次出现时**必须**用🎓段落解释并追加到 `docs/knowledge/12-flutter.md`（含"为什么需要"段）。详见 CLAUDE.md §7。

**Goal:** 在 Plan 1-8 完整后端基础上，写出爱宝项目**第一个 Flutter 客户端**。Plan 9-A 是"能跑通最小演示"的最薄切片：4 个屏幕（登录 → 主页 → 生成 → 播放）跑完一遍"AI 给我家娃讲个故事"的端到端旅程。BOOTSTRAP 问卷 / HEARTBEAT 时段问候 / 连续剧入口 / 错误屏 / iOS / Web 全部留给 Plan 9b/9c。完成后，把 Android 模拟器交到任何人手里，他都能从登录到听到 TTS 朗读，全程不写一行代码。

**Architecture:** 技术栈 = Flutter 3.29.3 + Material 3 + Riverpod（状态管理）+ dio（HTTP 客户端）+ go_router（声明式路由）+ just_audio（音频播放）+ flutter_secure_storage（JWT 持久化）。架构沿用三层：`api/`（HTTP 客户端 + DTO）/ `state/`（Riverpod Notifier 状态层）/ `screens/`（UI 屏幕）。4 屏用户旅程：app 启动 → `auth_state` 决定跳 `/login` 或 `/home` → 登录成功跳 `/home`（展示已有 child 卡片）→ tap "今天讲什么故事？" 跳 `/generate`（prompt + 时长 + 风格 + 提交）→ orchestrator 后端 15-20s 同步返回 storyId → 跳 `/player/:id`（展示故事文本 + just_audio 播放器，3s 轮询 `audio_url` 直到 ready）→ 按 play 听 TTS。

**Tech Stack:**
- Flutter 3.29.3 + Dart 3.7
- Material 3（`useMaterial3: true`）+ seed color `0xFF2E7D32`（深绿，暗示温馨自然）
- 状态管理：`flutter_riverpod ^2.5`
- HTTP：`dio ^5.7`（手写 ApiClient + JWT interceptor）
- 路由：`go_router ^14.6`
- 音频：`just_audio ^0.9.40`
- 持久化：`flutter_secure_storage ^9.2`（Android Keystore 后端，存 JWT）
- Lint：`flutter_lints ^5.0`（默认）
- 测试：`http_mock_adapter ^0.6` + `mocktail ^1.0`（仅对 ApiClient 写一组集成测试，不强求覆盖率）
- 目标平台：**Android only**（Plan 9-A）；iOS / Web / Desktop 留给后续 Plan
- Logo：🐼 emoji 占位（spec 之后专门设计）

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)
  - 第 4 章 MVP 功能清单（确认 9-A 仅做 §4.1 登录 + §4.3 故事生成 + §4.4 播放，**不**做 §4.2 BOOTSTRAP）
  - 第 5.1-5.3 用户流程
  - 第 9 章界面设计原则（爱宝定位 + 孩子主角 + 大字大按钮）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md) 第 4 章核心数据流（理解 API 契约）
- 已完成的 Plan：4（故事生成 API 形态） / 5（音频异步管线 + audio_url 3 态） / 6b（audio_status 字段） / 8（storyline 字段）
- 后端 handler 源码（**API 契约权威**）：
  - `server/internal/api/auth.go` —— `POST /api/v1/auth/sms/send`、`POST /api/v1/auth/login_or_register`
  - `server/internal/api/me.go` —— `GET /api/v1/me`
  - `server/internal/api/child.go` —— `POST/GET/PATCH /api/v1/children`
  - `server/internal/api/story.go` —— `POST /api/v1/stories/generate`、`GET /api/v1/stories/:id`
  - `server/internal/api/audio.go` —— `GET /api/v1/stories/:id/audio_url`
- 协作规则：[CLAUDE.md](../../../CLAUDE.md) §7"边做边学"——**硬要求**

**完成验收（Definition of Done）：**

1. `cd app && flutter pub get` 成功 + `flutter analyze` 输出 `No issues found!`
2. `flutter run -d <android-emulator>` 启动 app，冷启 ≤ 8s 显示登录屏
3. 输入手机号 `13900000001` → tap "发送验证码" → 后端日志可见验证码 `123456`（dev 环境固定）→ 输入 `123456` → tap "登录" → 成功跳转 `/home`
4. `/home` 展示已有 child "小宇" 的卡片（昵称 + 年龄），以及大按钮 "今天讲什么故事？"
   - 前置：通过 PowerShell 调 `POST /api/v1/children` 提前为该用户建好 child（用户手动做一次，详见 Task 11）
5. tap "今天讲什么故事？" → 进入 `/generate` → 输入 `讲个森林小冒险` + 选 5 分钟 + 选 `温馨治愈` → tap "开始" → 显示 🐼 "爱宝在想..." 等待动画 → 15-20s 后跳转到 `/player/{id}`
6. `/player/:id` 显示故事标题 + 全文（可滚动）+ 底部 just_audio 播放器；轮询 `audio_url` 每 3s 一次，ready 后 setUrl → tap play 真听到 TTS 朗读
7. 关掉 app → 重新启动 → 由于 JWT 已 secure_storage 持久化，**直接进 `/home`**（不重新登录）
8. 全流程后端**不重启**，模拟器**不清缓存**
9. 出错路径手动验证：故意输错验证码 → toast "验证码错误"；audio_status=failed → player 显示友好错误"音频生成失败"
10. 整个 9-A 任务流程产生的所有新 Flutter 概念都收口到 `docs/knowledge/12-flutter.md`（Task 12 验收）

---

## 范围决策记录（与用户对齐 —— 不要再争论）

| 维度 | 决策 |
|---|---|
| 项目位置 | `f:\claud\aibao_app\app\`（与 `server/` 同级） |
| Flutter package name | `aibao_app` |
| 目标平台 | Android only（emulator + 真机），iOS/Web/Desktop OUT |
| 状态管理 | `flutter_riverpod` 2.x（不用 Bloc / Provider 原生） |
| HTTP 客户端 | `dio` 5.x，手写 `ApiClient`，不引代码生成（不用 retrofit） |
| 音频 | `just_audio` 0.9+ |
| 路由 | `go_router` 14.x（声明式，支持深链） |
| 持久化 | `flutter_secure_storage`（仅存 JWT；不存业务数据 = 无离线缓存） |
| 主题 | Material 3 + 1 个 seed color `0xFF2E7D32` |
| Logo | 🐼 emoji 占位 |
| 错误处理 | 4xx/5xx → toast；audio_failed → player 内联错误条；不做错误屏 |
| 注册流程 | 手机验证码登录即注册；新孩子档案 9-A **不做 UI**（手动 API 建） |
| BOOTSTRAP / HEARTBEAT | OUT（9b） |
| 连续剧 UI | OUT（9b） |
| 录音输入 | OUT（9-A 仅文字 prompt） |
| 暗色模式 / 自定义字体 | OUT（用系统默认中文 fallback） |
| 测试 | 只对 ApiClient 写 mock 集成测试一组；不做 UI test / widget test 覆盖率 |
| Linting | `flutter_lints` 默认包，不额外定制 |
| 应用商店上架 | OUT（仅 `flutter run` 本地运行） |
| API base URL | dev 默认 `http://10.0.2.2:8080`（Android emulator → 宿主 127.0.0.1） |

---

## File Structure

```
app/
├── pubspec.yaml
├── analysis_options.yaml
├── android/                            (flutter create 默认生成；只动 AndroidManifest 加 INTERNET)
├── lib/
│   ├── main.dart                       App + ProviderScope + MaterialApp.router
│   ├── theme.dart                      Material 3 ThemeData (seed color)
│   ├── router.dart                     go_router 配置 + 登录 redirect
│   ├── api/
│   │   ├── api_client.dart             dio client + JWT interceptor + 类型化方法
│   │   ├── api_exception.dart          统一异常类
│   │   └── models/
│   │       ├── user.dart
│   │       ├── child.dart
│   │       ├── story.dart
│   │       └── audio_url.dart          (sealed: Ready / Pending / Failed)
│   ├── state/
│   │   ├── auth_state.dart             AuthNotifier + tokenProvider + meProvider
│   │   ├── child_state.dart            currentChildProvider
│   │   └── story_state.dart            storyGenerateProvider + audioUrlPollProvider
│   ├── screens/
│   │   ├── login_screen.dart
│   │   ├── home_screen.dart
│   │   ├── generate_screen.dart
│   │   └── player_screen.dart
│   └── widgets/
│       ├── duration_chips.dart         5/10/15 分钟选择
│       ├── style_dropdown.dart         5 种风格选择
│       └── waiting_aibao.dart          🐼 loading overlay
└── test/
    └── api_client_test.dart            dio mock 集成测试
```

---

## API 形态（先定好契约 —— 客户端按此对接）

### 1. `POST /api/v1/auth/sms/send`
**Request:** `{"phone": "13900000001"}`
**Response 200:** `{"sent": true}`
**Dev 行为:** 后端日志打印验证码 `123456`（开发环境固定）

### 2. `POST /api/v1/auth/login_or_register`
**Request:** `{"phone": "13900000001", "code": "123456", "nickname": ""}`（nickname 可空）
**Response 200:**
```json
{
  "access_token": "eyJhbGc...",
  "refresh_token": "eyJhbGc...",
  "user": {"id": 1, "nickname": "用户1234", "subscription_tier": "free"}
}
```

### 3. `GET /api/v1/me` (Bearer)
**Response 200:** `{"id": 1, "nickname": "...", "subscription_tier": "free"}`

### 4. `GET /api/v1/children` (Bearer)
**Response 200:**
```json
{"items": [{"id": 3, "user_id": 1, "nickname": "小宇", "gender": "boy", "birthday": "2020-03-15", "profile": "{}"}]}
```

### 5. `POST /api/v1/children` (Bearer) — 9-A 无 UI，仅 PowerShell 调
**Request:** `{"nickname": "小宇", "gender": "boy", "birthday": "2020-03-15"}`
**Response 201:** 同 item 形态

### 6. `POST /api/v1/stories/generate` (Bearer)
**Request:**
```json
{"child_id": 3, "prompt": "讲个森林小冒险", "duration": 5, "style": "温馨治愈", "topic": ""}
```
**Response 200:**
```json
{
  "id": 42,
  "title": "小宇的森林小冒险",
  "text": "从前...",
  "audio_object_key": "",
  "audio_status": "pending",
  "duration_minutes": 5,
  "style": "温馨治愈",
  "topic": "",
  "storyline_id": null,
  "episode_no": null,
  "created_at": "2026-05-15T08:23:45Z"
}
```
**错误：** 400 `invalid_duration` / 400 `redline_matched` / 403 `not_owner` / 404 `child_not_found` / 429 `rate_limited` / 503 `budget_exceeded` / 503 `generation_failed`

### 7. `GET /api/v1/stories/:id` (Bearer)
**Response 200:** 同上 Story 结构（`audio_status` 可能已变成 `ready`/`failed`）

### 8. `GET /api/v1/stories/:id/audio_url` (Bearer)
**3 态响应：**
- `200 {"audio_status": "ready", "url": "https://cos...", "expires_at": "..."}`
- `200 {"audio_status": "pending", "retry_after": 5}`
- `503 {"code": "audio_failed", "message": "音频生成失败，请稍后重新生成故事"}`

---

## 数据模型字段约定（Dart side）

### User
```dart
class User {
  final int id;
  final String nickname;
  final String subscriptionTier;
}
```

### Child
```dart
class Child {
  final int id;
  final int userId;
  final String nickname;
  final String gender;       // "boy" / "girl"
  final DateTime birthday;
  final String profileJson;  // raw JSON string，9-A 不解析
  int get ageYears => DateTime.now().difference(birthday).inDays ~/ 365;
}
```

### Story
```dart
class Story {
  final int id;
  final String title;
  final String text;
  final String audioObjectKey;
  final String audioStatus;  // pending/ready/failed
  final int durationMinutes;
  final String style;
  final String topic;
  final int? storylineId;
  final int? episodeNo;
  final DateTime createdAt;
}
```

### AudioUrlResponse (sealed)
```dart
sealed class AudioUrlResponse {}
class AudioReady extends AudioUrlResponse { final String url; final DateTime expiresAt; }
class AudioPending extends AudioUrlResponse { final int retryAfter; }
class AudioFailed extends AudioUrlResponse { final String message; }
```

---

# Tasks

## Task 0：Flutter 项目初始化

**Files:**
- Create: `app/` 整个目录（`flutter create` 生成）
- Modify: `app/pubspec.yaml`（加依赖）
- Modify: `app/analysis_options.yaml`（启用 flutter_lints）
- Modify: `app/android/app/src/main/AndroidManifest.xml`（确认 INTERNET 权限）

- [ ] **Step 0.1：先在 docs/knowledge/12-flutter.md 起个头**

`docs/knowledge/12-flutter.md`（新建）写入第一批词条骨架：
- 🎓 Flutter / Dart 是什么
- 🎓 widget / build / hot reload
- 🎓 pub / pubspec.yaml（类比 npm + package.json）

每条必须含"为什么需要"段。**这是项目硬要求，不能跳过。**

- [ ] **Step 0.2：创建项目**

```powershell
cd f:\claud\aibao_app
flutter create --org com.aibao --project-name aibao_app --platforms=android app
```

🎓 教学：`--org com.aibao` 指定包名前缀（Android 用 `com.aibao.aibao_app`）。Flutter 包名与 Java 包名规则一致。

- [ ] **Step 0.3：加依赖**

```powershell
cd app
flutter pub add flutter_riverpod dio just_audio go_router flutter_secure_storage
flutter pub add dev:http_mock_adapter dev:mocktail
```

确认 `pubspec.yaml` 应类似：
```yaml
name: aibao_app
description: "爱宝 - AI 儿童故事"
publish_to: 'none'
version: 0.1.0+1
environment:
  sdk: ^3.7.0

dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.8
  flutter_riverpod: ^2.5.1
  dio: ^5.7.0
  just_audio: ^0.9.40
  go_router: ^14.6.0
  flutter_secure_storage: ^9.2.2

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^5.0.0
  http_mock_adapter: ^0.6.1
  mocktail: ^1.0.4

flutter:
  uses-material-design: true
```

- [ ] **Step 0.4：验证模拟器能启动**

```powershell
flutter devices                        # 看到 emulator
flutter run -d emulator-5554           # 启动默认 counter app
```

模拟器看到 Flutter 默认计数器即可。Ctrl+C 退出。

- [ ] **Step 0.5：确认 AndroidManifest INTERNET**

打开 `app/android/app/src/main/AndroidManifest.xml`，确认含：
```xml
<uses-permission android:name="android.permission.INTERNET"/>
```
（`flutter create` 默认就有）

- [ ] **Step 0.6：commit**

```powershell
cd f:\claud\aibao_app
git add app/
git commit -m "feat(app): init flutter project (riverpod/dio/just_audio/go_router)"
```

---

## Task 1：主题与入口骨架

**Files:**
- Create: `app/lib/theme.dart`
- Modify: `app/lib/main.dart`（删除默认计数器，换成空 MaterialApp）

- [ ] **Step 1.1：theme.dart**

```dart
import 'package:flutter/material.dart';

const aibaoGreen = Color(0xFF2E7D32);

ThemeData aibaoTheme() {
  return ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(seedColor: aibaoGreen),
    textTheme: const TextTheme(
      headlineMedium: TextStyle(fontWeight: FontWeight.w600),
      bodyLarge: TextStyle(fontSize: 16, height: 1.5),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size.fromHeight(56),
        textStyle: const TextStyle(fontSize: 18),
      ),
    ),
  );
}
```

🎓 教学（Material 3 + ColorScheme.fromSeed）：M3 通过一颗种子色自动派生所有界面颜色（按钮、卡片、强调色等），不用手动配 10+ 颜色。"为什么需要"：避免设计稿和代码各画一套色板。

- [ ] **Step 1.2：main.dart 占位**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'theme.dart';

void main() => runApp(const ProviderScope(child: AibaoApp()));

class AibaoApp extends StatelessWidget {
  const AibaoApp({super.key});
  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: '爱宝',
      theme: aibaoTheme(),
      home: const Scaffold(body: Center(child: Text('🐼 爱宝启动中...'))),
    );
  }
}
```

🎓 教学（ProviderScope）：Riverpod 的根容器，必须包住整个 app，所有 provider 才能工作。类比"全局状态的总开关"。

- [ ] **Step 1.3：跑一下**

```powershell
flutter run -d emulator-5554
```

模拟器看到深绿底色 + "🐼 爱宝启动中..." 文字。Ctrl+C 退出。

- [ ] **Step 1.4：commit**

```powershell
git add app/lib/theme.dart app/lib/main.dart
git commit -m "feat(app): material 3 theme with aibao green seed"
```

---

## Task 2：API 客户端 + 异常类

**Files:**
- Create: `app/lib/api/api_exception.dart`
- Create: `app/lib/api/api_client.dart`

- [ ] **Step 2.1：api_exception.dart**

```dart
class ApiException implements Exception {
  final int? statusCode;
  final String reason;
  final String userMsg;
  ApiException({this.statusCode, required this.reason, required this.userMsg});
  @override
  String toString() => 'ApiException($statusCode $reason): $userMsg';
}
```

后端约定：所有错误返回 `{"reason": "...", "user_msg": "..."}` 或 `{"code": "...", "message": "..."}`（audio_url 是后者）。

- [ ] **Step 2.2：api_client.dart**

```dart
import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'api_exception.dart';

class ApiClient {
  final Dio _dio;
  final FlutterSecureStorage _storage;
  static const _tokenKey = 'access_token';

  ApiClient({String baseUrl = 'http://10.0.2.2:8080', Dio? dio, FlutterSecureStorage? storage})
      : _dio = dio ?? Dio(BaseOptions(
              baseUrl: baseUrl,
              connectTimeout: const Duration(seconds: 10),
              receiveTimeout: const Duration(seconds: 30),
              headers: {'Content-Type': 'application/json; charset=utf-8'},
            )),
        _storage = storage ?? const FlutterSecureStorage() {
    _dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) async {
        // 登录前的接口不带 token
        if (!options.path.startsWith('/api/v1/auth/')) {
          final token = await _storage.read(key: _tokenKey);
          if (token != null) {
            options.headers['Authorization'] = 'Bearer $token';
          }
        }
        handler.next(options);
      },
      onError: (e, handler) {
        // 不在这里 throw，留给 _wrap 统一翻译
        handler.next(e);
      },
    ));
    // dev 日志：打印请求/响应（生产前关掉）
    _dio.interceptors.add(LogInterceptor(requestBody: true, responseBody: true));
  }

  Future<void> setToken(String token) => _storage.write(key: _tokenKey, value: token);
  Future<void> clearToken() => _storage.delete(key: _tokenKey);
  Future<String?> readToken() => _storage.read(key: _tokenKey);

  Future<T> _wrap<T>(Future<Response> Function() fn, T Function(dynamic data) parse) async {
    try {
      final resp = await fn();
      return parse(resp.data);
    } on DioException catch (e) {
      final data = e.response?.data;
      String reason = 'network_error';
      String userMsg = '网络异常，请稍后重试';
      if (data is Map) {
        reason = (data['reason'] ?? data['code'] ?? reason).toString();
        userMsg = (data['user_msg'] ?? data['message'] ?? userMsg).toString();
      }
      throw ApiException(statusCode: e.response?.statusCode, reason: reason, userMsg: userMsg);
    }
  }

  // ---- auth ----
  Future<void> sendSmsCode(String phone) => _wrap(
      () => _dio.post('/api/v1/auth/sms/send', data: {'phone': phone}), (_) => null);

  Future<({String accessToken, String refreshToken, Map<String, dynamic> user})>
      loginOrRegister(String phone, String code, {String nickname = ''}) async {
    return _wrap(
      () => _dio.post('/api/v1/auth/login_or_register',
          data: {'phone': phone, 'code': code, 'nickname': nickname}),
      (data) => (
        accessToken: data['access_token'] as String,
        refreshToken: data['refresh_token'] as String,
        user: (data['user'] as Map).cast<String, dynamic>(),
      ),
    );
  }

  Future<Map<String, dynamic>> getMe() =>
      _wrap(() => _dio.get('/api/v1/me'), (d) => (d as Map).cast<String, dynamic>());

  Future<List<Map<String, dynamic>>> listChildren() => _wrap(
      () => _dio.get('/api/v1/children'),
      (d) => ((d['items'] ?? []) as List).cast<Map<String, dynamic>>());

  Future<Map<String, dynamic>> generateStory({
    required int childId, required String prompt,
    required int duration, required String style, String topic = '',
  }) => _wrap(
      () => _dio.post('/api/v1/stories/generate', data: {
            'child_id': childId, 'prompt': prompt,
            'duration': duration, 'style': style, 'topic': topic,
          }),
      (d) => (d as Map).cast<String, dynamic>());

  Future<Map<String, dynamic>> getStory(int id) => _wrap(
      () => _dio.get('/api/v1/stories/$id'),
      (d) => (d as Map).cast<String, dynamic>());

  Future<Map<String, dynamic>> getAudioUrl(int id) => _wrap(
      () => _dio.get('/api/v1/stories/$id/audio_url',
          options: Options(validateStatus: (s) => s != null && s < 600)),
      (d) => (d as Map).cast<String, dynamic>());
}
```

🎓 教学（dio interceptor）：拦截器 = 所有请求/响应都先经过的"中间件"。我们在请求前自动塞 JWT，比每次手动加省 100 行重复代码。"为什么需要"：如果没拦截器，每个 API 调用都要手动写 `headers['Authorization'] = ...`，一旦忘记一处就是 bug。

🎓 教学（`validateStatus`）：默认 dio 把 4xx/5xx 全当异常 throw；我们 audio_url 的 503 是**合法业务状态**（音频失败但不是网络错），所以放宽到 < 600 让自己解析。

- [ ] **Step 2.3：单元测试 api_client_test.dart**

`app/test/api_client_test.dart`：

```dart
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http_mock_adapter/http_mock_adapter.dart';
import 'package:aibao_app/api/api_client.dart';
import 'package:aibao_app/api/api_exception.dart';

void main() {
  late Dio dio; late DioAdapter adapter; late ApiClient client;

  setUp(() {
    dio = Dio(BaseOptions(baseUrl: 'http://test.local'));
    adapter = DioAdapter(dio: dio);
    client = ApiClient(dio: dio);
  });

  test('sendSmsCode 成功', () async {
    adapter.onPost('/api/v1/auth/sms/send',
        (s) => s.reply(200, {'sent': true}),
        data: {'phone': '13900000001'});
    await client.sendSmsCode('13900000001');
  });

  test('loginOrRegister 错误码翻译', () async {
    adapter.onPost('/api/v1/auth/login_or_register',
        (s) => s.reply(400, {'reason': 'invalid_code', 'user_msg': '验证码错误'}),
        data: {'phone': '13900000001', 'code': 'wrong', 'nickname': ''});
    expect(
      () => client.loginOrRegister('13900000001', 'wrong'),
      throwsA(isA<ApiException>()
          .having((e) => e.reason, 'reason', 'invalid_code')
          .having((e) => e.userMsg, 'userMsg', '验证码错误')),
    );
  });
}
```

跑测试：`flutter test`，全过。

- [ ] **Step 2.4：commit**

```powershell
git add app/lib/api/ app/test/api_client_test.dart
git commit -m "feat(api): dio-based ApiClient with JWT interceptor + typed methods"
```

---

## Task 3：DTO 模型

**Files:**
- Create: `app/lib/api/models/user.dart`
- Create: `app/lib/api/models/child.dart`
- Create: `app/lib/api/models/story.dart`
- Create: `app/lib/api/models/audio_url.dart`

- [ ] **Step 3.1：user.dart**

```dart
class User {
  final int id;
  final String nickname;
  final String subscriptionTier;
  const User({required this.id, required this.nickname, required this.subscriptionTier});
  factory User.fromJson(Map<String, dynamic> j) => User(
        id: j['id'] as int,
        nickname: (j['nickname'] ?? '') as String,
        subscriptionTier: (j['subscription_tier'] ?? 'free') as String,
      );
}
```

- [ ] **Step 3.2：child.dart**

```dart
class Child {
  final int id;
  final int userId;
  final String nickname;
  final String gender;
  final DateTime birthday;
  final String profileJson;
  const Child({
    required this.id, required this.userId, required this.nickname,
    required this.gender, required this.birthday, required this.profileJson,
  });
  factory Child.fromJson(Map<String, dynamic> j) => Child(
        id: j['id'] as int,
        userId: j['user_id'] as int,
        nickname: j['nickname'] as String,
        gender: j['gender'] as String,
        birthday: DateTime.parse(j['birthday'] as String),
        profileJson: (j['profile'] ?? '{}') as String,
      );

  int get ageYears {
    final now = DateTime.now();
    var age = now.year - birthday.year;
    if (now.month < birthday.month ||
        (now.month == birthday.month && now.day < birthday.day)) age--;
    return age;
  }
}
```

- [ ] **Step 3.3：story.dart**

```dart
class Story {
  final int id;
  final String title;
  final String text;
  final String audioObjectKey;
  final String audioStatus;  // pending / ready / failed
  final int durationMinutes;
  final String style;
  final String topic;
  final int? storylineId;
  final int? episodeNo;
  final DateTime createdAt;

  const Story({
    required this.id, required this.title, required this.text,
    required this.audioObjectKey, required this.audioStatus,
    required this.durationMinutes, required this.style, required this.topic,
    this.storylineId, this.episodeNo, required this.createdAt,
  });

  factory Story.fromJson(Map<String, dynamic> j) => Story(
        id: j['id'] as int,
        title: (j['title'] ?? '') as String,
        text: (j['text'] ?? '') as String,
        audioObjectKey: (j['audio_object_key'] ?? '') as String,
        audioStatus: (j['audio_status'] ?? 'pending') as String,
        durationMinutes: j['duration_minutes'] as int,
        style: (j['style'] ?? '') as String,
        topic: (j['topic'] ?? '') as String,
        storylineId: j['storyline_id'] as int?,
        episodeNo: j['episode_no'] as int?,
        createdAt: DateTime.parse(j['created_at'] as String),
      );
}
```

- [ ] **Step 3.4：audio_url.dart（sealed class）**

```dart
sealed class AudioUrlResponse {
  const AudioUrlResponse();
  factory AudioUrlResponse.fromJson(Map<String, dynamic> j, int statusCode) {
    if (statusCode == 503 || j['code'] == 'audio_failed') {
      return AudioFailed(message: (j['message'] ?? '音频生成失败') as String);
    }
    final status = (j['audio_status'] ?? 'pending') as String;
    if (status == 'ready') {
      return AudioReady(
        url: j['url'] as String,
        expiresAt: DateTime.parse(j['expires_at'] as String),
      );
    }
    if (status == 'failed') return AudioFailed(message: '音频生成失败');
    return AudioPending(retryAfter: (j['retry_after'] ?? 5) as int);
  }
}

class AudioReady extends AudioUrlResponse {
  final String url;
  final DateTime expiresAt;
  const AudioReady({required this.url, required this.expiresAt});
}

class AudioPending extends AudioUrlResponse {
  final int retryAfter;
  const AudioPending({required this.retryAfter});
}

class AudioFailed extends AudioUrlResponse {
  final String message;
  const AudioFailed({required this.message});
}
```

🎓 教学（sealed class + pattern matching）：Dart 3 引入的"封闭类"，子类有限可数，编译器会强制 `switch` 覆盖所有分支。比"用 String enum 表示状态"安全得多。"为什么需要"：避免漏写一个状态分支导致 UI 卡死。

- [ ] **Step 3.5：commit**

```powershell
git add app/lib/api/models/
git commit -m "feat(api): dto models with fromJson"
```

---

## Task 4：Auth 状态层

**Files:**
- Create: `app/lib/state/auth_state.dart`

- [ ] **Step 4.1：auth_state.dart**

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/models/user.dart';

final apiClientProvider = Provider<ApiClient>((_) => ApiClient());

enum AuthStatus { unknown, authenticated, anonymous }

class AuthState {
  final AuthStatus status;
  final User? user;
  final String? error;
  const AuthState(this.status, {this.user, this.error});
}

class AuthNotifier extends StateNotifier<AuthState> {
  final ApiClient api;
  AuthNotifier(this.api) : super(const AuthState(AuthStatus.unknown)) { _bootstrap(); }

  Future<void> _bootstrap() async {
    final token = await api.readToken();
    if (token == null) {
      state = const AuthState(AuthStatus.anonymous);
      return;
    }
    try {
      final me = await api.getMe();
      state = AuthState(AuthStatus.authenticated, user: User.fromJson(me));
    } on ApiException {
      await api.clearToken();
      state = const AuthState(AuthStatus.anonymous);
    }
  }

  Future<void> sendCode(String phone) async {
    await api.sendSmsCode(phone);
  }

  Future<void> login(String phone, String code) async {
    final r = await api.loginOrRegister(phone, code);
    await api.setToken(r.accessToken);
    state = AuthState(AuthStatus.authenticated, user: User.fromJson(r.user));
  }

  Future<void> logout() async {
    await api.clearToken();
    state = const AuthState(AuthStatus.anonymous);
  }
}

final authProvider = StateNotifierProvider<AuthNotifier, AuthState>(
  (ref) => AuthNotifier(ref.watch(apiClientProvider)),
);
```

🎓 教学（Riverpod 三种 provider）：
1. `Provider`：永不变的依赖（如 ApiClient）
2. `StateNotifierProvider`：可变状态 + 业务方法
3. `FutureProvider`：异步数据（一次性 fetch）
"为什么需要"：把"状态"和"UI"解耦——UI 只看状态，状态变更不需要 UI 显式重建。

- [ ] **Step 4.2：commit**

```powershell
git add app/lib/state/auth_state.dart
git commit -m "feat(state): auth notifier with secure jwt storage"
```

---

## Task 5：Child + Story 状态层

**Files:**
- Create: `app/lib/state/child_state.dart`
- Create: `app/lib/state/story_state.dart`

- [ ] **Step 5.1：child_state.dart**

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/models/child.dart';
import 'auth_state.dart';

final currentChildProvider = FutureProvider<Child?>((ref) async {
  final auth = ref.watch(authProvider);
  if (auth.status != AuthStatus.authenticated) return null;
  final api = ref.watch(apiClientProvider);
  final list = await api.listChildren();
  if (list.isEmpty) return null;
  return Child.fromJson(list.first);
});
```

🎓 教学（FutureProvider + ref.watch）：FutureProvider 自动管 loading/error/data 三态；`ref.watch(authProvider)` 表示"只要 auth 变了就重跑"，依赖跟踪自动。

- [ ] **Step 5.2：story_state.dart**

```dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/models/audio_url.dart';
import '../api/models/story.dart';
import 'auth_state.dart';

class GenerateInput {
  final int childId; final String prompt;
  final int duration; final String style;
  const GenerateInput({required this.childId, required this.prompt,
      required this.duration, required this.style});
}

// 一次性触发：生成故事
final storyGenerateProvider =
    FutureProvider.family<Story, GenerateInput>((ref, input) async {
  final api = ref.watch(apiClientProvider);
  final j = await api.generateStory(
    childId: input.childId, prompt: input.prompt,
    duration: input.duration, style: input.style,
  );
  return Story.fromJson(j);
});

// 单个故事详情
final storyByIdProvider = FutureProvider.family<Story, int>((ref, id) async {
  final api = ref.watch(apiClientProvider);
  return Story.fromJson(await api.getStory(id));
});

// audio_url 轮询：每 3s 重发，直到 ready 或 failed
final audioUrlPollProvider =
    StreamProvider.family<AudioUrlResponse, int>((ref, storyId) async* {
  final api = ref.watch(apiClientProvider);
  while (true) {
    Map<String, dynamic> data;
    int status = 200;
    try {
      data = await api.getAudioUrl(storyId);
    } catch (_) {
      yield const AudioFailed(message: '网络异常');
      return;
    }
    // 503 也能进来（validateStatus<600），但 dio 把 status 吃了，靠 body code 判断
    final resp = AudioUrlResponse.fromJson(data, status);
    yield resp;
    if (resp is AudioReady || resp is AudioFailed) return;
    await Future.delayed(const Duration(seconds: 3));
  }
});
```

🎓 教学（StreamProvider）：用于"多次产出值"的异步——比如轮询。`yield` 一次就推一帧到 UI。"为什么需要"：避免在 UI 里写 `Timer.periodic` + setState，更声明式。

- [ ] **Step 5.3：commit**

```powershell
git add app/lib/state/
git commit -m "feat(state): child + story state notifiers"
```

---

## Task 6：登录屏

**Files:**
- Create: `app/lib/screens/login_screen.dart`

- [ ] **Step 6.1：login_screen.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../api/api_exception.dart';
import '../state/auth_state.dart';

class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});
  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _phone = TextEditingController(text: '13900000001');
  final _code = TextEditingController();
  bool _sending = false;
  bool _logging = false;

  Future<void> _sendCode() async {
    setState(() => _sending = true);
    try {
      await ref.read(authProvider.notifier).sendCode(_phone.text.trim());
      if (mounted) _toast('验证码已发送（dev: 123456）');
    } on ApiException catch (e) {
      if (mounted) _toast(e.userMsg);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _login() async {
    setState(() => _logging = true);
    try {
      await ref.read(authProvider.notifier).login(_phone.text.trim(), _code.text.trim());
      if (mounted) context.go('/home');
    } on ApiException catch (e) {
      if (mounted) _toast(e.userMsg);
    } finally {
      if (mounted) setState(() => _logging = false);
    }
  }

  void _toast(String s) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 40),
              const Text('🐼', style: TextStyle(fontSize: 80), textAlign: TextAlign.center),
              const SizedBox(height: 12),
              Text('爱宝', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
              const SizedBox(height: 4),
              const Text('AI 故事伙伴', textAlign: TextAlign.center),
              const SizedBox(height: 48),
              TextField(
                controller: _phone, keyboardType: TextInputType.phone,
                decoration: const InputDecoration(labelText: '手机号', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 16),
              Row(children: [
                Expanded(child: TextField(
                  controller: _code, keyboardType: TextInputType.number,
                  decoration: const InputDecoration(labelText: '验证码', border: OutlineInputBorder()),
                )),
                const SizedBox(width: 12),
                FilledButton.tonal(
                  onPressed: _sending ? null : _sendCode,
                  child: Text(_sending ? '发送中...' : '发送验证码'),
                ),
              ]),
              const SizedBox(height: 32),
              FilledButton(
                onPressed: _logging ? null : _login,
                child: Text(_logging ? '登录中...' : '登录 / 注册'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
```

🎓 教学（ConsumerStatefulWidget）：带 Riverpod 的 StatefulWidget。`ref` 自动注入，可 `ref.read`（一次性读）或 `ref.watch`（订阅变更）。

- [ ] **Step 6.2：commit**

```powershell
git add app/lib/screens/login_screen.dart
git commit -m "feat(screen): login (phone + 6-digit code)"
```

---

## Task 7：主页屏

**Files:**
- Create: `app/lib/screens/home_screen.dart`

- [ ] **Step 7.1：home_screen.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../state/auth_state.dart';
import '../state/child_state.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final childAsync = ref.watch(currentChildProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Row(children: [Text('🐼 ', style: TextStyle(fontSize: 28)), Text('爱宝')]),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout),
            onPressed: () async {
              await ref.read(authProvider.notifier).logout();
              if (context.mounted) context.go('/login');
            },
          ),
        ],
      ),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: childAsync.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (e, _) => Center(child: Text('加载失败：$e')),
          data: (child) {
            if (child == null) {
              return const Center(
                child: Text('请先在后台为你的账号创建一个孩子档案（9-A 暂无 UI）',
                    textAlign: TextAlign.center),
              );
            }
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Row(children: [
                      const Text('👦', style: TextStyle(fontSize: 56)),
                      const SizedBox(width: 16),
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(child.nickname, style: Theme.of(context).textTheme.headlineSmall),
                          const SizedBox(height: 4),
                          Text('${child.ageYears} 岁'),
                        ],
                      ),
                    ]),
                  ),
                ),
                const Spacer(),
                FilledButton.icon(
                  icon: const Icon(Icons.auto_stories),
                  label: const Text('今天讲什么故事？'),
                  onPressed: () => context.go('/generate', extra: child.id),
                ),
                const SizedBox(height: 48),
              ],
            );
          },
        ),
      ),
    );
  }
}
```

- [ ] **Step 7.2：commit**

```powershell
git add app/lib/screens/home_screen.dart
git commit -m "feat(screen): home with child card"
```

---

## Task 8：生成屏 + 配套 widget

**Files:**
- Create: `app/lib/widgets/duration_chips.dart`
- Create: `app/lib/widgets/style_dropdown.dart`
- Create: `app/lib/widgets/waiting_aibao.dart`
- Create: `app/lib/screens/generate_screen.dart`

- [ ] **Step 8.1：duration_chips.dart**

```dart
import 'package:flutter/material.dart';

class DurationChips extends StatelessWidget {
  final int value;
  final ValueChanged<int> onChanged;
  const DurationChips({super.key, required this.value, required this.onChanged});
  @override
  Widget build(BuildContext context) {
    return Wrap(spacing: 12, children: [5, 10, 15].map((d) {
      return ChoiceChip(
        label: Text('$d 分钟'),
        selected: value == d,
        onSelected: (_) => onChanged(d),
      );
    }).toList());
  }
}
```

- [ ] **Step 8.2：style_dropdown.dart**

```dart
import 'package:flutter/material.dart';

const styleOptions = ['温馨治愈', '冒险探索', '搞笑欢乐', '神奇魔法', '科普认知'];

class StyleDropdown extends StatelessWidget {
  final String value;
  final ValueChanged<String> onChanged;
  const StyleDropdown({super.key, required this.value, required this.onChanged});
  @override
  Widget build(BuildContext context) {
    return DropdownButtonFormField<String>(
      value: value,
      decoration: const InputDecoration(labelText: '故事风格', border: OutlineInputBorder()),
      items: styleOptions
          .map((s) => DropdownMenuItem(value: s, child: Text(s)))
          .toList(),
      onChanged: (v) { if (v != null) onChanged(v); },
    );
  }
}
```

- [ ] **Step 8.3：waiting_aibao.dart**

```dart
import 'package:flutter/material.dart';

class WaitingAibao extends StatelessWidget {
  const WaitingAibao({super.key});
  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.black54,
      child: Center(
        child: Card(
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: const [
                Text('🐼', style: TextStyle(fontSize: 72)),
                SizedBox(height: 16),
                Text('爱宝在想...', style: TextStyle(fontSize: 18)),
                SizedBox(height: 16),
                CircularProgressIndicator(),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 8.4：generate_screen.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../api/api_exception.dart';
import '../state/story_state.dart';
import '../widgets/duration_chips.dart';
import '../widgets/style_dropdown.dart';
import '../widgets/waiting_aibao.dart';

class GenerateScreen extends ConsumerStatefulWidget {
  final int childId;
  const GenerateScreen({super.key, required this.childId});
  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  final _prompt = TextEditingController();
  int _duration = 5;
  String _style = '温馨治愈';
  bool _busy = false;

  Future<void> _submit() async {
    if (_prompt.text.trim().isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('请输入故事需求')));
      return;
    }
    setState(() => _busy = true);
    try {
      final story = await ref.read(storyGenerateProvider(GenerateInput(
        childId: widget.childId, prompt: _prompt.text.trim(),
        duration: _duration, style: _style,
      )).future);
      if (mounted) context.go('/player/${story.id}');
    } on ApiException catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.userMsg)));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('生成失败：$e')));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Stack(children: [
      Scaffold(
        appBar: AppBar(title: const Text('讲个故事')),
        body: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                controller: _prompt, maxLines: 3,
                decoration: const InputDecoration(
                  labelText: '想听什么故事？', hintText: '例：讲个森林小冒险',
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 24),
              const Text('时长', style: TextStyle(fontWeight: FontWeight.bold)),
              const SizedBox(height: 8),
              DurationChips(value: _duration, onChanged: (v) => setState(() => _duration = v)),
              const SizedBox(height: 24),
              StyleDropdown(value: _style, onChanged: (v) => setState(() => _style = v)),
              const Spacer(),
              FilledButton(
                onPressed: _busy ? null : _submit,
                child: const Text('开始'),
              ),
            ],
          ),
        ),
      ),
      if (_busy) const WaitingAibao(),
    ]);
  }
}
```

- [ ] **Step 8.5：commit**

```powershell
git add app/lib/widgets/ app/lib/screens/generate_screen.dart
git commit -m "feat(screen): generate (prompt + duration/style + loading)"
```

---

## Task 9：播放屏

**Files:**
- Create: `app/lib/screens/player_screen.dart`

- [ ] **Step 9.1：player_screen.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:just_audio/just_audio.dart';
import '../api/models/audio_url.dart';
import '../state/story_state.dart';

class PlayerScreen extends ConsumerStatefulWidget {
  final int storyId;
  const PlayerScreen({super.key, required this.storyId});
  @override
  ConsumerState<PlayerScreen> createState() => _PlayerScreenState();
}

class _PlayerScreenState extends ConsumerState<PlayerScreen> {
  final _player = AudioPlayer();
  String? _loadedUrl;
  DateTime? _urlExpiresAt;

  @override
  void dispose() {
    _player.dispose();
    super.dispose();
  }

  Future<void> _ensureUrl(AudioReady ready) async {
    // 14 分钟内不重复 setUrl；过期或换 URL 才重设
    if (_loadedUrl == ready.url) return;
    _loadedUrl = ready.url;
    _urlExpiresAt = ready.expiresAt;
    await _player.setUrl(ready.url);
  }

  @override
  Widget build(BuildContext context) {
    final storyAsync = ref.watch(storyByIdProvider(widget.storyId));
    final audioAsync = ref.watch(audioUrlPollProvider(widget.storyId));

    return Scaffold(
      appBar: AppBar(title: const Text('故事')),
      body: storyAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('加载失败：$e')),
        data: (story) => Column(children: [
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(story.title, style: Theme.of(context).textTheme.headlineSmall),
                  const SizedBox(height: 16),
                  Text(story.text, style: const TextStyle(fontSize: 16, height: 1.6)),
                ],
              ),
            ),
          ),
          Container(
            padding: const EdgeInsets.all(16),
            color: Theme.of(context).colorScheme.surfaceContainerHighest,
            child: audioAsync.when(
              loading: () => const _Status('🐼 音频准备中...'),
              error: (e, _) => _Status('音频加载失败：$e'),
              data: (resp) {
                return switch (resp) {
                  AudioPending() => const _Status('🐼 爱宝在录音中...'),
                  AudioFailed(message: final m) => _Status('❌ $m'),
                  AudioReady r => FutureBuilder(
                      future: _ensureUrl(r),
                      builder: (_, __) => _PlayerControls(player: _player),
                    ),
                };
              },
            ),
          ),
        ]),
      ),
    );
  }
}

class _Status extends StatelessWidget {
  final String text;
  const _Status(this.text);
  @override
  Widget build(BuildContext context) =>
      SizedBox(height: 80, child: Center(child: Text(text)));
}

class _PlayerControls extends StatelessWidget {
  final AudioPlayer player;
  const _PlayerControls({required this.player});
  @override
  Widget build(BuildContext context) {
    return StreamBuilder<PlayerState>(
      stream: player.playerStateStream,
      builder: (_, snap) {
        final playing = snap.data?.playing ?? false;
        return Row(children: [
          IconButton.filled(
            iconSize: 40,
            icon: Icon(playing ? Icons.pause : Icons.play_arrow),
            onPressed: () => playing ? player.pause() : player.play(),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: StreamBuilder<Duration>(
              stream: player.positionStream,
              builder: (_, posSnap) {
                final pos = posSnap.data ?? Duration.zero;
                final dur = player.duration ?? Duration.zero;
                return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Slider(
                    min: 0,
                    max: dur.inMilliseconds.toDouble().clamp(1, double.infinity),
                    value: pos.inMilliseconds.toDouble().clamp(0, dur.inMilliseconds.toDouble()),
                    onChanged: (v) => player.seek(Duration(milliseconds: v.toInt())),
                  ),
                  Text('${_fmt(pos)} / ${_fmt(dur)}', style: const TextStyle(fontSize: 12)),
                ]);
              },
            ),
          ),
        ]);
      },
    );
  }

  static String _fmt(Duration d) {
    final m = d.inMinutes.toString().padLeft(2, '0');
    final s = (d.inSeconds % 60).toString().padLeft(2, '0');
    return '$m:$s';
  }
}
```

🎓 教学（just_audio）：声明式音频播放器——`setUrl` 加载远端 mp3，`playerStateStream` 推送 playing/paused 状态，`positionStream` 推送当前秒数。我们不用手写 Timer 同步进度条。

- [ ] **Step 9.2：commit**

```powershell
git add app/lib/screens/player_screen.dart
git commit -m "feat(screen): player (text + just_audio with poll)"
```

---

## Task 10：路由 + 入口集成

**Files:**
- Create: `app/lib/router.dart`
- Modify: `app/lib/main.dart`

- [ ] **Step 10.1：router.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'screens/login_screen.dart';
import 'screens/home_screen.dart';
import 'screens/generate_screen.dart';
import 'screens/player_screen.dart';
import 'state/auth_state.dart';

GoRouter buildRouter(Ref ref) {
  return GoRouter(
    initialLocation: '/',
    refreshListenable: _AuthListenable(ref),
    redirect: (ctx, st) {
      final auth = ref.read(authProvider);
      final loggingIn = st.matchedLocation == '/login';
      if (auth.status == AuthStatus.unknown) return null;
      if (auth.status == AuthStatus.anonymous) return loggingIn ? null : '/login';
      if (loggingIn || st.matchedLocation == '/') return '/home';
      return null;
    },
    routes: [
      GoRoute(path: '/', builder: (_, __) => const Scaffold(body: Center(child: CircularProgressIndicator()))),
      GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
      GoRoute(path: '/home', builder: (_, __) => const HomeScreen()),
      GoRoute(
        path: '/generate',
        builder: (_, st) => GenerateScreen(childId: st.extra as int),
      ),
      GoRoute(
        path: '/player/:id',
        builder: (_, st) => PlayerScreen(storyId: int.parse(st.pathParameters['id']!)),
      ),
    ],
  );
}

class _AuthListenable extends ChangeNotifier {
  _AuthListenable(this.ref) {
    ref.listen(authProvider, (_, __) => notifyListeners());
  }
  final Ref ref;
}

final routerProvider = Provider<GoRouter>((ref) => buildRouter(ref));
```

🎓 教学（go_router redirect）：声明式重定向——每次路由变更前回调一次，返回新路径即跳转，返回 null 即放行。配合 Riverpod 的 `refreshListenable`，auth 状态变了路由自动重新评估。

- [ ] **Step 10.2：改 main.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'router.dart';
import 'theme.dart';

void main() => runApp(const ProviderScope(child: AibaoApp()));

class AibaoApp extends ConsumerWidget {
  const AibaoApp({super.key});
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

- [ ] **Step 10.3：跑通**

```powershell
flutter analyze    # 期望 0 issues
flutter run -d emulator-5554
```

App 启动 → 无 token 自动到 `/login`。

- [ ] **Step 10.4：commit**

```powershell
git add app/lib/router.dart app/lib/main.dart
git commit -m "feat(app): go_router with auth redirect"
```

---

## Task 11：端到端冒烟（手动）

不写代码，仅按步骤手动验证 DoD。

- [ ] **Step 11.1：起后端 + 准备数据**

```powershell
# Terminal A: 启动后端
cd f:\claud\aibao_app\server
make run-dev
```

```powershell
# Terminal B: 提前注册账号 + 建 child
$base = "http://127.0.0.1:8080"
Invoke-RestMethod -Method Post -Uri "$base/api/v1/auth/sms/send" `
  -ContentType "application/json" -Body '{"phone":"13900000001"}'

$login = Invoke-RestMethod -Method Post -Uri "$base/api/v1/auth/login_or_register" `
  -ContentType "application/json" `
  -Body '{"phone":"13900000001","code":"123456","nickname":""}'
$token = $login.access_token

Invoke-RestMethod -Method Post -Uri "$base/api/v1/children" `
  -Headers @{Authorization = "Bearer $token"} `
  -ContentType "application/json" `
  -Body '{"nickname":"小宇","gender":"boy","birthday":"2020-03-15"}'
```

- [ ] **Step 11.2：起模拟器跑 app**

```powershell
cd f:\claud\aibao_app\app
flutter run -d emulator-5554
```

- [ ] **Step 11.3：手动跑一遍 4 屏**

按 DoD §2-§6 一步步操作，确认每屏行为正确。

- [ ] **Step 11.4：DB 检查**

```powershell
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "SELECT id, child_id, title, audio_status FROM stories ORDER BY id DESC LIMIT 3;"
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "SELECT id, status FROM outbox_events ORDER BY id DESC LIMIT 3;"
```
Expected: 故事在 stories 表，outbox done，audio_status=ready。

- [ ] **Step 11.5：JWT 持久化验证**

模拟器内 swipe up 关掉 app → 重新点图标 → 直接进 `/home`，不要登录。

- [ ] **Step 11.6：调试小贴士（写到 devlog）**

记录如何查 dio 请求日志（`LogInterceptor` 已开）、如何看 just_audio 状态（断点 `playerStateStream`）、如何手动让 token 过期（PowerShell 删 keystore：在 emulator 内 `adb shell pm clear com.aibao.aibao_app`）。

---

## Task 12：知识库 + 文档收口

**Files:**
- Create / Modify: `docs/knowledge/12-flutter.md`
- Modify: `docs/knowledge/README.md`（加索引）
- Modify: `CLAUDE.md`（第 2 节加 Plan 9-A 完成标记）
- Modify: `MEMORY.md`（追加 Plan 9-A 关键决策）
- Create: `docs/devlog/2026-05-15-plan-09a-flutter-mvp.md`

- [ ] **Step 12.1：完成 12-flutter.md 全部词条**

至少包含（每条带"为什么需要"段）：
1. Flutter 是什么 / 与 React Native 区别
2. Dart 语言一句话
3. widget / build / hot reload
4. StatelessWidget vs StatefulWidget vs ConsumerWidget
5. pubspec.yaml / pub get（类比 npm + package.json）
6. Riverpod 三种 Provider（Provider / StateNotifierProvider / FutureProvider / StreamProvider）
7. dio 与 interceptor
8. sealed class + pattern matching（Dart 3）
9. go_router 声明式路由 + redirect
10. just_audio 与 Stream
11. flutter_secure_storage / Android Keystore（为什么不用 SharedPreferences）
12. `10.0.2.2` 是什么（Android emulator → 宿主 127.0.0.1 映射）

- [ ] **Step 12.2：CLAUDE.md 第 2 节"当前阶段"追加**

```
Plan 9-A 完成（2026-05-15）：Flutter Android MVP 4 屏跑通登录→主页→生成→播放，
不含 BOOTSTRAP/HEARTBEAT/连续剧。
```

- [ ] **Step 12.3：MEMORY.md 追加关键决策**

记录：Riverpod / dio / go_router / just_audio / flutter_secure_storage 选型理由 + 9-A 范围圈定。

- [ ] **Step 12.4：devlog 写一篇**

`docs/devlog/2026-05-15-plan-09a-flutter-mvp.md`：复盘 Plan 9-A 过程、踩过的坑（中文字体？模拟器网络？just_audio 在 emulator 上有无问题？）、给 9b 留的话。

- [ ] **Step 12.5：final commit**

```powershell
git add docs/ CLAUDE.md MEMORY.md
git commit -m "docs(plan9a): knowledge + devlog + claude.md sync"
```

---

## 附录 A：常见坑提示（实施时避雷）

1. **`10.0.2.2` 只对 Android emulator 生效**。如果改成在真机上调试（USB），需要把 `baseUrl` 改成"宿主机局域网 IP"（如 `http://192.168.1.10:8080`），并确保后端 `0.0.0.0:8080` 监听（而非 `127.0.0.1`）。可在 `ApiClient` 暴露 `baseUrl` 参数支持运行时切换。
2. **中文字体**：Flutter 3.29.3 Android 默认 fallback 是 PingFang/Source Han Sans，应该正常。若显示豆腐块再加 `google_fonts` 装 Noto Sans SC。
3. **dio + 中文**：dio 默认 UTF-8 编码 JSON body，**不会**复现 Plan 4 时 Git Bash curl 的 GBK bug（详见知识库 6.11）——可在 devlog 里点一句"客户端验证了那个 bug 真的只是 curl/Windows GBK 问题"。
4. **just_audio + Android 10+**：需 `INTERNET` 权限（默认有）。如果在低版本 Android 无法播放 HTTPS（COS 签名 URL），需 `network_security_config.xml`——不过 COS 用合法 CA 证书，应该不需要。
5. **secure_storage 模拟器 wipe**：模拟器恢复出厂会清掉 keystore，下次需要重新登录——这是预期行为。
6. **audio_url TTL 15 分钟**：用户在 player 暂停超 14 分钟后恢复，URL 可能已过期。9-A 不主动续约（简单做法），让 just_audio 在 setUrl 失败时 fail，UI 显示错误并提示"返回重听"。9b 再做自动重取。
7. **dio LogInterceptor 在生产要关**：上线前要把 `LogInterceptor` 用 `kReleaseMode` 包一下，否则 JWT 会进 logcat。
8. **403 not_owner 误踩**：测试时如果换了账号没清 secure_storage，会用旧 token 拿 child=3，触发 not_owner。`adb shell pm clear com.aibao.aibao_app` 清一下即可。

---

## 附录 B：TBD / Plan 9b 要做

- BOOTSTRAP 7 题问卷 UI（PATCH `/api/v1/bootstrap/answers`）
- HEARTBEAT 时段问候卡（早/中/晚不同主页 hero）
- 连续剧入口（home 增 "上次没听完的故事" + generate 提供 "续上次" 选项）
- 新孩子档案 UI（注册首屏 → 建 child 表单）
- 错误屏（断网 / 后端宕机 / token 过期友好提示）
- 暗色模式
- iOS 适配（需要重新跑 `flutter create --platforms=ios .` 在 app 目录）
- Web debug 通道（需后端配 CORS，9-A 不要求跑通）
- 应用商店上架（图标 / 截图 / 隐私政策 / 备案）
- 录音输入 prompt（speech_to_text 包 + 麦克风权限）
- 自定义 🐼 logo（spec 之后设计）
- 单元测试 / widget test 全覆盖

---

**Plan 9-A 结束。** 实施完成后下一步选 Plan 9b（功能补齐）或 Plan 10（部署上线）。
