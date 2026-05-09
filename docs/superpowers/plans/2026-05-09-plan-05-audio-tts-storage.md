# Plan 5：TTS 音频合成 + COS 存储 + 异步音频管线

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Plan 4（已能同步生成故事文本）之上，让爱宝"开口"。完成后端到端可演示：用户调 `POST /api/v1/stories/generate` → 同步返回故事文本 + `audio_status:"pending"` → 客户端轮询 `GET /api/v1/stories/{id}/audio_url` → 几秒后返回 15 分钟有效的 COS 签名 URL，浏览器/播放器可直接播放。这是产品第一次"像它的吉祥物"——百变熊猫终于能用声音陪孩子。

**Architecture:** 同步线只多做一件事：在 Plan 4 已有的 `CreateWithOutbox` 事务里**多写一条** `tts_synthesis` outbox 事件。异步线新增一个 handler：Worker 拉到 `tts_synthesis` 事件 → 从 DB 重取最新 story 文本 → 调 `gateway/tts.Minimax.Synthesize` 拿到 mp3 字节 → 调 `gateway/storage.COS.Upload` 上传 → `MarkAudioReady(audio_object_key, format, size, duration)`。客户端的 `GET /audio_url` 接口读 `stories.audio_status` 三态分支（pending/ready/failed），ready 时由 storage 现签 URL 返回，pending 时返回 `retry_after:5`。BGM/混音/cue 解析**不在本 plan**，留给 Plan 5b/6。

**Tech Stack:**
- Go 1.24+ + Gin + GORM + PostgreSQL（已有）
- Minimax T2A：**无官方 Go SDK**，直连 REST `https://api.minimax.chat/v1/t2a_v2?GroupId=<gid>`，body/响应都是 JSON，音频字段是 hex 编码的 mp3 字节——用 `net/http` + `encoding/json` + `encoding/hex` 即可
- 腾讯云 COS：`github.com/tencentyun/cos-go-sdk-v5 v0.7.73`（go.mod 已存在为 indirect，本 plan 真正 import 后自动转 direct）
- 复用 Plan 1-4：worker / outbox / metrics / repository / userctx / api.RespondError / pkg/config

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)（第 5 章故事+朗读体验、第 7 章红线对儿童声音的隐含要求）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - 第 4 章核心数据流（**核心**——同步段返回文本，异步段补音频）
  - 第 5 章 stories 表（`audio_object_key/audio_format/audio_size_bytes/audio_duration_seconds` 已存在；本 plan 加 `audio_status/audio_failed_at`）
  - 第 6 章 Gateway 抽象层（**核心**——TTS/Storage 必须照 Plan 4 的 `gateway/llm` 同样的接口+mock+真实实现三件套结构）
  - 第 8 章音频编排（本 plan 只做最简版：纯人声，无 BGM 混音）
  - 第 9 章 Outbox Pattern（**核心**——事件 schema 沿用 Plan 4 表，仅新增 event_type）
- Plan 4：[2026-05-08-plan-04-story-generation.md](2026-05-08-plan-04-story-generation.md)（**必读**——本 plan 是 Plan 4 的直接延续，复用 Worker/Outbox/Metrics 框架，扩 `CreateWithOutbox` 签名为 `[]*OutboxEvent`，扩 main.go 装配）
- CLAUDE.md（4.2 内容安全；4.4 注释/文档风格）

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；新增 service+gateway 覆盖率 ≥ 70%
2. `make migrate-up` 应用 `000004_audio_status` 后，`\d stories` 显示新增列 `audio_status varchar(16) DEFAULT 'pending'` 与 `audio_failed_at timestamptz NULL`，并存在 `stories_audio_status_idx`
3. `make run-dev` 启动后能完成完整流程：
   - 前置：登录 + 创建孩子（Plan 2 流程）
   - `POST /api/v1/stories/generate` → 200，返回 body 含 `audio_status:"pending"`
   - 立即 `GET /api/v1/stories/{id}/audio_url` → 200 `{"audio_status":"pending","retry_after":5}`
   - 轮询 5-15 秒后 → 200 `{"audio_status":"ready","url":"https://<bucket>.cos.ap-shanghai.myqcloud.com/...","expires_at":"<+15min>"}`
   - 把 url 粘到浏览器，能听到 mp3（孩子主角名/爱宝出现在朗读中）
4. 真实拿到 mp3 文件大小 > 0，COS 控制台能看到对象，签名 URL 在 15 分钟后失效（手动验证一次即可）
5. Worker 日志可见 `tts.synthesize.ok` + `storage.upload.ok` + `worker.mark_done`；失败路径会写 `stories.audio_status='failed'`
6. 所有权校验：用户 A 拿用户 B 的 storyId 调 `/audio_url` → 403 `not_owner`；不存在的 id → 404
7. 业务 metrics 在 `/metrics` 端点可见：`tts_call_duration_seconds` / `tts_call_total` / `storage_upload_duration_seconds` / `audio_pending_count` / `audio_ready_total` / `audio_failed_total`
8. **API Key/SecretId/SecretKey 绝不出现在 git/commit/log/test 文件**——所有引用走环境变量；`server/config/config.dev.yaml` 仅留注释占位
9. `golangci-lint run ./...` 0 issues

---

## 范围决策记录（与用户对齐）

| 维度 | 决策 |
|---|---|
| TTS 厂商 | Minimax，模型 `speech-01-turbo`（备选 `speech-02-hd`，先 turbo 够用）|
| 默认 voice_id | `female-tianmei`（甜美女声，TBD-confirm with Minimax 控制台；后续可在 child profile 加 `preferred_voice` 字段做个性化） |
| 音频参数 | mp3 / 32kHz / 128kbps / 单声道——和儿童故事场景匹配，文件最小 |
| 存储厂商 | 腾讯云 COS，region `ap-shanghai` |
| 对象 key 格式 | `audio/{child_id}/{story_id}-{ulid}.mp3`——按孩子分桶便于运维清理 |
| 签名 URL TTL | 15 分钟（够一次完整朗读+缓冲；过期后客户端再请求一次 /audio_url 即可） |
| 同步 vs 异步 | 同步只返文本，音频走 outbox+worker。理由：TTS 一段 ~10 分钟故事 8-15 秒，叠加文本 LLM 21 秒后用户已经太久；先返文本立即可读 |
| BGM/混音 | **不做**，留 Plan 5b/6（需要 ffmpeg + cue parser，复杂度独立） |
| 重试 | Worker 复用 Plan 4 指数退避；max_attempts=5；仍失败 → `audio_status='failed'` 客户端 503 |
| Outbox 载荷 | 仅 `{story_id}`——handler 从 DB 重取最新 text，避免 payload 膨胀也避免 stale 文本 |
| 鉴权 | `/audio_url` 走 JWT；不挂 rate-limit/budget 中间件——读多、零外部成本 |
| 失败兜底 | 若 Storage `GetPresignedURL` 调用失败，handler 不能 500，退回 `audio_status:"failed"` 让客户端重新发起或 fallback 到无声播放 |

---

## File Structure

### 数据迁移

| 文件 | 职责 |
|---|---|
| `server/migrations/000004_audio_status.up.sql` | 加 `stories.audio_status`、`stories.audio_failed_at` 列 + 索引 |
| `server/migrations/000004_audio_status.down.sql` | 反向 |

### 配置扩展

| 文件 | 修改 |
|---|---|
| `server/internal/pkg/config/config.go` | 新增 TTSConfig + StorageConfig 块 |
| `server/internal/pkg/config/config_test.go` | 默认值测试 |
| `server/config/config.dev.yaml` + `config.yaml.example` | dev 默认值 + 注释占位 |

### TTS Gateway

| 文件 | 职责 |
|---|---|
| `server/internal/gateway/tts/tts.go` | `Client` 接口 + 公共 types（`SynthesizeRequest/Response`、错误常量）|
| `server/internal/gateway/tts/minimax.go` | Minimax `t2a_v2` HTTP 实现 |
| `server/internal/gateway/tts/minimax_test.go` | 单元测试（用 `httptest.Server` 打桩，**不**打真 API）|
| `server/internal/gateway/tts/mock.go` | Mock 实现 |
| `server/internal/gateway/tts/mock_test.go` | mock 行为测试 |

### Storage Gateway

| 文件 | 职责 |
|---|---|
| `server/internal/gateway/storage/storage.go` | `Client` 接口（`Upload/HeadObject/Delete/GetPresignedURL`）+ 公共 types |
| `server/internal/gateway/storage/cos.go` | 腾讯云 COS 实现 |
| `server/internal/gateway/storage/cos_test.go` | 用 `httptest.Server` 打桩 cos endpoint，验证请求路径/header；签名 URL 离线测试 |
| `server/internal/gateway/storage/mock.go` | 内存 Mock（map[key][]byte）|
| `server/internal/gateway/storage/mock_test.go` | mock 行为测试 |

### Data model + Repos

| 文件 | 修改 |
|---|---|
| `server/internal/model/story.go` | Story 结构加 `AudioStatus`/`AudioFailedAt`；新增 `AudioStatus*` 三常量；新增 `EventTypeTTSSynthesis` |
| `server/internal/repository/story_repo.go` | `CreateWithOutbox` 签名变 `events []*model.OutboxEvent`；新增 `MarkAudioReady` / `MarkAudioFailed` |
| `server/internal/repository/story_repo_test.go` | 更新调用点 + 新增 MarkAudio*-* 测试 |

### Worker handler

| 文件 | 职责 |
|---|---|
| `server/internal/worker/handlers/tts_synthesis.go` | 新 handler |
| `server/internal/worker/handlers/tts_synthesis_test.go` | 测试 |

### Service / Orchestrator

| 文件 | 修改 |
|---|---|
| `server/internal/service/story/orchestrator.go` | 在事务里**同时**写 memory_update + tts_synthesis 两条 outbox；返回时 `audio_status="pending"` |
| `server/internal/service/story/orchestrator_test.go` | 调用点更新 |

### API 层

| 文件 | 职责 |
|---|---|
| `server/internal/api/audio.go` | `GET /api/v1/stories/:id/audio_url` handler + 所有权校验 |
| `server/internal/api/audio_test.go` | handler 测试 |
| `server/internal/api/router.go` | `RouterDeps.Audio` + 注册路由（JWT 组下，不挂限流/预算）|
| `server/internal/api/story.go` | 序列化时把 `audio_status` 拍到响应 body（model 已有字段，json tag 即可） |

### Metrics

| 文件 | 修改 |
|---|---|
| `server/internal/metrics/business.go` | +TTSCallDuration / TTSCallTotal / StorageUploadDuration / AudioPendingCount / AudioReadyTotal / AudioFailedTotal |
| `server/internal/metrics/business_test.go` | 用例追加 |

### 装配

| 文件 | 修改 |
|---|---|
| `server/cmd/server/main.go` | 装配 TTS client、Storage client、注册 tts_synthesis handler 到既有 Worker、构建 AudioHandler 注入 RouterDeps、env 兼容 fallback |
| `server/Makefile` | `run-dev` 透传 7 个新 env |

---

## API 形态（先定好契约）

### POST `/api/v1/stories/generate`（Plan 4 已存在，本 plan 仅扩响应）

**Response 200（变化部分加粗）：**

```json
{
  "id": 42,
  "title": "小宇和爱宝奥特曼的勇敢冒险",
  "text": "...",
  "audio_object_key": "",
  "audio_status": "pending",
  "duration_minutes": 10,
  "style": "温馨治愈",
  "topic": "勇敢",
  "created_at": "2026-05-09T10:23:45Z"
}
```

