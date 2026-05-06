# Git 知识

Git 是这个项目的版本控制工具。下面所有词条按"先基础后进阶"排序。

---

## 1.1 Git（版本控制系统）

**一句话**：帮你记录代码每一次变化、能随时回到任何历史版本的工具。

**生活类比**：写小说时"另存为 v1.docx / v2.docx / v3-真的最终版.docx"——这就是手工版本控制，混乱又易丢。Git 把这件事做到极致：每次"存档"都自动有时间戳和说明，永远找得回。

**关键特性**：
- 只在你执行 `git commit` 时保存"快照"
- 每个快照有唯一 ID（哈希值，类似 `8a17a07`）
- 可以随时切到任何历史快照、对比差异、回滚
- 多人协作时能合并各自的修改

**何时引入**：执行 Task 0 之前。

---

## 1.2 `git init` 与 Git 仓库

**一句话**：把当前文件夹变成 Git 仓库，从此 Git 开始追踪这里的变化。

**它做什么**：在当前目录创建一个隐藏的 `.git/` 文件夹——里面是 Git 的"账本"。

- **`git init` 之前**：只是普通文件夹，Git 不知道
- **`git init` 之后**：变成 Git 仓库，可以 commit、可以查历史

只需要做一次。之后这个文件夹永远是 Git 仓库（除非手动删 `.git/`）。

**何时引入**：Task 0，本项目根目录。

---

## 1.3 Git 全局身份（user.name / user.email）

**一句话**：每次 commit 都要记下"是谁做的"，全局身份是"默认作者"。

**生活类比**：写信前在信纸右下角签名。Git 强制每次 commit 都签名。

```bash
git config --global user.name "lzy"
git config --global user.email "332803710@qq.com"
```

- **不必真实**——只是元数据。但若以后推 GitHub，邮箱要和 GitHub 注册的一致才能正确认领 commit。
- **每台电脑只配一次**

**何时引入**：Task 0。

---

## 1.4 分支（branch）

**一句话**：Git 里的"平行宇宙"，可以同时存在多条开发线互不干扰。

**生活类比**：写论文有"主稿"和"试验稿"。在试验稿里改完不满意可以丢掉，主稿不受影响；满意就把试验稿合并进主稿。

- 默认主分支以前叫 `master`，业界已普遍改名为 `main`
- MVP 阶段我们只用 `main` 一条分支

**何时引入**：Task 0（仓库初始化时设默认分支为 `main`）。

---

## 1.5 工作流：工作区 → 暂存区 → 仓库

**一句话**：改文件 → 选要打包的 → 拍快照。三个阶段，对应三个 Git 命令。

```
你修改文件
    ↓
git add <文件>      ← "我想把这些放进下一次快照"（叫"暂存区 staging"）
    ↓
git commit -m "..."  ← "拍个快照，附上说明"
    ↓
快照永久存进 .git/，可以随时回到这个点
```

**为什么要分两步（add 然后 commit）**：让你能精挑细选——比如同时改了 5 个文件，但本次只想 commit 其中 2 个相关的，剩下的留到下次。

**何时引入**：Task 0 后第一次 commit。

---

## 1.6 commit message（提交说明）

**一句话**：commit 时附上的一句话，说明这次改了什么、为什么改。

**好的 commit message**：
- 标题行 ≤ 72 字符，简明扼要
- 空一行后写正文，详细说"为什么"而不是"是什么"
- 写给 3 个月后的自己看（那时你已经忘了细节）

**业界惯例：Conventional Commits**

每个 commit message 第一行用前缀分类：

| 前缀 | 含义 | 例 |
|---|---|---|
| `feat` | 新功能 | `feat(config): load yaml with env override` |
| `fix` | 修 bug | `fix(auth): correct jwt expiration parsing` |
| `refactor` | 重构（不改外部行为） | `refactor(config): extract envPrefix const` |
| `docs` | 文档变更 | `docs: clarify Go version requirement` |
| `chore` | 杂事（配置、工具、不影响功能） | `chore: bootstrap project skeleton` |
| `test` | 加 / 改测试 | `test(traceid): add edge case for empty ctx` |
| `style` | 格式调整（不改逻辑） | `style: gofmt all files` |

**好处**：人和机器都能从前缀快速判断 commit 性质；自动生成 changelog 也方便。

**何时引入**：Task 0 baseline commit。

---

## 1.7 `.gitignore`

**一句话**：告诉 Git "这些文件你假装看不见"。

**生活类比**：搬家时往收纳箱里塞东西——但有些东西（比如垃圾、过期的食物）你会主动跳过，不打包。`.gitignore` 就是那张"不打包清单"。

**典型该忽略的东西**：
- 编译产物（`/bin/`、`*.exe`）
- 日志文件（`*.log`）
- IDE 个人配置（`.idea/`、`.vscode/`）
- 临时文件（`*.tmp`、`*.bak`）
- **敏感配置（密钥、生产配置）—— 最关键！**

**为什么"密钥绝不能进 git"**：
Git 历史是**永久**的。一旦你 commit 了密钥，即使下一个 commit 删掉，它依然永远存在 `.git/objects/` 里——任何能拿到这份仓库的人都能翻出来。GitHub 上每天都有人因为不小心 commit 密钥被黑客盗用云账号。

**多层 `.gitignore`**：
- 根目录的管整个仓库
- 子目录可以再加自己的，管局部规则（如 `server/.gitignore` 专管 Go 项目产物）

**何时引入**：Task 0 创建根目录 `.gitignore`；Task 1 创建 `server/.gitignore`。

---

## 1.8 `.gitkeep` 占位文件

**一句话**：让 Git 追踪一个"暂时还空着"的目录。

**为什么需要**：Git **只追踪文件，不追踪空目录**。如果你 `mkdir empty/` 然后 commit，Git 当作什么都没发生。

**解决方案**：在空目录里放一个名叫 `.gitkeep` 的空文件，让目录变得"有内容"，Git 就会记录这个目录的存在。

**注意**：`.gitkeep` **不是 Git 的特殊语法**——只是社区约定俗成的名字，你叫 `.placeholder` 也行。等真实文件落进这个目录，就该删掉它。

**何时引入**：Task 1 给所有规划好的空目录加 `.gitkeep`；Task 2、3 起逐个删除已有真实文件的目录里的 `.gitkeep`。

---

## 1.9 commit 哈希（SHA）

**一句话**：每个 commit 的"身份证号"。

形如 `8a17a07`（前 7 位）或 `8a17a07b3c5d2e1f9...`（完整 40 位）。

- 计算自 commit 内容的 SHA-1 哈希，**不可伪造**
- 改动任何字节都会变成完全不同的哈希
- 通常引用时用前 7 位即可（碰撞概率极低）

**用法举例**：
- `git show 8a17a07` 看那个 commit 的详情
- `git diff 8a17a07 HEAD` 对比那个 commit 和当前
- `git revert 8a17a07` 反向 commit（撤销那次改动）

**何时引入**：Task 0 第一个 commit 后。

---

## 1.10 LF vs CRLF（换行符）

**一句话**：Unix 用 `\n`（LF）做换行，Windows 用 `\r\n`（CRLF）。Git 在 Windows 上会自动转换。

**警告 `LF will be replaced by CRLF`**：意思是"接下来会发生这个转换"——Git 会让仓库里永远是 LF（统一），本机文件是 CRLF（Windows 友好）。**这只是提示，不是错误**。

**何时引入**：Task 0 第一次 commit 看到这条警告。
