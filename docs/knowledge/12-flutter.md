# 12. Flutter 客户端开发

爱宝项目首次引入 Flutter 在 Plan 9-A（2026-05-16）。本文件记录从零接客户端时绕不开的概念 + 真机调试踩到的工程坑。

---

## 12.1 Widget / StatefulWidget / StatelessWidget 三件套

🎓 Flutter UI 的最基本抽象。

**是什么**：
- **Widget**：UI 的"配置说明"。它是 immutable（不可变）的——每次状态变化都会**新建一个**新 widget 树，不是原地改。
- **StatelessWidget**：build 一次确定就不再变的 widget（如一个标题文字、一个图标）。
- **StatefulWidget**：自己有内部可变状态的 widget。它本身仍然 immutable，但配套一个 `State<T>` 类承载可变状态。State 通过 `setState(() { ... })` 触发框架重新调用 build。

**生活类比**：Widget 像建筑图纸（不可变），State 像房子里的家具摆放（可变）。每次 setState 就是"按新图纸 + 旧家具，框架自己 diff 出哪些墙要换"。

**在本项目中**：
- 4 个屏都是 `ConsumerStatefulWidget`（Riverpod 提供的 StatefulWidget 升级版，能 watch provider）
- 输入框、计时器、播放进度都是 State 管理的

**为什么需要**：理解"widget 是 immutable + State 是 mutable + setState 触发 diff"是写 Flutter 不出 bug 的基本功。不少初学者会把数据塞到 widget 的 final 字段然后困惑"为啥改了没效果"——那是因为 widget 已经 immutable 了，必须走 State。

**何时引入**：Plan 9-A Task 1。

---

## 12.2 Riverpod 状态管理 2.x

🎓 Flutter 没原生 DI / 全局状态框架，Riverpod 是事实标准。

**是什么**：把状态和服务"挂"在一个全局 ProviderScope 上，widget 通过 `ref.watch()` / `ref.read()` 读写。三类常用 provider：

| Provider | 用途 | 本项目示例 |
|---|---|---|
| `Provider<T>` | 不变值 / 服务单例 | `apiClientProvider` 提供 dio 实例 |
| `StateNotifierProvider<N, T>` | 可变状态 + 通知（2.x 旧 API） | `authProvider` 持有登录状态 |
| `AsyncNotifierProvider<N, T>` | 异步加载状态（含 loading/error/data 三态） | `storyGenerationProvider` 处理生成流程 |
| `FutureProvider.family<T, Arg>` | 参数化 future | 按 child_id 查孩子档案 |

**版本说明**：本项目用 `flutter_riverpod 2.6.1`。3.x 系列**不兼容 Flutter SDK 3.29.3**——切了一次 3.x 报一堆 API 改名错误。Plan 9-A 锁定 2.6.1。

**生活类比**：Provider 像"全局公告栏"。Widget 订阅公告，公告内容变就自动重 build。

**为什么需要**：Flutter 没原生 DI 框架；setState 只能管单个 widget 的本地状态，跨屏共享（如登录态）必须有全局机制。Riverpod 比官方推荐的 Provider 包多一层 `ref` 抽象，更易测、更易组合。3 类 provider 的区分让"什么场景用什么"有标准答案，不会全部塞一个万能容器。

**何时引入**：Plan 9-A Task 4（auth notifier）。

---

## 12.3 dio HTTP 拦截器与 baseUrl 策略

🎓 dio 是 Flutter 主流 HTTP 库（类似 Go 的 `net/http` + middleware）。

**是什么**：

- **Interceptor（拦截器）**：在每个请求 / 响应前后跑的 hook。类似 Go 的 middleware 链。本项目用 `onRequest` 自动附 JWT 头（从 secure storage 拿 token 塞 Authorization）+ `onResponse` 统一处理 401（清登录态跳回 login）。
- **baseUrl**：dio 配置里的统一前缀。但**手机的 localhost 是手机自己**，所以 Flutter 真机调试时不能写死 `localhost`：

| 场景 | baseUrl | 配合 |
|---|---|---|
| Android **模拟器** | `http://10.0.2.2:8080` | emulator 内部把 10.0.2.2 路由到宿主机 |
| Android **真机**（USB） | `http://127.0.0.1:8080` | + `adb reverse tcp:8080 tcp:8080`（手机的 8080 反向映射回 PC） |
| Android **真机**（WiFi） | `http://192.168.x.y:8080` | 用 PC 在局域网的 IP；防火墙允许入站 |

本项目最终选 `127.0.0.1` + adb reverse（Issue 4 修复后）。

**生活类比**：adb reverse 是给手机装的"私人快递员"——手机说"我要寄给 localhost"，快递员偷偷把包裹送到 PC 那边的 8080。

**为什么需要**：客户端代码不能写死 `localhost`——你的"本机"和手机的"本机"不是同一台。真机调试必须用 ADB 反向隧道或宿主局域网 IP。这是几乎所有 mobile dev 第一周必踩的坑。

**何时引入**：Plan 9-A Task 3 / Issue 4。

---

## 12.4 go_router + Riverpod 桥接

🎓 go_router 是 Flutter 官方推荐的声明式路由库。

**是什么**：go_router 提供 `refreshListenable` 参数——传入一个 `Listenable`，路由系统监听它变化时重跑 redirect 函数。问题是 Riverpod 的 state 不是 Listenable。

**桥接方法**：写一个 `GoRouterRefreshNotifier extends ChangeNotifier`，在构造时订阅 authProvider，每次 auth state 变就 `notifyListeners()`。把这个 notifier 给 go_router。