`audio_object_key` 在事务返回那一刻仍为空字符串——它是 Worker 上传成功后才回填。客户端不应依赖它，应只看 `audio_status` + 调 `/audio_url`。

### GET `/api/v1/stories/{id}/audio_url`（新）
带 Bearer JWT。

**Response 200（ready）：**

```json
{
  "audio_status": "ready",
  "url": "https://aibao-prod-1234.cos.ap-shanghai.myqcloud.com/audio/7/42-01HXY...mp3?q-sign-algorithm=...",
  "expires_at": "2026-05-09T10:38:45Z"
}
```

**Response 200（pending）：**

```json
{
  "audio_status": "pending",
  "retry_after": 5
}
```

**Response 503（failed）：**

```json
{
  "code": "audio_failed",
  "message": "音频生成失败，请稍后重新生成故事"
}
```

**错误：**
- 401 无 token
- 403 `not_owner`（story.child_id 对应的 child.user_id != ctxUserID）
- 404 `story_not_found`

---

## 数据模型字段约定（仅新增/变化）

### stories 表新增列

| 字段 | 类型 | 说明 |
|---|---|---|
| audio_status | varchar(16) NOT NULL DEFAULT 'pending' | pending / ready / failed |
| audio_failed_at | timestamptz NULL | 仅 failed 时填 |

索引：`stories_audio_status_idx ON stories(audio_status) WHERE audio_status='pending'`——Worker 不直接查这个（Worker 查 outbox），但运维想看"卡住的待合成"时这个部分索引很便宜。

### outbox_events 不变；仅新增 event_type

| event_type | payload schema |
|---|---|
| `tts_synthesis` | `{"story_id": 42}` —— **故意保持极简**，handler 重取最新 text |

🎓 **为什么这样设计 outbox 载荷？** 反直觉但重要：Plan 4 的 `memory_update` 事件载荷里塞了 `title/summary/used_fallback`，因为那些是"事件发生那一刻的快照"，记忆就该是快照。但 TTS 不一样——如果将来加了"重新生成 / 编辑故事文本"功能，事件如果带着旧 text，handler 就会合成出和数据库不一致的音频。所以 TTS 事件**只带 story_id**，handler 进来时现取最新 `stories.text_content`。这同时让单条 outbox row 永远 < 1KB，pg 的 jsonb 也开心。

---

# Tasks

## Task 0：迁移文件 `000004_audio_status`

**Files:**
- Create: `server/migrations/000004_audio_status.up.sql`
- Create: `server/migrations/000004_audio_status.down.sql`

- [ ] **Step 0.1：up SQL**

`server/migrations/000004_audio_status.up.sql`：

```sql
ALTER TABLE stories
    ADD COLUMN IF NOT EXISTS audio_status     VARCHAR(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS audio_failed_at  TIMESTAMPTZ NULL;

-- Partial index: only 'pending' rows are interesting for ops dashboards.
-- Worker itself joins via outbox_events, not via this index.
CREATE INDEX IF NOT EXISTS stories_audio_status_idx
    ON stories(audio_status)
    WHERE audio_status = 'pending';
```

- [ ] **Step 0.2：down SQL**

```sql
DROP INDEX IF EXISTS stories_audio_status_idx;
ALTER TABLE stories
    DROP COLUMN IF EXISTS audio_failed_at,
    DROP COLUMN IF EXISTS audio_status;
```

- [ ] **Step 0.3：跑迁移验证**

```bash
cd server && make migrate-up
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\d stories" | grep audio_
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\di stories_audio_status_idx"
```

Expected：两列存在；索引存在且为 partial（`Where: audio_status = 'pending'::text`）。

- [ ] **Step 0.4：commit**

```bash
git add server/migrations/000004_audio_status.up.sql \
        server/migrations/000004_audio_status.down.sql
git commit -m "feat(db): stories.audio_status + audio_failed_at columns"
```

---

## Task 1：扩展配置（TTSConfig + StorageConfig）

**Files:**
- Modify: `server/internal/pkg/config/config.go`
- Modify: `server/internal/pkg/config/config_test.go`
- Modify: `server/config/config.dev.yaml` + `config.yaml.example`

- [ ] **Step 1.1：扩展 Config 结构体**

`Config` struct 末尾追加两个字段：

```go
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	SMS      SMSConfig      `mapstructure:"sms"`
	Crypto   CryptoConfig   `mapstructure:"crypto"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	TTS      TTSConfig      `mapstructure:"tts"`     // 新增
	Storage  StorageConfig  `mapstructure:"storage"` // 新增
}

// TTSConfig holds TTS provider parameters.
type TTSConfig struct {
	Provider       string  `mapstructure:"provider"`        // "minimax" / "mock"
	BaseURL        string  `mapstructure:"base_url"`        // https://api.minimax.chat
	GroupID        string  `mapstructure:"group_id"`        // env AIBAO_TTS_MINIMAX_GROUP_ID
	APIKey         string  `mapstructure:"api_key"`         // env AIBAO_TTS_MINIMAX_API_KEY
	Model          string  `mapstructure:"model"`           // "speech-01-turbo"
	VoiceID        string  `mapstructure:"voice_id"`        // "female-tianmei"
	Format         string  `mapstructure:"format"`          // "mp3"
	SampleRate     int     `mapstructure:"sample_rate"`     // 32000
	Bitrate        int     `mapstructure:"bitrate"`         // 128000
	Speed          float64 `mapstructure:"speed"`           // 1.0
	TimeoutSeconds int     `mapstructure:"timeout_seconds"` // 60
}

// StorageConfig holds object storage provider parameters.
type StorageConfig struct {
	Provider          string `mapstructure:"provider"`            // "cos" / "mock"
	Bucket            string `mapstructure:"bucket"`              // env AIBAO_STORAGE_COS_BUCKET
	Region            string `mapstructure:"region"`              // ap-shanghai
	AppID             string `mapstructure:"app_id"`              // env AIBAO_STORAGE_COS_APPID
	SecretID          string `mapstructure:"secret_id"`           // env AIBAO_STORAGE_COS_SECRET_ID
	SecretKey         string `mapstructure:"secret_key"`          // env AIBAO_STORAGE_COS_SECRET_KEY
	PresignedTTLSec   int    `mapstructure:"presigned_ttl_seconds"` // 900 (15min)
	UploadTimeoutSec  int    `mapstructure:"upload_timeout_seconds"` // 30
}
```

`applyDefaultsAndValidate(c, path)` 末尾追加：

```go
	if c.TTS.Provider == "" {
		c.TTS.Provider = "minimax"
	}
	if c.TTS.Provider == "minimax" {
		if c.TTS.GroupID == "" {
			return fmt.Errorf("config %s: tts.group_id is required (set AIBAO_TTS_MINIMAX_GROUP_ID)", path)
		}
		if c.TTS.APIKey == "" {
			return fmt.Errorf("config %s: tts.api_key is required (set AIBAO_TTS_MINIMAX_API_KEY)", path)
		}
	}
	if c.TTS.BaseURL == "" {
		c.TTS.BaseURL = "https://api.minimax.chat"
	}
	if c.TTS.Model == "" {
		c.TTS.Model = "speech-01-turbo"
	}
	if c.TTS.VoiceID == "" {
		c.TTS.VoiceID = "female-tianmei" // TBD-confirm in Minimax console
	}
	if c.TTS.Format == "" {
		c.TTS.Format = "mp3"
	}
	if c.TTS.SampleRate == 0 {
		c.TTS.SampleRate = 32000
	}
	if c.TTS.Bitrate == 0 {
		c.TTS.Bitrate = 128000
	}
	if c.TTS.Speed == 0 {
		c.TTS.Speed = 1.0
	}
	if c.TTS.TimeoutSeconds == 0 {
		c.TTS.TimeoutSeconds = 60
	}

	if c.Storage.Provider == "" {
		c.Storage.Provider = "cos"
	}
	if c.Storage.Provider == "cos" {
		if c.Storage.Bucket == "" {
			return fmt.Errorf("config %s: storage.bucket is required (set AIBAO_STORAGE_COS_BUCKET)", path)
		}
		if c.Storage.Region == "" {
			return fmt.Errorf("config %s: storage.region is required", path)
		}
		if c.Storage.SecretID == "" {
			return fmt.Errorf("config %s: storage.secret_id is required (set AIBAO_STORAGE_COS_SECRET_ID)", path)
		}
		if c.Storage.SecretKey == "" {
			return fmt.Errorf("config %s: storage.secret_key is required (set AIBAO_STORAGE_COS_SECRET_KEY)", path)
		}
	}
	if c.Storage.PresignedTTLSec == 0 {
		c.Storage.PresignedTTLSec = 900
	}
	if c.Storage.UploadTimeoutSec == 0 {
		c.Storage.UploadTimeoutSec = 30
	}
```

`Load(path)` 中的 `binds` 列表追加：

```go
		"tts.provider", "tts.base_url", "tts.group_id", "tts.api_key",
		"tts.model", "tts.voice_id", "tts.format", "tts.sample_rate",
		"tts.bitrate", "tts.speed", "tts.timeout_seconds",
		"storage.provider", "storage.bucket", "storage.region", "storage.app_id",
		"storage.secret_id", "storage.secret_key",
		"storage.presigned_ttl_seconds", "storage.upload_timeout_seconds",
```

🎓 **env 变量名映射**：viper 默认把 `tts.api_key` 映射到 `AIBAO_TTS_API_KEY`。但我们对外宣传的是 `AIBAO_TTS_MINIMAX_API_KEY`（同 Plan 4 的 `AIBAO_LLM_DOUBAO_API_KEY` 套路——名字里塞厂商名是为了将来同时挂多个 provider 不冲突）。和 Plan 4 一样，在 main.go 里做一次 fallback：如果 `AIBAO_TTS_MINIMAX_API_KEY` / `AIBAO_TTS_MINIMAX_GROUP_ID` 非空就注入 cfg。COS 的 6 个 env 同理。

- [ ] **Step 1.2：扩 `writeValidConfig` helper + 新测试**

`config_test.go` 的 `writeValidConfig` 末尾追加：

```yaml
tts:
  provider: mock
  group_id: dev-gid
  api_key: dev-tts-key
storage:
  provider: mock
  bucket: dev-bucket
  region: ap-shanghai
  secret_id: dev-sid
  secret_key: dev-skey
```

新增测试：

```go
func TestLoad_TTSStorageDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "minimax", cfg.TTS.Provider)
	assert.Equal(t, "speech-01-turbo", cfg.TTS.Model)
	assert.Equal(t, "female-tianmei", cfg.TTS.VoiceID)
	assert.Equal(t, "mp3", cfg.TTS.Format)
	assert.Equal(t, 32000, cfg.TTS.SampleRate)
	assert.InDelta(t, 1.0, cfg.TTS.Speed, 0.001)
	assert.Equal(t, "cos", cfg.Storage.Provider)
	assert.Equal(t, 900, cfg.Storage.PresignedTTLSec)
}

func TestLoad_TTSMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// provider=minimax (default) but no group_id
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tts.group_id")
}
```

- [ ] **Step 1.3：跑测试**

```bash
cd /f/claud/aibao_app/server && go test -count=1 ./internal/pkg/config/ -v
```

- [ ] **Step 1.4：dev yaml 追加 tts/storage 段**

`server/config/config.dev.yaml` 末尾追加：

```yaml

tts:
  provider: minimax
  base_url: https://api.minimax.chat
  model: speech-01-turbo
  voice_id: female-tianmei  # TBD-confirm with Minimax console
  format: mp3
  sample_rate: 32000
  bitrate: 128000
  speed: 1.0
  timeout_seconds: 60
  # group_id: from env AIBAO_TTS_MINIMAX_GROUP_ID
  # api_key:  from env AIBAO_TTS_MINIMAX_API_KEY

