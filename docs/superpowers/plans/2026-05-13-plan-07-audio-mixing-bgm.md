# Plan 7：音频混音 + BGM 素材库（10 首 mood-matched 首 BGM）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Plan 5 异步音频管线之上，让真实故事最终的 mp3 同时含有 **TTS 人声 + 背景音乐**——孩子听到的不再是干巴巴的朗读，而是有情绪渲染的"广播剧"。完成后端到端可演示：用户调 `POST /stories/generate` → Worker 通过新的 `audio.Orchestrator` 编排（cue 解析 → TTS 合成 → BGM 按情绪匹配 → ffmpeg 混音 → COS 上传）→ `stories.has_bgm=true` → 客户端通过既有 `GET /stories/:id/audio_url` 拿到的 mp3 一播即响起 BGM。**SFX 音效**为下一期（7b），本 plan 只搭好 cue 解析的 position-tracking 底座但不真混 SFX。

**Architecture:** 现有"tts_synthesis Worker 直调 TTS+Upload"的内联实现，被重构为"Worker 调 audio.Orchestrator"的编排实现。Orchestrator 内部按 6 步走：(1) `cue_parser` 从 LLM 文本中分离 clean text 和 cue 标记（`[音效:xxx]`/`[BGM情绪:xxx]`），并记录每个 cue 的字符偏移；(2) `tts.Client` 用 clean text 合成主轨 mp3；(3) `bgm_repo.PickByMood` 根据 cue 解析出的 mood（或 style fallback）从 `bgm_assets` 表里随机抽一条；(4) `bgm_cache.GetLocalPath` 用 per-filename 互斥锁懒下载 BGM 文件到 `./cache/bgm/`；(5) `mixer.MixWithBGM` 用 `exec.Command` 调系统 ffmpeg，TTS 0 dB + BGM 循环并衰减 -18 dB，输出 128kbps/32kHz/2-channel mp3；(6) 上传到 COS，`MarkAudioReady(..., hasBGM=true)`。任一外部依赖失败（BGM 缺失 / ffmpeg 未装 / mix 超时）→ 优雅降级到纯 TTS，故事照常 ship、`has_bgm=false`、warning 日志 + metrics。

**Tech Stack:**
- Go 1.24+ + Gin + GORM + PostgreSQL（已有）
- **新增 OS 依赖：ffmpeg**——开发机（Windows）走 `winget install Gyan.FFmpeg`；生产镜像走 `apk add --no-cache ffmpeg`（alpine 基础镜像）。Go 通过 `os/exec.Command` 调外部二进制，**不绑 cgo、不依赖任何 Go 音频处理库**
- 复用 Plan 1-6：safety / prompt / repository / userctx / gateway/tts / gateway/storage / api / metrics / pkg/config
- 配置项扩 `cfg.Audio { FFmpegPath string; BGMCacheDir string; BGMVolumeDB float64; MixTimeoutSec int }`

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)
  - 第 4 章 MVP 范围（**核心**——确认 BGM 在 MVP 内，SFX 推到 7b）
  - 第 5.1 完整生成流程（朗读阶段的"BGM 渲染"步骤就是本 plan 实现的）
  - 第 5.2-5.4 故事参数 / 风格映射 / 串联机制
  - 第 7 章红线（音频内容不直接受红线管控，但 BGM 选型要避免恐怖、紧张元素）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - **第 8 章 Audio Orchestration**（**核心蓝图**——本 plan 是它的代码落地）
  - 第 6 章 Gateway 抽象（TTS / Storage 不变，只在新 service 层加 Orchestrator）
  - 第 9 章 Outbox（本 plan 不动 Worker 主循环，只重构 handler）
  - 第 14 章成本（BGM 是一次性素材成本，混音是 CPU 成本，无外部 API 费用）
- Plan 4：[2026-05-08-plan-04-story-generation.md](2026-05-08-plan-04-story-generation.md)（**必读**——`stories.has_bgm` 字段就是 Plan 4 留下的占位）
- Plan 5：[2026-05-09-plan-05-audio-tts-storage.md](2026-05-09-plan-05-audio-tts-storage.md)（**必读**——本 plan 直接重构其 `tts_synthesis` handler）
- Plan 6 / 6b：[2026-05-11-plan-06-bootstrap-and-memory.md](2026-05-11-plan-06-bootstrap-and-memory.md) / [2026-05-12-plan-06b-known-issue-fixes.md](2026-05-12-plan-06b-known-issue-fixes.md)（参考 voice/style 决策的口径一致性）
- `server/safety/system_prompt.tmpl`——确认 LLM 已被指示输出 `[音效:xxx]` 和 `[BGM情绪:xxx]` 标记
- `server/internal/worker/handlers/tts_synthesis.go`——本 plan 重构对象
- CLAUDE.md（4.2 内容安全；4.4 不写套话注释；第 7 章必须解释知识点 + 同步落 `docs/knowledge/`）

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；新增 service/audio 包覆盖率 ≥ 70%；mixer 真实 ffmpeg 测试在无 ffmpeg 环境自动 `t.Skip`
2. 本机 `ffmpeg -version` 可执行；生产 Dockerfile 已追加 `apk add ffmpeg`（Plan 7 仅在文档中标注，实际 Dockerfile 改由 Plan 8 部署期落地）
3. `make migrate-up` 应用 `000006_bgm_assets` 后，`\d bgm_assets` 显示 8 列 + 2 索引
4. `make seed-bgm` 跑完后，`SELECT mood, count(*) FROM bgm_assets WHERE active GROUP BY mood;` 返回 5 mood × 2 = 10 行
5. 10 个 BGM mp3 文件已上传到 COS `bgm/<mood>/<filename>.mp3`（**用户手工任务**，本 plan 提供 manifest 占位）
6. `make run-dev` 启动后跑完整故事生成：
   - `POST /stories/generate` → 200，返回 storyId
   - 5-15 秒后 Worker 完成混音：`SELECT audio_status, has_bgm, audio_duration_seconds FROM stories WHERE id=?` → `ready, true, >0`
   - `GET /stories/:id/audio_url` 拿到签名 URL；下载 mp3，本地用 `ffprobe -show_streams` 或耳朵确认有两条音轨/BGM 听得到
7. 降级链路：手动将 cache/bgm 目录清空 + 切断 COS 访问 → Worker 仍完成 `audio_status=ready`，但 `has_bgm=false`，日志含 `audio.mix.degraded.bgm_unavailable` warning
8. ffmpeg 未安装环境：`cfg.Audio.FFmpegPath="/nonexistent/ffmpeg"` 启动后跑一次故事 → has_bgm=false，warning `audio.mix.degraded.ffmpeg_missing`，metrics `audio_mix_total{status="degraded"}` +1
9. 业务 metrics 在 `/metrics` 可见：`audio_mix_duration_seconds`、`audio_mix_total{provider,status}`、`bgm_not_found_total{mood}`
10. `golangci-lint run ./...` 0 issues
11. 知识库新增 3 条词条：(03.X) 外部进程调用 vs 库绑定；(09.X) ffmpeg filter_complex 基础；(05.X) 懒下载缓存的 per-key 互斥锁模式

---

## 范围决策记录（与用户对齐）

| 维度 | 决策 |
|---|---|
| BGM 来源 | Plan 7 内置 **10 首 first-party BGM**（5 mood × 2 variants），用户手工提供 / 采购素材库。SFX 音效**推迟到 Plan 7b** |
| ffmpeg 形式 | OS 安装的二进制（Windows: `winget install Gyan.FFmpeg`；Linux/prod: `apk add ffmpeg`）。Go 走 `os/exec.Command`。**不**用 cgo 绑定的 Go 库（`goav` / `go-libav` 编译复杂、跨平台坑多）|
| ffmpeg 路径配置 | `cfg.Audio.FFmpegPath` 可配置，默认 `"ffmpeg"`（让 OS 在 PATH 中查找）|
| cue 时间对齐 | cue parser 提取 cue 标记 + 字符偏移；混音时 `position / total_chars * audio_duration_sec` 估算 SFX 时间。Plan 7 **只搭好这个基础设施**，不真混 SFX（推 7b）|
| Mixer 策略 | TTS 主轨 0 dB；BGM 循环到 TTS 长度，衰减到 -18 dB（约人声音量的 1/8）；输出 2-channel 32kHz 128kbps mp3。单次 ffmpeg 调用，`filter_complex` 一句话 |
| BGM 存储 | BGM 文件存 COS 私有 bucket，路径 `bgm/<mood>/<filename>.mp3`。Worker 启动**不**预下载；首次用到时**懒下载**到本地 `./cache/bgm/`（gitignored），用 `sync.Map[filename]*sync.Once` 保证并发只下一次 |
| BGM repo | 新表 `bgm_assets`（id / mood / filename / object_key / duration_sec / license / active / created_at）。选取语义：`WHERE mood=$1 AND active ORDER BY RANDOM() LIMIT 1`——同 mood 多个 variant 自然多样化 |
| 5 个 mood ↔ 目录约定 | 温馨治愈 → warm；冒险探索 → adventure；搞笑欢乐 → funny；神奇魔法 → magic；科普认知 → curious。**LLM 输出的 cue 用中文**（如 `[BGM情绪:温馨]`），parser 内部维护中→英 map |
| Mood 兜底链 | (1) 故事文本中第一个 `[BGM情绪:xxx]` cue → (2) `story.Style` 风格名直接映射 → (3) 退到 `warm` 默认 |
| 降级语义 | 三个降级点：BGM 选取失败 / BGM 文件下载失败 / ffmpeg 混音失败——**统一返回纯 TTS 字节流，has_bgm=false，warning 日志，metrics 计 `status="degraded"`**。故事一定 ship |
| Mixer 超时 | 30 秒（cfg.Audio.MixTimeoutSec）。`exec.CommandContext` + `cancel()` 强杀子进程 |
| 子进程 stderr | 完整捕获到 `bytes.Buffer`，失败时整段写入日志（ffmpeg 错误堆栈对排查至关重要），**绝不**只看 exit code |
| 临时文件 | `os.CreateTemp("", "aibao-mix-*.mp3")`；`defer os.Remove`；不允许累积在 `/tmp` 或 Windows `%TEMP%` |
| 接口契约 | **不增加任何新 HTTP 接口**。`GET /stories/:id` 响应里的 `has_bgm` 字段现在会真为 `true` |
| 测试策略 | mixer 真实 ffmpeg 测试用 fixture（小 TTS mp3 + 小 BGM mp3）；CI 无 ffmpeg → `t.Skip`；orchestrator 测试 mock 掉所有外部依赖（包括 mixer 抽 interface）|

---

## File Structure

### 数据迁移

| 文件 | 职责 |
|---|---|
| `server/migrations/000006_bgm_assets.up.sql` | `bgm_assets` 表 + 索引 |
| `server/migrations/000006_bgm_assets.down.sql` | 反向 |

### 配置扩展

| 文件 | 修改 |
|---|---|
| `server/internal/pkg/config/config.go` | 新增 `AudioConfig` 块 + 默认值 |
| `server/internal/pkg/config/config_test.go` | 补 `TestLoad_AudioDefaults` |
| `server/config/config.dev.yaml` + `config.yaml.example` | 追加 `audio:` 段 |

### Data model + Repo

