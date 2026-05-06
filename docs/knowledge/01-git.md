# Git 知识

## 1.1 Git
**版本控制系统**——记录代码每次变化，能随时回到任何历史版本。  
**类比**：自动版"另存为 v1 / v2 / v3 final"，但永远找得回。

## 1.2 `git init` / Git 仓库
在当前文件夹创建 `.git/` 隐藏目录，让 Git 开始追踪这里的变化。每个项目只做一次。

## 1.3 全局身份（user.name / user.email）
每个 commit 都要"签名"。全局身份是默认作者，每台电脑配一次。  
不必真实——只是元数据；推 GitHub 时才需要邮箱匹配。

## 1.4 分支（branch）
Git 里的"平行宇宙"，多条开发线互不干扰。  
默认主分支以前叫 `master`，业界已改名 `main`。MVP 阶段我们只用 `main`。

## 1.5 工作流：工作区 → 暂存区 → 仓库
```
改文件 → git add <文件>  → git commit -m "..."
         （选中要打包的）   （拍快照）
```
分两步是为了**精挑细选**——同时改了 5 个文件可以只 commit 其中 2 个。

## 1.6 commit message + Conventional Commits
commit 时附的一句话说明。业界惯例用前缀分类：

| 前缀 | 含义 |
|---|---|
| `feat` | 新功能 |
| `fix` | 修 bug |
| `refactor` | 重构（不改外部行为） |
| `docs` | 文档 |
| `chore` | 杂事（配置、工具） |
| `test` | 加 / 改测试 |

好的 message 写给"3 个月后的自己"——讲清楚**为什么**改，不只是改了什么。

## 1.7 `.gitignore`
告诉 Git "这些文件假装看不见"——编译产物、日志、IDE 配置、**密钥**。  
**铁律**：密钥**绝不能 commit**。Git 历史是永久的，删了下个 commit 也救不回来。

## 1.8 `.gitkeep` 占位
Git 不追踪空目录。在空目录里放一个空文件 `.gitkeep`，Git 就会记录这目录。  
不是 Git 语法，只是社区惯例的名字。真实文件落进来后就该删掉它。

## 1.9 commit 哈希（SHA）
每个 commit 的"身份证号"，如 `8a17a07`。不可伪造——改任何字节哈希就变。  
通常用前 7 位即可：`git show 8a17a07`、`git diff 8a17a07 HEAD`。

## 1.10 LF vs CRLF
Unix 用 `\n`（LF），Windows 用 `\r\n`（CRLF）。Git 在 Windows 上自动转换。  
看到 `LF will be replaced by CRLF` **是提示不是错误**。