storage:
  provider: cos
  region: ap-shanghai
  presigned_ttl_seconds: 900
  upload_timeout_seconds: 30
  # bucket:     from env AIBAO_STORAGE_COS_BUCKET
  # app_id:     from env AIBAO_STORAGE_COS_APPID
  # secret_id:  from env AIBAO_STORAGE_COS_SECRET_ID
  # secret_key: from env AIBAO_STORAGE_COS_SECRET_KEY
```

`server/config/config.yaml.example` 同步追加；`secret_id/secret_key/api_key` 处加注释强调 prod 必须 env 注入，**绝不能写到文件**。

- [ ] **Step 1.5：commit**

```bash
git add server/internal/pkg/config server/config
git commit -m "feat(config): tts + storage config blocks with env binding"
```

---

## Task 2：业务 Metrics 扩充

**Files:**
- Modify: `server/internal/metrics/business.go`
- Modify: `server/internal/metrics/business_test.go`

- [ ] **Step 2.1：扩 Business 结构 + NewBusiness**

在 `Business` struct 末尾追加：

```go
type Business struct {
	StoryGenerateTotal     *prometheus.CounterVec
	StoryGenerateDuration  prometheus.Histogram
	LLMCallDuration        *prometheus.HistogramVec
	LLMCallTotal           *prometheus.CounterVec
	SafetyFailTotal        *prometheus.CounterVec
	OutboxPendingCount     prometheus.Gauge
	OutboxDeadTotal        *prometheus.CounterVec
	LLMBudgetUsedYuan      prometheus.Gauge
	ExternalAPIErrorTotal  *prometheus.CounterVec

	// Plan 5
	TTSCallDuration        *prometheus.HistogramVec // labels: provider
	TTSCallTotal           *prometheus.CounterVec   // labels: provider, status
	StorageUploadDuration  *prometheus.HistogramVec // labels: provider
	AudioPendingCount      prometheus.Gauge
	AudioReadyTotal        prometheus.Counter
	AudioFailedTotal       *prometheus.CounterVec   // labels: stage (tts/storage/db)
}
```

`NewBusiness` 末尾追加注册：

```go
		TTSCallDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "tts_call_duration_seconds",
				Help:    "TTS API call duration by provider.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
			}, []string{"provider"},
		),
		TTSCallTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tts_call_total",
				Help: "Total TTS API calls by provider and status.",
			}, []string{"provider", "status"},
		),
		StorageUploadDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "storage_upload_duration_seconds",
				Help:    "Object storage upload duration by provider.",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),
			}, []string{"provider"},
		),
		AudioPendingCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audio_pending_count",
				Help: "Stories with audio_status='pending' waiting for synthesis.",
			},
		),
		AudioReadyTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "audio_ready_total",
				Help: "Stories that successfully reached audio_status='ready'.",
			},
		),
		AudioFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audio_failed_total",
				Help: "Stories that ended in audio_status='failed', labeled by failing stage.",
			}, []string{"stage"},
		),
```

记得在 `NewBusiness` 末尾的 `reg.MustRegister(...)` 调用里把 6 个新 collector 都加进去（沿用 Plan 4 的注册风格——一次性 MustRegister）。

- [ ] **Step 2.2：扩测试**

`business_test.go` 的断言列表末尾追加：

```go
		"tts_call_duration_seconds",
		"tts_call_total",
		"storage_upload_duration_seconds",
		"audio_pending_count",
		"audio_ready_total",
		"audio_failed_total",
```

并在 testing 主流程里调一次 `bm.TTSCallTotal.WithLabelValues("minimax","ok").Inc()` / `bm.AudioFailedTotal.WithLabelValues("tts").Inc()` 等，确保 vec 真的能被 vector 化。

- [ ] **Step 2.3：跑 + commit**

```bash
go test -count=1 ./internal/metrics/...
git add server/internal/metrics
git commit -m "feat(metrics): tts + storage + audio business metrics"
```

---

## Task 3：Model 扩展（Story.AudioStatus + 事件类型）

**Files:**
- Modify: `server/internal/model/story.go`

- [ ] **Step 3.1：加字段 + 常量**

在 Story struct 中插入两行（建议放在 AudioDurationSeconds 之后，DurationMinutes 之前）：

```go
	AudioStatus          string    `gorm:"column:audio_status" json:"audio_status"`
	AudioFailedAt        *time.Time `gorm:"column:audio_failed_at" json:"-"`
```

文件末尾追加：

```go
// Audio status constants for Story.AudioStatus.
const (
	AudioStatusPending = "pending"
	AudioStatusReady   = "ready"
	AudioStatusFailed  = "failed"
)
```

并在已有的 `Outbox event types` 常量块加一项：

```go
const (
	EventTypeMemoryUpdate  = "memory_update"
	EventTypeTTSSynthesis  = "tts_synthesis" // Plan 5
)
```

- [ ] **Step 3.2：build sanity**

```bash
go build ./...
```

不在这步 commit；下一 Task 修 repo 时一起提交（避免中间 commit 编译不过）。

---

## Task 4：Repo `CreateWithOutbox` 改签 + MarkAudio*

**Files:**
- Modify: `server/internal/repository/story_repo.go`
- Modify: `server/internal/repository/story_repo_test.go`
- Modify: `server/internal/service/story/orchestrator.go`（同步改调用点，Task 10 还会再动）
- Modify: `server/internal/service/story/orchestrator_test.go`

> **目的**：Plan 4 的 `CreateWithOutbox` 只接收一个事件指针；Plan 5 起需要原子写入 N 个事件（memory_update + tts_synthesis），未来还可能加更多。改签为 slice 是最直接的做法。

- [ ] **Step 4.1：改 StoryRepo 接口**

`server/internal/repository/story_repo.go`：

```go
// StoryRepo is the data-access surface for stories.
type StoryRepo interface {
	// CreateWithOutbox inserts story + elements + N outbox events in ONE transaction.
	// On success, story.ID, each element.ID, and each event.ID are populated;
	// every event.AggregateID is auto-set to story.ID if nil.
	CreateWithOutbox(ctx context.Context, story *model.Story, elements []*model.StoryElement, events []*model.OutboxEvent) error

	// FindByID returns the story with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id int64) (*model.Story, error)

	// MarkAudioReady atomically updates a story to audio_status='ready' and
	// fills audio_object_key/format/size/duration. Idempotent: if audio_status
	// is already 'ready' with same key, this is a no-op.
	MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int) error

	// MarkAudioFailed sets audio_status='failed' and stamps audio_failed_at.
	MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error
}
```

实现：

```go
func (r *storyRepo) CreateWithOutbox(
	ctx context.Context,
	story *model.Story,
	elements []*model.StoryElement,
	events []*model.OutboxEvent,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(story).Error; err != nil {
			return err
		}
		for _, e := range elements {
			e.StoryID = story.ID
			if err := tx.Create(e).Error; err != nil {
				return err
			}
		}
		for _, ev := range events {
			if ev.AggregateID == nil {
				ev.AggregateID = &story.ID
			}
			if err := tx.Create(ev).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *storyRepo) MarkAudioReady(
	ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int,
) error {
	return r.db.WithContext(ctx).
		Model(&model.Story{}).
		Where("id = ?", storyID).
		Updates(map[string]any{
			"audio_status":           model.AudioStatusReady,
			"audio_object_key":       objectKey,
			"audio_format":           format,
			"audio_size_bytes":       sizeBytes,
			"audio_duration_seconds": durationSec,
			"audio_failed_at":        nil,
		}).Error
}

func (r *storyRepo) MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.Story{}).
		Where("id = ?", storyID).
		Updates(map[string]any{
			"audio_status":    model.AudioStatusFailed,
			"audio_failed_at": now,
		}).Error
	// errMsg currently NOT persisted on stories table to keep the schema slim;
	// it's logged + emitted as a metric label upstream. If we later need it for
	// support tooling, add audio_error_message column in a later migration.
	_ = errMsg
}
```

注意 `time` import + 上面注释里的 `_ = errMsg` 写法——保留参数让接口稳定，不持久化避免 schema 过早膨胀（YAGNI）。

- [ ] **Step 4.2：改 orchestrator 接口里同名方法的契约**

`server/internal/service/story/orchestrator.go` 里的 `StoryRepo` interface（注意：编排器持有的是局部 minimal interface，而不是 repository.StoryRepo）：

```go
type StoryRepo interface {
	CreateWithOutbox(ctx context.Context, story *model.Story, elements []*model.StoryElement, events []*model.OutboxEvent) error
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}
```

并在 Generate() 里把现有的单事件传入改成 `[]*model.OutboxEvent{event}`，这一改最小改动让 build 过；Task 10 再把第二个事件加进去。

- [ ] **Step 4.3：改测试**

`story_repo_test.go`：所有 `CreateWithOutbox(ctx, s, elems, ev)` 改为 `CreateWithOutbox(ctx, s, elems, []*model.OutboxEvent{ev})`。新增两个测试：

```go
func TestStoryRepo_MarkAudioReady(t *testing.T) {
	repo, db, cleanup := setupForTest(t)
	defer cleanup()

	s := seedStory(t, db) // helper: child + story (audio_status='pending')
	require.NoError(t, repo.MarkAudioReady(context.Background(), s.ID,
		"audio/1/42-x.mp3", "mp3", 12345, 600))

	got, err := repo.FindByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, model.AudioStatusReady, got.AudioStatus)
	assert.Equal(t, "audio/1/42-x.mp3", got.AudioObjectKey)
	assert.Equal(t, int64(12345), got.AudioSizeBytes)
	assert.Equal(t, 600, got.AudioDurationSeconds)
}

func TestStoryRepo_MarkAudioFailed(t *testing.T) {
	repo, db, cleanup := setupForTest(t)
	defer cleanup()

	s := seedStory(t, db)
	require.NoError(t, repo.MarkAudioFailed(context.Background(), s.ID, "minimax 502"))

	got, err := repo.FindByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, model.AudioStatusFailed, got.AudioStatus)
	require.NotNil(t, got.AudioFailedAt)
}