| 文件 | 修改/新增 |
|---|---|
| `server/internal/model/story.go` | 新增 `BGMAsset` 结构体 + `MoodWarm/MoodAdventure/MoodFunny/MoodMagic/MoodCurious` 常量 + `MoodFromCueZh(string) string` helper |
| `server/internal/repository/bgm_repo.go` | `BGMRepo` 接口（`PickByMood`/`List`/`Insert`）+ GORM 实现 |
| `server/internal/repository/bgm_repo_test.go` | 集成测试（testcontainers）|
| `server/internal/repository/story_repo.go` | `MarkAudioReady` 签名加 `hasBGM bool` 参数（或改用 struct 入参以避免 API 增长）|
| `server/internal/repository/story_repo_test.go` | 既有测试同步改 |

### Audio service（新包）

| 文件 | 职责 |
|---|---|
| `server/internal/service/audio/cue_parser.go` | cue 正则提取、字符偏移记录、style→mood 兜底 |
| `server/internal/service/audio/cue_parser_test.go` | 单测，纯字符串处理无外部依赖 |
| `server/internal/service/audio/bgm_cache.go` | 本地 BGM 缓存，per-filename `sync.Once` 互斥 |
| `server/internal/service/audio/bgm_cache_test.go` | 用 mock storage.Client 测懒下载 + 并发去重 |
| `server/internal/service/audio/mixer.go` | `Mixer` 接口 + `FFmpegMixer` 实现（`exec.CommandContext`）|
| `server/internal/service/audio/mixer_test.go` | 真实 ffmpeg 测试（fixture mp3），ffmpeg 不可用时 t.Skip |
| `server/internal/service/audio/orchestrator.go` | `Orchestrator.Compose(ctx, req)`——6 步编排 + 降级 |
| `server/internal/service/audio/orchestrator_test.go` | 全 mock，覆盖正常路径 + 3 个降级路径 |
| `server/internal/service/audio/testdata/tts_sample.mp3` | 2 秒人声 fixture（任何 TTS 输出截一段）|
| `server/internal/service/audio/testdata/bgm_sample.mp3` | 2 秒 BGM fixture |

### Worker 重构

| 文件 | 修改 |
|---|---|
| `server/internal/worker/handlers/tts_synthesis.go` | 内联 TTS+Upload → 改为调 `audio.Orchestrator.Compose` |
| `server/internal/worker/handlers/tts_synthesis_test.go` | 用 mock Orchestrator 替换 mock tts+storage |

### Metrics

| 文件 | 修改 |
|---|---|
| `server/internal/metrics/business.go` | 新增 `AudioMixDuration` / `AudioMixTotal` / `BGMNotFoundTotal` |
| `server/internal/metrics/business_test.go` | 同步补 |

### Seed CLI

| 文件 | 职责 |
|---|---|
| `server/cmd/seed-bgm/main.go` | 读 YAML manifest → upsert 进 `bgm_assets` 表 |
| `server/safety/bgm_manifest.yaml` | 10 条占位记录（mood / filename / object_key / duration_sec / license）|

### main.go 装配

| 文件 | 修改 |
|---|---|
| `server/cmd/server/main.go` | 构造 BGMRepo / BGMCache / FFmpegMixer / audio.Orchestrator；注入到 tts_synthesis handler |

### Makefile + .gitignore

| 文件 | 修改 |
|---|---|
| `Makefile` | 新增 `seed-bgm` target、`audio-mix-test` 手工冒烟 target |
| `.gitignore` | 追加 `server/cache/` |

### 文档

| 文件 | 修改 |
|---|---|
| `docs/devlog/2026-05-13.md` | 实施日志（新增）|
| `CLAUDE.md` | "已落地的能力"段追加 Plan 7 项 |
| `MEMORY.md` | 同步当前阶段 |
| `docs/knowledge/03-go-engineering.md` | 词条：外部进程调用 vs 库绑定 |
| `docs/knowledge/09-observability.md` | 词条：ffmpeg filter_complex 基础（也可放新 11-audio-and-media.md）|
| `docs/knowledge/05-software-design.md` | 词条：懒下载缓存的 per-key 互斥锁模式 |

---

## API 形态

**本 plan 不新增任何 HTTP 接口。** 既有接口语义微变：

### `GET /api/v1/stories/:id`
响应 JSON 中 `has_bgm` 字段，在 Plan 7 之后会真的为 `true`（成功混音）或 `false`（降级）。客户端无感。

### `GET /api/v1/stories/:id/audio_url`
返回的 15 分钟签名 URL 指向的 mp3，现在是**TTS+BGM 混音后的成品**，不再是纯 TTS。

---

## 数据模型字段约定

### bgm_assets 表

| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigserial PK | |
| mood | varchar(20) NOT NULL | warm/adventure/funny/magic/curious |
| filename | varchar(100) NOT NULL UNIQUE | 如 `warm_01.mp3` |
| object_key | varchar(500) NOT NULL | COS 路径 `bgm/warm/warm_01.mp3` |
| duration_sec | int NOT NULL | 原始 BGM 时长（仅展示用，混音以 TTS 时长为准）|
| license | varchar(100) NOT NULL DEFAULT '' | 版权来源（"CC0" / "Pixabay-自有" / "采购-XX素材库" 等）|
| active | bool NOT NULL DEFAULT TRUE | 软下架开关 |
| created_at | timestamptz NOT NULL DEFAULT NOW() | |

索引：`(mood, active)`、`(filename)`（UNIQUE）

### cfg.Audio 块

```yaml
audio:
  ffmpeg_path: "ffmpeg"          # 默认从 PATH 查找；prod 可改为绝对路径
  bgm_cache_dir: "./cache/bgm"   # 工作目录相对路径
  bgm_volume_db: -18             # BGM 相对 TTS 的衰减
  mix_timeout_sec: 30
  mix_sample_rate: 32000
  mix_bitrate_kbps: 128
```

对应 `AudioConfig` Go 结构体：

```go
type AudioConfig struct {
    FFmpegPath     string  `mapstructure:"ffmpeg_path"`
    BGMCacheDir    string  `mapstructure:"bgm_cache_dir"`
    BGMVolumeDB    float64 `mapstructure:"bgm_volume_db"`
    MixTimeoutSec  int     `mapstructure:"mix_timeout_sec"`
    MixSampleRate  int     `mapstructure:"mix_sample_rate"`
    MixBitrateKbps int     `mapstructure:"mix_bitrate_kbps"`
}
```

---

# Tasks

## Task 0：迁移 `000006_bgm_assets`

**Files:**
- Create: `server/migrations/000006_bgm_assets.up.sql`
- Create: `server/migrations/000006_bgm_assets.down.sql`

- [ ] **Step 0.1：up SQL**

```sql
CREATE TABLE IF NOT EXISTS bgm_assets (
    id           BIGSERIAL PRIMARY KEY,
    mood         VARCHAR(20)  NOT NULL,
    filename     VARCHAR(100) NOT NULL UNIQUE,
    object_key   VARCHAR(500) NOT NULL,
    duration_sec INT          NOT NULL DEFAULT 0,
    license      VARCHAR(100) NOT NULL DEFAULT '',
    active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS bgm_assets_mood_active_idx ON bgm_assets(mood, active);
```

- [ ] **Step 0.2：down SQL**

```sql
DROP TABLE IF EXISTS bgm_assets;
```

- [ ] **Step 0.3：跑迁移**

```bash
cd server && make migrate-up
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\d bgm_assets"
```
Expected: 表存在，含 `bgm_assets_mood_active_idx` 与 `bgm_assets_filename_key`（UNIQUE 自带索引）。

- [ ] **Step 0.4：commit**

```bash
git add server/migrations/000006_bgm_assets.up.sql server/migrations/000006_bgm_assets.down.sql
git commit -m "feat(db): bgm_assets table for mood-matched background music"
```

---

## Task 1：模型 + 常量 + Mood 映射

**Files:**
- Modify: `server/internal/model/story.go`
- Create: `server/internal/model/story_mood_test.go`

- [ ] **Step 1.1：追加常量与结构体**

在 `story.go` 末尾追加：

```go
// Mood constants drive BGM selection and ffmpeg input lookup.
// 对应 LLM 输出 cue 的中文 → 英文目录名。
const (
    MoodWarm      = "warm"      // 温馨治愈
    MoodAdventure = "adventure" // 冒险探索
    MoodFunny     = "funny"     // 搞笑欢乐
    MoodMagic     = "magic"     // 神奇魔法
    MoodCurious   = "curious"   // 科普认知
)

// BGMAsset is one row in bgm_assets.
type BGMAsset struct {
    ID          int64     `gorm:"column:id;primaryKey" json:"id"`
    Mood        string    `gorm:"column:mood" json:"mood"`
    Filename    string    `gorm:"column:filename" json:"filename"`
    ObjectKey   string    `gorm:"column:object_key" json:"object_key"`
    DurationSec int       `gorm:"column:duration_sec" json:"duration_sec"`
    License     string    `gorm:"column:license" json:"license"`
    Active      bool      `gorm:"column:active" json:"active"`
    CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (BGMAsset) TableName() string { return "bgm_assets" }

// MoodFromCueZh maps the Chinese mood label from LLM (e.g. "温馨") to the
// internal English mood key. Returns "" if no match (caller should fallback).
func MoodFromCueZh(zh string) string {
    switch strings.TrimSpace(zh) {
    case "温馨", "温馨治愈", "治愈":
        return MoodWarm
    case "冒险", "冒险探索", "探险":
        return MoodAdventure
    case "搞笑", "搞笑欢乐", "欢乐":
        return MoodFunny
    case "魔法", "神奇魔法", "神奇":
        return MoodMagic
    case "科普", "科普认知", "认知", "好奇":
        return MoodCurious
    default:
        return ""
    }
}

// MoodFromStyle maps the story.Style (Chinese) to mood key.
// 作为 cue 缺失时的兜底。
func MoodFromStyle(style string) string {
    if m := MoodFromCueZh(style); m != "" {
        return m
    }
    return MoodWarm // 最终兜底
}
```

如缺 `strings` import，记得追加。

- [ ] **Step 1.2：写最小单测**

`story_mood_test.go`：

```go
package model

import "testing"

func TestMoodFromCueZh(t *testing.T) {
    cases := map[string]string{
        "温馨":     MoodWarm,
        "温馨治愈":   MoodWarm,
        "冒险":     MoodAdventure,
        "搞笑欢乐":   MoodFunny,
        "神奇":     MoodMagic,
        "科普":     MoodCurious,
        " 治愈 ":   MoodWarm,
        "悬疑":     "",
    }
    for in, want := range cases {
        if got := MoodFromCueZh(in); got != want {
            t.Errorf("MoodFromCueZh(%q) = %q, want %q", in, got, want)
        }
    }
}

func TestMoodFromStyle_Fallback(t *testing.T) {
    if MoodFromStyle("奇怪的风格") != MoodWarm {
        t.Fatal("expected warm fallback")
    }
}
```

- [ ] **Step 1.3：跑测试**

```bash
cd /f/claud/aibao_app/server && go test -count=1 ./internal/model/... -v
```

- [ ] **Step 1.4：commit**

```bash
git add server/internal/model/
git commit -m "feat(model): BGMAsset + mood constants + cue/style→mood maps"
```

🎓 **教学点：为什么把映射放在 model 包**——`model` 是项目里"基础数据型"的归属地。Mood 既是数据库列的取值集，又是业务逻辑路径选择的 key，让它跟 `Story`/`BGMAsset` 住一起，service 层和 worker 层都能用，避免循环依赖。把它放 service 包 = "我有一个 service 函数返回常量给别的 service 用"，气味不对。

---

## Task 2：BGMRepo

**Files:**
- Create: `server/internal/repository/bgm_repo.go`
- Create: `server/internal/repository/bgm_repo_test.go`

- [ ] **Step 2.1：接口与实现**