**生活类比**：go_router 听不懂 Riverpod 的"广播频道"，所以塞一个翻译官（ChangeNotifier）在中间——Riverpod 喊话 → 翻译官 → go_router 听见 → 重判断要不要跳转。

**在本项目中**：登录成功后 authProvider 从 `unauthenticated` 切到 `authenticated`，notifier 立刻 fire，go_router 的 redirect 重跑，把当前 `/login` 重定向到 `/home`。Splash → Login → Home 全自动切换。

**为什么需要**：路由层必须响应 auth 状态变化（不能让登录页一直停留在已登录用户面前；不能让 home 屏被未登录用户访问）。go_router + Riverpod 是 Flutter 生态最常用组合，这个桥接套路是标准答案，第一次写时容易卡很久。

**何时引入**：Plan 9-A Task 10（commit e908985）。

---

## 12.5 Android 平台配置最低集合（dev/debug）

🎓 Flutter 真机 dev 必做 5 件事，少一件就报错。

| 配置 | 文件 | 值 | 为什么 |
|---|---|---|---|
| INTERNET 权限 | `AndroidManifest.xml` | `<uses-permission android:name="android.permission.INTERNET" />` | release 模式不自动注入 |
| cleartextTraffic | `AndroidManifest.xml` `<application>` | `android:usesCleartextTraffic="true"` | Android 9+ 默认禁 http；**prod 上线必须改 https 删此项** |
| compileSdk | `build.gradle.kts` | `36` | flutter_secure_storage 要求 |
| ndkVersion | `build.gradle.kts` | `"27.0.12077973"` | just_audio / audio_session 要求 |
| minSdk | `build.gradle.kts` | `23` | flutter_secure_storage 要求；覆盖 Android 6.0+ 99%+ 设备 |

**生活类比**：debug 模式 Flutter 像"妈妈帮你叠好衣服"，release 模式是"自己出门带行李"——必须自己显式声明所有要带的东西。

**在本项目中**：Plan 9-A Issue 1/2/3 + commit 5420b3f 一次性钉死全部 5 项。

**为什么需要**：debug 模式 Flutter tool 隐式注入很多东西（INTERNET 权限自动加、cleartext 自动开）；release 模式必须显式声明。开发者**第一次切 release 总踩这 5 坑**。Plan 10 部署前必须把 cleartextTraffic 删掉改 https，不然 App Store / 应用市场审核会拒。

**何时引入**：Plan 9-A Task 11 / Issue 1+2+3。

---

## 12.6 ColorOS / 国产 Android 的 SELinux + R8 联动 NPE

🎓 夹层 bug 经典——AOSP 没问题、PC curl 没问题、调试模式没问题，**只在国产 release 模式才有**。

**是什么**：
- **R8**：Android release 构建默认开启的代码 minification 工具（类似 ProGuard 升级版），把类名、方法名、线程名重命名成 `a / b / c / r1.a.g` 这种短名。
- **SELinux**：Linux 内核级的强制访问控制。Android 内置策略+ OEM 自定义策略。
- **联动**：国产 ROM（OPPO ColorOS / 华为 EMUI / 小米 MIUI）会基于线程名 / 类名做额外白名单控制。当 R8 重命名 ExoPlayer 的 "Loader" 线程后，ColorOS SELinux 不认识新名字 → DNS 系统调用经 `dnsproxyd` socket 被拒 → DNS 返回 null → ExoPlayer 内部 NPE。

**logcat 关键证据**：
```
ExoPlayer:Loade: avc: denied { connectto }
for path="/dev/socket/dnsproxyd"
```

**调试时绕过办法**：用 `flutter run`（debug 模式，不开 R8）。

**生产解决办法**：
- (a) Play Store / 应用市场签名 release apk 的 obfuscation seed 与本地不同，多数情况无问题
- (b) 极端情况在 `proguard-rules.pro` 显式 keep ExoPlayer 内部线程类不被改名：
  ```
  -keep class com.google.android.exoplayer2.** { *; }
  ```

**为什么需要这条**：这是夹层 bug——单看 AOSP / PC / debug 都不可复现，标准搜索引擎结果稀少。**只在国产 ROM + R8 + ExoPlayer 三件套**才出现，是 Flutter + 国产 Android 真机的标志性问题。Plan 10 部署前必须用 proguard rules 解决，不然真机用户播放故障。

**何时引入**：Plan 9-A Task 11 / Issue 6。

---

## 12.7 flutter run --release vs --debug 调试取舍

🎓 看似 release "更接近生产" 应该早跑，实际正好相反。

| 模式 | 特点 | 适合 |
|---|---|---|
| `--debug`（默认） | hot reload + 完整 stack trace + 不 minify + 隐式补全很多 manifest 项 | 日常 dev |
| `--release` | minify + AOT 编译 + 性能接近生产 + 严格按 manifest | 上线前 dry-run |

**真踩到的坑**（Plan 9-A）：直觉先跑 `--release`（"更真实"），结果连撞 Issue 3（INTERNET 权限）+ Issue 6（ExoPlayer NPE）两个 release-only 坑。改用 `--debug` 立刻通。

**结论**：
1. 日常业务 dev 用 debug
2. release 留到 **Plan 10 部署前**做一次彻底验证
3. release 跑通的标准：(a) 真机 + (b) 国产 ROM + (c) Play Store 签名 apk + (d) 端到端业务全部走通

**为什么需要**：Flutter 工程师圈共识——"先 debug 跑通业务，release 留到部署前再 dry-run"。直觉用 release "更接近生产" 反而坑多，因为你在调试业务的同时还要解 release-only 的平台坑，定位时间 ×3。

**何时引入**：Plan 9-A Task 11 / Issue 6 复盘。

---