func TestStoryRepo_CreateWithMultipleEvents(t *testing.T) {
	repo, _, cleanup := setupForTest(t)
	defer cleanup()

	story := &model.Story{ChildID: seedChildID(t), DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1", TextContent: "x"}
	evs := []*model.OutboxEvent{
		{EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending},
		{EventType: model.EventTypeTTSSynthesis, Payload: []byte(`{}`), Status: model.OutboxStatusPending},
	}
	require.NoError(t, repo.CreateWithOutbox(context.Background(), story, nil, evs))
	require.NotZero(t, story.ID)
	for _, e := range evs {
		require.NotZero(t, e.ID)
		require.NotNil(t, e.AggregateID)
		assert.Equal(t, story.ID, *e.AggregateID)
	}
}
```

- [ ] **Step 4.4：跑 + commit**

```bash
go build ./...
go test -count=1 -tags=integration ./internal/repository/...
go test -count=1 ./internal/service/story/...
git add server/internal/model server/internal/repository server/internal/service/story
git commit -m "feat(repo): CreateWithOutbox accepts []events; add MarkAudio{Ready,Failed}"
```

---

## Task 5：TTS Gateway 接口 + Mock

**Files:**
- Create: `server/internal/gateway/tts/tts.go`
- Create: `server/internal/gateway/tts/mock.go`
- Create: `server/internal/gateway/tts/mock_test.go`

> **目的**：和 `gateway/llm` 同构——接口在 tts.go，真实实现 minimax.go，mock 单独文件，所有上游 service 只依赖接口。

- [ ] **Step 5.1：tts.go**

```go
// Package tts is the TTS provider abstraction. Audio worker depends on this
// interface, not on any concrete provider.
package tts

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout is returned when synthesis exceeds its deadline.
var ErrTimeout = errors.New("tts call timeout")

// ErrUpstream is returned when the TTS provider returned an error.
var ErrUpstream = errors.New("tts upstream error")

// SynthesizeRequest is the structured input.
type SynthesizeRequest struct {
	Text       string  // story text to read aloud
	VoiceID    string  // provider-specific voice id
	Model      string  // e.g. "speech-01-turbo"
	Format     string  // "mp3"
	SampleRate int     // Hz, e.g. 32000
	Bitrate    int     // bps, e.g. 128000
	Speed      float64 // 0.5..2.0
}

// SynthesizeResponse holds the resulting audio bytes plus metadata.
type SynthesizeResponse struct {
	Audio           []byte        // raw mp3 (or other format) bytes
	Format          string        // echoed back
	DurationSeconds int           // best-effort, 0 if unknown
	Provider        string        // "minimax"
	Latency         time.Duration
}

// Client is the TTS provider abstraction.
type Client interface {
	Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error)
	HealthCheck(ctx context.Context) error
}
```

- [ ] **Step 5.2：mock.go**

```go
package tts

import (
	"bytes"
	"context"
	"errors"
	"time"
)

// MockClient is a deterministic in-memory TTS for tests.
// Returned audio bytes are tiny (4-byte fake header + text length marker).
type MockClient struct {
	failNext bool
	calls    int
}

// NewMock constructs a MockClient.
func NewMock() *MockClient { return &MockClient{} }

// FailNext makes the next Synthesize call return ErrUpstream.
func (m *MockClient) FailNext() { m.failNext = true }

// Calls returns how many Synthesize calls were made.
func (m *MockClient) Calls() int { return m.calls }

// Synthesize returns fake bytes proportional to text length.
func (m *MockClient) Synthesize(_ context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	m.calls++
	if m.failNext {
		m.failNext = false
		return nil, errors.New("mock: forced failure: " + ErrUpstream.Error())
	}
	if req.Text == "" {
		return nil, errors.New("tts: empty text")
	}
	var buf bytes.Buffer
	buf.WriteString("MP3 ") // fake magic
	buf.WriteString(req.Text)
	return &SynthesizeResponse{
		Audio:           buf.Bytes(),
		Format:          req.Format,
		DurationSeconds: len([]rune(req.Text)) / 4, // ~4 chars/sec heuristic
		Provider:        "mock",
		Latency:         5 * time.Millisecond,
	}, nil
}

// HealthCheck always passes for mock.
func (m *MockClient) HealthCheck(_ context.Context) error { return nil }
```

- [ ] **Step 5.3：mock_test.go**

```go
package tts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMock_HappyPath(t *testing.T) {
	m := NewMock()
	resp, err := m.Synthesize(context.Background(), SynthesizeRequest{
		Text: "小宇在森林里冒险", Format: "mp3",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Audio)
	assert.Equal(t, "mp3", resp.Format)
	assert.Equal(t, 1, m.Calls())
}

func TestMock_FailNext(t *testing.T) {
	m := NewMock()
	m.FailNext()
	_, err := m.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	// next call recovers
	_, err = m.Synthesize(context.Background(), SynthesizeRequest{Text: "y"})
	require.NoError(t, err)
}

func TestMock_EmptyText(t *testing.T) {
	m := NewMock()
	_, err := m.Synthesize(context.Background(), SynthesizeRequest{Text: ""})
	require.Error(t, err)
}
```

- [ ] **Step 5.4：跑 + commit**

```bash
go test -count=1 ./internal/gateway/tts/...
git add server/internal/gateway/tts
git commit -m "feat(gateway/tts): client interface + mock"
```

---

## Task 6：TTS Gateway Minimax 实现

**Files:**
- Create: `server/internal/gateway/tts/minimax.go`
- Create: `server/internal/gateway/tts/minimax_test.go`

> **目的**：Minimax 没有官方 Go SDK；直连 REST 即可，body/响应都是 JSON。**测试不打真 API**——用 `httptest.Server` 桩。

- [ ] **Step 6.1：minimax.go**

```go
package tts

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MinimaxConfig holds settings for the Minimax client.
type MinimaxConfig struct {
	BaseURL        string // https://api.minimax.chat
	GroupID        string // tenant id (URL query GroupId)
	APIKey         string // Bearer token
	TimeoutSeconds int
}

// MinimaxClient calls Minimax T2A v2 (text-to-audio).
type MinimaxClient struct {
	cfg     MinimaxConfig
	http    *http.Client
}

// NewMinimax constructs a MinimaxClient. Returns error if required fields missing.
func NewMinimax(cfg MinimaxConfig) (*MinimaxClient, error) {
	if cfg.GroupID == "" {
		return nil, errors.New("minimax: group id required (set AIBAO_TTS_MINIMAX_GROUP_ID)")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("minimax: api key required (set AIBAO_TTS_MINIMAX_API_KEY)")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.chat"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &MinimaxClient{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}, nil
}

// minimaxT2AReq is the request body schema for /v1/t2a_v2.
type minimaxT2AReq struct {
	Model         string                 `json:"model"`
	Text          string                 `json:"text"`
	VoiceSetting  minimaxVoiceSetting    `json:"voice_setting"`
	AudioSetting  minimaxAudioSetting    `json:"audio_setting"`
}

type minimaxVoiceSetting struct {
	VoiceID string  `json:"voice_id"`
	Speed   float64 `json:"speed"`
	Vol     float64 `json:"vol"`
	Pitch   float64 `json:"pitch"`
}

type minimaxAudioSetting struct {
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
	Channel    int    `json:"channel"`
}

// minimaxT2AResp is the (subset of) response we care about.
// Audio is hex-encoded; ExtraInfo carries the duration in milliseconds.
type minimaxT2AResp struct {
	Data struct {
		Audio string `json:"audio"` // hex-encoded mp3
		Status int   `json:"status"`
	} `json:"data"`
	ExtraInfo struct {
		AudioLength int `json:"audio_length"` // ms
	} `json:"extra_info"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// Synthesize calls /v1/t2a_v2.
func (m *MinimaxClient) Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	body := minimaxT2AReq{
		Model: req.Model,
		Text:  req.Text,
		VoiceSetting: minimaxVoiceSetting{
			VoiceID: req.VoiceID,
			Speed:   req.Speed,
			Vol:     1.0,
			Pitch:   0,
		},
		AudioSetting: minimaxAudioSetting{
			SampleRate: req.SampleRate,
			Bitrate:    req.Bitrate,
			Format:     req.Format,
			Channel:    1,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal req: %w", err)
	}

	url := fmt.Sprintf("%s/v1/t2a_v2?GroupId=%s", m.cfg.BaseURL, m.cfg.GroupID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("new req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := m.http.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: http %d: %s", ErrUpstream, resp.StatusCode, truncate(string(rb), 200))
	}

	var parsed minimaxT2AResp
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
	}
	if parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: minimax %d: %s", ErrUpstream, parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}
	if parsed.Data.Audio == "" {
		return nil, fmt.Errorf("%w: empty audio payload", ErrUpstream)
	}

	audioBytes, err := hex.DecodeString(parsed.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("%w: hex decode: %v", ErrUpstream, err)
	}

	durSec := parsed.ExtraInfo.AudioLength / 1000

	return &SynthesizeResponse{
		Audio:           audioBytes,
		Format:          req.Format,
		DurationSeconds: durSec,
		Provider:        "minimax",
		Latency:         latency,
	}, nil
}

// HealthCheck does NOT call Minimax (would burn quota); just validates config presence.
func (m *MinimaxClient) HealthCheck(_ context.Context) error {
	if m.cfg.GroupID == "" || m.cfg.APIKey == "" {
		return errors.New("minimax: not configured")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

🎓 **为什么不调真 API 在测试里**：每次 minimax 调用都扣量、有外网依赖、需要在 CI 配秘钥。我们用 `httptest.Server` 模拟 minimax 回包，**只验证我们发出的请求是对的**（URL/header/body）和**我们解析回包是对的**（hex decode、错误码映射）。真实联调留在 Task 15 的人工 smoke。

- [ ] **Step 6.2：minimax_test.go**

```go
package tts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimax_NewRequiresKeys(t *testing.T) {
	_, err := NewMinimax(MinimaxConfig{})
	require.Error(t, err)
	_, err = NewMinimax(MinimaxConfig{GroupID: "g"})
	require.Error(t, err)
}

func TestMinimax_HappyPath(t *testing.T) {
	wantBody := []string{"speech-01-turbo", "小宇", "female-tianmei", "32000", "mp3"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/t2a_v2", r.URL.Path)
		assert.Equal(t, "test-group", r.URL.Query().Get("GroupId"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		for _, w := range wantBody {
			assert.Contains(t, body, w)
		}
		fakeAudio := []byte{0xff, 0xfb, 0x90, 0x40} // a few mp3 bytes
		hexed := hex.EncodeToString(fakeAudio)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"audio": hexed, "status": 2},
			"extra_info": map[string]any{"audio_length": 12345},
			"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}))
	defer srv.Close()

	c, err := NewMinimax(MinimaxConfig{
		BaseURL: srv.URL, GroupID: "test-group", APIKey: "test-key", TimeoutSeconds: 5,
	})
	require.NoError(t, err)

	resp, err := c.Synthesize(context.Background(), SynthesizeRequest{
		Text: "小宇", Model: "speech-01-turbo", VoiceID: "female-tianmei",
		Format: "mp3", SampleRate: 32000, Bitrate: 128000, Speed: 1.0,
	})
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xfb, 0x90, 0x40}, resp.Audio)
	assert.Equal(t, 12, resp.DurationSeconds) // 12345ms / 1000
	assert.Equal(t, "minimax", resp.Provider)
}

func TestMinimax_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "tts upstream error"))
}

func TestMinimax_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"audio": ""},
			"base_resp": map[string]any{"status_code": 1004, "status_msg": "rate limit"},
		})
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1004")
}

func TestMinimax_BadHex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"audio": "not-hex!!!"},
			"base_resp": map[string]any{"status_code": 0},
		})
	}))
	defer srv.Close()
	c, _ := NewMinimax(MinimaxConfig{BaseURL: srv.URL, GroupID: "g", APIKey: "k", TimeoutSeconds: 5})
	_, err := c.Synthesize(context.Background(), SynthesizeRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hex decode")
}
```

- [ ] **Step 6.3：跑 + commit**

```bash
go test -count=1 ./internal/gateway/tts/...
golangci-lint run ./internal/gateway/tts/...
git add server/internal/gateway/tts
git commit -m "feat(gateway/tts): minimax t2a_v2 implementation"
```

---

## Task 7：Storage Gateway 接口 + Mock

**Files:**
- Create: `server/internal/gateway/storage/storage.go`
- Create: `server/internal/gateway/storage/mock.go`
- Create: `server/internal/gateway/storage/mock_test.go`

- [ ] **Step 7.1：storage.go**

```go
// Package storage is the object storage abstraction. Audio worker depends on
// this interface, not on any concrete provider.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned when an object key does not exist.
var ErrNotFound = errors.New("storage object not found")

// ErrUpstream is returned for any other provider-side error.
var ErrUpstream = errors.New("storage upstream error")