```go
package repository

import (
    "context"
    "errors"

    "github.com/aibao/server/internal/model"
    "gorm.io/gorm"
)

// BGMRepo is the read/write surface over bgm_assets.
type BGMRepo interface {
    // PickByMood randomly returns one active asset of the given mood.
    // Returns (nil, nil) when no row matches—the caller is expected to degrade.
    PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error)
    // List returns all active assets (ordered by mood, filename) for admin/seed.
    List(ctx context.Context) ([]*model.BGMAsset, error)
    // Upsert inserts or updates by filename (used by seed CLI).
    Upsert(ctx context.Context, a *model.BGMAsset) error
}

type bgmRepo struct{ db *gorm.DB }

func NewBGMRepo(db *gorm.DB) BGMRepo { return &bgmRepo{db: db} }

func (r *bgmRepo) PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error) {
    var a model.BGMAsset
    err := r.db.WithContext(ctx).
        Where("mood = ? AND active = TRUE", mood).
        Order("RANDOM()").
        Limit(1).
        Take(&a).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &a, nil
}

func (r *bgmRepo) List(ctx context.Context) ([]*model.BGMAsset, error) {
    var out []*model.BGMAsset
    err := r.db.WithContext(ctx).
        Where("active = TRUE").
        Order("mood, filename").
        Find(&out).Error
    return out, err
}

func (r *bgmRepo) Upsert(ctx context.Context, a *model.BGMAsset) error {
    // ON CONFLICT (filename) DO UPDATE
    return r.db.WithContext(ctx).
        Exec(`INSERT INTO bgm_assets(mood, filename, object_key, duration_sec, license, active)
              VALUES (?, ?, ?, ?, ?, ?)
              ON CONFLICT (filename) DO UPDATE
              SET mood=EXCLUDED.mood, object_key=EXCLUDED.object_key,
                  duration_sec=EXCLUDED.duration_sec, license=EXCLUDED.license,
                  active=EXCLUDED.active`,
            a.Mood, a.Filename, a.ObjectKey, a.DurationSec, a.License, a.Active,
        ).Error
}
```

- [ ] **Step 2.2：集成测试**

`bgm_repo_test.go` 用 testcontainers PostgreSQL（项目内已有 helper `testdb.New(t)`）：

```go
//go:build integration

package repository_test

import (
    "context"
    "testing"

    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/repository"
    "github.com/aibao/server/internal/testdb"
    "github.com/stretchr/testify/require"
)

func TestBGMRepo_Lifecycle(t *testing.T) {
    db := testdb.New(t)
    repo := repository.NewBGMRepo(db)
    ctx := context.Background()

    // Upsert 10
    moods := []string{model.MoodWarm, model.MoodAdventure, model.MoodFunny, model.MoodMagic, model.MoodCurious}
    for _, m := range moods {
        for i := 1; i <= 2; i++ {
            require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
                Mood: m, Filename: m + "_0" + string(rune('0'+i)) + ".mp3",
                ObjectKey: "bgm/" + m + "/" + m + "_0" + string(rune('0'+i)) + ".mp3",
                DurationSec: 60, License: "CC0", Active: true,
            }))
        }
    }

    // List = 10
    all, err := repo.List(ctx)
    require.NoError(t, err)
    require.Len(t, all, 10)

    // PickByMood returns one of two warm rows
    pick, err := repo.PickByMood(ctx, model.MoodWarm)
    require.NoError(t, err)
    require.NotNil(t, pick)
    require.Equal(t, model.MoodWarm, pick.Mood)

    // Unknown mood → (nil, nil)
    nope, err := repo.PickByMood(ctx, "unknown")
    require.NoError(t, err)
    require.Nil(t, nope)

    // Upsert idempotent (re-run same filename, change license)
    require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
        Mood: model.MoodWarm, Filename: "warm_01.mp3",
        ObjectKey: "bgm/warm/warm_01.mp3", DurationSec: 60,
        License: "UPDATED", Active: true,
    }))
    all2, _ := repo.List(ctx)
    require.Len(t, all2, 10) // 仍然 10 行
}
```

- [ ] **Step 2.3：跑集成测试**

```bash
cd /f/claud/aibao_app/server && go test -tags=integration -count=1 ./internal/repository/ -run BGMRepo -v
```

- [ ] **Step 2.4：commit**

```bash
git add server/internal/repository/bgm_repo*.go
git commit -m "feat(repo): BGMRepo with PickByMood/List/Upsert"
```

---

## Task 3：扩展配置（AudioConfig）

**Files:**
- Modify: `server/internal/pkg/config/config.go`
- Modify: `server/internal/pkg/config/config_test.go`
- Modify: `server/config/config.dev.yaml` + `config.yaml.example`

- [ ] **Step 3.1：Config 结构追加**

```go
type Config struct {
    // ... 既有字段 ...
    Audio AudioConfig `mapstructure:"audio"`
}

type AudioConfig struct {
    FFmpegPath     string  `mapstructure:"ffmpeg_path"`
    BGMCacheDir    string  `mapstructure:"bgm_cache_dir"`
    BGMVolumeDB    float64 `mapstructure:"bgm_volume_db"`
    MixTimeoutSec  int     `mapstructure:"mix_timeout_sec"`
    MixSampleRate  int     `mapstructure:"mix_sample_rate"`
    MixBitrateKbps int     `mapstructure:"mix_bitrate_kbps"`
}
```

`applyDefaultsAndValidate` 追加：

```go
    if c.Audio.FFmpegPath == "" {
        c.Audio.FFmpegPath = "ffmpeg"
    }
    if c.Audio.BGMCacheDir == "" {
        c.Audio.BGMCacheDir = "./cache/bgm"
    }
    if c.Audio.BGMVolumeDB == 0 {
        c.Audio.BGMVolumeDB = -18
    }
    if c.Audio.MixTimeoutSec == 0 {
        c.Audio.MixTimeoutSec = 30
    }
    if c.Audio.MixSampleRate == 0 {
        c.Audio.MixSampleRate = 32000
    }
    if c.Audio.MixBitrateKbps == 0 {
        c.Audio.MixBitrateKbps = 128
    }
```

`Load(path)` 的 `binds` 列表追加：

```go
    "audio.ffmpeg_path", "audio.bgm_cache_dir", "audio.bgm_volume_db",
    "audio.mix_timeout_sec", "audio.mix_sample_rate", "audio.mix_bitrate_kbps",
```

- [ ] **Step 3.2：测试**

`config_test.go` 追加：

```go
func TestLoad_AudioDefaults(t *testing.T) {
    cfg := loadMinimalConfig(t) // 既有 helper
    assert.Equal(t, "ffmpeg", cfg.Audio.FFmpegPath)
    assert.Equal(t, "./cache/bgm", cfg.Audio.BGMCacheDir)
    assert.InDelta(t, -18.0, cfg.Audio.BGMVolumeDB, 0.001)
    assert.Equal(t, 30, cfg.Audio.MixTimeoutSec)
    assert.Equal(t, 32000, cfg.Audio.MixSampleRate)
    assert.Equal(t, 128, cfg.Audio.MixBitrateKbps)
}
```

- [ ] **Step 3.3：dev yaml**

`server/config/config.dev.yaml` 追加：

```yaml

audio:
  ffmpeg_path: "ffmpeg"
  bgm_cache_dir: "./cache/bgm"
  bgm_volume_db: -18
  mix_timeout_sec: 30
  mix_sample_rate: 32000
  mix_bitrate_kbps: 128
```

`server/config/config.yaml.example` 同步追加（含注释说明 prod 可改绝对路径）。

- [ ] **Step 3.4：跑测试 + commit**

```bash
go test -count=1 ./internal/pkg/config/...
git add server/internal/pkg/config server/config
git commit -m "feat(config): audio block (ffmpeg path / bgm cache / mix params)"
```

---

## Task 4：业务 Metrics

**Files:**
- Modify: `server/internal/metrics/business.go`
- Modify: `server/internal/metrics/business_test.go`

- [ ] **Step 4.1：追加 3 个指标**

`business.go` 中 `Business` struct 追加字段：

```go
AudioMixDuration  *prometheus.HistogramVec // labels: provider
AudioMixTotal     *prometheus.CounterVec   // labels: provider, status (ok/fail/degraded)
BGMNotFoundTotal  *prometheus.CounterVec   // labels: mood
```

`NewBusiness(reg)` 注册：

```go
b.AudioMixDuration = promauto.With(reg).NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "audio_mix_duration_seconds",
        Help:    "End-to-end audio mixing duration (TTS+BGM via ffmpeg).",
        Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s..~51s
    },
    []string{"provider"},
)
b.AudioMixTotal = promauto.With(reg).NewCounterVec(
    prometheus.CounterOpts{
        Name: "audio_mix_total",
        Help: "Count of audio mix attempts by provider and status.",
    },
    []string{"provider", "status"},
)
b.BGMNotFoundTotal = promauto.With(reg).NewCounterVec(
    prometheus.CounterOpts{
        Name: "bgm_not_found_total",
        Help: "Count of BGM lookups that returned no row for a mood.",
    },
    []string{"mood"},
)
```

- [ ] **Step 4.2：测试**

`business_test.go` 现有 `TestBusinessMetricsExposed` 测试追加期望名：

```go
expected := []string{
    // ... 既有 ...
    "audio_mix_duration_seconds",
    "audio_mix_total",
    "bgm_not_found_total",
}
```

- [ ] **Step 4.3：跑测试 + commit**

```bash
go test -count=1 ./internal/metrics/...
git add server/internal/metrics
git commit -m "feat(metrics): audio_mix_* and bgm_not_found_total"
```

---

## Task 5：cue_parser

**Files:**
- Create: `server/internal/service/audio/cue_parser.go`
- Create: `server/internal/service/audio/cue_parser_test.go`

- [ ] **Step 5.1：包注释与类型**

```go
// Package audio implements the post-LLM audio orchestration:
//   parse cue markers → call TTS on clean text → pick BGM by mood →
//   ffmpeg-mix TTS+BGM → return final mp3 bytes.
//
// cue_parser.go is pure string processing—no I/O, no external deps.
package audio

import (
    "regexp"
    "strings"

    "github.com/aibao/server/internal/model"
)

// CueType is "sfx" (sound effect) or "bgm" (background music mood).
type CueType string

const (
    CueTypeSFX CueType = "sfx"
    CueTypeBGM CueType = "bgm"
)

// Cue records one marker extracted from the story text.
type Cue struct {
    Type       CueType
    Label      string // e.g. "门铃" or "温馨"
    CharOffset int    // byte offset INTO CleanText where this cue WOULD have been
}

// ParseResult is the full output of Parse.
type ParseResult struct {
    CleanText string // text with all [音效:...] and [BGM情绪:...] markers stripped
    Cues      []Cue
    BGMMood   string // resolved mood key (e.g. "warm"); always non-empty after Parse
}
```

- [ ] **Step 5.2：正则与解析**