// UploadInput is the input to Upload.
type UploadInput struct {
	Key         string // e.g. "audio/7/42-01HXY...mp3"
	Body        io.Reader
	Size        int64  // bytes; >=0 advisory
	ContentType string // "audio/mpeg" for mp3
}

// ObjectMeta is what HeadObject returns.
type ObjectMeta struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	LastModified time.Time
}

// Client is the object storage abstraction.
type Client interface {
	Upload(ctx context.Context, in UploadInput) error
	HeadObject(ctx context.Context, key string) (*ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	// GetPresignedURL returns a time-limited GET URL for the object.
	// ttl=0 means use provider default.
	GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
}
```

- [ ] **Step 7.2：mock.go**

```go
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// MockClient is an in-memory storage for tests.
type MockClient struct {
	mu      sync.Mutex
	objects map[string][]byte
	failNext bool
}

// NewMock constructs a MockClient.
func NewMock() *MockClient {
	return &MockClient{objects: map[string][]byte{}}
}

// FailNext makes the next operation return ErrUpstream.
func (m *MockClient) FailNext() { m.failNext = true }

// Upload stores body bytes under key.
func (m *MockClient) Upload(_ context.Context, in UploadInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return ErrUpstream
	}
	b, err := io.ReadAll(in.Body)
	if err != nil {
		return err
	}
	m.objects[in.Key] = b
	return nil
}

// HeadObject returns size+contenttype.
func (m *MockClient) HeadObject(_ context.Context, key string) (*ObjectMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return &ObjectMeta{
		Key: key, Size: int64(len(b)), ContentType: "audio/mpeg",
		ETag: "mock", LastModified: time.Now(),
	}, nil
}

// Delete removes the object.
func (m *MockClient) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// GetPresignedURL returns a fake but parsable URL with the expiry baked in.
func (m *MockClient) GetPresignedURL(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if _, ok := m.objects[key]; !ok {
		return "", time.Time{}, ErrNotFound
	}
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	exp := time.Now().Add(ttl)
	return fmt.Sprintf("https://mock-bucket.local/%s?expires=%d", key, exp.Unix()), exp, nil
}

// Read is a test-only helper (not on Client interface).
func (m *MockClient) Read(key string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.objects[key]
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// Has is a test-only helper.
func (m *MockClient) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok
}

// _ silences "unused" lint until Plan 5b uses bytes.Buffer flows.
var _ = bytes.NewReader
```

- [ ] **Step 7.3：mock_test.go**

```go
package storage

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockStorage_RoundTrip(t *testing.T) {
	m := NewMock()
	require.NoError(t, m.Upload(context.Background(), UploadInput{
		Key: "a.mp3", Body: bytes.NewReader([]byte("hello")), Size: 5, ContentType: "audio/mpeg",
	}))
	meta, err := m.HeadObject(context.Background(), "a.mp3")
	require.NoError(t, err)
	assert.Equal(t, int64(5), meta.Size)

	url, exp, err := m.GetPresignedURL(context.Background(), "a.mp3", 10*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, url, "a.mp3")
	assert.True(t, exp.After(time.Now()))

	require.NoError(t, m.Delete(context.Background(), "a.mp3"))
	_, err = m.HeadObject(context.Background(), "a.mp3")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMockStorage_FailNext(t *testing.T) {
	m := NewMock()
	m.FailNext()
	err := m.Upload(context.Background(), UploadInput{Key: "x", Body: bytes.NewReader([]byte("y"))})
	assert.ErrorIs(t, err, ErrUpstream)
}
```

- [ ] **Step 7.4：跑 + commit**

```bash
go test -count=1 ./internal/gateway/storage/...
git add server/internal/gateway/storage/storage.go server/internal/gateway/storage/mock.go server/internal/gateway/storage/mock_test.go
git commit -m "feat(gateway/storage): client interface + in-memory mock"
```

---

## Task 8：Storage Gateway COS 实现

**Files:**
- Create: `server/internal/gateway/storage/cos.go`
- Create: `server/internal/gateway/storage/cos_test.go`

> **目的**：用 `cos-go-sdk-v5` 实现 Upload/Head/Delete/GetPresignedURL。测试用 httptest 桩 cos endpoint，验证请求路径/头部；签名 URL 由 SDK 离线生成，不需要打真 COS。

- [ ] **Step 8.1：cos.go**

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// COSConfig holds settings for the Tencent COS client.
type COSConfig struct {
	Bucket    string // e.g. "aibao-prod"
	Region    string // e.g. "ap-shanghai"
	AppID     string // numeric appid (used in BucketURL: <bucket>-<appid>.cos.<region>.myqcloud.com)
	SecretID  string
	SecretKey string
	UploadTimeout time.Duration
}

// COSClient implements Client over Tencent Cloud COS.
type COSClient struct {
	c   *cos.Client
	cfg COSConfig
}

// NewCOS constructs a COSClient. Returns error if required fields missing.
func NewCOS(cfg COSConfig) (*COSClient, error) {
	if cfg.Bucket == "" || cfg.Region == "" {
		return nil, errors.New("cos: bucket and region are required")
	}
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return nil, errors.New("cos: secret_id/secret_key required (set AIBAO_STORAGE_COS_SECRET_*)")
	}
	if cfg.UploadTimeout == 0 {
		cfg.UploadTimeout = 30 * time.Second
	}
	bucketHost := cfg.Bucket
	if cfg.AppID != "" {
		bucketHost = fmt.Sprintf("%s-%s", cfg.Bucket, cfg.AppID)
	}
	bucketURL := &url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.cos.%s.myqcloud.com", bucketHost, cfg.Region),
	}
	b := &cos.BaseURL{BucketURL: bucketURL}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID: cfg.SecretID, SecretKey: cfg.SecretKey,
		},
		Timeout: cfg.UploadTimeout,
	})
	return &COSClient{c: c, cfg: cfg}, nil
}

// Upload PUTs the object body.
func (s *COSClient) Upload(ctx context.Context, in UploadInput) error {
	opts := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: in.ContentType,
		},
	}
	if in.Size > 0 {
		opts.ObjectPutHeaderOptions.ContentLength = in.Size
	}
	_, err := s.c.Object.Put(ctx, in.Key, in.Body, opts)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return nil
}

// HeadObject inspects object metadata.
func (s *COSClient) HeadObject(ctx context.Context, key string) (*ObjectMeta, error) {
	resp, err := s.c.Object.Head(ctx, key, nil)
	if err != nil {
		// COS SDK returns a typed not-found via http 404; coerce.
		var cerr *cos.ErrorResponse
		if errors.As(err, &cerr) && cerr.Response != nil && cerr.Response.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	size := resp.ContentLength
	return &ObjectMeta{
		Key:          key,
		Size:         size,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: time.Now(), // COS Last-Modified parsing left out for brevity
	}, nil
}

// Delete removes the object.
func (s *COSClient) Delete(ctx context.Context, key string) error {
	_, err := s.c.Object.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return nil
}

// GetPresignedURL returns a presigned GET URL valid for ttl.
func (s *COSClient) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	u, err := s.c.Object.GetPresignedURL(ctx, http.MethodGet, key, s.cfg.SecretID, s.cfg.SecretKey, ttl, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: presign: %v", ErrUpstream, err)
	}
	return u.String(), time.Now().Add(ttl), nil
}
```

- [ ] **Step 8.2：cos_test.go**

```go
package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCOS_NewValidates(t *testing.T) {
	_, err := NewCOS(COSConfig{})
	require.Error(t, err)
	_, err = NewCOS(COSConfig{Bucket: "b", Region: "ap-shanghai"})
	require.Error(t, err)
}

// TestCOS_PresignURL: signing happens entirely client-side, so we can verify
// it without ever touching real COS.
func TestCOS_PresignURL(t *testing.T) {
	c, err := NewCOS(COSConfig{
		Bucket: "aibao-test", Region: "ap-shanghai",
		AppID: "1234567890", SecretID: "AKID...", SecretKey: "secret",
	})
	require.NoError(t, err)
	u, exp, err := c.GetPresignedURL(context.Background(), "audio/1/2-x.mp3", 5*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, u, "aibao-test-1234567890.cos.ap-shanghai.myqcloud.com")
	assert.Contains(t, u, "audio/1/2-x.mp3")
	assert.Contains(t, u, "q-sign-algorithm=") // COS V5 signature query
	assert.True(t, exp.After(time.Now()))
}

// TestCOS_UploadAgainstStub: point COSClient at httptest, verify PUT.
func TestCOS_UploadAgainstStub(t *testing.T) {
	t.Skip("stub-server based COS upload integration test deferred — SDK requires" +
		" host header matching the bucket; revisit when we add a docker-compose " +
		"COS-emulator (e.g. minio with COS shim) in CI.")
}
```

🎓 **为什么 Upload 用 stub 测试被 skip 掉**：cos-go-sdk-v5 严格校验请求 Host 与 BucketURL 一致；用普通 httptest 把 BucketURL 改成 127.0.0.1 以后，SDK 会照常发请求，但拦截鉴权重写时碰到 host 不匹配会出诡异错误。最干净的办法是上一个 minio + COS 兼容层（或类似 fakegcs 的 fakecos）做 docker compose；本 plan 不投入这个工作量，**真实联调留给 Task 15 的人工 smoke**——本地开发用 mock provider 切换。

- [ ] **Step 8.3：跑 + commit**

```bash
go mod tidy
go test -count=1 ./internal/gateway/storage/...
golangci-lint run ./internal/gateway/storage/...
git add server/internal/gateway/storage/cos.go server/internal/gateway/storage/cos_test.go server/go.mod server/go.sum
git commit -m "feat(gateway/storage): tencent cos implementation"
```

---

## Task 9：Worker Handler `tts_synthesis`

**Files:**
- Create: `server/internal/worker/handlers/tts_synthesis.go`
- Create: `server/internal/worker/handlers/tts_synthesis_test.go`

> **目的**：handler 收到 `{story_id}` 事件 → 重取最新故事文本 → 调 TTS → 调 Storage → MarkAudioReady。任何阶段失败 → MarkAudioFailed + 把 error 返回给 Worker（Worker 按指数退避重试，达到 max_attempts 后 outbox 进 dead 状态，但 stories 已经在 failed 状态）。

- [ ] **Step 9.1：tts_synthesis.go**

```go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
)

// TTSSynthesisHandler synthesizes audio for a story and uploads it to storage.
// Idempotent: re-running on a story already in 'ready' state will overwrite the
// object key (acceptable: bytes are deterministic given same text+voice).
type TTSSynthesisHandler struct {
	stories StoryReader      // FindByID
	repo    StoryAudioWriter // MarkAudioReady / MarkAudioFailed
	tts     tts.Client
	storage storage.Client
	cfg     TTSHandlerConfig
	bm      *metrics.Business // optional; nil ok
}

// StoryReader is the minimal read surface this handler needs.
type StoryReader interface {
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}

// StoryAudioWriter is the minimal write surface this handler needs.
type StoryAudioWriter interface {
	MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int) error
	MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error
}

// TTSHandlerConfig captures the synthesis params drawn from cfg.TTS.
type TTSHandlerConfig struct {
	Provider   string // for metric labels
	Model      string
	VoiceID    string
	Format     string
	SampleRate int
	Bitrate    int
	Speed      float64
}

// NewTTSSynthesisHandler constructs the handler.
func NewTTSSynthesisHandler(
	stories StoryReader, repo StoryAudioWriter,
	t tts.Client, s storage.Client,
	cfg TTSHandlerConfig, bm *metrics.Business,
) *TTSSynthesisHandler {
	return &TTSSynthesisHandler{stories: stories, repo: repo, tts: t, storage: s, cfg: cfg, bm: bm}
}

type ttsSynthesisPayload struct {
	StoryID int64 `json:"story_id"`
}

// Handle is the worker entry point.
func (h *TTSSynthesisHandler) Handle(ctx context.Context, e *model.OutboxEvent) error {
	lg := logger.FromCtx(ctx).With("module", "tts_handler", "event_id", e.ID)

	var p ttsSynthesisPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if p.StoryID == 0 {
		return errors.New("payload missing story_id")
	}

	story, err := h.stories.FindByID(ctx, p.StoryID)
	if err != nil {
		return fmt.Errorf("load story %d: %w", p.StoryID, err)
	}
	if story.AudioStatus == model.AudioStatusReady && story.AudioObjectKey != "" {
		// already done by an earlier handler run
		lg.Info("tts.skip.already_ready", "story_id", p.StoryID, "key", story.AudioObjectKey)
		return nil
	}

	// 1) TTS
	tStart := time.Now()
	resp, err := h.tts.Synthesize(ctx, tts.SynthesizeRequest{
		Text: story.TextContent, VoiceID: h.cfg.VoiceID, Model: h.cfg.Model,
		Format: h.cfg.Format, SampleRate: h.cfg.SampleRate, Bitrate: h.cfg.Bitrate,
		Speed: h.cfg.Speed,
	})
	if h.bm != nil {
		h.bm.TTSCallDuration.WithLabelValues(h.cfg.Provider).Observe(time.Since(tStart).Seconds())
	}
	if err != nil {
		if h.bm != nil {
			h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "fail").Inc()
			h.bm.AudioFailedTotal.WithLabelValues("tts").Inc()
		}
		// mark stories.audio_status='failed' so /audio_url returns 503
		if mErr := h.repo.MarkAudioFailed(ctx, p.StoryID, err.Error()); mErr != nil {
			lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
		}
		return fmt.Errorf("tts synthesize: %w", err)
	}
	if h.bm != nil {
		h.bm.TTSCallTotal.WithLabelValues(h.cfg.Provider, "ok").Inc()
	}
	lg.Info("tts.synthesize.ok", "story_id", p.StoryID, "bytes", len(resp.Audio), "dur_sec", resp.DurationSeconds)

	// 2) Storage upload
	key := buildObjectKey(story.ChildID, story.ID, h.cfg.Format)
	uStart := time.Now()
	err = h.storage.Upload(ctx, storage.UploadInput{
		Key: key, Body: bytes.NewReader(resp.Audio), Size: int64(len(resp.Audio)),
		ContentType: contentTypeFor(h.cfg.Format),
	})
	if h.bm != nil {
		h.bm.StorageUploadDuration.WithLabelValues("cos").Observe(time.Since(uStart).Seconds())
	}
	if err != nil {
		if h.bm != nil {
			h.bm.AudioFailedTotal.WithLabelValues("storage").Inc()
		}
		if mErr := h.repo.MarkAudioFailed(ctx, p.StoryID, err.Error()); mErr != nil {
			lg.Error("tts.mark_failed_persist_err", "err", mErr.Error())
		}
		return fmt.Errorf("storage upload: %w", err)
	}
	lg.Info("storage.upload.ok", "story_id", p.StoryID, "key", key)

	// 3) Persist
	if err := h.repo.MarkAudioReady(ctx, p.StoryID, key, h.cfg.Format, int64(len(resp.Audio)), resp.DurationSeconds); err != nil {
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

// buildObjectKey returns "audio/{child_id}/{story_id}-{ulid}.{ext}".
func buildObjectKey(childID, storyID int64, format string) string {
	id := ulid.Make().String()
	return fmt.Sprintf("audio/%d/%d-%s.%s", childID, storyID, id, format)
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

🎓 **`oklog/ulid` 是什么**：ULID = "Universally unique Lexicographically sortable Identifier"。它和 UUID 解决一样的问题（全局唯一），但额外可读性是按生成时间排序——所以同一个孩子的故事按对象 key 排序就是时间顺序，运维清理"30 天前的音频"就是一个 key prefix 比较。如果不想加新依赖，也可以 `time.Now().UnixNano()` 拼一个简单 id，但 ULID 更稳。**实施时**：检查 go.mod 有没有 `github.com/oklog/ulid/v2`，没有就 `go get github.com/oklog/ulid/v2`。

- [ ] **Step 9.2：tts_synthesis_test.go**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/model"
)

type fakeStoryRW struct {
	story        *model.Story
	readyKey     string
	readyFormat  string
	readyBytes   int64
	readyDur     int
	failedID     int64
	failedErrMsg string
	loadErr      error
}

func (f *fakeStoryRW) FindByID(_ context.Context, id int64) (*model.Story, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.story, nil
}
func (f *fakeStoryRW) MarkAudioReady(_ context.Context, id int64, key, fmtStr string, sz int64, dur int) error {
	f.readyKey, f.readyFormat, f.readyBytes, f.readyDur = key, fmtStr, sz, dur
	return nil
}
func (f *fakeStoryRW) MarkAudioFailed(_ context.Context, id int64, msg string) error {
	f.failedID, f.failedErrMsg = id, msg
	return nil
}

func mkPayload(id int64) []byte {
	b, _ := json.Marshal(map[string]any{"story_id": id})
	return b
}

func TestTTSHandler_HappyPath(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "小宇冒险"}}
	mockTTS := tts.NewMock()
	mockSt := storage.NewMock()
	h := NewTTSSynthesisHandler(rw, rw, mockTTS, mockSt, TTSHandlerConfig{
		Provider: "mock", Model: "speech-01-turbo", VoiceID: "v",
		Format: "mp3", SampleRate: 32000, Bitrate: 128000, Speed: 1.0,
	}, nil)

	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{ID: 1, Payload: mkPayload(42)}))

	assert.Contains(t, rw.readyKey, "audio/7/42-")
	assert.Equal(t, "mp3", rw.readyFormat)
	assert.True(t, rw.readyBytes > 0)
	assert.True(t, mockSt.Has(rw.readyKey))
}

func TestTTSHandler_AlreadyReady_Skip(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{
		ID: 42, ChildID: 7, TextContent: "x",
		AudioStatus: model.AudioStatusReady, AudioObjectKey: "audio/7/42-y.mp3",
	}}
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), storage.NewMock(), TTSHandlerConfig{Format: "mp3"}, nil)
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)}))
	assert.Empty(t, rw.readyKey, "should not re-upload")
}

func TestTTSHandler_TTSFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockTTS := tts.NewMock()
	mockTTS.FailNext()
	h := NewTTSSynthesisHandler(rw, rw, mockTTS, storage.NewMock(), TTSHandlerConfig{Format: "mp3"}, nil)

	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
	assert.NotEmpty(t, rw.failedErrMsg)
}

func TestTTSHandler_StorageFailure_MarksFailed(t *testing.T) {
	rw := &fakeStoryRW{story: &model.Story{ID: 42, ChildID: 7, TextContent: "x"}}
	mockSt := storage.NewMock()
	mockSt.FailNext()
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), mockSt, TTSHandlerConfig{Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(42)})
	require.Error(t, err)
	assert.Equal(t, int64(42), rw.failedID)
}

func TestTTSHandler_BadPayload(t *testing.T) {
	h := NewTTSSynthesisHandler(&fakeStoryRW{}, &fakeStoryRW{}, tts.NewMock(), storage.NewMock(), TTSHandlerConfig{Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: []byte("not json")})
	require.Error(t, err)
}

func TestTTSHandler_StoryNotFound(t *testing.T) {
	rw := &fakeStoryRW{loadErr: errors.New("not found")}
	h := NewTTSSynthesisHandler(rw, rw, tts.NewMock(), storage.NewMock(), TTSHandlerConfig{Format: "mp3"}, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: mkPayload(99)})
	require.Error(t, err)
}
```

- [ ] **Step 9.3：跑 + commit**

```bash
go test -count=1 ./internal/worker/handlers/...
golangci-lint run ./internal/worker/handlers/...
git add server/internal/worker/handlers/tts_synthesis.go \
        server/internal/worker/handlers/tts_synthesis_test.go \
        server/go.mod server/go.sum
git commit -m "feat(worker): tts_synthesis handler with idempotent re-runs"
```

---

## Task 10：Orchestrator 同时写两条事件

**Files:**
- Modify: `server/internal/service/story/orchestrator.go`
- Modify: `server/internal/service/story/orchestrator_test.go`

> **目的**：故事生成事务里现在要原子写入 memory_update + tts_synthesis 两条事件；响应里把 `audio_status` 设为 `pending`。

- [ ] **Step 10.1：修改 Generate() 末尾构事件 + 响应**

把现有的 `event := &model.OutboxEvent{...}` 段替换成下面：

```go
	memPayload, _ := json.Marshal(map[string]any{
		"story_id":      0,
		"child_id":      child.ID,
		"title":         story.Title,
		"summary":       summarize(llmText, 200),
		"used_fallback": usedFallback,
	})
	memEvent := &model.OutboxEvent{
		EventType: model.EventTypeMemoryUpdate,
		Payload:   memPayload,
		Status:    model.OutboxStatusPending,
	}

	ttsPayload, _ := json.Marshal(map[string]any{
		"story_id": 0, // patched after insert
	})
	ttsEvent := &model.OutboxEvent{
		EventType: model.EventTypeTTSSynthesis,
		Payload:   ttsPayload,
		Status:    model.OutboxStatusPending,
	}

	story.AudioStatus = model.AudioStatusPending

	if err := o.d.Stories.CreateWithOutbox(ctx, story, elements, []*model.OutboxEvent{memEvent, ttsEvent}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "story_persist_failed", "服务暂时不可用，请稍后再试")
	}

	// patch story_id back into payloads (best-effort; events already stored
	// with story_id=0 — handlers tolerate this for memory_update; for tts
	// the worker reads aggregate_id when story_id is 0).
	patched, _ := json.Marshal(map[string]any{
		"story_id":      story.ID,
		"child_id":      child.ID,
		"title":         story.Title,
		"summary":       summarize(llmText, 200),
		"used_fallback": usedFallback,
	})
	memEvent.Payload = patched

	patchedTTS, _ := json.Marshal(map[string]any{"story_id": story.ID})
	ttsEvent.Payload = patchedTTS
```

🎓 **关于 story_id=0 的小尴尬**：和 Plan 4 一样，事件在 INSERT 时还不知道 story.ID。Plan 4 的 memory_update handler 现在**没有依赖** payload.story_id（只用 child_id 写 memories），所以 0 没事。Plan 5 的 tts_synthesis handler 是**必须**要 story_id 的——为了不被这个 0 卡住，**两个解法二选一**：

(a) 让 handler 优先看 `outbox_events.aggregate_id`（事务里已经回填了 story.ID），payload 仅作冗余备查；
(b) 在事务里事先 reserve story.ID（例如 `SELECT nextval('stories_id_seq')`），把 ID 提前塞进 payload。

**采用 (a)**——更简单，不依赖具体 sequence。修改 Task 9 的 handler：先 fall back to aggregate_id：

```go
	storyID := p.StoryID
	if storyID == 0 && e.AggregateID != nil {
		storyID = *e.AggregateID
	}
	if storyID == 0 {
		return errors.New("payload missing story_id and event missing aggregate_id")
	}
```

**重要**：实施 Task 10 时**回头同步改 Task 9 的 handler 代码 + 测试**，确保从 `e.AggregateID` 也能拿到 storyID。把这写入 Task 9 的 commit 或单独 fix-up commit 都行。

- [ ] **Step 10.2：测试更新**

`orchestrator_test.go` 里的 `fakeStoryRepo.CreateWithOutbox` mock 改成接 `events []*model.OutboxEvent`，断言 `len(events) == 2` 且类型分别是 `memory_update` 和 `tts_synthesis`：

```go
func TestOrchestrator_EmitsBothEvents(t *testing.T) {
	repo := &fakeStoryRepo{}
	// ... wire up Orchestrator with mocks ...
	_, err := orch.Generate(ctx, validParams)
	require.NoError(t, err)
	require.Len(t, repo.lastEvents, 2)
	types := []string{repo.lastEvents[0].EventType, repo.lastEvents[1].EventType}
	assert.Contains(t, types, model.EventTypeMemoryUpdate)
	assert.Contains(t, types, model.EventTypeTTSSynthesis)
}

func TestOrchestrator_StoryHasPendingAudio(t *testing.T) {
	// ...
	got, _ := orch.Generate(ctx, validParams)
	assert.Equal(t, model.AudioStatusPending, got.AudioStatus)
}
```

- [ ] **Step 10.3：跑 + commit**

```bash
go test -count=1 ./internal/service/story/... ./internal/worker/handlers/...
golangci-lint run ./internal/service/story/... ./internal/worker/handlers/...
git add server/internal/service/story server/internal/worker/handlers
git commit -m "feat(orchestrator): emit memory_update + tts_synthesis events; story.audio_status=pending"
```

---

## Task 11：Audio 接口 `GET /stories/:id/audio_url`

**Files:**
- Create: `server/internal/api/audio.go`
- Create: `server/internal/api/audio_test.go`

> **目的**：客户端轮询的接口。三态分支：ready 现签 URL；pending 返回 retry_after；failed 返 503。所有权校验复用既有 ChildRepo。

- [ ] **Step 11.1：audio.go**

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/userctx"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

// AudioStoryReader is the minimal story-read surface this handler needs.
type AudioStoryReader interface {
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}

// AudioChildReader is the minimal child-read surface this handler needs.
type AudioChildReader interface {
	FindByID(ctx context.Context, id int64) (*model.Child, error)
}

// AudioHandler serves GET /stories/:id/audio_url.
type AudioHandler struct {
	stories AudioStoryReader
	children AudioChildReader
	storage storage.Client
	ttl     time.Duration
}

// NewAudioHandler constructs the handler.
func NewAudioHandler(s AudioStoryReader, c AudioChildReader, st storage.Client, ttl time.Duration) *AudioHandler {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &AudioHandler{stories: s, children: c, storage: st, ttl: ttl}
}

// RegisterRoutes hooks the handler into a gin group.
func (h *AudioHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stories/:id/audio_url", h.GetAudioURL)
}

// audioURLResponse is the success body.
type audioURLResponse struct {
	AudioStatus string    `json:"audio_status"`
	URL         string    `json:"url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	RetryAfter  int       `json:"retry_after,omitempty"`
}

// GetAudioURL is the gin handler.
func (h *AudioHandler) GetAudioURL(c *gin.Context) {
	storyID, err := parseInt64Param(c, "id")
	if err != nil {
		RespondError(c, apperr.New(apperr.CodeInvalidArgument, "invalid_id", "故事 ID 非法"))
		return
	}

	uid, ok := userctx.UserID(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthenticated", "请先登录"))
		return
	}

	story, err := h.stories.FindByID(c.Request.Context(), storyID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			RespondError(c, apperr.New(apperr.CodeNotFound, "story_not_found", "故事不存在"))
			return
		}
		RespondError(c, apperr.Wrap(err, apperr.CodeInternal, "story_load_failed", "服务暂时不可用"))
		return
	}

	child, err := h.children.FindByID(c.Request.Context(), story.ChildID)
	if err != nil || child.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权访问该故事"))
		return
	}

	switch story.AudioStatus {
	case model.AudioStatusReady:
		if story.AudioObjectKey == "" {
			// shouldn't happen, but be defensive
			c.JSON(http.StatusOK, audioURLResponse{AudioStatus: model.AudioStatusPending, RetryAfter: 5})
			return
		}
		url, exp, err := h.storage.GetPresignedURL(c.Request.Context(), story.AudioObjectKey, h.ttl)
		if err != nil {
			// fail-open: degrade to 'failed' on the wire — client will retry-or-fallback,
			// but we must never 500 on a read endpoint.
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "audio_failed",
				"message": "音频生成失败，请稍后重新生成故事",
			})
			return
		}
		c.JSON(http.StatusOK, audioURLResponse{
			AudioStatus: model.AudioStatusReady,
			URL:         url,
			ExpiresAt:   exp,
		})
	case model.AudioStatusFailed:
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    "audio_failed",
			"message": "音频生成失败，请稍后重新生成故事",
		})
	default: // pending
		c.JSON(http.StatusOK, audioURLResponse{
			AudioStatus: model.AudioStatusPending,
			RetryAfter:  5,
		})
	}
}
```

🎓 **`fail-open` 是什么意思**：网关或读接口失败时，**不要把内部错误抛给用户**——尤其当故障是"我们能恢复的"。这里 `GetPresignedURL` 失败可能是 cos sdk 临时报错或 secret 配错，但用户 GET /audio_url 的语义只是"问一下音频好了吗"，没必要 500。我们退回到客户端能处理的状态（`failed`），客户端可以重新发起或不播放音频。这种"读接口永不 500"的纪律对前端体验帮助巨大。

- [ ] **Step 11.2：audio_test.go**

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/userctx"
)

type stubStoryReader struct {
	s   *model.Story
	err error
}

func (r *stubStoryReader) FindByID(_ context.Context, _ int64) (*model.Story, error) {
	return r.s, r.err
}

type stubChildReader struct {
	c   *model.Child
	err error
}

func (r *stubChildReader) FindByID(_ context.Context, _ int64) (*model.Child, error) {
	return r.c, r.err
}

func mkRouter(h *AudioHandler, uid int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), uid))
		c.Next()
	})
	g := r.Group("/api/v1")
	h.RegisterRoutes(g)
	return r
}