```go
// Order matters: BGM cue may also match the broader bracket pattern; we extract
// both kinds in one pass to preserve textual order and offsets.
var cueRe = regexp.MustCompile(`\[(音效|BGM情绪):([^\]]+)\]`)

// Parse extracts cues from text and returns clean text plus offsets relative
// to the clean text. If no [BGM情绪:...] cue is present, BGMMood falls back
// to MoodFromStyle(fallbackStyle); if that also yields "", it returns MoodWarm.
func Parse(text, fallbackStyle string) ParseResult {
    var (
        clean strings.Builder
        cues  []Cue
        last  int
    )
    bgmMood := ""

    idxs := cueRe.FindAllStringSubmatchIndex(text, -1)
    for _, m := range idxs {
        start, end := m[0], m[1]
        kindStart, kindEnd := m[2], m[3]
        labelStart, labelEnd := m[4], m[5]

        // append text before this cue
        clean.WriteString(text[last:start])
        offset := clean.Len()

        kind := text[kindStart:kindEnd]
        label := strings.TrimSpace(text[labelStart:labelEnd])

        var ct CueType
        if kind == "音效" {
            ct = CueTypeSFX
        } else {
            ct = CueTypeBGM
            if bgmMood == "" {
                if m := model.MoodFromCueZh(label); m != "" {
                    bgmMood = m
                }
            }
        }
        cues = append(cues, Cue{Type: ct, Label: label, CharOffset: offset})
        last = end
    }
    clean.WriteString(text[last:])

    if bgmMood == "" {
        bgmMood = model.MoodFromStyle(fallbackStyle)
    }
    return ParseResult{
        CleanText: clean.String(),
        Cues:      cues,
        BGMMood:   bgmMood,
    }
}
```

🎓 **教学点：byte offset vs rune offset**——Go 字符串切片是字节级，中文一个字 3 字节。本 plan 用 byte offset 是因为后续 SFX 时间估算 `offset / total_bytes * duration_sec` 对中文均匀分布的故事而言误差可控；如果未来要做更精确的"按字符计时"，应改用 `utf8.RuneCountInString` 重算。先简单后复杂。

- [ ] **Step 5.3：单测**

```go
package audio

import (
    "testing"

    "github.com/aibao/server/internal/model"
    "github.com/stretchr/testify/assert"
)

func TestParse_StripsMarkers(t *testing.T) {
    in := "从前有一只小熊[音效:风声]他走在森林里。[BGM情绪:温馨]终于回家了。"
    r := Parse(in, "温馨治愈")
    assert.Equal(t, "从前有一只小熊他走在森林里。终于回家了。", r.CleanText)
    assert.Len(t, r.Cues, 2)
    assert.Equal(t, CueTypeSFX, r.Cues[0].Type)
    assert.Equal(t, "风声", r.Cues[0].Label)
    assert.Equal(t, CueTypeBGM, r.Cues[1].Type)
    assert.Equal(t, model.MoodWarm, r.BGMMood)
}

func TestParse_NoBGMCue_FallbackStyle(t *testing.T) {
    r := Parse("纯文本无 cue", "冒险探索")
    assert.Equal(t, "纯文本无 cue", r.CleanText)
    assert.Empty(t, r.Cues)
    assert.Equal(t, model.MoodAdventure, r.BGMMood)
}

func TestParse_NoBGMCueNoStyle_FallbackWarm(t *testing.T) {
    r := Parse("光秃秃", "")
    assert.Equal(t, model.MoodWarm, r.BGMMood)
}

func TestParse_FirstBGMCueWins(t *testing.T) {
    in := "[BGM情绪:温馨]开头[BGM情绪:冒险]中段"
    r := Parse(in, "")
    assert.Equal(t, model.MoodWarm, r.BGMMood)
    assert.Len(t, r.Cues, 2)
}

func TestParse_OffsetsAreIntoCleanText(t *testing.T) {
    in := "ABC[音效:bell]DEF"
    r := Parse(in, "")
    // CleanText = "ABCDEF"，cue 应当在 offset=3 (即 D 之前)
    assert.Equal(t, "ABCDEF", r.CleanText)
    assert.Equal(t, 3, r.Cues[0].CharOffset)
}
```

- [ ] **Step 5.4：跑测试 + commit**

```bash
go test -count=1 ./internal/service/audio/ -run Parse -v
git add server/internal/service/audio/cue_parser*.go
git commit -m "feat(audio): cue parser with offset tracking + mood fallback"
```

---

## Task 6：BGM 本地缓存（per-filename 互斥锁懒下载）

**Files:**
- Create: `server/internal/service/audio/bgm_cache.go`
- Create: `server/internal/service/audio/bgm_cache_test.go`

- [ ] **Step 6.1：接口与实现**

```go
package audio

import (
    "context"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sync"

    "github.com/aibao/server/internal/gateway/storage"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/pkg/logger"
)

// BGMCache returns local file paths for BGM assets, downloading from storage
// on first use. Concurrent calls for the same filename block on a single
// download (per-key sync.Once pattern).
type BGMCache interface {
    GetLocalPath(ctx context.Context, asset *model.BGMAsset) (string, error)
}

type bgmCache struct {
    storage storage.Client
    dir     string

    mu      sync.Mutex
    onceMap map[string]*sync.Once
    errMap  map[string]error
}

// NewBGMCache constructs a cache. dir is the on-disk root (e.g. "./cache/bgm").
// Caller is expected to ensure dir is writable; we MkdirAll on first use.
func NewBGMCache(s storage.Client, dir string) BGMCache {
    return &bgmCache{
        storage: s, dir: dir,
        onceMap: map[string]*sync.Once{},
        errMap:  map[string]error{},
    }
}

func (c *bgmCache) GetLocalPath(ctx context.Context, asset *model.BGMAsset) (string, error) {
    if asset == nil {
        return "", errors.New("bgm cache: nil asset")
    }
    local := filepath.Join(c.dir, asset.Filename)

    // Fast path: file exists.
    if _, err := os.Stat(local); err == nil {
        return local, nil
    }

    // Slow path: lock per-filename, then sync.Once-download.
    c.mu.Lock()
    once, ok := c.onceMap[asset.Filename]
    if !ok {
        once = &sync.Once{}
        c.onceMap[asset.Filename] = once
    }
    c.mu.Unlock()

    once.Do(func() {
        c.errMap[asset.Filename] = c.downloadLocked(ctx, asset, local)
    })

    if err := c.errMap[asset.Filename]; err != nil {
        return "", err
    }
    return local, nil
}

func (c *bgmCache) downloadLocked(ctx context.Context, asset *model.BGMAsset, local string) error {
    lg := logger.FromCtx(ctx).With("module", "bgm_cache", "filename", asset.Filename)
    if err := os.MkdirAll(c.dir, 0o755); err != nil {
        return fmt.Errorf("mkdir %s: %w", c.dir, err)
    }
    body, err := c.storage.Download(ctx, asset.ObjectKey)
    if err != nil {
        return fmt.Errorf("download %s: %w", asset.ObjectKey, err)
    }
    defer body.Close()

    tmp := local + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return fmt.Errorf("create %s: %w", tmp, err)
    }
    n, copyErr := io.Copy(f, body)
    closeErr := f.Close()
    if copyErr != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("copy: %w", copyErr)
    }
    if closeErr != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("close: %w", closeErr)
    }
    if err := os.Rename(tmp, local); err != nil {
        return fmt.Errorf("rename: %w", err)
    }
    lg.Info("bgm.cache.downloaded", "bytes", n)
    return nil
}
```

🎓 **教学点：per-key `sync.Once` 模式**——单一 `sync.Once` 只能"全局只做一次"。若同一 cache 实例要支持"按 filename 每个只下载一次"，就维护 `map[string]*sync.Once`，Lazy 创建。**关键陷阱**：读写 map 必须加锁（`c.mu`），但 `once.Do` 本身的并发安全由 `sync.Once` 自带，不要再套大锁——否则一个慢下载会卡住所有其他 filename。

⚠️ **失败缓存的争议点**：当前实现 `errMap[filename]` 一旦失败永久缓存——下次重试同一文件不再下载，直接返回旧错。生产可考虑"失败后清除 once 让下次重新尝试"。Plan 7 保留当前简单语义：Worker 失败重试会调多次 Compose，每次创建**新** Orchestrator 实例（main.go 装配为单例，所以 cache 也是单例——这个问题真实存在）。**记录为 TBD**：若 P1 发现有 BGM 短暂不可用导致永久降级，Plan 7b 引入 TTL 失败缓存。

- [ ] **Step 6.2：测试（mock storage）**

`bgm_cache_test.go`：

```go
package audio_test

import (
    "bytes"
    "context"
    "errors"
    "io"
    "os"
    "path/filepath"
    "sync"
    "sync/atomic"
    "testing"

    "github.com/aibao/server/internal/gateway/storage"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/service/audio"
    "github.com/stretchr/testify/require"
)

type fakeStorage struct {
    downloads int64
    body      []byte
    err       error
}

func (f *fakeStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
    atomic.AddInt64(&f.downloads, 1)
    if f.err != nil {
        return nil, f.err
    }
    return io.NopCloser(bytes.NewReader(f.body)), nil
}

// ... 其他 storage.Client 接口方法可以返回 nil/panic（测试不调）

func TestBGMCache_DownloadsOnFirstUse(t *testing.T) {
    dir := t.TempDir()
    fs := &fakeStorage{body: []byte("FAKEMP3")}
    c := audio.NewBGMCache(fs, dir)
    asset := &model.BGMAsset{Filename: "warm_01.mp3", ObjectKey: "bgm/warm/warm_01.mp3"}

    p1, err := c.GetLocalPath(context.Background(), asset)
    require.NoError(t, err)
    require.Equal(t, filepath.Join(dir, "warm_01.mp3"), p1)

    b, _ := os.ReadFile(p1)
    require.Equal(t, []byte("FAKEMP3"), b)
    require.EqualValues(t, 1, fs.downloads)

    // 第二次：缓存命中，不再下载
    p2, err := c.GetLocalPath(context.Background(), asset)
    require.NoError(t, err)
    require.Equal(t, p1, p2)
    require.EqualValues(t, 1, fs.downloads)
}

func TestBGMCache_ConcurrentSameFile_OnlyOneDownload(t *testing.T) {
    dir := t.TempDir()
    fs := &fakeStorage{body: []byte("FAKEMP3")}
    c := audio.NewBGMCache(fs, dir)
    asset := &model.BGMAsset{Filename: "warm_01.mp3", ObjectKey: "bgm/warm/warm_01.mp3"}

    var wg sync.WaitGroup
    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err := c.GetLocalPath(context.Background(), asset)
            require.NoError(t, err)
        }()
    }
    wg.Wait()
    require.EqualValues(t, 1, fs.downloads)
}

func TestBGMCache_DownloadError_Propagates(t *testing.T) {
    dir := t.TempDir()
    fs := &fakeStorage{err: errors.New("net down")}
    c := audio.NewBGMCache(fs, dir)
    _, err := c.GetLocalPath(context.Background(), &model.BGMAsset{
        Filename: "x.mp3", ObjectKey: "bgm/x.mp3",
    })
    require.Error(t, err)
}
```

注意：`storage.Client` 接口若还有 `Upload`/`SignURL` 等方法，fakeStorage 需补全（或包内提供 `NewMock()`）。

- [ ] **Step 6.3：跑测试 + commit**

```bash
go test -count=1 ./internal/service/audio/ -run BGMCache -v
git add server/internal/service/audio/bgm_cache*.go
git commit -m "feat(audio): bgm local cache with per-filename sync.Once download"
```

---

## Task 7：Mixer（exec.Command ffmpeg）

**Files:**
- Create: `server/internal/service/audio/mixer.go`
- Create: `server/internal/service/audio/mixer_test.go`
- Create: `server/internal/service/audio/testdata/tts_sample.mp3`（手工/CI 提供）
- Create: `server/internal/service/audio/testdata/bgm_sample.mp3`

- [ ] **Step 7.1：接口与实现**