func TestAudioURL_Pending(t *testing.T) {
	st := storage.NewMock()
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusPending}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		st, 15*time.Minute,
	)
	r := mkRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var body audioURLResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "pending", body.AudioStatus)
	assert.Equal(t, 5, body.RetryAfter)
}

func TestAudioURL_Ready(t *testing.T) {
	st := storage.NewMock()
	require.NoError(t, st.Upload(context.Background(), storage.UploadInput{
		Key: "audio/7/1-x.mp3", Body: bytesReader("xxx"), Size: 3,
	}))
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{
			ID: 1, ChildID: 7, AudioStatus: model.AudioStatusReady, AudioObjectKey: "audio/7/1-x.mp3",
		}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		st, 15*time.Minute,
	)
	r := mkRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var body audioURLResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ready", body.AudioStatus)
	assert.Contains(t, body.URL, "audio/7/1-x.mp3")
}

func TestAudioURL_Failed(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusFailed}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		storage.NewMock(), 15*time.Minute,
	)
	r := mkRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 503, w.Code)
}

func TestAudioURL_NotOwner(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusReady, AudioObjectKey: "k"}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 1234}}, // owned by someone else
		storage.NewMock(), 15*time.Minute,
	)
	r := mkRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 403, w.Code)
}

// helpers...
func bytesReader(s string) interface{ Read([]byte) (int, error) } {
	// minimal io.Reader from string
	return &stringReader{s: s}
}

type stringReader struct{ s string; pos int }
func (r *stringReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.pos:])
	r.pos += n
	return n, nil
}
```

记得 `import "io"`。

- [ ] **Step 11.3：跑 + commit**

```bash
go test -count=1 ./internal/api/...
golangci-lint run ./internal/api/...
git add server/internal/api/audio.go server/internal/api/audio_test.go
git commit -m "feat(api): GET /stories/:id/audio_url with three-state response"
```

---

## Task 12：Router 注入 AudioHandler

**Files:**
- Modify: `server/internal/api/router.go`

- [ ] **Step 12.1：扩 RouterDeps + 注册路由**

```go
type RouterDeps struct {
	// ...existing...
	Story        *StoryHandler
	GenRateLimit gin.HandlerFunc
	BudgetGuard  gin.HandlerFunc

	// Plan 5
	Audio *AudioHandler
}
```

在 `NewRouter` 的 `if deps.JWT != nil { auth := r.Group(...); ... }` 块里追加：

```go
		if deps.Audio != nil {
			deps.Audio.RegisterRoutes(auth) // 不挂 GenRateLimit/BudgetGuard：读接口
		}
```

- [ ] **Step 12.2：build + commit（联同 Task 13 一起 commit）**

下一 Task main.go 装配；不在此处单独 commit，Task 13 一起。

---

## Task 13：main.go 装配

**Files:**
- Modify: `server/cmd/server/main.go`

> **目的**：装配 TTS Client、Storage Client、TTS handler，并注册到 Plan 4 已经在跑的 Worker。env fallback 同 Plan 4 套路。

- [ ] **Step 13.1：env fallback**

在已有的 LLM env fallback 之后追加：

```go
	if cfg.TTS.GroupID == "" {
		if v := os.Getenv("AIBAO_TTS_MINIMAX_GROUP_ID"); v != "" {
			cfg.TTS.GroupID = v
		}
	}
	if cfg.TTS.APIKey == "" {
		if v := os.Getenv("AIBAO_TTS_MINIMAX_API_KEY"); v != "" {
			cfg.TTS.APIKey = v
		}
	}
	if cfg.Storage.SecretID == "" {
		if v := os.Getenv("AIBAO_STORAGE_COS_SECRET_ID"); v != "" {
			cfg.Storage.SecretID = v
		}
	}
	if cfg.Storage.SecretKey == "" {
		if v := os.Getenv("AIBAO_STORAGE_COS_SECRET_KEY"); v != "" {
			cfg.Storage.SecretKey = v
		}
	}
	if cfg.Storage.Bucket == "" {
		if v := os.Getenv("AIBAO_STORAGE_COS_BUCKET"); v != "" {
			cfg.Storage.Bucket = v
		}
	}
	if cfg.Storage.Region == "" {
		if v := os.Getenv("AIBAO_STORAGE_COS_REGION"); v != "" {
			cfg.Storage.Region = v
		}
	}
	if cfg.Storage.AppID == "" {
		if v := os.Getenv("AIBAO_STORAGE_COS_APPID"); v != "" {
			cfg.Storage.AppID = v
		}
	}
```

- [ ] **Step 13.2：装配 TTS / Storage client**

```go
	// TTS client
	var ttsClient tts.Client
	switch cfg.TTS.Provider {
	case "minimax":
		ttsClient, err = tts.NewMinimax(tts.MinimaxConfig{
			BaseURL:        cfg.TTS.BaseURL,
			GroupID:        cfg.TTS.GroupID,
			APIKey:         cfg.TTS.APIKey,
			TimeoutSeconds: cfg.TTS.TimeoutSeconds,
		})
		if err != nil {
			return fmt.Errorf("init minimax tts: %w", err)
		}
	case "mock":
		ttsClient = tts.NewMock()
	default:
		return fmt.Errorf("unknown tts provider: %s", cfg.TTS.Provider)
	}

	// Storage client
	var storageClient storage.Client
	switch cfg.Storage.Provider {
	case "cos":
		storageClient, err = storage.NewCOS(storage.COSConfig{
			Bucket:        cfg.Storage.Bucket,
			Region:        cfg.Storage.Region,
			AppID:         cfg.Storage.AppID,
			SecretID:      cfg.Storage.SecretID,
			SecretKey:     cfg.Storage.SecretKey,
			UploadTimeout: time.Duration(cfg.Storage.UploadTimeoutSec) * time.Second,
		})
		if err != nil {
			return fmt.Errorf("init cos: %w", err)
		}
	case "mock":
		storageClient = storage.NewMock()
	default:
		return fmt.Errorf("unknown storage provider: %s", cfg.Storage.Provider)
	}