```go
package audio

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "time"

    "github.com/aibao/server/internal/pkg/logger"
)

// Mixer mixes a TTS audio byte stream with a BGM file on disk into a single mp3.
type Mixer interface {
    // MixWithBGM returns (mixedBytes, durationSec, error).
    // On any error the caller is expected to fall back to ttsBytes unchanged.
    MixWithBGM(ctx context.Context, ttsBytes []byte, bgmPath string) ([]byte, int, error)
}

// FFmpegMixer is the production Mixer that shells out to ffmpeg.
type FFmpegMixer struct {
    Binary       string  // ffmpeg path (cfg.Audio.FFmpegPath)
    BGMVolumeDB  float64 // e.g. -18
    SampleRate   int     // e.g. 32000
    BitrateKbps  int     // e.g. 128
    Timeout      time.Duration
}

func NewFFmpegMixer(binary string, bgmVolDB float64, sr, br int, timeout time.Duration) *FFmpegMixer {
    return &FFmpegMixer{Binary: binary, BGMVolumeDB: bgmVolDB, SampleRate: sr, BitrateKbps: br, Timeout: timeout}
}

// ErrMixerUnavailable means ffmpeg cannot be located/executed.
var ErrMixerUnavailable = errors.New("ffmpeg unavailable")

// ErrMixerFailed wraps a ffmpeg execution failure.
type ErrMixerFailed struct {
    Stderr string
    Err    error
}

func (e *ErrMixerFailed) Error() string {
    return fmt.Sprintf("ffmpeg mix failed: %v; stderr: %s", e.Err, e.Stderr)
}
func (e *ErrMixerFailed) Unwrap() error { return e.Err }

func (m *FFmpegMixer) MixWithBGM(ctx context.Context, ttsBytes []byte, bgmPath string) ([]byte, int, error) {
    if _, err := exec.LookPath(m.Binary); err != nil {
        return nil, 0, fmt.Errorf("%w: %v", ErrMixerUnavailable, err)
    }
    lg := logger.FromCtx(ctx).With("module", "mixer")

    // Write TTS to temp file.
    ttsTmp, err := os.CreateTemp("", "aibao-tts-*.mp3")
    if err != nil {
        return nil, 0, fmt.Errorf("create tts tmp: %w", err)
    }
    defer os.Remove(ttsTmp.Name())
    if _, err := ttsTmp.Write(ttsBytes); err != nil {
        ttsTmp.Close()
        return nil, 0, fmt.Errorf("write tts tmp: %w", err)
    }
    ttsTmp.Close()

    // Output temp file.
    outTmp, err := os.CreateTemp("", "aibao-mix-*.mp3")
    if err != nil {
        return nil, 0, fmt.Errorf("create out tmp: %w", err)
    }
    outPath := outTmp.Name()
    outTmp.Close()
    defer os.Remove(outPath)

    cctx, cancel := context.WithTimeout(ctx, m.Timeout)
    defer cancel()

    // filter graph:
    //   [1:a]volume=-18dB[bgm];
    //   [0:a][bgm]amix=inputs=2:duration=first:dropout_transition=0[out]
    // BGM 的"循环到 TTS 长度"由输入级 `-stream_loop -1`(infinite stream)
    // + amix 的 `duration=first`（输出长度由第一个输入决定，即 TTS）共同完成。
    // 不在 filter 内再次 aloop —— 否则两个循环器叠加行为不可预测。
    filter := fmt.Sprintf(
        "[1:a]volume=%.1fdB[bgm];[0:a][bgm]amix=inputs=2:duration=first:dropout_transition=0[out]",
        m.BGMVolumeDB,
    )

    args := []string{
        "-y",
        "-i", ttsTmp.Name(),
        "-stream_loop", "-1", "-i", bgmPath,
        "-filter_complex", filter,
        "-map", "[out]",
        "-c:a", "libmp3lame",
        "-b:a", fmt.Sprintf("%dk", m.BitrateKbps),
        "-ar", fmt.Sprintf("%d", m.SampleRate),
        "-ac", "2",
        outPath,
    }

    cmd := exec.CommandContext(cctx, m.Binary, args...)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr

    tStart := time.Now()
    if err := cmd.Run(); err != nil {
        lg.Warn("mixer.ffmpeg.fail", "err", err.Error(), "stderr_head", head(stderr.String(), 500))
        return nil, 0, &ErrMixerFailed{Stderr: stderr.String(), Err: err}
    }
    elapsed := time.Since(tStart)

    out, err := os.ReadFile(outPath)
    if err != nil {
        return nil, 0, fmt.Errorf("read output: %w", err)
    }

    // duration_sec: parse from ffmpeg stderr "Duration:" if present; fallback to len/bitrate estimate.
    dur := parseDurationFromStderr(stderr.String())
    if dur <= 0 {
        dur = int(float64(len(out)*8) / float64(m.BitrateKbps*1000))
    }

    lg.Info("mixer.ok", "elapsed_ms", elapsed.Milliseconds(),
        "out_bytes", len(out), "dur_sec", dur)
    return out, dur, nil
}

func head(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n] + "..."
}

// parseDurationFromStderr extracts "Duration: HH:MM:SS.xx" from ffmpeg stderr.
// Returns 0 if not found.
func parseDurationFromStderr(s string) int {
    // 实现略 —— 用 regexp `Duration: (\d+):(\d+):(\d+)`. Plan 7 允许返回 0 走 fallback 估算。
    return 0
}
```

🎓 **教学点：filter_complex 在干嘛**
- `[1:a]` = 第二个输入（BGM）的音频流；`[0:a]` = 第一个输入（TTS）的音频流
- `volume=-18dB` = 衰减 BGM 到大约 1/8 响度（每 -6 dB 减半）
- `aloop=loop=-1:size=2e9` = 无限循环 BGM（size 是单段最大采样数，2e9 是 ffmpeg 文档推荐）
- `amix=inputs=2:duration=first` = 把两路混合，输出时长以第一路（TTS）为准——BGM 多出来的部分被丢弃
- `[out]` = 给混合后的流命名以便 `-map "[out]"` 引用

⚠️ **`-stream_loop -1` vs `aloop`**：两种循环 BGM 的方式。`-stream_loop` 在容器层重复输入文件解码循环（CPU 更省），`aloop` 在音频滤波层循环采样（更精确但更费内存）。**两个都加是冗余**——实际只要其中之一就够了。本 plan 保留 `-stream_loop -1` + `amix duration=first` 即可，可以从 filter 里删去 `aloop` 段。**TBD（实现者验证）**：以一个 5 分钟 TTS + 30 秒 BGM 测试，确保 BGM 真的循环铺满。若不行，再加回 `aloop`。

- [ ] **Step 7.2：测试 fixture**

`testdata/tts_sample.mp3` 和 `testdata/bgm_sample.mp3` ——**手工放置**或在 README 中说明用 ffmpeg 现场生成：

```bash
# 生成 2 秒静音 mp3 占位（人耳听不出，但 ffprobe 能解）
ffmpeg -f lavfi -i "anullsrc=r=22050:cl=mono" -t 2 -q:a 9 -acodec libmp3lame \
  server/internal/service/audio/testdata/tts_sample.mp3
ffmpeg -f lavfi -i "sine=frequency=440:duration=2:sample_rate=22050" -q:a 9 \
  -acodec libmp3lame server/internal/service/audio/testdata/bgm_sample.mp3
```

- [ ] **Step 7.3：单测**

```go
package audio_test

import (
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    "github.com/aibao/server/internal/service/audio"
    "github.com/stretchr/testify/require"
)

func TestFFmpegMixer_RealMix(t *testing.T) {
    if _, err := exec.LookPath("ffmpeg"); err != nil {
        t.Skip("ffmpeg not installed; skipping integration mix test")
    }
    tts, err := os.ReadFile("testdata/tts_sample.mp3")
    require.NoError(t, err)

    m := audio.NewFFmpegMixer("ffmpeg", -18, 32000, 128, 30*time.Second)
    out, dur, err := m.MixWithBGM(context.Background(), tts, "testdata/bgm_sample.mp3")
    require.NoError(t, err)
    require.NotEmpty(t, out)
    require.True(t, len(out) > 1024, "mixed output should be a real mp3, got %d bytes", len(out))
    require.GreaterOrEqual(t, dur, 0)
}

func TestFFmpegMixer_BinaryMissing(t *testing.T) {
    m := audio.NewFFmpegMixer("/definitely/not/ffmpeg", -18, 32000, 128, 5*time.Second)
    _, _, err := m.MixWithBGM(context.Background(), []byte("x"), "/tmp/nope.mp3")
    require.ErrorIs(t, err, audio.ErrMixerUnavailable)
}

func TestFFmpegMixer_BGMMissing(t *testing.T) {
    if _, err := exec.LookPath("ffmpeg"); err != nil {
        t.Skip("ffmpeg not installed")
    }
    m := audio.NewFFmpegMixer("ffmpeg", -18, 32000, 128, 10*time.Second)
    _, _, err := m.MixWithBGM(context.Background(), []byte("not real mp3"), "/tmp/does-not-exist.mp3")
    require.Error(t, err)
    var fe *audio.ErrMixerFailed
    require.ErrorAs(t, err, &fe)
    require.NotEmpty(t, fe.Stderr) // ffmpeg 错误信息有捕获
}
```

- [ ] **Step 7.4：跑测试 + commit**

```bash
go test -count=1 ./internal/service/audio/ -run Mixer -v
git add server/internal/service/audio/mixer*.go server/internal/service/audio/testdata/
git commit -m "feat(audio): FFmpegMixer with timeout, stderr capture, ErrMixerUnavailable"
```

---

## Task 8：Orchestrator

**Files:**
- Create: `server/internal/service/audio/orchestrator.go`
- Create: `server/internal/service/audio/orchestrator_test.go`

- [ ] **Step 8.1：类型 + 接口**

```go
package audio

import (
    "context"
    "errors"
    "time"

    "github.com/aibao/server/internal/gateway/tts"
    "github.com/aibao/server/internal/metrics"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/pkg/logger"
)

// BGMPicker is the minimal surface Orchestrator needs from BGMRepo.
type BGMPicker interface {
    PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error)
}

// ComposeRequest is the input to Compose.
type ComposeRequest struct {
    StoryID    int64
    ChildID    int64
    StoryText  string  // raw LLM output (with cues)
    Style      string  // story.Style for mood fallback
    Voice      tts.SynthesizeRequest // voice/model/format etc. pre-filled by caller
}

// ComposeResponse is what Orchestrator returns.
type ComposeResponse struct {
    AudioBytes      []byte
    AudioFormat     string
    AudioDurationSec int
    HasBGM          bool
    Mood            string
    ParseResult     ParseResult
}

// Orchestrator wires parser → tts → bgm pick → cache → mixer.
type Orchestrator struct {
    tts     tts.Client
    bgm     BGMPicker
    cache   BGMCache
    mixer   Mixer
    bm      *metrics.Business
    provider string // metric label (e.g. "ffmpeg")
}

func NewOrchestrator(t tts.Client, bgm BGMPicker, c BGMCache, m Mixer, bm *metrics.Business) *Orchestrator {
    return &Orchestrator{tts: t, bgm: bgm, cache: c, mixer: m, bm: bm, provider: "ffmpeg"}
}
```

- [ ] **Step 8.2：Compose 主流程**