```

- [ ] **Step 13.3：注册 tts_synthesis handler 到既有 Worker**

在 Plan 4 已存在的 `if cfg.Worker.Enabled { w := worker.New(...); w.Register(model.EventTypeMemoryUpdate, ...); go w.Run(ctx) }` 块里，把 TTS handler 注册**插在 `go w.Run(ctx)` 之前**：

```go
		w.Register(model.EventTypeTTSSynthesis, handlers.NewTTSSynthesisHandler(
			storyRepo, // implements StoryReader (FindByID)
			storyRepo, // implements StoryAudioWriter (MarkAudio*)
			ttsClient, storageClient,
			handlers.TTSHandlerConfig{
				Provider:   cfg.TTS.Provider,
				Model:      cfg.TTS.Model,
				VoiceID:    cfg.TTS.VoiceID,
				Format:     cfg.TTS.Format,
				SampleRate: cfg.TTS.SampleRate,
				Bitrate:    cfg.TTS.Bitrate,
				Speed:      cfg.TTS.Speed,
			},
			bm, // metrics.Business pointer (already created in Plan 4)
		))
```

- [ ] **Step 13.4：构建 AudioHandler 注入 RouterDeps**

```go
	audioHandler := api.NewAudioHandler(
		storyRepo, childRepo, storageClient,
		time.Duration(cfg.Storage.PresignedTTLSec)*time.Second,
	)

	router := api.NewRouter(api.RouterDeps{
		// ...existing...
		Story:        storyHandler,
		GenRateLimit: genRateLimit,
		BudgetGuard:  budgetGuard,
		Audio:        audioHandler, // 新
	})
```

- [ ] **Step 13.5：build clean + commit**

```bash
go build ./...
golangci-lint run ./...
git add server/cmd/server/main.go server/internal/api/router.go
git commit -m "feat(server): wire tts/storage clients + tts_synthesis handler + audio routes"
```

---

## Task 14：Makefile 透传 7 个 env

**Files:**
- Modify: `server/Makefile`

- [ ] **Step 14.1：扩 run-dev**

```makefile
run-dev: build
	AIBAO_CONFIG=$(CONFIG_DEV) \
	AIBAO_POSTGRES_PASSWORD=aibao \
	AIBAO_AUTH_JWT_SECRET=dev-jwt-secret-change-me \
	AIBAO_CRYPTO_PHONE_AES_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
	AIBAO_CRYPTO_SAFEHASH_SALT=dev-safehash-salt \
	AIBAO_LLM_DOUBAO_API_KEY=$$AIBAO_LLM_DOUBAO_API_KEY \
	AIBAO_TTS_MINIMAX_GROUP_ID=$$AIBAO_TTS_MINIMAX_GROUP_ID \
	AIBAO_TTS_MINIMAX_API_KEY=$$AIBAO_TTS_MINIMAX_API_KEY \
	AIBAO_STORAGE_COS_SECRET_ID=$$AIBAO_STORAGE_COS_SECRET_ID \
	AIBAO_STORAGE_COS_SECRET_KEY=$$AIBAO_STORAGE_COS_SECRET_KEY \
	AIBAO_STORAGE_COS_BUCKET=$$AIBAO_STORAGE_COS_BUCKET \
	AIBAO_STORAGE_COS_REGION=$$AIBAO_STORAGE_COS_REGION \
	AIBAO_STORAGE_COS_APPID=$$AIBAO_STORAGE_COS_APPID \
	./$(BINARY)
```

- [ ] **Step 14.2：commit**

```bash
git add server/Makefile
git commit -m "chore(make): pass-through minimax + cos env to run-dev"
```

---

## Task 15：端到端 smoke

> 这一节人工执行，输出贴到 devlog。**所有命令在 PowerShell 里跑**——包含中文 prompt 时**用 UTF-8 字节体**而不是字符串字面量（教训见 [docs/knowledge/06-testing.md 6.11]）。

- [ ] **Step 15.1：导出 7 个 env 后启动**

```powershell
$env:AIBAO_TTS_MINIMAX_GROUP_ID = "<你的 group id>"
$env:AIBAO_TTS_MINIMAX_API_KEY  = "<你的 api key>"
$env:AIBAO_STORAGE_COS_SECRET_ID  = "<你的 secret id>"
$env:AIBAO_STORAGE_COS_SECRET_KEY = "<你的 secret key>"
$env:AIBAO_STORAGE_COS_BUCKET     = "<bucket name without -appid>"
$env:AIBAO_STORAGE_COS_REGION     = "ap-shanghai"
$env:AIBAO_STORAGE_COS_APPID      = "<appid 数字串>"
$env:AIBAO_LLM_DOUBAO_API_KEY     = "<plan 4 的豆包 key，也要带>"

cd f:\claud\aibao_app\server
make migrate-up
make run-dev
```

- [ ] **Step 15.2：登录 + 创建孩子（同 Plan 4 Task 20.2）**

省略复制——见 Plan 4 同名 task；得到 `$TOKEN` 与 child_id=1。

- [ ] **Step 15.3：生成故事 + 看 audio_status**

```powershell
$body = [System.Text.Encoding]::UTF8.GetBytes('{"child_id":1,"prompt":"讲个奥特曼睡前故事","duration":10,"style":"温馨治愈","topic":"勇敢"}')
$resp = curl --noproxy "*" -s -X POST http://localhost:8080/api/v1/stories/generate `
  -H "Authorization: Bearer $env:TOKEN" -H "Content-Type: application/json" `
  --data-binary "@-" <<< $body
$resp | ConvertFrom-Json
```

Expected 200，`.audio_status == "pending"`，`.id` 例如 1。记下 `STORY_ID=$resp.id`。

- [ ] **Step 15.4：立即查 audio_url（应 pending）**

```powershell
curl --noproxy "*" -s -H "Authorization: Bearer $env:TOKEN" `
  http://localhost:8080/api/v1/stories/$STORY_ID/audio_url
```

Expected：`{"audio_status":"pending","retry_after":5}`。

- [ ] **Step 15.5：5-15 秒后再查（应 ready）**

```powershell
Start-Sleep -Seconds 15
curl --noproxy "*" -s -H "Authorization: Bearer $env:TOKEN" `
  http://localhost:8080/api/v1/stories/$STORY_ID/audio_url
```

Expected：`{"audio_status":"ready","url":"https://...","expires_at":"..."}`。
**把 url 粘到浏览器** → 应该能听到 mp3。要求：
- 主角名（孩子昵称）多次出现
- "爱宝奥特曼" 词出现
- 没有"血腥/鬼/死"等红线词
- 音质清晰，播放时长接近 8-12 分钟（duration 10）

- [ ] **Step 15.6：所有权 / not found 边界**

```powershell
# 用别人的 storyId（先用第二个用户登录拿 token2，再访问第一个用户的 story）
curl --noproxy "*" -s -i -H "Authorization: Bearer $env:TOKEN2" `
  http://localhost:8080/api/v1/stories/$STORY_ID/audio_url | Select-Object -First 1
# Expect: HTTP/1.1 403

# 不存在的 id
curl --noproxy "*" -s -i -H "Authorization: Bearer $env:TOKEN" `
  http://localhost:8080/api/v1/stories/999999/audio_url | Select-Object -First 1
# Expect: HTTP/1.1 404
```

- [ ] **Step 15.7：Outbox + DB 状态**

```powershell
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "SELECT id, child_id, audio_status, audio_object_key, audio_size_bytes, audio_duration_seconds FROM stories WHERE id=$STORY_ID;"
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "SELECT id, event_type, status, attempts FROM outbox_events WHERE aggregate_id=$STORY_ID ORDER BY id;"
```

Expected：
- stories 行 audio_status='ready'，audio_object_key 形如 `audio/1/1-01HXY...mp3`，audio_size_bytes > 0
- outbox_events 两行（memory_update + tts_synthesis）都 status='done'

- [ ] **Step 15.8：metrics**

```powershell
curl --noproxy "*" -s http://localhost:8080/metrics | Select-String -Pattern "^(tts_call|storage_upload|audio_)"
```

Expected：能看到 `tts_call_duration_seconds_*`、`tts_call_total{provider="minimax",status="ok"}`、`storage_upload_duration_seconds_*`、`audio_ready_total`、`audio_pending_count`、`audio_failed_total{*}`。

- [ ] **Step 15.9：写 devlog**

`docs/devlog/2026-05-XX-plan-05.md`，记录：commit 数 / 测试数 / 覆盖率 / 6 段冒烟实际输出 / 已知问题 / TBD-confirm 状态（voice_id 是否已经在 Minimax 控制台核对）。

---

## Task 16：Devlog + CLAUDE/MEMORY 同步

- [ ] **Step 16.1：更新 CLAUDE.md**

把第 2 章 "当前阶段" 改为 Plan 5 完成；"已落地的能力" 加一条 "TTS（Minimax）+ COS + 异步音频管线"；"端到端可演示接口" 加 `GET /api/v1/stories/:id/audio_url`。

- [ ] **Step 16.2：更新 MEMORY.md**

新增 Plan 5 完成的决策摘要：选 Minimax + COS 的理由、voice_id 选择、签名 URL TTL = 15 分钟、handler 用 `e.AggregateID` fallback story_id 的小坑、payload 极简的设计原则。

- [ ] **Step 16.3：commit**

```bash
git add docs/devlog/ CLAUDE.md MEMORY.md
git commit -m "docs: Plan 5 complete — tts + storage + audio pipeline"
```

---

## 完成验收清单

- [ ] `go build ./...` 通过
- [ ] `make test` 全部通过
- [ ] `make test-integration` 全部通过（Docker 必需）
- [ ] `make lint` 0 issues
- [ ] 新增 service+gateway 覆盖率 ≥ 70%
- [ ] migration 通过 `make migrate-up` 添加 audio_status / audio_failed_at 列
- [ ] `make run-dev` 启动后 15.x 全部冒烟通过
- [ ] 至少跑通 1 次真实 Minimax 合成 + COS 上传 + 浏览器播放
- [ ] outbox_events 中 tts_synthesis 1-15 秒内 pending → done
- [ ] stories.audio_status 1-15 秒内 pending → ready
- [ ] **API Key / SecretId / SecretKey 不在 git/log/test 任何位置出现明文**
- [ ] voice_id 已在 Minimax 控制台核对（解除 TBD-confirm）

---

## 后续 Plan 衔接

Plan 5b/6 起会用到：
- `gateway/storage` —— 也用作头像/封面图等其它 binary
- `gateway/tts` —— 接入 BGM 混音前需要的 cue 语义参数（`<break time="0.5s">` 等）
- Worker —— Plan 6 加 `storyline_update` / `episode_link` event_type
- `model.Story.AudioStatus` —— Plan 5b BGM 混音可能新增 `mixing` 中间状态

下一份 plan（Plan 5b：BGM 混音 + cue 解析 + ffmpeg 集成）会引入：
- `service/audio.MixOrchestrator`（ffmpeg-go 调度）
- `safety/cues/*.json` BGM 库
- Worker 新事件 `audio_mixdown`

或者 Plan 6（Storyline + 多集叙事）：
- 跨故事的角色/地点/物品复用
- `active_storylines` 表 + storyline_id 外键回填