```go
func (o *Orchestrator) Compose(ctx context.Context, req ComposeRequest) (*ComposeResponse, error) {
    lg := logger.FromCtx(ctx).With("module", "audio.orchestrator", "story_id", req.StoryID)

    // 1. Parse cues.
    pr := Parse(req.StoryText, req.Style)
    lg.Info("audio.parse", "mood", pr.BGMMood, "cue_count", len(pr.Cues),
        "clean_len", len(pr.CleanText))

    // 2. TTS on clean text.
    voiceReq := req.Voice
    voiceReq.Text = pr.CleanText
    ttsResp, err := o.tts.Synthesize(ctx, voiceReq)
    if err != nil {
        return nil, err // TTS 失败属硬错，不降级（既有 handler 行为）
    }

    base := &ComposeResponse{
        AudioBytes:       ttsResp.Audio,
        AudioFormat:      voiceReq.Format,
        AudioDurationSec: ttsResp.DurationSeconds,
        HasBGM:           false,
        Mood:             pr.BGMMood,
        ParseResult:      pr,
    }

    // 3. Pick BGM by mood.
    asset, err := o.bgm.PickByMood(ctx, pr.BGMMood)
    if err != nil {
        lg.Warn("audio.bgm.pick.err", "mood", pr.BGMMood, "err", err.Error())
        return o.degrade(ctx, base, "bgm_pick_err"), nil
    }
    if asset == nil {
        if o.bm != nil {
            o.bm.BGMNotFoundTotal.WithLabelValues(pr.BGMMood).Inc()
        }
        lg.Warn("audio.bgm.not_found", "mood", pr.BGMMood)
        return o.degrade(ctx, base, "bgm_not_found"), nil
    }

    // 4. Local cache.
    bgmPath, err := o.cache.GetLocalPath(ctx, asset)
    if err != nil {
        lg.Warn("audio.bgm.cache.err", "filename", asset.Filename, "err", err.Error())
        return o.degrade(ctx, base, "bgm_unavailable"), nil
    }

    // 5. Mix.
    tStart := time.Now()
    mixed, dur, err := o.mixer.MixWithBGM(ctx, ttsResp.Audio, bgmPath)
    if o.bm != nil {
        o.bm.AudioMixDuration.WithLabelValues(o.provider).Observe(time.Since(tStart).Seconds())
    }
    if err != nil {
        status := "fail"
        if errors.Is(err, ErrMixerUnavailable) {
            status = "degraded"
        }
        if o.bm != nil {
            o.bm.AudioMixTotal.WithLabelValues(o.provider, status).Inc()
        }
        lg.Warn("audio.mix.fail", "err", err.Error())
        return o.degrade(ctx, base, "mixer_fail"), nil
    }
    if o.bm != nil {
        o.bm.AudioMixTotal.WithLabelValues(o.provider, "ok").Inc()
    }

    base.AudioBytes = mixed
    base.AudioDurationSec = dur
    base.HasBGM = true
    lg.Info("audio.mix.ok", "mood", pr.BGMMood, "bgm", asset.Filename,
        "out_bytes", len(mixed), "dur_sec", dur)
    return base, nil
}

func (o *Orchestrator) degrade(ctx context.Context, base *ComposeResponse, reason string) *ComposeResponse {
    if o.bm != nil {
        o.bm.AudioMixTotal.WithLabelValues(o.provider, "degraded").Inc()
    }
    logger.FromCtx(ctx).Warn("audio.mix.degraded", "reason", reason, "mood", base.Mood)
    return base // HasBGM 仍为 false，AudioBytes 仍为纯 TTS
}
```

- [ ] **Step 8.3：测试（全 mock）**

```go
package audio_test

import (
    "context"
    "errors"
    "testing"

    "github.com/aibao/server/internal/gateway/tts"
    "github.com/aibao/server/internal/metrics"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/service/audio"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/stretchr/testify/require"
)

type stubTTS struct {
    resp tts.SynthesizeResponse
    err  error
}

func (s *stubTTS) Synthesize(ctx context.Context, r tts.SynthesizeRequest) (tts.SynthesizeResponse, error) {
    return s.resp, s.err
}

type stubBGM struct {
    asset *model.BGMAsset
    err   error
}

func (s *stubBGM) PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error) {
    return s.asset, s.err
}

type stubCache struct {
    path string
    err  error
}

func (s *stubCache) GetLocalPath(ctx context.Context, a *model.BGMAsset) (string, error) {
    return s.path, s.err
}

type stubMixer struct {
    out      []byte
    dur      int
    err      error
}

func (s *stubMixer) MixWithBGM(ctx context.Context, tts []byte, bgm string) ([]byte, int, error) {
    return s.out, s.dur, s.err
}

func newOrch(t *testing.T, ts tts.Client, bgm audio.BGMPicker, c audio.BGMCache, m audio.Mixer) *audio.Orchestrator {
    bm := metrics.NewBusiness(prometheus.NewRegistry())
    return audio.NewOrchestrator(ts, bgm, c, m, bm)
}

func TestOrchestrator_HappyPath(t *testing.T) {
    o := newOrch(t,
        &stubTTS{resp: tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 10}},
        &stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3", ObjectKey: "bgm/warm/warm_01.mp3"}},
        &stubCache{path: "/tmp/warm_01.mp3"},
        &stubMixer{out: []byte("MIXED"), dur: 10},
    )
    resp, err := o.Compose(context.Background(), audio.ComposeRequest{
        StoryText: "[BGM情绪:温馨]Hello", Style: "温馨治愈",
        Voice: tts.SynthesizeRequest{Format: "mp3"},
    })
    require.NoError(t, err)
    require.True(t, resp.HasBGM)
    require.Equal(t, []byte("MIXED"), resp.AudioBytes)
    require.Equal(t, model.MoodWarm, resp.Mood)
}

func TestOrchestrator_Degrades_BGMNotFound(t *testing.T) {
    o := newOrch(t,
        &stubTTS{resp: tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
        &stubBGM{asset: nil},
        &stubCache{},
        &stubMixer{},
    )
    resp, err := o.Compose(context.Background(), audio.ComposeRequest{
        StoryText: "无 cue", Style: "冒险探索",
        Voice: tts.SynthesizeRequest{Format: "mp3"},
    })
    require.NoError(t, err)
    require.False(t, resp.HasBGM)
    require.Equal(t, []byte("TTS"), resp.AudioBytes)
}

func TestOrchestrator_Degrades_MixerUnavailable(t *testing.T) {
    o := newOrch(t,
        &stubTTS{resp: tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
        &stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3"}},
        &stubCache{path: "/tmp/warm_01.mp3"},
        &stubMixer{err: audio.ErrMixerUnavailable},
    )
    resp, err := o.Compose(context.Background(), audio.ComposeRequest{
        StoryText: "x", Style: "温馨治愈", Voice: tts.SynthesizeRequest{Format: "mp3"},
    })
    require.NoError(t, err)
    require.False(t, resp.HasBGM)
}

func TestOrchestrator_TTSError_Hard(t *testing.T) {
    o := newOrch(t,
        &stubTTS{err: errors.New("tts down")},
        &stubBGM{}, &stubCache{}, &stubMixer{},
    )
    _, err := o.Compose(context.Background(), audio.ComposeRequest{
        StoryText: "x", Voice: tts.SynthesizeRequest{Format: "mp3"},
    })
    require.Error(t, err) // TTS 失败属硬错
}

func TestOrchestrator_Degrades_CacheError(t *testing.T) {
    o := newOrch(t,
        &stubTTS{resp: tts.SynthesizeResponse{Audio: []byte("TTS"), DurationSeconds: 5}},
        &stubBGM{asset: &model.BGMAsset{Mood: model.MoodWarm, Filename: "warm_01.mp3"}},
        &stubCache{err: errors.New("cos 500")},
        &stubMixer{},
    )
    resp, err := o.Compose(context.Background(), audio.ComposeRequest{
        StoryText: "x", Style: "温馨", Voice: tts.SynthesizeRequest{Format: "mp3"},
    })
    require.NoError(t, err)
    require.False(t, resp.HasBGM)
}
```

- [ ] **Step 8.4：跑测试 + commit**

```bash
go test -count=1 ./internal/service/audio/... -v
git add server/internal/service/audio/orchestrator*.go
git commit -m "feat(audio): Orchestrator with 4-path degradation (bgm/cache/mixer/unavailable)"
```

---

## Task 9：重构 tts_synthesis handler

**Files:**
- Modify: `server/internal/worker/handlers/tts_synthesis.go`
- Modify: `server/internal/worker/handlers/tts_synthesis_test.go`
- Modify: `server/internal/repository/story_repo.go`（`MarkAudioReady` 增 `hasBGM`）

- [ ] **Step 9.1：repo 签名调整**

`story_repo.go`：

```go
// 接口
type StoryRepo interface {
    // ... 既有 ...
    MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string,
        sizeBytes int64, durationSec int, hasBGM bool) error
    // ...
}

// 实现 updates map 追加：
"has_bgm": hasBGM,
```

`story_repo_test.go` 既有 `MarkAudioReady` 调用补 `false` 或 `true` 参数。

- [ ] **Step 9.2：handler 改写**

```go
package handlers

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/aibao/server/internal/gateway/storage"
    "github.com/aibao/server/internal/gateway/tts"
    "github.com/aibao/server/internal/metrics"
    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/pkg/logger"
    "github.com/aibao/server/internal/service/audio"
)

type StoryReader interface {
    FindByID(ctx context.Context, id int64) (*model.Story, error)
}

type StoryAudioWriter interface {
    MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string,
        sizeBytes int64, durationSec int, hasBGM bool) error
    MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error
}

type TTSHandlerConfig struct {
    Provider   string
    Model      string
    VoiceID    string
    Format     string
    SampleRate int
    Bitrate    int
    Speed      float64
}

// Composer is what TTSSynthesisHandler needs from audio.Orchestrator.
type Composer interface {
    Compose(ctx context.Context, req audio.ComposeRequest) (*audio.ComposeResponse, error)
}

type TTSSynthesisHandler struct {
    stories  StoryReader
    repo     StoryAudioWriter
    composer Composer
    storage  storage.Client
    cfg      TTSHandlerConfig
    bm       *metrics.Business
}

func NewTTSSynthesisHandler(
    stories StoryReader, repo StoryAudioWriter,
    composer Composer, s storage.Client,
    cfg TTSHandlerConfig, bm *metrics.Business,
) *TTSSynthesisHandler {
    return &TTSSynthesisHandler{stories: stories, repo: repo, composer: composer, storage: s, cfg: cfg, bm: bm}
}

type ttsSynthesisPayload struct {
    StoryID int64 `json:"story_id"`
}

func (h *TTSSynthesisHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
    lg := logger.FromCtx(ctx).With("module", "tts_handler", "event_id", e.ID)

    var p ttsSynthesisPayload
    if err := json.Unmarshal(e.Payload, &p); err != nil {
        return fmt.Errorf("decode payload: %w", err)
    }
    storyID := p.StoryID
    if storyID == 0 && e.AggregateID != nil {
        storyID = *e.AggregateID
    }
    if storyID == 0 {
        return errors.New("payload missing story_id and event missing aggregate_id")
    }

    story, err := h.stories.FindByID(ctx, storyID)
    if err != nil {
        return fmt.Errorf("load story %d: %w", storyID, err)
    }
    if story == nil {
        return fmt.Errorf("story %d not found", storyID)
    }
    if story.AudioStatus == model.AudioStatusReady && story.AudioObjectKey != "" {
        lg.Info("tts.skip.already_ready", "story_id", storyID, "key", story.AudioObjectKey)
        return nil
    }

    composed, err := h.composer.Compose(ctx, audio.ComposeRequest{
        StoryID:   story.ID,
        ChildID:   story.ChildID,
        StoryText: story.TextContent,
        Style:     story.Style,
        Voice: tts.SynthesizeRequest{
            VoiceID: h.cfg.VoiceID, Model: h.cfg.Model,
            Format: h.cfg.Format, SampleRate: h.cfg.SampleRate,
            Bitrate: h.cfg.Bitrate, Speed: h.cfg.Speed,
        },
    })
    if err != nil {
        if h.bm != nil {
            h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "fail").Inc()
            h.bm.AudioFailedTotal.WithLabelValues("tts").Inc()
        }
        if mErr := h.repo.MarkAudioFailed(ctx, storyID, err.Error()); mErr != nil {
            lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
        }
        return fmt.Errorf("audio compose: %w", err)
    }
    if h.bm != nil {
        h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "ok").Inc()
    }
    lg.Info("audio.compose.ok", "story_id", storyID,
        "bytes", len(composed.AudioBytes), "dur_sec", composed.AudioDurationSec,
        "has_bgm", composed.HasBGM, "mood", composed.Mood)

    key := buildObjectKey(story.ChildID, story.ID, h.cfg.Format)
    uStart := time.Now()
    err = h.storage.Upload(ctx, storage.UploadInput{
        Key: key, Body: bytes.NewReader(composed.AudioBytes), Size: int64(len(composed.AudioBytes)),
        ContentType: contentTypeFor(h.cfg.Format),
    })
    if h.bm != nil {
        h.bm.StorageUploadDuration.WithLabelValues("cos").Observe(time.Since(uStart).Seconds())
    }
    if err != nil {
        if h.bm != nil {
            h.bm.AudioFailedTotal.WithLabelValues("storage").Inc()
        }
        if mErr := h.repo.MarkAudioFailed(ctx, storyID, err.Error()); mErr != nil {
            lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
        }
        return fmt.Errorf("storage upload: %w", err)
    }
    lg.Info("storage.upload.ok", "story_id", storyID, "key", key)

    if err := h.repo.MarkAudioReady(ctx, storyID, key, h.cfg.Format,
        int64(len(composed.AudioBytes)), composed.AudioDurationSec, composed.HasBGM); err != nil {
        if h.bm != nil {
            h.bm.AudioFailedTotal.WithLabelValues("db").Inc()
        }
        return fmt.Errorf("mark audio ready: %w", err)
    }
    if h.bm != nil {
        h.bm.AudioReadyTotal.Inc()
    }
    return nil
}

func buildObjectKey(childID, storyID int64, format string) string {
    return fmt.Sprintf("audio/%d/%d-%d.%s", childID, storyID, time.Now().UnixNano(), format)
}

func contentTypeFor(format string) string {
    switch format {
    case "mp3":
        return "audio/mpeg"
    case "wav":
        return "audio/wav"
    case "pcm":
        return "audio/L16"
    default:
        return "application/octet-stream"
    }
}
```

- [ ] **Step 9.3：测试改写**

既有 `tts_synthesis_test.go` 中的 mock `tts.Client` 替换为 mock `Composer`。骨架：

```go
type stubComposer struct {
    resp *audio.ComposeResponse
    err  error
}

func (s *stubComposer) Compose(ctx context.Context, req audio.ComposeRequest) (*audio.ComposeResponse, error) {
    return s.resp, s.err
}

func TestTTSHandler_Compose_OK_HasBGM(t *testing.T) {
    // ... 装配 stubComposer 返回 HasBGM=true ...
    // assert: repo.MarkAudioReady 被调用且 hasBGM=true
}

func TestTTSHandler_Compose_OK_Degraded(t *testing.T) {
    // stubComposer 返回 HasBGM=false（纯 TTS 字节）
    // assert: MarkAudioReady 调用且 hasBGM=false
}

func TestTTSHandler_Compose_Error_MarksFailed(t *testing.T) {
    // stubComposer 返回 err
    // assert: MarkAudioFailed 被调用
}
```

- [ ] **Step 9.4：跑测试 + commit**

```bash
go test -count=1 ./internal/worker/... ./internal/repository/...
git add server/internal/worker server/internal/repository
git commit -m "refactor(worker): tts_synthesis calls audio.Orchestrator; MarkAudioReady takes has_bgm"
```

---

## Task 10：main.go 装配

**Files:**
- Modify: `server/cmd/server/main.go`

- [ ] **Step 10.1：构造 audio 链 + 注入 handler**

在 `main.go` 既有 TTS/Storage 装配之后追加：

```go
// --- Audio orchestrator wiring (Plan 7) ---
bgmRepo := repository.NewBGMRepo(db)
bgmCache := audio.NewBGMCache(storageClient, cfg.Audio.BGMCacheDir)
ffmpegMixer := audio.NewFFmpegMixer(
    cfg.Audio.FFmpegPath,
    cfg.Audio.BGMVolumeDB,
    cfg.Audio.MixSampleRate,
    cfg.Audio.MixBitrateKbps,
    time.Duration(cfg.Audio.MixTimeoutSec)*time.Second,
)
audioOrch := audio.NewOrchestrator(ttsClient, bgmRepo, bgmCache, ffmpegMixer, businessMetrics)

ttsHandler := handlers.NewTTSSynthesisHandler(
    storyRepo, storyRepo, audioOrch, storageClient,
    handlers.TTSHandlerConfig{
        Provider: cfg.TTS.Provider, Model: cfg.TTS.Model,
        VoiceID: cfg.TTS.VoiceID, Format: cfg.TTS.Format,
        SampleRate: cfg.TTS.SampleRate, Bitrate: cfg.TTS.Bitrate,
        Speed: cfg.TTS.Speed,
    },
    businessMetrics,
)
worker.RegisterHandler("tts_synthesis", ttsHandler.Handle)
```

启动时附带健康检查（**软警告**，不阻塞启动）：

```go
if _, err := exec.LookPath(cfg.Audio.FFmpegPath); err != nil {
    slog.Warn("ffmpeg not found in PATH; all stories will degrade to TTS-only",
        "configured_path", cfg.Audio.FFmpegPath, "err", err.Error())
}
```

- [ ] **Step 10.2：本地起服务 + 冒烟**

```bash
cd server && make run-dev
# 另一个 shell：
curl -X POST localhost:8080/api/v1/stories/generate -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"child_id":1,"prompt":"讲个奥特曼故事","duration":5,"style":"温馨治愈","topic":"勇敢"}'
# 等 10 秒
docker exec aibao-postgres-dev psql -U aibao -d aibao -c \
  "SELECT id, audio_status, has_bgm, audio_duration_seconds FROM stories ORDER BY id DESC LIMIT 1;"
```

Expected：`audio_status=ready`, `has_bgm=true`（前提：已 seed-bgm + 已上传 BGM 文件）。

- [ ] **Step 10.3：commit**

```bash
git add server/cmd/server/main.go
git commit -m "feat(bootstrap): wire BGMRepo+Cache+Mixer+Orchestrator into tts_synthesis handler"
```

---

## Task 11：seed-bgm CLI + manifest

**Files:**
- Create: `server/cmd/seed-bgm/main.go`
- Create: `server/safety/bgm_manifest.yaml`

- [ ] **Step 11.1：manifest**

`server/safety/bgm_manifest.yaml`：

```yaml
# BGM manifest for seed-bgm CLI.
# 10 first-party assets: 5 moods × 2 variants.
# 用户在采购/录制完真实音频后，更新 license / duration_sec 字段，
# 然后将 mp3 文件上传到 COS 对应 object_key 路径。
bgm:
  - mood: warm
    filename: warm_01.mp3
    object_key: bgm/warm/warm_01.mp3
    duration_sec: 60
    license: "TBD"
  - mood: warm
    filename: warm_02.mp3
    object_key: bgm/warm/warm_02.mp3
    duration_sec: 60
    license: "TBD"
  - mood: adventure
    filename: adventure_01.mp3
    object_key: bgm/adventure/adventure_01.mp3
    duration_sec: 60
    license: "TBD"
  - mood: adventure
    filename: adventure_02.mp3
    object_key: bgm/adventure/adventure_02.mp3
    duration_sec: 60
    license: "TBD"
  - mood: funny
    filename: funny_01.mp3
    object_key: bgm/funny/funny_01.mp3
    duration_sec: 60
    license: "TBD"
  - mood: funny
    filename: funny_02.mp3
    object_key: bgm/funny/funny_02.mp3
    duration_sec: 60
    license: "TBD"
  - mood: magic
    filename: magic_01.mp3
    object_key: bgm/magic/magic_01.mp3
    duration_sec: 60
    license: "TBD"
  - mood: magic
    filename: magic_02.mp3
    object_key: bgm/magic/magic_02.mp3
    duration_sec: 60
    license: "TBD"
  - mood: curious
    filename: curious_01.mp3
    object_key: bgm/curious/curious_01.mp3
    duration_sec: 60
    license: "TBD"
  - mood: curious
    filename: curious_02.mp3
    object_key: bgm/curious/curious_02.mp3
    duration_sec: 60
    license: "TBD"
```

- [ ] **Step 11.2：CLI**

`server/cmd/seed-bgm/main.go`：

```go
// Command seed-bgm reads a YAML manifest and upserts bgm_assets rows.
//
//   go run ./cmd/seed-bgm -manifest=server/safety/bgm_manifest.yaml
package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "os"

    "github.com/aibao/server/internal/model"
    "github.com/aibao/server/internal/pkg/config"
    "github.com/aibao/server/internal/pkg/db"
    "github.com/aibao/server/internal/repository"
    "gopkg.in/yaml.v3"
)

type manifest struct {
    BGM []struct {
        Mood        string `yaml:"mood"`
        Filename    string `yaml:"filename"`
        ObjectKey   string `yaml:"object_key"`
        DurationSec int    `yaml:"duration_sec"`
        License     string `yaml:"license"`
    } `yaml:"bgm"`
}

func main() {
    manifestPath := flag.String("manifest", "server/safety/bgm_manifest.yaml", "path to BGM manifest")
    configPath := flag.String("config", "server/config/config.dev.yaml", "path to server config")
    flag.Parse()

    cfg, err := config.Load(*configPath)
    must(err)

    raw, err := os.ReadFile(*manifestPath)
    must(err)
    var m manifest
    must(yaml.Unmarshal(raw, &m))

    database, err := db.Open(cfg.Postgres)
    must(err)

    repo := repository.NewBGMRepo(database)
    ctx := context.Background()
    for _, b := range m.BGM {
        err := repo.Upsert(ctx, &model.BGMAsset{
            Mood: b.Mood, Filename: b.Filename, ObjectKey: b.ObjectKey,
            DurationSec: b.DurationSec, License: b.License, Active: true,
        })
        if err != nil {
            slog.Error("seed.bgm.upsert.fail", "filename", b.Filename, "err", err.Error())
            os.Exit(2)
        }
        fmt.Printf("upserted %s (mood=%s)\n", b.Filename, b.Mood)
    }
    fmt.Printf("seeded %d BGM assets\n", len(m.BGM))
}

func must(err error) {
    if err != nil {
        slog.Error("seed.bgm.fatal", "err", err.Error())
        os.Exit(1)
    }
}
```

- [ ] **Step 11.3：跑 + commit**

```bash
cd server && go run ./cmd/seed-bgm
docker exec aibao-postgres-dev psql -U aibao -d aibao -c \
  "SELECT mood, count(*) FROM bgm_assets WHERE active GROUP BY mood ORDER BY mood;"
```

Expected: 5 行，each count=2。

```bash
git add server/cmd/seed-bgm server/safety/bgm_manifest.yaml
git commit -m "feat(cli): seed-bgm CLI + 10-entry BGM manifest with TBD licenses"
```

---

## Task 12：Makefile + .gitignore

**Files:**
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 12.1：Makefile target**

追加：

```makefile
.PHONY: seed-bgm
seed-bgm: ## Upsert BGM rows from manifest YAML
	cd server && go run ./cmd/seed-bgm \
	  -manifest=safety/bgm_manifest.yaml \
	  -config=config/config.dev.yaml

.PHONY: audio-mix-test
audio-mix-test: ## Manual smoke: generate a real story end-to-end and inspect mp3
	@echo "1) Ensure ffmpeg installed: ffmpeg -version"
	@echo "2) Ensure 10 BGM files uploaded to COS under bgm/<mood>/"
	@echo "3) make run-dev (in another shell)"
	@echo "4) POST /api/v1/stories/generate as documented"
	@echo "5) ffprobe -i <downloaded mp3>  # 应看到 stereo, 32kHz, ~128kbps"
```

- [ ] **Step 12.2：.gitignore**

追加：

```
server/cache/
```

- [ ] **Step 12.3：commit**

```bash
git add Makefile .gitignore
git commit -m "build: seed-bgm + audio-mix-test targets; ignore server/cache/"
```

---

## Task 13：端到端冒烟（手工）

**这是用户手工任务**，不要在 AI 实施时执行。Plan 7 完成的认定标准之一。

- [ ] **Step 13.1：安装 ffmpeg**

Windows：
```powershell
winget install Gyan.FFmpeg
# 重启 shell 后：ffmpeg -version
```

Linux：
```bash
apt-get install ffmpeg   # 或 alpine 容器内 apk add --no-cache ffmpeg
```

- [ ] **Step 13.2：准备 10 个 BGM 文件**

用户自行采购 / 录制 / 用 CC0 素材库（Pixabay、free-music-archive 等）下载 10 首 mp3。文件名严格按 `warm_01.mp3` / `warm_02.mp3` / `adventure_01.mp3` / ... / `curious_02.mp3`。每首 30-90 秒即可（混音时会自动循环）。

上传到 COS（用腾讯云控制台或 `coscmd upload`）：

```
bgm/warm/warm_01.mp3
bgm/warm/warm_02.mp3
bgm/adventure/adventure_01.mp3
...
bgm/curious/curious_02.mp3
```

- [ ] **Step 13.3：更新 manifest license + duration**

用户在 `bgm_manifest.yaml` 把 `license: "TBD"` 改成真实来源，`duration_sec` 改成 ffprobe 测得的值，重跑 `make seed-bgm`。

- [ ] **Step 13.4：完整流程跑一遍**

```bash
make run-dev
# 另一 shell：
# 1) 登录拿 token（Plan 2 流程）
# 2) 创建孩子（Plan 2）
# 3) 生成故事：
curl -X POST http://localhost:8080/api/v1/stories/generate \
  -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" \
  -d '{"child_id":1,"prompt":"讲个森林冒险","duration":5,"style":"冒险探索","topic":"勇敢"}'
# 4) 等 10-15 秒，查库：
docker exec aibao-postgres-dev psql -U aibao -d aibao -c \
  "SELECT id, audio_status, has_bgm FROM stories ORDER BY id DESC LIMIT 1;"
# Expected: audio_status=ready, has_bgm=true
# 5) 拿签名 URL：
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8080/api/v1/stories/<id>/audio_url
# 6) 下载 mp3 并播放，耳朵确认背景音乐能听到
```

- [ ] **Step 13.5：降级冒烟**

```bash
# 关闭 ffmpeg 路径：
AIBAO_AUDIO_FFMPEG_PATH=/nonexistent ./bin/server
# 重跑步骤 3-4，期望 has_bgm=false 且日志含 audio.mix.degraded
```

---

## Task 14：文档与知识库同步

**Files:**
- Create: `docs/devlog/2026-05-13.md`
- Modify: `CLAUDE.md`
- Modify: `MEMORY.md`
- Modify: `docs/knowledge/03-go-engineering.md`
- Modify: `docs/knowledge/05-software-design.md`
- Modify or Create: `docs/knowledge/09-observability.md`（或新建 `docs/knowledge/11-audio-and-media.md`）
- Modify: `docs/knowledge/README.md`（更新索引）

- [ ] **Step 14.1：devlog**

`docs/devlog/2026-05-13.md`，简述实施：本日完成 Plan 7：建 bgm_assets 表、写 cue parser / bgm cache / mixer / orchestrator 四件套、重构 tts_synthesis handler、新增 seed-bgm CLI 与 10 条 manifest 占位。10 个真实 BGM 文件待用户后续采购上传。

- [ ] **Step 14.2：CLAUDE.md 更新**

"已落地的能力"段落底部追加：

```markdown
- **Plan 7：音频混音 + BGM**（cue parser + bgm cache + ffmpeg mixer + orchestrator；tts_synthesis handler 重构为编排链；10 首 BGM 经 manifest 管理）
```

"端到端可演示接口"段：把 audio_url 注释改为"返回 TTS+BGM 混音后的成品"。

"当前阶段"段更新为：`Plan 7 完成（2026-05-13）：真实 BGM 混音落地（用户需手工采集 10 首 BGM 并上传到 COS）。下一步：Plan 7b SFX 音效 / Plan 8 部署 / Flutter 客户端`。

- [ ] **Step 14.3：MEMORY.md 同步**

记录决策：ffmpeg 走 OS 二进制不绑 cgo；BGM mood 5 类 × 2 variants；降级哲学（任意一步失败都不阻塞故事）。

- [ ] **Step 14.4：knowledge 词条**

`docs/knowledge/03-go-engineering.md` 新增词条：

> ### 外部进程调用（exec.Command）vs 库绑定（cgo）
> **一句话定义：** Go 调用外部程序的两种姿势——通过子进程跑独立二进制，或者通过 cgo 绑定 C 库直接链入。
> **生活类比：** 子进程调用像"叫外卖"，cgo 绑定像"自家厨房做饭"。外卖（exec）：餐厅独立，菜没了不影响你的厨房；cgo：菜放在自家冰箱，冰箱坏了所有菜一起完蛋。
> **为什么需要：** 复杂二进制（ffmpeg / ImageMagick）的 cgo 绑定有几个大坑：(1) 跨平台编译困难——Windows 上 cgo 几乎等于地狱；(2) 内存错误会让整个 Go 进程崩溃，子进程崩了我们只是 mix 失败而已；(3) 升级 ffmpeg 不需要重新编译 Go 程序。代价是每次调用要 fork 进程（约 10-50ms 开销）+ 子进程超时/僵尸要主动管理。
> **在本项目中怎么用：** `service/audio/mixer.go` 用 `exec.CommandContext(ctx, "ffmpeg", args...)`，stderr 全捕获到 `bytes.Buffer`、用 `context.WithTimeout` 自动 kill 超时进程、defer 清理临时文件。
> **何时引入：** 2026-05-13 Plan 7 Task 7。

`docs/knowledge/05-software-design.md` 新增词条：

> ### 懒下载缓存 + per-key sync.Once 模式
> **一句话定义：** 一种"按需获取、并发去重"的资源加载模式——首次访问触发下载，后续访问直接命中缓存，且多个 goroutine 同时请求同一个 key 时只下载一次。
> **生活类比：** 像图书馆借书。书第一次有人要 → 工作人员去库房取（其他人等）；第二个人同时要同一本 → 跟着前面那个一起等同一趟；后续人来 → 书已经在借阅台，直接拿。
> **为什么需要：** 朴素的"if 文件不存在 then 下载"在并发下会出现 N 个 goroutine 同时下载同一文件，浪费带宽 + 写入打架（部分文件、损坏 mp3）。`sync.Once` 是 Go 提供的"全局一次"原语，但只能"全局一次"，不能"按 filename 各一次"。所以维护 `map[string]*sync.Once` + 大锁（保护 map 创建），让每个 filename 拥有自己的 Once。
> **在本项目中怎么用：** `service/audio/bgm_cache.go`。注意陷阱：失败一次会被永久缓存（`errMap`），生产可能要 TTL 失败缓存。
> **何时引入：** 2026-05-13 Plan 7 Task 6。

`docs/knowledge/11-audio-and-media.md`（新建）：

> ### ffmpeg filter_complex 基础
> **一句话定义：** ffmpeg 把多路输入按图状管线处理的小语言。
> **生活类比：** 像调音台——每路音频是一根输入线（`[0:a]`/`[1:a]`），每个滤镜（volume、aloop、amix）是一个旋钮，输出标签（`[out]`）是总线。filter_complex 字符串就是一张声音管线图的文字版。
> **为什么需要：** 简单的"输入→输出"用 `-af` 就够，多输入混音/合并必须用 filter_complex。我们的混音 = 衰减 BGM + 循环铺满 TTS 长度 + 两路相加，在普通 `-af` 里根本写不出。
> **关键节点：**
> - `[N:a]` 引用第 N+1 个 `-i` 输入的音频流（zero-indexed）
> - `volume=-18dB` 衰减
> - `aloop=loop=-1:size=2e9` 音频流循环（vs `-stream_loop` 在容器层循环）
> - `amix=inputs=2:duration=first:dropout_transition=0` 多路加权混合，`duration=first` 让输出长度等于第一路（TTS）
> - `[out]` 给最终流命名 → `-map "[out]"` 引用
> **本项目中：** `service/audio/mixer.go`。`-stream_loop -1 -i <bgm>` 已经让 BGM 循环，filter 里 `aloop` 段是冗余的（实现者 TBD：测试后决定保留哪种）。
> **何时引入：** 2026-05-13 Plan 7 Task 7。

`docs/knowledge/README.md` 索引补 3 条新词条编号。

- [ ] **Step 14.5：commit**

```bash
git add docs CLAUDE.md MEMORY.md
git commit -m "docs(plan-7): devlog + knowledge entries (exec.Command / sync.Once / ffmpeg filters)"
```

---

# 实施完成检查清单

- [ ] migration 000006 已 apply，`bgm_assets` 8 列 + 2 索引存在
- [ ] `make seed-bgm` 跑过，5 mood × 2 = 10 行
- [ ] `cfg.Audio` 6 字段加载验证 + dev yaml 已配
- [ ] 3 个新指标在 `/metrics` 可见：`audio_mix_duration_seconds` / `audio_mix_total` / `bgm_not_found_total`
- [ ] `service/audio` 包测试覆盖率 ≥ 70%；mixer 真实测试 ffmpeg 在装时跑通、不在时 Skip
- [ ] `worker/handlers/tts_synthesis_test.go` 全过（mock Composer 替代 mock TTS）
- [ ] `repository.StoryRepo.MarkAudioReady` 签名含 `hasBGM bool`，所有调用点已改
- [ ] `main.go` 启动时 ffmpeg 路径软健康检查日志可见
- [ ] manifest YAML 10 行，license 字段标 `TBD` 等用户填
- [ ] `.gitignore` 含 `server/cache/`
- [ ] CLAUDE.md / MEMORY.md / devlog / 3 条知识词条全部 commit
- [ ] `golangci-lint run ./...` 0 issues
- [ ] **手工冒烟（用户）**：装 ffmpeg + 上传 10 首 BGM + 跑端到端 → 听到的 mp3 既有人声又有 BGM；降级路径手动验证 has_bgm=false

---

# 后续规划（不在本 plan 内）

- **Plan 7b：SFX 音效**——利用 cue_parser 已记录的 `CharOffset`，在 mixer 中追加多路 SFX 输入，按字符偏移估算的时间点用 `adelay` 滤镜对齐。10 类 SFX × 1 variant = 10 个素材，沿用相同 manifest 模式。
- **Plan 7c：自适应混音**——根据 cue 密度调整 BGM 音量曲线（旁白多时拉低 BGM，对白少时拉高）。需 `sidechaincompress` 滤镜或预 RMS 分析。
- **Plan 8：部署**——Dockerfile 加 `apk add ffmpeg`；k8s manifest 增加 BGM 缓存目录的 PVC 或者 init-container 预热缓存。
