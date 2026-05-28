# Plan 11 — AI 大纲预览 + 成本可观测 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把"用户填表手选"的故事生成 UX 替换为"AI 推断 + 大纲卡预览-确认"两阶段（11A），同时上线成本可观测基础设施（11B），为未来商业化定价提供数据支撑。

**Architecture:** 11A 新增 `service/outline/`（独立编排）+ `service/outlinecontract/`（中立接口包，解耦 story 与 outline）+ `outline_events` append-only PG 表（生命周期）+ Redis 5min TTL 票据（payload）。11B Thin Slice 必须与 11A 同 sprint 上线（否则 outline 成本数据无法补齐）—— `pkg/cost` PriceBook + Calculator、`pkg/idhash` HMAC、`cost_events` 表、`service/cost/Recorder` 异步 Flusher、Gateway 暴露 Usage（**不调 Recorder**，业务方调）。Gateway → service 反向依赖在 CI 编译期 enforce。Full Build（aggregator + CLI + 对账）后跟 1-2 周。

**Tech Stack:** Go 1.23 + Gin + GORM + golang-migrate + Redis + Prometheus client + viper + slog；豆包 1.5-lite-32k（大纲 LLM，response_format=json）+ doubao-pro-32k（正文）+ Minimax t2a-v2 TTS + 腾讯云 COS；Flutter 3.29.3 + Riverpod 2.6.1 + dio + go_router + just_audio。

**Spec 锚点：**
- 主：`docs/superpowers/specs/2026-05-25-plan-11a-ai-outline-preview.md`
- 主：`docs/superpowers/specs/2026-05-25-plan-11b-cost-observability.md`
- 辅：`docs/superpowers/specs/2026-04-28-aibao-tech-architecture.md`（§3.5 分层强约束）

**编排约束（spec 已明示）：**
1. **Thin Slice 同 sprint**：Task 1-18 必须一起上线；不允许 11A handler 早于 11B Recorder 上线（会丢 outline 成本数据，永远补不回）
2. **Full Build 后跟**：Task 31-35 在 Thin Slice 灰度稳定 1 周后开始
3. **后端先于 Flutter**：Task 27-30 在后端冒烟通过后进
4. **Flutter feature_flag.outline_enabled 控灰度**：上线第 1 天 flag=false 后端先跑，第 2 天朋友试用打开

---

## 文件结构（落地决策）

### 新增文件

```
server/internal/pkg/
├── idhash/
│   ├── idhash.go              HMAC-SHA256 截断 12 hex + domain separation
│   └── idhash_test.go
└── cost/
    ├── pricebook.go           PriceBookKey / PriceEntry / PriceBook interface
    ├── pricebook_yaml.go      从 config.yaml 加载（不支持 hot-reload）
    ├── calculator.go          Calc(key, usage) → yuan + snapshot
    └── calculator_test.go

server/internal/service/
├── outlinecontract/           中立合约包（无实现、无 IO、仅接口 + DTO + errors）
│   ├── resolver.go            OutlineResolver interface + Outline DTO
│   └── errors.go              ErrOutlineExpired / Forbidden / NotFound
├── outline/                   实现方
│   ├── service.go             Preview(ctx, in) 主编排
│   ├── llm_prompt.go          大纲 LLM prompt 模板（含 prompt_version 常量）
│   ├── llm_parser.go          response_format=json 解析 + enum 校验 + repair retry
│   ├── safety_check.go        OutlineSafetyCheck（红线 / 害怕 / 主角 / IP）
│   ├── cache.go               Redis SET/GET 包装（payload 含 ownership 字段）
│   ├── events.go              outline_events 表读写（append-only）
│   ├── housekeeping.go        过期扫描（主动 + 兜底）
│   ├── resolver_impl.go       实现 outlinecontract.OutlineResolver
│   └── service_test.go
└── cost/
    ├── recorder.go            Record(ctx, evt) 同步 Prometheus + 异步入队
    ├── flusher.go             后台 goroutine 批量 INSERT cost_events
    ├── aggregator.go          [Full Build] 按 user/day/purpose 聚合查询
    ├── report.go              [Full Build] CLI 渲染逻辑
    └── recorder_test.go

server/internal/api/
└── outline.go                 POST /outlines/preview + /outlines/:id/refresh handler

server/internal/model/
├── outline_event.go           GORM model
└── cost_event.go              GORM model

server/migrations/
├── 000008_outline_events.up.sql / down.sql
├── 000009_cost_events.up.sql / down.sql
└── 000010_cost_events_indexes.up.sql / down.sql    # 拆开方便 rollback

server/cmd/
└── cost-report/
    └── main.go                [Full Build] CLI 入口

server/config/
└── config.yaml.example         补 cost.price_book + outline.llm 段
```

### 修改文件

```
server/internal/gateway/llm/llm.go         GenerateResponse 已含 InputTokens/OutputTokens；新增 Provider/Model 完整 + 加 Usage 子结构（封装现有字段，无破坏）
server/internal/gateway/tts/tts.go         SynthesizeResponse 新增 CharCount 字段
server/internal/gateway/storage/storage.go Upload 改返回 (bytesUploaded int64, err error)
server/internal/service/story/orchestrator.go   注入 OutlineResolver；新增 step 0 HydrateFromOutline；ResolveOutline 走 outlinecontract；所有 LLM/TTS/storage 调用点改为拿 Usage 调 Recorder
server/internal/api/story.go               /stories/generate handler：新增 outline_id 字段；storyline_id/outline_id 互斥校验
server/internal/api/middleware/ratelimit.go  新加 outline 端点共享桶 5/min
server/internal/router.go                  注册新 /outlines/* 路由 + handler
server/config/config.yaml.example          见上
.github/workflows/...  或  Makefile        加 deps-lint check（gateway 不依赖 service）

app/lib/api/api_client.dart                新增 previewOutline / refreshOutline 方法
app/lib/screens/generate_screen.dart       重做（极简版：prompt + duration + "让爱宝想想"）
app/lib/screens/outline_screen.dart        新增（大纲卡 + 倒计时 + 三按钮）
app/lib/providers/outline_provider.dart    新增（previewProvider + currentOutlineProvider）
app/lib/router.dart                        新增 outline 路由
app/lib/feature_flags.dart                 新增 outline_enabled flag
```

---

## Sprint A — 11B Thin Slice 基础设施（Task 1-10）

> 必须先于 11A handler 上线。这一段做完后 cost_events 已经在记账，outline_events 表已就位。

### Task 1: PriceBook 单价校对（先做）

**Files:**
- Modify: `server/config/config.yaml.example`
- Create: `docs/superpowers/specs/2026-05-25-plan-11b-pricebook-校对.md`（记录校对依据 + 链接 + 截图）

- [ ] **Step 1: 拉豆包官方计费**

打开 https://console.volcengine.com/ark/region:ark+cn-beijing/openManagement → 复制 doubao-pro-32k / doubao-1.5-lite-32k 的 input/output 单价（元/百万 tokens）。

- [ ] **Step 2: 拉 Minimax 官方计费**

打开 https://www.minimaxi.com/document/price → 复制 t2a-v2 单价（按字数 or 按秒）。

- [ ] **Step 3: 拉腾讯云 COS 计费**

打开 https://buy.cloud.tencent.com/price/cos → 复制 ap-hongkong 标准存储 PUT/GET/带宽单价。

- [ ] **Step 4: 写入 config.yaml.example**

```yaml
cost:
  price_book_version: v20260525-1
  pricing_source: |
    doubao: 火山引擎控制台 2026-05-25 截图
    minimax: minimaxi.com/document/price 2026-05-25
    cos: buy.cloud.tencent.com/price/cos 2026-05-25
  entries:
    - provider: doubao
      model: doubao-1.5-pro-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: <填入实际数字>
      output: <填入实际数字>
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: <填入实际数字>
      output: <填入实际数字>
    - provider: minimax
      model: t2a-v2
      billing_mode: standard
      unit: yuan_per_1k_chars
      chars: <填入实际数字>
    - provider: tencent_cos
      model: hk-standard
      billing_mode: standard
      put_yuan_per_10k_requests: <填入>
      bandwidth_yuan_per_gb: <填入>
```

- [ ] **Step 5: 写校对记录文档 + commit**

```bash
git add server/config/config.yaml.example docs/superpowers/specs/2026-05-25-plan-11b-pricebook-校对.md
git commit -m "spec(plan11): PriceBook v20260525-1 单价校对记录"
```

---

### Task 2: `pkg/idhash` HMAC 工具

**Files:**
- Create: `server/internal/pkg/idhash/idhash.go`
- Create: `server/internal/pkg/idhash/idhash_test.go`

- [ ] **Step 1: 写失败的单测**

```go
// server/internal/pkg/idhash/idhash_test.go
package idhash_test

import (
	"testing"

	"github.com/aibao/server/internal/pkg/idhash"
)

func TestHash_DomainSeparation(t *testing.T) {
	h := idhash.New("test-secret")
	user42 := h.Hash("user", 42)
	child42 := h.Hash("child", 42)
	if user42 == child42 {
		t.Fatalf("expected domain-separated hashes to differ, both = %s", user42)
	}
}

func TestHash_Stable(t *testing.T) {
	h := idhash.New("test-secret")
	a := h.Hash("user", 42)
	b := h.Hash("user", 42)
	if a != b {
		t.Fatalf("expected stable hash, got %s vs %s", a, b)
	}
	if len(a) != 12 {
		t.Fatalf("expected 12 hex chars, got %d (%s)", len(a), a)
	}
}

func TestHash_SecretChange(t *testing.T) {
	h1 := idhash.New("secret-a")
	h2 := idhash.New("secret-b")
	if h1.Hash("user", 42) == h2.Hash("user", 42) {
		t.Fatalf("expected different secrets to produce different hashes")
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd server && go test ./internal/pkg/idhash/...`
Expected: FAIL（package idhash 不存在）

- [ ] **Step 3: 写实现**

```go
// server/internal/pkg/idhash/idhash.go
package idhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Hasher struct {
	secret []byte
}

func New(secret string) *Hasher {
	return &Hasher{secret: []byte(secret)}
}

// Hash returns HMAC-SHA256(secret, "<domain>:<id>") truncated to 12 hex chars.
// Domain separation prevents same-ID cross-table linkage (user:42 ≠ child:42).
func (h *Hasher) Hash(domain string, id int64) string {
	m := hmac.New(sha256.New, h.secret)
	fmt.Fprintf(m, "%s:%d", domain, id)
	sum := m.Sum(nil)
	return hex.EncodeToString(sum)[:12]
}
```

- [ ] **Step 4: 运行测试通过**

Run: `cd server && go test ./internal/pkg/idhash/... -v`
Expected: PASS（3 tests）

- [ ] **Step 5: Commit**

```bash
git add server/internal/pkg/idhash/
git commit -m "feat(idhash): HMAC-SHA256 12-hex with domain separation (Plan 11B §6.3)"
```

---

### Task 3: `pkg/cost` PriceBook 接口 + 加载

**Files:**
- Create: `server/internal/pkg/cost/pricebook.go`
- Create: `server/internal/pkg/cost/pricebook_yaml.go`
- Create: `server/internal/pkg/cost/pricebook_test.go`

- [ ] **Step 1: 写接口 + DTO**

```go
// server/internal/pkg/cost/pricebook.go
package cost

import "errors"

var ErrPriceMiss = errors.New("pricebook: entry not found")

type Usage struct {
	TokensIn     int
	TokensOut    int
	TokensCached int
	Chars        int
	Bytes        int64
	AudioSeconds float64
}

type PriceBookKey struct {
	Provider    string
	Model       string
	BillingMode string // "standard" / "cached" / "batch" / "reasoning"
}

// PriceEntry is what gets snapshotted into cost_events.unit_price_snapshot.
type PriceEntry struct {
	Key                       PriceBookKey
	Unit                      string  // "yuan_per_1m_tokens" / "yuan_per_1k_chars" / etc.
	InputPrice                float64 // for LLM
	OutputPrice               float64 // for LLM
	CharsPrice                float64 // for TTS by chars
	PutPer10kRequests         float64 // for storage
	BandwidthYuanPerGB        float64 // for storage
}

type PriceBook interface {
	Lookup(key PriceBookKey) (PriceEntry, error)
	Version() string
}
```

- [ ] **Step 2: 写 YAML 加载实现**

```go
// server/internal/pkg/cost/pricebook_yaml.go
package cost

import (
	"fmt"

	"github.com/spf13/viper"
)

type yamlEntry struct {
	Provider                  string  `mapstructure:"provider"`
	Model                     string  `mapstructure:"model"`
	BillingMode               string  `mapstructure:"billing_mode"`
	Unit                      string  `mapstructure:"unit"`
	Input                     float64 `mapstructure:"input"`
	Output                    float64 `mapstructure:"output"`
	Chars                     float64 `mapstructure:"chars"`
	PutYuanPer10kRequests     float64 `mapstructure:"put_yuan_per_10k_requests"`
	BandwidthYuanPerGB        float64 `mapstructure:"bandwidth_yuan_per_gb"`
}

type yamlPriceBook struct {
	version string
	entries map[PriceBookKey]PriceEntry
}

func (b *yamlPriceBook) Version() string { return b.version }

func (b *yamlPriceBook) Lookup(key PriceBookKey) (PriceEntry, error) {
	if key.BillingMode == "" {
		key.BillingMode = "standard"
	}
	e, ok := b.entries[key]
	if !ok {
		return PriceEntry{}, fmt.Errorf("%w: %+v", ErrPriceMiss, key)
	}
	return e, nil
}

// LoadFromViper reads cost.price_book_version + cost.entries from the given viper instance.
// Hot-reload is NOT supported (spec §5.2) — caller restarts the process to apply changes.
func LoadFromViper(v *viper.Viper) (PriceBook, error) {
	version := v.GetString("cost.price_book_version")
	if version == "" {
		return nil, fmt.Errorf("cost.price_book_version is required")
	}
	var raw []yamlEntry
	if err := v.UnmarshalKey("cost.entries", &raw); err != nil {
		return nil, fmt.Errorf("decode cost.entries: %w", err)
	}
	pb := &yamlPriceBook{version: version, entries: map[PriceBookKey]PriceEntry{}}
	for _, r := range raw {
		key := PriceBookKey{Provider: r.Provider, Model: r.Model, BillingMode: r.BillingMode}
		if key.BillingMode == "" {
			key.BillingMode = "standard"
		}
		pb.entries[key] = PriceEntry{
			Key: key, Unit: r.Unit,
			InputPrice: r.Input, OutputPrice: r.Output,
			CharsPrice: r.Chars,
			PutPer10kRequests: r.PutYuanPer10kRequests,
			BandwidthYuanPerGB: r.BandwidthYuanPerGB,
		}
	}
	return pb, nil
}
```

- [ ] **Step 3: 写单测**

```go
// server/internal/pkg/cost/pricebook_test.go
package cost_test

import (
	"errors"
	"testing"

	"github.com/aibao/server/internal/pkg/cost"
	"github.com/spf13/viper"
)

func TestPriceBook_LoadAndLookup(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test-1
  entries:
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	pb, err := cost.LoadFromViper(v)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pb.Version() != "v-test-1" {
		t.Errorf("expected version v-test-1, got %s", pb.Version())
	}
	e, err := pb.Lookup(cost.PriceBookKey{Provider: "doubao", Model: "doubao-1.5-lite-32k", BillingMode: "standard"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if e.InputPrice != 0.30 || e.OutputPrice != 0.60 {
		t.Errorf("unexpected prices: %+v", e)
	}
}

func TestPriceBook_Miss(t *testing.T) {
	v := viper.New()
	v.Set("cost.price_book_version", "v-test-1")
	pb, _ := cost.LoadFromViper(v)
	_, err := pb.Lookup(cost.PriceBookKey{Provider: "unknown", Model: "x"})
	if !errors.Is(err, cost.ErrPriceMiss) {
		t.Fatalf("expected ErrPriceMiss, got %v", err)
	}
}
```

> 注：测试文件需 import `strings`。

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/pkg/cost/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/pkg/cost/pricebook.go server/internal/pkg/cost/pricebook_yaml.go server/internal/pkg/cost/pricebook_test.go
git commit -m "feat(cost): PriceBook interface + YAML loader (Plan 11B §5.2)"
```

---

### Task 4: `pkg/cost` Calculator（纯函数）

**Files:**
- Create: `server/internal/pkg/cost/calculator.go`
- Create: `server/internal/pkg/cost/calculator_test.go`

- [ ] **Step 1: 写失败的单测**

```go
// server/internal/pkg/cost/calculator_test.go
package cost_test

import (
	"testing"

	"github.com/aibao/server/internal/pkg/cost"
)

func TestCalc_LLMTokens(t *testing.T) {
	entry := cost.PriceEntry{
		Unit:        "yuan_per_1m_tokens",
		InputPrice:  0.30,
		OutputPrice: 0.60,
	}
	yuan := cost.Calc(entry, cost.Usage{TokensIn: 600, TokensOut: 400})
	want := (600*0.30 + 400*0.60) / 1_000_000 // 0.000420
	if abs(yuan-want) > 1e-9 {
		t.Errorf("LLM calc: want %.9f, got %.9f", want, yuan)
	}
}

func TestCalc_TTSChars(t *testing.T) {
	entry := cost.PriceEntry{
		Unit:       "yuan_per_1k_chars",
		CharsPrice: 0.85,
	}
	yuan := cost.Calc(entry, cost.Usage{Chars: 1418})
	want := 1418 * 0.85 / 1000
	if abs(yuan-want) > 1e-9 {
		t.Errorf("TTS calc: want %.9f, got %.9f", want, yuan)
	}
}

func TestCalc_ZeroUsage(t *testing.T) {
	yuan := cost.Calc(cost.PriceEntry{Unit: "yuan_per_1m_tokens"}, cost.Usage{})
	if yuan != 0 {
		t.Errorf("zero usage should be 0, got %f", yuan)
	}
}

func TestCalc_UnknownUnit(t *testing.T) {
	yuan := cost.Calc(cost.PriceEntry{Unit: "bogus"}, cost.Usage{TokensIn: 100})
	if yuan != 0 {
		t.Errorf("unknown unit should be 0 (defensive), got %f", yuan)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd server && go test ./internal/pkg/cost/... -run TestCalc`
Expected: FAIL（Calc 未定义）

- [ ] **Step 3: 写实现**

```go
// server/internal/pkg/cost/calculator.go
package cost

// Calc returns yuan cost for the given usage under the given price entry.
// Unknown units return 0 (defensive — surface as a metric/log warning elsewhere).
// Calc is pure: no IO, no global state, no logging.
func Calc(entry PriceEntry, u Usage) float64 {
	switch entry.Unit {
	case "yuan_per_1m_tokens":
		return (float64(u.TokensIn)*entry.InputPrice + float64(u.TokensOut)*entry.OutputPrice) / 1_000_000.0
	case "yuan_per_1k_chars":
		return float64(u.Chars) * entry.CharsPrice / 1000.0
	case "yuan_per_audio_second":
		return u.AudioSeconds * entry.CharsPrice // CharsPrice 字段复用，避免再加字段
	default:
		return 0
	}
}
```

- [ ] **Step 4: 运行测试通过**

Run: `cd server && go test ./internal/pkg/cost/... -v`
Expected: PASS（含 Task 3 的 PriceBook tests）

- [ ] **Step 5: Commit**

```bash
git add server/internal/pkg/cost/calculator.go server/internal/pkg/cost/calculator_test.go
git commit -m "feat(cost): pure Calc for LLM/TTS/audio-seconds (Plan 11B §3.1)"
```

---

### Task 5: `outline_events` migration（append-only）

**Files:**
- Create: `server/migrations/000008_outline_events.up.sql`
- Create: `server/migrations/000008_outline_events.down.sql`

- [ ] **Step 1: 写 up migration**

```sql
-- server/migrations/000008_outline_events.up.sql
CREATE TABLE outline_events (
    id                     BIGSERIAL PRIMARY KEY,
    occurred_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    outline_id             VARCHAR(64) NOT NULL,
    outline_group_id       VARCHAR(64) NOT NULL,
    user_id                BIGINT NOT NULL,
    child_id_hash          VARCHAR(32) NOT NULL,
    outcome                VARCHAR(16) NOT NULL CHECK (outcome IN ('pending','accepted','refreshed','expired')),
    outline_prompt_version VARCHAR(32),
    duration_min           INTEGER,
    trace_id               VARCHAR(64)
);

CREATE INDEX idx_outline_events_outline_id ON outline_events(outline_id);
CREATE INDEX idx_outline_events_group ON outline_events(outline_group_id);
CREATE INDEX idx_outline_events_user_day ON outline_events(user_id, occurred_at);

-- Plan 11A §5.5: append-only event stream. "最新生命周期" via:
--   SELECT DISTINCT ON (outline_id) * FROM outline_events
--   ORDER BY outline_id, occurred_at DESC, id DESC;
-- expired 也是追加新行，不 UPDATE。
COMMENT ON TABLE outline_events IS 'Plan 11A append-only event stream for outline lifecycle';
```

- [ ] **Step 2: 写 down migration**

```sql
-- server/migrations/000008_outline_events.down.sql
DROP TABLE IF EXISTS outline_events;
```

- [ ] **Step 3: 本地 dev 库跑 up + down + up**

```bash
cd server
migrate -path ./migrations -database "$AIBAO_DB_DSN" up
psql "$AIBAO_DB_DSN" -c "\d outline_events"  # 验证表结构
migrate -path ./migrations -database "$AIBAO_DB_DSN" down 1
psql "$AIBAO_DB_DSN" -c "\d outline_events" 2>&1 | grep -q "does not exist" && echo "down OK"
migrate -path ./migrations -database "$AIBAO_DB_DSN" up
```

Expected: 三步均成功；`\d outline_events` 显示 7 列 + 3 索引。

- [ ] **Step 4: Commit**

```bash
git add server/migrations/000008_outline_events.up.sql server/migrations/000008_outline_events.down.sql
git commit -m "feat(db): migration 000008 outline_events append-only (Plan 11A §5.5)"
```

---

### Task 6: `cost_events` migration

**Files:**
- Create: `server/migrations/000009_cost_events.up.sql`
- Create: `server/migrations/000009_cost_events.down.sql`

- [ ] **Step 1: 写 up migration**

```sql
-- server/migrations/000009_cost_events.up.sql
CREATE TABLE cost_events (
    id                     BIGSERIAL PRIMARY KEY,
    event_id               VARCHAR(96) NOT NULL UNIQUE,
    occurred_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id                BIGINT,
    child_id_hash          VARCHAR(32),
    purpose                VARCHAR(32) NOT NULL,
    provider               VARCHAR(32) NOT NULL,
    model                  VARCHAR(64),
    billing_mode           VARCHAR(32) NOT NULL DEFAULT 'standard',
    tokens_in              INTEGER,
    tokens_out             INTEGER,
    tokens_cached          INTEGER,
    chars                  INTEGER,
    bytes                  BIGINT,
    audio_seconds          NUMERIC(8, 2),
    cost_yuan              NUMERIC(12, 6) NOT NULL,
    currency               VARCHAR(8) NOT NULL DEFAULT 'CNY',
    price_version          VARCHAR(32) NOT NULL,
    unit_price_snapshot    JSONB,
    outcome                VARCHAR(16) NOT NULL CHECK (outcome IN ('ok','fallback','fail')),
    duration_ms            INTEGER,
    story_id               BIGINT,
    outline_id             VARCHAR(64),
    outline_group_id       VARCHAR(64),
    outline_prompt_version VARCHAR(32),
    trace_id               VARCHAR(64)
);

CREATE INDEX idx_cost_events_occurred ON cost_events(occurred_at);
CREATE INDEX idx_cost_events_user_day ON cost_events(user_id, occurred_at);
CREATE INDEX idx_cost_events_purpose ON cost_events(purpose, occurred_at);
CREATE INDEX idx_cost_events_outline ON cost_events(outline_id) WHERE outline_id IS NOT NULL;
CREATE INDEX idx_cost_events_outline_group ON cost_events(outline_group_id) WHERE outline_group_id IS NOT NULL;

COMMENT ON TABLE cost_events IS 'Plan 11B fact-source for cost; Prometheus is approximate observation';
COMMENT ON COLUMN cost_events.event_id IS 'Idempotency key: {trace_id}:{purpose}:{stage}:{attempt}';
COMMENT ON COLUMN cost_events.outcome IS 'Call result (ok/fallback/fail); NOT lifecycle — see outline_events.outcome for lifecycle';
```

- [ ] **Step 2: 写 down migration**

```sql
-- server/migrations/000009_cost_events.down.sql
DROP TABLE IF EXISTS cost_events;
```

- [ ] **Step 3: 跑 up + down + up（同 Task 5 命令模式）**

Expected: 表 + 5 索引创建成功。

- [ ] **Step 4: Commit**

```bash
git add server/migrations/000009_cost_events.up.sql server/migrations/000009_cost_events.down.sql
git commit -m "feat(db): migration 000009 cost_events with event_id idempotency + price snapshot (Plan 11B §5.1)"
```

---

### Task 7: `model.CostEvent` + `model.OutlineEvent` GORM model

**Files:**
- Create: `server/internal/model/cost_event.go`
- Create: `server/internal/model/outline_event.go`

- [ ] **Step 1: 写 CostEvent model**

```go
// server/internal/model/cost_event.go
package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type PriceSnapshot map[string]any

func (p PriceSnapshot) Value() (driver.Value, error) { return json.Marshal(p) }
func (p *PriceSnapshot) Scan(v any) error {
	if v == nil {
		*p = nil
		return nil
	}
	b, ok := v.([]byte)
	if !ok {
		return errors.New("PriceSnapshot.Scan: expected []byte")
	}
	return json.Unmarshal(b, p)
}

type CostEvent struct {
	ID                   int64         `gorm:"primaryKey"`
	EventID              string        `gorm:"column:event_id;uniqueIndex"`
	OccurredAt           time.Time     `gorm:"column:occurred_at"`
	UserID               *int64        `gorm:"column:user_id"`
	ChildIDHash          string        `gorm:"column:child_id_hash"`
	Purpose              string        `gorm:"column:purpose"`
	Provider             string        `gorm:"column:provider"`
	Model                string        `gorm:"column:model"`
	BillingMode          string        `gorm:"column:billing_mode"`
	TokensIn             int           `gorm:"column:tokens_in"`
	TokensOut            int           `gorm:"column:tokens_out"`
	TokensCached         int           `gorm:"column:tokens_cached"`
	Chars                int           `gorm:"column:chars"`
	Bytes                int64         `gorm:"column:bytes"`
	AudioSeconds         float64       `gorm:"column:audio_seconds"`
	CostYuan             float64       `gorm:"column:cost_yuan"`
	Currency             string        `gorm:"column:currency"`
	PriceVersion         string        `gorm:"column:price_version"`
	UnitPriceSnapshot    PriceSnapshot `gorm:"column:unit_price_snapshot;type:jsonb"`
	Outcome              string        `gorm:"column:outcome"`
	DurationMs           int           `gorm:"column:duration_ms"`
	StoryID              *int64        `gorm:"column:story_id"`
	OutlineID            string        `gorm:"column:outline_id"`
	OutlineGroupID       string        `gorm:"column:outline_group_id"`
	OutlinePromptVersion string        `gorm:"column:outline_prompt_version"`
	TraceID              string        `gorm:"column:trace_id"`
}

func (CostEvent) TableName() string { return "cost_events" }
```

- [ ] **Step 2: 写 OutlineEvent model**

```go
// server/internal/model/outline_event.go
package model

import "time"

type OutlineEvent struct {
	ID                   int64     `gorm:"primaryKey"`
	OccurredAt           time.Time `gorm:"column:occurred_at"`
	OutlineID            string    `gorm:"column:outline_id"`
	OutlineGroupID       string    `gorm:"column:outline_group_id"`
	UserID               int64     `gorm:"column:user_id"`
	ChildIDHash          string    `gorm:"column:child_id_hash"`
	Outcome              string    `gorm:"column:outcome"`
	OutlinePromptVersion string    `gorm:"column:outline_prompt_version"`
	DurationMin          int       `gorm:"column:duration_min"`
	TraceID              string    `gorm:"column:trace_id"`
}

func (OutlineEvent) TableName() string { return "outline_events" }
```

- [ ] **Step 3: Compile check**

Run: `cd server && go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add server/internal/model/cost_event.go server/internal/model/outline_event.go
git commit -m "feat(model): CostEvent + OutlineEvent GORM models"
```

---

### Task 8: `service/cost/Recorder` 同步入队 + Prometheus

**Files:**
- Create: `server/internal/service/cost/recorder.go`
- Create: `server/internal/service/cost/recorder_test.go`
- Modify: `server/internal/metrics/metrics.go`（添加 cost_yuan_total / cost_event_record_failed_total）

- [ ] **Step 1: 加 Prometheus metric 定义**

```go
// server/internal/metrics/metrics.go (附加到现有文件末尾)
var (
	CostYuanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cost_yuan_total",
		Help: "Cumulative cost in yuan, by provider/model/purpose/outcome",
	}, []string{"provider", "model", "purpose", "outcome"})

	CostEventRecordFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cost_event_record_failed_total",
		Help: "Cost recorder failures",
	}, []string{"reason"})

	CostFlusherBatchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "cost_flusher_batch_size",
		Help:    "Flusher batch size distribution",
		Buckets: []float64{1, 5, 10, 50, 100, 500, 1000},
	})

	CostFlusherLagSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cost_flusher_lag_seconds",
		Help: "Age of oldest queued cost event in seconds",
	})

	CostOutlineJoinMissTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cost_outline_join_miss_total",
		Help: "outline_events accepted but cost_events missing outline LLM row",
	})
)
```

- [ ] **Step 2: 写 Recorder 接口 + 入队 + Prometheus 同步写**

```go
// server/internal/service/cost/recorder.go
package cost

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/logger"
)

const (
	queueCapacity = 10_000
)

var eventIDRe = regexp.MustCompile(`^[a-f0-9]{8,}:[a-z_]+:[a-z_]+:\d+$`)

var ErrBadEventID = errors.New("recorder: bad event_id format")

// RecordInput is what callers pass. Business code constructs this; Recorder
// does NOT auto-generate event_id (spec §5.1.1).
type RecordInput struct {
	EventID              string
	UserID               *int64
	ChildIDHash          string
	Purpose              string // outline|story|tts|chapter_hook|memory_summary|storage_put
	Provider             string
	Model                string
	BillingMode          string // "standard" if empty
	Usage                pkgcost.Usage
	Outcome              string // ok|fallback|fail
	DurationMs           int
	StoryID              *int64
	OutlineID            string
	OutlineGroupID       string
	OutlinePromptVersion string
	TraceID              string
}

type Recorder struct {
	pb     pkgcost.PriceBook
	queue  chan *model.CostEvent
	once   sync.Once
	closed chan struct{}
}

func NewRecorder(pb pkgcost.PriceBook) *Recorder {
	return &Recorder{
		pb:     pb,
		queue:  make(chan *model.CostEvent, queueCapacity),
		closed: make(chan struct{}),
	}
}

// Record validates input, calculates cost, increments Prometheus counters,
// and enqueues to the flusher. Returns nil even on Prometheus/queue failures
// (business path must NEVER break on cost recording, spec §3.3).
func (r *Recorder) Record(ctx context.Context, in RecordInput) error {
	if !eventIDRe.MatchString(in.EventID) {
		metrics.CostEventRecordFailedTotal.WithLabelValues("bad_event_id").Inc()
		logger.From(ctx).Warn("cost.record.bad_event_id", "event_id", in.EventID)
		return ErrBadEventID
	}
	billing := in.BillingMode
	if billing == "" {
		billing = "standard"
	}
	key := pkgcost.PriceBookKey{Provider: in.Provider, Model: in.Model, BillingMode: billing}
	entry, err := r.pb.Lookup(key)
	if err != nil {
		metrics.CostEventRecordFailedTotal.WithLabelValues("price_miss").Inc()
		logger.From(ctx).Warn("cost.record.price_miss", "key", key)
		return nil // business continues
	}
	yuan := pkgcost.Calc(entry, in.Usage)

	// snapshot price entry for audit
	snap := map[string]any{
		"unit":               entry.Unit,
		"input":              entry.InputPrice,
		"output":             entry.OutputPrice,
		"chars":              entry.CharsPrice,
		"put_per_10k":        entry.PutPer10kRequests,
		"bandwidth_per_gb":   entry.BandwidthYuanPerGB,
	}
	snapBytes, _ := json.Marshal(snap) // for log only; gorm Value() will re-marshal
	_ = snapBytes

	// Prometheus (sync)
	metrics.CostYuanTotal.WithLabelValues(in.Provider, in.Model, in.Purpose, in.Outcome).Add(yuan)

	// Build event
	evt := &model.CostEvent{
		EventID:              in.EventID,
		OccurredAt:           time.Now(),
		UserID:               in.UserID,
		ChildIDHash:          in.ChildIDHash,
		Purpose:              in.Purpose,
		Provider:             in.Provider,
		Model:                in.Model,
		BillingMode:          billing,
		TokensIn:             in.Usage.TokensIn,
		TokensOut:            in.Usage.TokensOut,
		TokensCached:         in.Usage.TokensCached,
		Chars:                in.Usage.Chars,
		Bytes:                in.Usage.Bytes,
		AudioSeconds:         in.Usage.AudioSeconds,
		CostYuan:             yuan,
		Currency:             "CNY",
		PriceVersion:         r.pb.Version(),
		UnitPriceSnapshot:    snap,
		Outcome:              in.Outcome,
		DurationMs:           in.DurationMs,
		StoryID:              in.StoryID,
		OutlineID:            in.OutlineID,
		OutlineGroupID:       in.OutlineGroupID,
		OutlinePromptVersion: in.OutlinePromptVersion,
		TraceID:              in.TraceID,
	}

	// Async enqueue (non-blocking)
	select {
	case r.queue <- evt:
	default:
		metrics.CostEventRecordFailedTotal.WithLabelValues("queue_full").Inc()
		logger.From(ctx).Warn("cost.record.queue_full")
	}
	return nil
}

// Drain returns the queue for the Flusher to consume.
func (r *Recorder) Drain() <-chan *model.CostEvent {
	return r.queue
}

func (r *Recorder) Close() {
	r.once.Do(func() {
		close(r.closed)
		close(r.queue)
	})
}

func (r *Recorder) Closed() <-chan struct{} { return r.closed }
```

> 注：`logger.From(ctx)` 假设 Plan 1 已有；如未导出可直接用 slog default。

- [ ] **Step 3: 写 unit test（bad event_id + price_miss + 入队成功 + 队列满）**

```go
// server/internal/service/cost/recorder_test.go
package cost_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aibao/server/internal/service/cost"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/spf13/viper"
)

func newTestPB(t *testing.T) pkgcost.PriceBook {
	v := viper.New()
	v.SetConfigType("yaml")
	v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test
  entries:
    - provider: doubao
      model: lite
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`))
	pb, err := pkgcost.LoadFromViper(v)
	if err != nil {
		t.Fatalf("pb: %v", err)
	}
	return pb
}

func TestRecorder_BadEventID(t *testing.T) {
	r := cost.NewRecorder(newTestPB(t))
	err := r.Record(context.Background(), cost.RecordInput{EventID: "bogus", Provider: "doubao", Model: "lite", Purpose: "outline", Outcome: "ok"})
	if err != cost.ErrBadEventID {
		t.Fatalf("want ErrBadEventID, got %v", err)
	}
}

func TestRecorder_PriceMiss_BusinessContinues(t *testing.T) {
	r := cost.NewRecorder(newTestPB(t))
	err := r.Record(context.Background(), cost.RecordInput{
		EventID:  "abcdef12:outline:llm_call:1",
		Provider: "unknown", Model: "xyz",
		Purpose: "outline", Outcome: "ok",
	})
	if err != nil {
		t.Fatalf("price miss must not break business: %v", err)
	}
}

func TestRecorder_Enqueue(t *testing.T) {
	r := cost.NewRecorder(newTestPB(t))
	_ = r.Record(context.Background(), cost.RecordInput{
		EventID:  "abcdef12:outline:llm_call:1",
		Provider: "doubao", Model: "lite",
		Purpose: "outline", Outcome: "ok",
		Usage:   pkgcost.Usage{TokensIn: 600, TokensOut: 400},
	})
	select {
	case evt := <-r.Drain():
		if evt.EventID != "abcdef12:outline:llm_call:1" {
			t.Errorf("unexpected event_id: %s", evt.EventID)
		}
		want := (600*0.30 + 400*0.60) / 1_000_000.0
		if abs(evt.CostYuan-want) > 1e-9 {
			t.Errorf("cost want %.9f got %.9f", want, evt.CostYuan)
		}
	default:
		t.Fatalf("expected event in queue")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/service/cost/... -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/cost/recorder.go server/internal/service/cost/recorder_test.go server/internal/metrics/metrics.go
git commit -m "feat(cost): Recorder with event_id validation + Prometheus sync + async queue (Plan 11B)"
```

---

### Task 9: `service/cost/Flusher` 后台批量写

**Files:**
- Create: `server/internal/service/cost/flusher.go`
- Create: `server/internal/service/cost/flusher_test.go`

- [ ] **Step 1: 写 Flusher**

```go
// server/internal/service/cost/flusher.go
package cost

import (
	"context"
	"errors"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
	"gorm.io/gorm"
)

const (
	flushInterval = 60 * time.Second
	maxBatch      = 200
	shutdownGrace = 5 * time.Second
)

type Flusher struct {
	r  *Recorder
	db *gorm.DB
}

func NewFlusher(r *Recorder, db *gorm.DB) *Flusher {
	return &Flusher{r: r, db: db}
}

// Run blocks until ctx is cancelled, then performs a final flush within shutdownGrace.
func (f *Flusher) Run(ctx context.Context) {
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()
	batch := make([]*model.CostEvent, 0, maxBatch)
	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
			f.drainAndFlush(drainCtx, batch)
			cancel()
			return
		case <-tick.C:
			batch = f.collect(batch[:0])
			f.flush(ctx, batch)
		}
	}
}

func (f *Flusher) collect(batch []*model.CostEvent) []*model.CostEvent {
	for len(batch) < maxBatch {
		select {
		case evt, ok := <-f.r.Drain():
			if !ok {
				return batch
			}
			batch = append(batch, evt)
		default:
			return batch
		}
	}
	return batch
}

func (f *Flusher) drainAndFlush(ctx context.Context, batch []*model.CostEvent) {
	batch = f.collect(batch[:0])
	for len(batch) > 0 {
		f.flush(ctx, batch)
		batch = f.collect(batch[:0])
	}
}

func (f *Flusher) flush(ctx context.Context, batch []*model.CostEvent) {
	if len(batch) == 0 {
		return
	}
	metrics.CostFlusherBatchSize.Observe(float64(len(batch)))
	// ON CONFLICT (event_id) DO NOTHING — idempotent
	err := f.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: false}).
		Clauses(clauseOnConflictDoNothing()).
		Create(&batch).Error
	if err != nil && !errors.Is(err, context.Canceled) {
		metrics.CostEventRecordFailedTotal.WithLabelValues("db_write").Inc()
		logger.From(ctx).Error("cost.flush.failed", "err", err, "batch_size", len(batch))
	}
}

// helper: use gorm.io/gorm/clause to avoid importing in test
func clauseOnConflictDoNothing() any { // returns clause.OnConflict
	type onConflict = struct {
		Columns   []any
		DoNothing bool
	}
	// real import done at top of file in production; sketch shown for plan readability
	return nil // see Step 2 patch
}
```

- [ ] **Step 2: 替换上面的 clause helper 为真实 import**

修改 imports + helper：

```go
import (
	// ... existing ...
	"gorm.io/gorm/clause"
)

// 删除 clauseOnConflictDoNothing 函数；flush 内直接：
//   Clauses(clause.OnConflict{DoNothing: true}).
```

最终 flush 体：

```go
func (f *Flusher) flush(ctx context.Context, batch []*model.CostEvent) {
	if len(batch) == 0 {
		return
	}
	metrics.CostFlusherBatchSize.Observe(float64(len(batch)))
	err := f.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&batch).Error
	if err != nil && !errors.Is(err, context.Canceled) {
		metrics.CostEventRecordFailedTotal.WithLabelValues("db_write").Inc()
		logger.From(ctx).Error("cost.flush.failed", "err", err, "batch_size", len(batch))
	}
}
```

- [ ] **Step 3: 写集成测（testcontainers PG）**

```go
// server/internal/service/cost/flusher_test.go
package cost_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/service/cost"
	"github.com/spf13/viper"
	// reuse existing test helper that spins up PG via testcontainers
)

func TestFlusher_Idempotent(t *testing.T) {
	db := startTestPG(t) // helper from existing test infra
	db.AutoMigrate(&model.CostEvent{})

	v := viper.New()
	v.SetConfigType("yaml")
	v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test
  entries:
    - provider: doubao
      model: lite
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`))
	pb, _ := pkgcost.LoadFromViper(v)
	r := cost.NewRecorder(pb)
	f := cost.NewFlusher(r, db)

	ctx, cancel := context.WithCancel(context.Background())
	go f.Run(ctx)

	// record same event_id twice
	in := cost.RecordInput{
		EventID: "abcdef12:outline:llm_call:1",
		Provider: "doubao", Model: "lite",
		Purpose: "outline", Outcome: "ok",
		Usage: pkgcost.Usage{TokensIn: 100, TokensOut: 50},
	}
	_ = r.Record(ctx, in)
	_ = r.Record(ctx, in)

	// trigger flush by waiting > flushInterval — but for test, force manually:
	// easier: cancel ctx → final flush
	cancel()
	time.Sleep(500 * time.Millisecond)

	var cnt int64
	db.Model(&model.CostEvent{}).Where("event_id = ?", in.EventID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected 1 row (idempotent), got %d", cnt)
	}
}
```

> 假设 `startTestPG` 已在现有 testcontainers helper 中提供（Plan 1 已有）。若名字不同，按现有 convention 调整。

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/service/cost/... -v -run TestFlusher`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/cost/flusher.go server/internal/service/cost/flusher_test.go
git commit -m "feat(cost): async Flusher with ON CONFLICT DO NOTHING idempotency (Plan 11B §3.3)"
```

---

### Task 10: Gateway 暴露完整 Usage（不调 Recorder）

**Files:**
- Modify: `server/internal/gateway/llm/llm.go`
- Modify: `server/internal/gateway/tts/tts.go`
- Modify: `server/internal/gateway/storage/storage.go`
- Modify: `server/internal/gateway/storage/cos.go`（Upload 返回 bytes）
- Modify: `server/internal/gateway/storage/mock.go`（同步）

- [ ] **Step 1: LLM Gateway — 已有 Tokens/Latency，确认无破坏**

```go
// server/internal/gateway/llm/llm.go
// GenerateResponse 现状：Text / InputTokens / OutputTokens / Provider / Model / Latency
// 满足 Plan 11B 需求，无需改动。仅添加注释：
type GenerateResponse struct {
	Text         string
	InputTokens  int           // Plan 11B: business pulls these into pkgcost.Usage
	OutputTokens int
	Provider     string
	Model        string
	Latency      time.Duration
}
```

只补注释，不改字段。

- [ ] **Step 2: TTS Gateway — 加 CharCount**

```go
// server/internal/gateway/tts/tts.go
type SynthesizeResponse struct {
	Audio           []byte
	Format          string
	DurationSeconds int
	CharCount       int           // Plan 11B: 计费基础（按字数）
	Provider        string
	Latency         time.Duration
}
```

修改 Minimax 实现把 request 的 char count 写入响应（用 `utf8.RuneCountInString(req.Text)`，因为 Minimax 按字符计）：

```go
// server/internal/gateway/tts/minimax.go (摘录)
return &SynthesizeResponse{
	Audio:           audioBytes,
	Format:          req.Format,
	DurationSeconds: durSec,
	CharCount:       utf8.RuneCountInString(req.Text),
	Provider:        "minimax",
	Latency:         time.Since(start),
}, nil
```

Mock 实现同样填 CharCount。

- [ ] **Step 3: Storage Gateway — Upload 返回 bytes**

```go
// server/internal/gateway/storage/storage.go
type Client interface {
	Upload(ctx context.Context, in UploadInput) (bytesUploaded int64, err error)
	HeadObject(ctx context.Context, key string) (*ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
}
```

```go
// server/internal/gateway/storage/cos.go (摘录: Upload)
func (s *COSClient) Upload(ctx context.Context, in UploadInput) (int64, error) {
	// ... existing 上传逻辑 ...
	// 在成功 PUT 后，bytes 即 in.Size（若 0 则尝试从 Content-Length response 拿）
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return in.Size, nil
	}
	// ... error paths return 0, err ...
}
```

- [ ] **Step 4: 修所有 Upload 调用方（编译驱动）**

```bash
cd server && go build ./... 2>&1 | grep "Upload" | head -20
```

修改 `service/audio/orchestrator.go` 等所有调用方：
```go
bytesUp, err := s.storage.Upload(ctx, in)
if err != nil { ... }
// bytesUp will be used in Task 11 (Recorder call)
```

暂存：本任务先编译通过，Recorder 调用在 Task 11 加入。

- [ ] **Step 5: 编译 + 运行现有测试**

```bash
cd server && go build ./... && go test ./internal/gateway/...
```

Expected: 通过。Storage 测试如有 `_, err := Upload(...)` 模式要改成 `_, _ = Upload(...)`。

- [ ] **Step 6: Commit**

```bash
git add server/internal/gateway/
git commit -m "feat(gateway): expose Usage data (CharCount on TTS, bytesUploaded on storage) — no Recorder coupling (Plan 11B §3.1)"
```

---

### Task 11: Orchestrator 接入 Recorder（11B Thin Slice 收尾）

**Files:**
- Modify: `server/internal/service/story/orchestrator.go`
- Modify: `server/internal/service/audio/orchestrator.go`（TTS + storage 上传两处）
- Modify: `server/internal/service/memory/summarizer.go`（memory_summary purpose）
- Modify: `server/internal/service/story/chapter_hook.go`（chapter_hook purpose）
- Modify: `server/cmd/server/main.go`（wire Recorder + Flusher）

- [ ] **Step 1: main.go 创建 Recorder + Flusher 并 wire 到 services**

```go
// server/cmd/server/main.go (添加在 PG init 之后)
pb, err := pkgcost.LoadFromViper(viper.GetViper())
if err != nil { log.Fatalf("pricebook: %v", err) }
recorder := costsvc.NewRecorder(pb)
flusher := costsvc.NewFlusher(recorder, db)
go flusher.Run(ctx) // ctx 来自 main 的 signal.NotifyContext

storyOrch := story.NewOrchestrator(..., recorder) // 新增参数
audioOrch := audio.NewOrchestrator(..., recorder)
// 其他需要 record 的 service 同样注入
```

- [ ] **Step 2: story Orchestrator 接入 recorder（正文 LLM call 之后）**

```go
// server/internal/service/story/orchestrator.go (LLM 调用后)
resp, err := o.llm.Generate(ctx, req)
// ... 现有错误处理 ...
_ = o.recorder.Record(ctx, costsvc.RecordInput{
	EventID:     fmt.Sprintf("%s:story:llm_call:%d", traceID, attempt),
	UserID:      &userID,
	ChildIDHash: o.idHasher.Hash("child", childID),
	Purpose:     "story",
	Provider:    resp.Provider,
	Model:       resp.Model,
	Usage:       pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
	Outcome:     "ok",
	DurationMs:  int(resp.Latency.Milliseconds()),
	StoryID:     &storyID,
	OutlineID:   in.OutlineID, // empty if no outline
	TraceID:     traceID,
})
```

> `o.recorder` 在构造函数加入；`o.idHasher` 同上（pkg/idhash.Hasher 注入）。

- [ ] **Step 3: 类似改 audio (TTS + storage_put)**

```go
// audio/orchestrator.go after TTS
_ = o.recorder.Record(ctx, costsvc.RecordInput{
	EventID:     fmt.Sprintf("%s:tts:synthesize:%d", traceID, attempt),
	Purpose:     "tts",
	Provider:    ttsResp.Provider,
	Model:       "t2a-v2",
	Usage:       pkgcost.Usage{Chars: ttsResp.CharCount, AudioSeconds: float64(ttsResp.DurationSeconds)},
	Outcome:     "ok",
	DurationMs:  int(ttsResp.Latency.Milliseconds()),
	StoryID:     &storyID,
	TraceID:     traceID,
})

// after storage Upload
bytesUp, err := o.storage.Upload(ctx, uploadIn)
if err == nil {
	_ = o.recorder.Record(ctx, costsvc.RecordInput{
		EventID:    fmt.Sprintf("%s:storage_put:upload:1", traceID),
		Purpose:    "storage_put",
		Provider:   "tencent_cos",
		Model:      "hk-standard",
		Usage:      pkgcost.Usage{Bytes: bytesUp},
		Outcome:    "ok",
		StoryID:    &storyID,
		TraceID:    traceID,
	})
}
```

- [ ] **Step 4: memory summarizer + chapter hook 接入**

```go
// memory/summarizer.go after LLM call
_ = recorder.Record(ctx, costsvc.RecordInput{
	EventID:  fmt.Sprintf("%s:memory_summary:llm_call:1", traceID),
	Purpose:  "memory_summary",
	Provider: resp.Provider, Model: resp.Model,
	Usage:    pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
	Outcome:  "ok",
	StoryID:  &storyID,
	TraceID:  traceID,
})

// chapter_hook.go 同样模式
```

- [ ] **Step 5: 跑全量 build + 全量 test**

```bash
cd server && go build ./... && go test ./...
```

Expected: 全过。已有 story/audio 测试可能因构造函数参数变化需要小改（注入 nil recorder 或 fake）。

- [ ] **Step 6: 集成 smoke 验证（手工）**

```bash
# 跑一次完整故事生成（已有的 plan9d smoke 脚本）
pwsh -File scripts/plan9d-api-smoke.ps1

# 查 cost_events 是否有 4 类事件
psql $AIBAO_DB_DSN -c "SELECT purpose, COUNT(*), SUM(cost_yuan) FROM cost_events GROUP BY purpose"
```

Expected: story / tts / memory_summary / storage_put 都有行，cost_yuan 非 0。

- [ ] **Step 7: Commit**

```bash
git add server/
git commit -m "feat(cost): wire Recorder into story/audio/memory/chapter_hook orchestrators (Plan 11B Thin Slice 收尾)"
```

---

## Sprint B — 11A 大纲后端（Task 12-22）

> 11B Thin Slice 已就绪，cost_events 已记账；现在加 11A handler，outline LLM call 立即享受成本归集。

### Task 12: `service/outlinecontract/` 中立合约包

**Files:**
- Create: `server/internal/service/outlinecontract/resolver.go`
- Create: `server/internal/service/outlinecontract/errors.go`

- [ ] **Step 1: 写 errors**

```go
// server/internal/service/outlinecontract/errors.go
package outlinecontract

import "errors"

var (
	ErrOutlineExpired   = errors.New("outline: expired or not found in cache")
	ErrOutlineForbidden = errors.New("outline: ownership mismatch")
	ErrOutlineNotFound  = errors.New("outline: not found")
)
```

- [ ] **Step 2: 写 Outline DTO + OutlineResolver 接口**

```go
// server/internal/service/outlinecontract/resolver.go
package outlinecontract

import "context"

// Outline is the resolved outline DTO consumed by story orchestrator.
// All fields are post-validation, post-safety, ready for prompt injection.
type Outline struct {
	OutlineID            string
	Title                string
	Synopsis             string
	EducationalValue     string
	Themes               []string
	Style                string
	DurationMin          int
	SceneSeed            string
	OutlineGroupID       string
	VariantIndex         int
	ParentOutlineID      string
	OutlinePromptVersion string
}

// OutlineResolver resolves an outline_id to its full Outline payload,
// enforcing user_id + child_id + outline_id triple ownership.
// Returns ErrOutlineExpired / ErrOutlineForbidden / ErrOutlineNotFound.
type OutlineResolver interface {
	Resolve(ctx context.Context, outlineID string, userID, childID int64) (*Outline, error)
}
```

- [ ] **Step 3: Compile**

Run: `cd server && go build ./internal/service/outlinecontract/...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/outlinecontract/
git commit -m "feat(outlinecontract): neutral interface package for story↔outline decoupling (Plan 11A §7.5)"
```

---

### Task 13: `service/outline/cache.go` Redis 票据存储

**Files:**
- Create: `server/internal/service/outline/cache.go`
- Create: `server/internal/service/outline/cache_test.go`

- [ ] **Step 1: 写 Cache 接口 + Redis 实现**

```go
// server/internal/service/outline/cache.go
package outline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/redis/go-redis/v9"
)

const cacheTTL = 5 * time.Minute

var ErrCacheMiss = errors.New("outline cache: miss")

// cachedOutline is what we store in Redis — Outline + ownership fields + prompt text.
// outline_id NEVER appears in logs / metric labels (spec §5.2).
type cachedOutline struct {
	outlinecontract.Outline
	UserID     int64     `json:"user_id"`
	ChildID    int64     `json:"child_id"`
	PromptText string    `json:"prompt_text"`
	CreatedAt  time.Time `json:"created_at"`
}

type Cache struct {
	rdb *redis.Client
}

func NewCache(rdb *redis.Client) *Cache { return &Cache{rdb: rdb} }

func cacheKey(outlineID string) string { return "outline:" + outlineID }

// Set stores the outline with 5min TTL.
func (c *Cache) Set(ctx context.Context, co cachedOutline) error {
	b, err := json.Marshal(co)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.rdb.Set(ctx, cacheKey(co.OutlineID), b, cacheTTL).Err()
}

// Get retrieves the outline. Returns ErrCacheMiss if not found / expired.
func (c *Cache) Get(ctx context.Context, outlineID string) (*cachedOutline, error) {
	raw, err := c.rdb.Get(ctx, cacheKey(outlineID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var co cachedOutline
	if err := json.Unmarshal(raw, &co); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &co, nil
}

// Invalidate deletes the outline immediately (for refresh path).
func (c *Cache) Invalidate(ctx context.Context, outlineID string) error {
	return c.rdb.Del(ctx, cacheKey(outlineID)).Err()
}
```

- [ ] **Step 2: 写 test（testcontainers Redis）**

```go
// server/internal/service/outline/cache_test.go
package outline_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
	// reuse existing testcontainers Redis helper
)

func TestCache_SetGet(t *testing.T) {
	rdb := startTestRedis(t)
	c := outline.NewCache(rdb)
	ctx := context.Background()

	co := outline.NewCachedOutline(outlinecontract.Outline{
		OutlineID: "ol_test_123",
		Title:     "test",
		Style:     "冒险探索",
	}, 42, 7, "原 prompt 文本")
	if err := c.Set(ctx, co); err != nil { t.Fatalf("set: %v", err) }

	got, err := c.Get(ctx, "ol_test_123")
	if err != nil { t.Fatalf("get: %v", err) }
	if got.Title != "test" { t.Errorf("title mismatch: %s", got.Title) }
	if got.UserID != 42 { t.Errorf("user mismatch: %d", got.UserID) }
}

func TestCache_Miss(t *testing.T) {
	rdb := startTestRedis(t)
	c := outline.NewCache(rdb)
	_, err := c.Get(context.Background(), "ol_nonexistent")
	if err != outline.ErrCacheMiss { t.Errorf("want ErrCacheMiss, got %v", err) }
}

func TestCache_Invalidate(t *testing.T) {
	rdb := startTestRedis(t)
	c := outline.NewCache(rdb)
	ctx := context.Background()
	co := outline.NewCachedOutline(outlinecontract.Outline{OutlineID: "ol_inv"}, 1, 1, "")
	c.Set(ctx, co)
	c.Invalidate(ctx, "ol_inv")
	_, err := c.Get(ctx, "ol_inv")
	if err != outline.ErrCacheMiss { t.Errorf("want miss after invalidate") }
}

var _ = time.Second // keep import
```

> 同时在 `cache.go` 暴露一个构造 helper：
> ```go
> func NewCachedOutline(o outlinecontract.Outline, userID, childID int64, prompt string) cachedOutline {
>     return cachedOutline{Outline: o, UserID: userID, ChildID: childID, PromptText: prompt, CreatedAt: time.Now()}
> }
> ```

- [ ] **Step 3: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestCache`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/outline/cache.go server/internal/service/outline/cache_test.go
git commit -m "feat(outline): Redis 5min TTL cache for outline tickets (Plan 11A §5.2)"
```

---

### Task 14: `service/outline/events.go` PG append-only 写入

**Files:**
- Create: `server/internal/service/outline/events.go`
- Create: `server/internal/service/outline/events_test.go`

- [ ] **Step 1: 写 EventStore**

```go
// server/internal/service/outline/events.go
package outline

import (
	"context"
	"time"

	"github.com/aibao/server/internal/model"
	"gorm.io/gorm"
)

// outline_events.outcome values (append-only event stream)
const (
	OutcomePending   = "pending"
	OutcomeAccepted  = "accepted"
	OutcomeRefreshed = "refreshed"
	OutcomeExpired   = "expired"
)

type EventStore struct {
	db *gorm.DB
}

func NewEventStore(db *gorm.DB) *EventStore { return &EventStore{db: db} }

// Append inserts a new event row. Caller is responsible for not double-counting
// (e.g. don't append expired if accepted already exists for this outline_id).
func (s *EventStore) Append(ctx context.Context, evt model.OutlineEvent) error {
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(&evt).Error
}

// LatestOutcome returns the most recent outcome for an outline_id, or "" if no rows.
func (s *EventStore) LatestOutcome(ctx context.Context, outlineID string) (string, error) {
	var evt model.OutlineEvent
	err := s.db.WithContext(ctx).
		Where("outline_id = ?", outlineID).
		Order("occurred_at DESC, id DESC").
		Limit(1).
		First(&evt).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return evt.Outcome, nil
}

// MarkExpiredIfPending uses INSERT WHERE NOT EXISTS to add an expired row
// only if no terminal outcome exists yet. Idempotent (spec §5.5).
func (s *EventStore) MarkExpiredIfPending(ctx context.Context, evt model.OutlineEvent) error {
	const sql = `
INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome, outline_prompt_version, duration_min, trace_id)
SELECT $1, $2, $3, $4, $5, 'expired', $6, $7, $8
WHERE NOT EXISTS (
    SELECT 1 FROM outline_events
    WHERE outline_id = $2 AND outcome IN ('accepted', 'refreshed', 'expired')
)`
	return s.db.WithContext(ctx).Exec(sql,
		time.Now(), evt.OutlineID, evt.OutlineGroupID, evt.UserID, evt.ChildIDHash,
		evt.OutlinePromptVersion, evt.DurationMin, evt.TraceID,
	).Error
}

// ScanPendingOlderThan returns outline rows with outcome=pending older than threshold,
// for which no terminal outcome exists. Used by housekeeping + on-demand sweep.
func (s *EventStore) ScanPendingOlderThan(ctx context.Context, threshold time.Time, userID *int64, limit int) ([]model.OutlineEvent, error) {
	q := s.db.WithContext(ctx).
		Raw(`
SELECT DISTINCT ON (outline_id) *
FROM outline_events
WHERE occurred_at < ?
  AND outcome = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM outline_events e2
      WHERE e2.outline_id = outline_events.outline_id
        AND e2.outcome IN ('accepted','refreshed','expired')
  )
  AND (? OR user_id = ?)
ORDER BY outline_id, occurred_at DESC, id DESC
LIMIT ?`,
			threshold, userID == nil, ptrInt64(userID), limit)
	var out []model.OutlineEvent
	err := q.Scan(&out).Error
	return out, err
}

func ptrInt64(p *int64) int64 {
	if p == nil { return 0 }
	return *p
}
```

- [ ] **Step 2: 写 test**

```go
// server/internal/service/outline/events_test.go
package outline_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/outline"
)

func TestEventStore_AppendAndLatest(t *testing.T) {
	db := startTestPG(t)
	db.AutoMigrate(&model.OutlineEvent{})
	s := outline.NewEventStore(db)
	ctx := context.Background()

	_ = s.Append(ctx, model.OutlineEvent{OutlineID: "ol_a", OutlineGroupID: "g_a", UserID: 1, ChildIDHash: "h", Outcome: "pending"})
	_ = s.Append(ctx, model.OutlineEvent{OutlineID: "ol_a", OutlineGroupID: "g_a", UserID: 1, ChildIDHash: "h", Outcome: "accepted"})

	got, _ := s.LatestOutcome(ctx, "ol_a")
	if got != "accepted" {
		t.Fatalf("want accepted, got %s", got)
	}
}

func TestEventStore_MarkExpiredIfPending_Idempotent(t *testing.T) {
	db := startTestPG(t)
	db.AutoMigrate(&model.OutlineEvent{})
	s := outline.NewEventStore(db)
	ctx := context.Background()

	_ = s.Append(ctx, model.OutlineEvent{OutlineID: "ol_b", OutlineGroupID: "g_b", UserID: 1, ChildIDHash: "h", Outcome: "pending"})
	_ = s.MarkExpiredIfPending(ctx, model.OutlineEvent{OutlineID: "ol_b", OutlineGroupID: "g_b", UserID: 1, ChildIDHash: "h"})
	_ = s.MarkExpiredIfPending(ctx, model.OutlineEvent{OutlineID: "ol_b", OutlineGroupID: "g_b", UserID: 1, ChildIDHash: "h"})

	var cnt int64
	db.Model(&model.OutlineEvent{}).Where("outline_id = ? AND outcome = 'expired'", "ol_b").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected 1 expired row (idempotent), got %d", cnt)
	}

	// already accepted → mark expired noop
	_ = s.Append(ctx, model.OutlineEvent{OutlineID: "ol_c", OutlineGroupID: "g_c", UserID: 1, ChildIDHash: "h", Outcome: "accepted"})
	_ = s.MarkExpiredIfPending(ctx, model.OutlineEvent{OutlineID: "ol_c", OutlineGroupID: "g_c", UserID: 1, ChildIDHash: "h"})
	var cntC int64
	db.Model(&model.OutlineEvent{}).Where("outline_id = ? AND outcome = 'expired'", "ol_c").Count(&cntC)
	if cntC != 0 {
		t.Fatalf("expected no expired row when accepted exists, got %d", cntC)
	}
}

var _ = time.Second
```

- [ ] **Step 3: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestEventStore`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/outline/events.go server/internal/service/outline/events_test.go
git commit -m "feat(outline): EventStore append-only with idempotent MarkExpiredIfPending (Plan 11A §5.5)"
```

---

### Task 15: `service/outline/llm_prompt.go` + `llm_parser.go`

**Files:**
- Create: `server/internal/service/outline/llm_prompt.go`
- Create: `server/internal/service/outline/llm_parser.go`
- Create: `server/internal/service/outline/llm_parser_test.go`

- [ ] **Step 1: 写 prompt 模板 + 版本常量**

```go
// server/internal/service/outline/llm_prompt.go
package outline

import (
	"fmt"
	"strings"
)

// OutlinePromptVersion bumps on every prompt template change.
// Tracked in Redis payload + outline_events + cost_events for A/B / regression analysis.
const OutlinePromptVersion = "v20260525-1"

// BuildPrompt assembles system + user messages for outline LLM call.
// Returns (systemMsg, userMsg).
func BuildPrompt(in OutlinePromptInput) (string, string) {
	sys := strings.TrimSpace(`
你是儿童故事策划师。家长会用一句话告诉你孩子今晚想听什么样的故事，
你需要返回一个**结构化大纲 JSON**，让家长在生成正文前确认。

要求：
1. 必须返回合法 JSON，包含字段：title / synopsis / themes / style / educational_value
2. title：故事标题，5-12 个汉字
3. synopsis：3-5 句梗概，80-120 汉字，必须以孩子（昵称见用户消息）为主角
4. themes：1-3 个教育主题，从中国传统教育价值观词库选（如"勇气"/"友谊"/"诚实"）
5. style：必须从下列 5 选 1：温馨治愈 / 冒险探索 / 搞笑欢乐 / 神奇魔法 / 科普认知
6. educational_value：1 句话说明孩子能学到什么，自然口语
7. 不要包含任何成人内容、暴力、恐怖、政治词汇
8. 不要让孩子之外的角色（如奥特曼）抢主角位置——孩子永远是主角，IP 是陪伴
`)
	user := fmt.Sprintf(strings.TrimSpace(`
孩子昵称：%s
孩子年龄：%d 岁
本次需求：%s
时长档位：%d 分钟（仅作时长参考，无需写入 JSON 的 duration_min 字段）

请返回大纲 JSON。
`), in.ChildNickname, in.ChildAge, in.UserPrompt, in.DurationMin)
	return sys, user
}

type OutlinePromptInput struct {
	ChildNickname string
	ChildAge      int
	UserPrompt    string
	DurationMin   int
}
```

- [ ] **Step 2: 写 parser + 校验**

```go
// server/internal/service/outline/llm_parser.go
package outline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidJSON     = errors.New("outline parser: invalid JSON")
	ErrInvalidStyle    = errors.New("outline parser: invalid style enum")
	ErrSynopsisLength  = errors.New("outline parser: synopsis length out of range")
	ErrThemesCount     = errors.New("outline parser: themes count out of range")
	ErrTitleLength     = errors.New("outline parser: title length out of range")
	ErrUnknownField    = errors.New("outline parser: unknown field present")
)

var validStyles = map[string]bool{
	"温馨治愈": true, "冒险探索": true, "搞笑欢乐": true, "神奇魔法": true, "科普认知": true,
}

type RawOutline struct {
	Title            string   `json:"title"`
	Synopsis         string   `json:"synopsis"`
	Themes           []string `json:"themes"`
	Style            string   `json:"style"`
	EducationalValue string   `json:"educational_value"`
}

// Parse extracts the structured outline from the LLM response text.
// Strict schema: unknown fields rejected; ranges validated.
func Parse(raw string) (*RawOutline, error) {
	raw = strings.TrimSpace(raw)
	// strip markdown code fence if present
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var ro RawOutline
	if err := dec.Decode(&ro); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if n := utf8.RuneCountInString(ro.Title); n < 5 || n > 16 {
		return nil, fmt.Errorf("%w: got %d runes", ErrTitleLength, n)
	}
	if n := utf8.RuneCountInString(ro.Synopsis); n < 60 || n > 160 {
		return nil, fmt.Errorf("%w: got %d runes", ErrSynopsisLength, n)
	}
	if len(ro.Themes) < 1 || len(ro.Themes) > 3 {
		return nil, fmt.Errorf("%w: got %d themes", ErrThemesCount, len(ro.Themes))
	}
	if !validStyles[ro.Style] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStyle, ro.Style)
	}
	return &ro, nil
}
```

- [ ] **Step 3: 写 parser test (含 negative cases)**

```go
// server/internal/service/outline/llm_parser_test.go
package outline_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/service/outline"
)

func TestParse_Happy(t *testing.T) {
	raw := `{
  "title": "小宇的星空冒险",
  "synopsis": "小宇遇到爱宝，他们一起穿越到星空之上。途中遇到流星雨，小宇展现出勇气，主动想办法保护小动物，最终大家平安回家。",
  "themes": ["勇气", "团队合作"],
  "style": "冒险探索",
  "educational_value": "学到遇到困难不害怕、和小伙伴一起想办法"
}`
	ro, err := outline.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ro.Style != "冒险探索" {
		t.Errorf("style: %s", ro.Style)
	}
}

func TestParse_InvalidStyle(t *testing.T) {
	raw := `{"title":"测试标题","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"恐怖","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrInvalidStyle) {
		t.Fatalf("want ErrInvalidStyle, got %v", err)
	}
}

func TestParse_UnknownField(t *testing.T) {
	raw := `{"title":"测试标题","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x","extra":"bad"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrInvalidJSON) {
		t.Fatalf("want ErrInvalidJSON (unknown field), got %v", err)
	}
}

func TestParse_SynopsisTooShort(t *testing.T) {
	raw := `{"title":"测试标题","synopsis":"太短了","themes":["勇气"],"style":"冒险探索","educational_value":"x"}`
	_, err := outline.Parse(raw)
	if !errors.Is(err, outline.ErrSynopsisLength) {
		t.Fatalf("want ErrSynopsisLength, got %v", err)
	}
}

func TestParse_MarkdownFenced(t *testing.T) {
	raw := "```json\n" + `{"title":"测试标题","synopsis":"` + strings.Repeat("一", 70) + `","themes":["勇气"],"style":"冒险探索","educational_value":"x"}` + "\n```"
	_, err := outline.Parse(raw)
	if err != nil {
		t.Fatalf("markdown-fenced should parse: %v", err)
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestParse`
Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/outline/llm_prompt.go server/internal/service/outline/llm_parser.go server/internal/service/outline/llm_parser_test.go
git commit -m "feat(outline): LLM prompt template + strict JSON parser with validation (Plan 11A §5.1)"
```

---

### Task 16: `service/outline/safety_check.go` 输出端安全校验

**Files:**
- Create: `server/internal/service/outline/safety_check.go`
- Create: `server/internal/service/outline/safety_check_test.go`

- [ ] **Step 1: 写 OutlineSafetyCheck**

```go
// server/internal/service/outline/safety_check.go
package outline

import (
	"context"
	"errors"
	"strings"

	"github.com/aibao/server/internal/service/safety"
)

var (
	ErrSafetyRedline           = errors.New("outline safety: redline word hit")
	ErrSafetyChildFears        = errors.New("outline safety: child fears hit")
	ErrSafetyProtagonistMissing = errors.New("outline safety: child not the protagonist")
	ErrSafetyIPMisuse          = errors.New("outline safety: IP misuse (IP as protagonist)")
)

type SafetyCheckInput struct {
	Outline       RawOutline
	ChildNickname string
	ChildFears    []string // personalized fears from child profile / [[bootstrap-fears]]
	IPBlacklist   []string
	IPWhitelist   []string // 可作"陪伴角色"出现，但不能是主角
}

// SafetyCheckResult.Category aligns with safety.Category for unified reporting.
type SafetyCheckResult struct {
	OK       bool
	Reason   error
	Category string // "redline" / "fears_personalized" / "protagonist_missing" / "ip_misuse"
}

// Check inspects title + synopsis + educational_value for safety violations.
// Returns first violation; caller may attempt 1 repair retry.
func Check(ctx context.Context, matcher *safety.Matcher, in SafetyCheckInput) SafetyCheckResult {
	combined := in.Outline.Title + "\n" + in.Outline.Synopsis + "\n" + in.Outline.EducationalValue

	// 1. 红线词扫描（复用 safety.Matcher）
	if hit := matcher.FirstHit(combined); hit != "" {
		return SafetyCheckResult{Reason: ErrSafetyRedline, Category: "redline"}
	}

	// 2. 个性化害怕词扫描
	for _, fear := range in.ChildFears {
		if fear == "" { continue }
		if strings.Contains(combined, fear) {
			return SafetyCheckResult{Reason: ErrSafetyChildFears, Category: "fears_personalized"}
		}
	}

	// 3. 主角校验：synopsis 必须含 child nickname
	if in.ChildNickname != "" && !strings.Contains(in.Outline.Synopsis, in.ChildNickname) {
		return SafetyCheckResult{Reason: ErrSafetyProtagonistMissing, Category: "protagonist_missing"}
	}

	// 4. IP 同人化：synopsis 命中 IP 黑名单直接拒
	for _, ip := range in.IPBlacklist {
		if ip == "" { continue }
		if strings.Contains(in.Outline.Synopsis, ip) {
			return SafetyCheckResult{Reason: ErrSafetyIPMisuse, Category: "ip_misuse"}
		}
	}

	// 5. IP 白名单：可出现，但不能在 title（标题视为"主角位")
	for _, ip := range in.IPWhitelist {
		if ip == "" { continue }
		if strings.Contains(in.Outline.Title, ip) {
			return SafetyCheckResult{Reason: ErrSafetyIPMisuse, Category: "ip_misuse"}
		}
	}

	return SafetyCheckResult{OK: true}
}
```

> 注：`safety.Matcher.FirstHit` 应已在 Plan 3 提供；如未导出，新加 method 或 reuse 现有 `Check` 调用。

- [ ] **Step 2: 写 test**

```go
// server/internal/service/outline/safety_check_test.go
package outline_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/safety"
)

func newMatcher(t *testing.T, words ...string) *safety.Matcher {
	// 假设 safety.NewMatcherFromWords(words []string) 存在；
	// 如不存在，按 Plan 3 现有 API 调整（如 safety.NewMatcherForTest）
	m, err := safety.NewMatcherFromWords(words)
	if err != nil { t.Fatalf("matcher: %v", err) }
	return m
}

func TestSafetyCheck_OK(t *testing.T) {
	m := newMatcher(t, "血", "杀", "鬼")
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title:    "小宇的星空冒险",
			Synopsis: "小宇遇到爱宝" + strings.Repeat("一", 65),
			Themes:   []string{"勇气"},
			Style:    "冒险探索",
			EducationalValue: "学到勇敢",
		},
		ChildNickname: "小宇",
	})
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
}

func TestSafetyCheck_ProtagonistMissing(t *testing.T) {
	m := newMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title: "奥特曼大冒险", Synopsis: "奥特曼独自" + strings.Repeat("一", 60),
			Themes: []string{"勇气"}, Style: "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
	})
	if !errors.Is(res.Reason, outline.ErrSafetyProtagonistMissing) {
		t.Fatalf("want ProtagonistMissing, got %v", res.Reason)
	}
}

func TestSafetyCheck_Redline(t *testing.T) {
	m := newMatcher(t, "杀", "血")
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title: "小宇的冒险", Synopsis: "小宇遇到恶龙要杀" + strings.Repeat("一", 50),
			Themes: []string{"勇气"}, Style: "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
	})
	if !errors.Is(res.Reason, outline.ErrSafetyRedline) {
		t.Fatalf("want Redline, got %v", res.Reason)
	}
}

func TestSafetyCheck_ChildFears(t *testing.T) {
	m := newMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title: "小宇的冒险", Synopsis: "小宇遇到一只大狗" + strings.Repeat("一", 50),
			Themes: []string{"勇气"}, Style: "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
		ChildFears:    []string{"大狗"},
	})
	if !errors.Is(res.Reason, outline.ErrSafetyChildFears) {
		t.Fatalf("want ChildFears, got %v", res.Reason)
	}
}

func TestSafetyCheck_IPWhitelistInTitle(t *testing.T) {
	m := newMatcher(t)
	res := outline.Check(context.Background(), m, outline.SafetyCheckInput{
		Outline: outline.RawOutline{
			Title: "小宇和奥特曼的冒险", // 奥特曼出现在标题 → IP 抢主角
			Synopsis: "小宇和爱宝一起" + strings.Repeat("一", 60),
			Themes: []string{"勇气"}, Style: "冒险探索",
			EducationalValue: "x",
		},
		ChildNickname: "小宇",
		IPWhitelist:   []string{"奥特曼"},
	})
	if !errors.Is(res.Reason, outline.ErrSafetyIPMisuse) {
		t.Fatalf("want IPMisuse, got %v", res.Reason)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestSafetyCheck`
Expected: PASS (5 tests)

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/outline/safety_check.go server/internal/service/outline/safety_check_test.go
git commit -m "feat(outline): OutlineSafetyCheck — redline/fears/protagonist/IP (Plan 11A §5.3)"
```

---

### Task 17: `service/outline/service.go` Preview 主编排

**Files:**
- Create: `server/internal/service/outline/service.go`
- Create: `server/internal/service/outline/service_test.go`
- Modify: `server/internal/metrics/metrics.go`（新增 outline_outcome_total, outline_safety_repair_total, outline_preview_duration_seconds）

- [ ] **Step 1: 加 outline metrics**

```go
// 追加到 server/internal/metrics/metrics.go
var (
	OutlineOutcomeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outline_outcome_total",
		Help: "Outline lifecycle outcomes",
	}, []string{"outcome"}) // pending / accepted / refreshed / expired

	OutlineSafetyRepairTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outline_safety_repair_total",
		Help: "OutlineSafetyCheck repair retries",
	}, []string{"category", "result"}) // result: success / give_up

	OutlinePreviewDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "outline_preview_duration_seconds",
		Help:    "End-to-end preview LLM call duration",
		Buckets: []float64{0.5, 1, 2, 3, 5, 8, 15},
	})
)
```

- [ ] **Step 2: 写 Service.Preview**

```go
// server/internal/service/outline/service.go
package outline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/idhash"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/traceid"
	"github.com/aibao/server/internal/service/cost"
	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/aibao/server/internal/service/safety"
)

const sceneSeedCount = 80

type Service struct {
	llm         llm.Client
	llmModel    string // "doubao-1.5-lite-32k"
	matcher     *safety.Matcher
	preCheck    *safety.PreChecker // existing in Plan 3
	cache       *Cache
	events      *EventStore
	recorder    *cost.Recorder
	idHasher    *idhash.Hasher
}

type Deps struct {
	LLM      llm.Client
	LLMModel string
	Matcher  *safety.Matcher
	PreCheck *safety.PreChecker
	Cache    *Cache
	Events   *EventStore
	Recorder *cost.Recorder
	IDHasher *idhash.Hasher
}

func NewService(d Deps) *Service {
	return &Service{
		llm: d.LLM, llmModel: d.LLMModel, matcher: d.Matcher, preCheck: d.PreCheck,
		cache: d.Cache, events: d.Events, recorder: d.Recorder, idHasher: d.IDHasher,
	}
}

type PreviewInput struct {
	UserID        int64
	ChildID       int64
	ChildNickname string
	ChildAge      int
	ChildFears    []string
	IPBlacklist   []string
	IPWhitelist   []string
	Prompt        string
	DurationMin   int

	// For refresh: non-empty = re-generate with same prompt+duration, new outline_id
	ParentOutlineID string
}

type PreviewResult struct {
	OutlineID string
	Outline   outlinecontract.Outline
	ExpiresAt time.Time
}

// Preview runs the full outline preview pipeline:
// PreCheck → LLM (with 1 schema repair retry) → OutlineSafetyCheck (with 1 safety repair retry)
// → inject scene_seed → Redis Set → outline_events pending → return.
func (s *Service) Preview(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	start := time.Now()
	traceID := traceid.From(ctx)

	// 1. Input PreCheck (full)
	if reason, cat, _ := s.preCheck.Check(ctx, in.Prompt, in.IPBlacklist); reason != "" {
		return nil, apperr.SafetyRejected(reason, cat)
	}

	// 2. LLM call (with 1 schema repair retry)
	sys, usr := BuildPrompt(OutlinePromptInput{
		ChildNickname: in.ChildNickname, ChildAge: in.ChildAge,
		UserPrompt: in.Prompt, DurationMin: in.DurationMin,
	})
	var ro *RawOutline
	var llmResp *llm.GenerateResponse
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		messages := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: usr}}
		if attempt == 2 && lastErr != nil {
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("上次返回不合规：%v。请重新返回严格符合 schema 的 JSON。", lastErr)})
		}
		resp, err := s.llm.Generate(ctx, llm.GenerateRequest{
			Model: s.llmModel, Messages: messages, Temperature: 0.8,
		})
		if err != nil {
			lastErr = err
			continue
		}
		llmResp = resp
		s.recordCost(ctx, in, traceID, "outline", "llm_call", attempt, resp, "ok")

		ro, lastErr = Parse(resp.Text)
		if lastErr == nil { break }
	}
	if ro == nil {
		logger.From(ctx).Warn("outline.preview.llm_failed", "err", lastErr)
		return nil, apperr.LLMFailed(lastErr)
	}

	// 3. OutlineSafetyCheck (with 1 repair retry)
	safetyIn := SafetyCheckInput{
		Outline: *ro, ChildNickname: in.ChildNickname, ChildFears: in.ChildFears,
		IPBlacklist: in.IPBlacklist, IPWhitelist: in.IPWhitelist,
	}
	res := Check(ctx, s.matcher, safetyIn)
	if !res.OK {
		metrics.OutlineSafetyRepairTotal.WithLabelValues(res.Category, "retry").Inc()
		// 1 repair retry: ask LLM to avoid the specific category
		hint := fmt.Sprintf("上一版本命中安全规则（%s），请重新生成，避免该类内容。", res.Category)
		messages := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: usr}, {Role: "user", Content: hint}}
		resp, err := s.llm.Generate(ctx, llm.GenerateRequest{Model: s.llmModel, Messages: messages, Temperature: 0.6})
		if err == nil {
			s.recordCost(ctx, in, traceID, "outline", "safety_repair", 1, resp, "ok")
			if ro2, perr := Parse(resp.Text); perr == nil {
				safetyIn.Outline = *ro2
				res2 := Check(ctx, s.matcher, safetyIn)
				if res2.OK {
					ro = ro2
					metrics.OutlineSafetyRepairTotal.WithLabelValues(res.Category, "success").Inc()
				} else {
					metrics.OutlineSafetyRepairTotal.WithLabelValues(res.Category, "give_up").Inc()
					return nil, apperr.SafetyRejected(res2.Reason.Error(), res2.Category)
				}
			}
		} else {
			metrics.OutlineSafetyRepairTotal.WithLabelValues(res.Category, "give_up").Inc()
			return nil, apperr.SafetyRejected(res.Reason.Error(), res.Category)
		}
	}

	// 4. Inject scene_seed + group/variant metadata
	outlineID := newOutlineID()
	groupID := outlineID
	variantIdx := 0
	if in.ParentOutlineID != "" {
		// refresh path: same group as parent, variant++
		if parent, err := s.cache.Get(ctx, in.ParentOutlineID); err == nil {
			groupID = parent.OutlineGroupID
			variantIdx = parent.VariantIndex + 1
		}
	}
	sceneSeed := pickSceneSeed(outlineID)

	resolved := outlinecontract.Outline{
		OutlineID:            outlineID,
		Title:                ro.Title,
		Synopsis:             ro.Synopsis,
		Themes:               ro.Themes,
		Style:                ro.Style,
		EducationalValue:     ro.EducationalValue,
		DurationMin:          in.DurationMin,
		SceneSeed:            sceneSeed,
		OutlineGroupID:       groupID,
		VariantIndex:         variantIdx,
		ParentOutlineID:      in.ParentOutlineID,
		OutlinePromptVersion: OutlinePromptVersion,
	}

	// 5. Persist: Redis + outline_events pending
	co := NewCachedOutline(resolved, in.UserID, in.ChildID, in.Prompt)
	if err := s.cache.Set(ctx, co); err != nil {
		return nil, fmt.Errorf("cache set: %w", err)
	}
	if err := s.events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: groupID,
		UserID: in.UserID, ChildIDHash: s.idHasher.Hash("child", in.ChildID),
		Outcome: OutcomePending, OutlinePromptVersion: OutlinePromptVersion,
		DurationMin: in.DurationMin, TraceID: traceID,
	}); err != nil {
		// roll back Redis (best-effort)
		_ = s.cache.Invalidate(ctx, outlineID)
		return nil, fmt.Errorf("events append: %w", err)
	}
	metrics.OutlineOutcomeTotal.WithLabelValues(OutcomePending).Inc()
	metrics.OutlinePreviewDurationSeconds.Observe(time.Since(start).Seconds())

	return &PreviewResult{
		OutlineID: outlineID,
		Outline:   resolved,
		ExpiresAt: time.Now().Add(cacheTTL),
	}, nil
}

func (s *Service) recordCost(ctx context.Context, in PreviewInput, traceID, purpose, stage string, attempt int, resp *llm.GenerateResponse, outcome string) {
	if resp == nil { return }
	userID := in.UserID
	_ = s.recorder.Record(ctx, cost.RecordInput{
		EventID:              fmt.Sprintf("%s:%s:%s:%d", traceID, purpose, stage, attempt),
		UserID:               &userID,
		ChildIDHash:          s.idHasher.Hash("child", in.ChildID),
		Purpose:              purpose,
		Provider:             resp.Provider,
		Model:                resp.Model,
		Usage:                pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
		Outcome:              outcome,
		DurationMs:           int(resp.Latency.Milliseconds()),
		OutlinePromptVersion: OutlinePromptVersion,
		TraceID:              traceID,
	})
}

func newOutlineID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "ol_" + hex.EncodeToString(b)
}

// pickSceneSeed deterministically picks one of 80 seeds based on outline_id hash.
// 复用 Plan 9c SceneSeed pool (已存在于 prompt builder, 此处假设有 GetSceneSeed(idx) helper)
func pickSceneSeed(outlineID string) string {
	idx := int(outlineID[3]) % sceneSeedCount
	return fmt.Sprintf("S%03d", idx)
}

var _ = errors.New // keep import if unused above
```

> 注：`apperr.SafetyRejected` / `apperr.LLMFailed` 假设 Plan 3 已有，否则按现有 apperr API 调整。`pickSceneSeed` 简化为 hash-based，正式实施时复用 Plan 9c `prompt.SceneSeeds` 表。

- [ ] **Step 3: 写 service test（Mock LLM + testcontainers Redis+PG）**

```go
// server/internal/service/outline/service_test.go
package outline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/idhash"
	"github.com/aibao/server/internal/service/cost"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/safety"
	"github.com/spf13/viper"
)

type fakeLLM struct {
	responses []string
	idx       int
}

func (f *fakeLLM) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	r := f.responses[f.idx]
	f.idx++
	return &llm.GenerateResponse{Text: r, InputTokens: 600, OutputTokens: 400, Provider: "doubao", Model: "doubao-1.5-lite-32k"}, nil
}
func (f *fakeLLM) HealthCheck(ctx context.Context) error { return nil }

func TestPreview_Happy(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{}, &model.CostEvent{})
	rdb := startTestRedis(t)
	v := viper.New(); v.SetConfigType("yaml"); v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test
  entries:
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60`))
	pb, _ := pkgcost.LoadFromViper(v)
	rec := cost.NewRecorder(pb)
	matcher, _ := safety.NewMatcherFromWords([]string{"血", "杀"})
	pre := safety.NewPreCheckerFromMatcher(matcher) // adjust to existing constructor

	svc := outline.NewService(outline.Deps{
		LLM: &fakeLLM{responses: []string{`{"title":"小宇的冒险","synopsis":"小宇遇到爱宝` + strings.Repeat("一", 60) + `","themes":["勇气"],"style":"冒险探索","educational_value":"学到勇敢"}`}},
		LLMModel: "doubao-1.5-lite-32k",
		Matcher: matcher, PreCheck: pre,
		Cache: outline.NewCache(rdb),
		Events: outline.NewEventStore(db),
		Recorder: rec,
		IDHasher: idhash.New("test-secret"),
	})

	res, err := svc.Preview(context.Background(), outline.PreviewInput{
		UserID: 1, ChildID: 7,
		ChildNickname: "小宇", ChildAge: 5,
		Prompt: "想听冒险故事", DurationMin: 5,
	})
	if err != nil { t.Fatalf("preview: %v", err) }
	if !strings.HasPrefix(res.OutlineID, "ol_") { t.Errorf("bad outline_id: %s", res.OutlineID) }
	if res.Outline.Style != "冒险探索" { t.Errorf("style: %s", res.Outline.Style) }
}
```

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestPreview`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/outline/service.go server/internal/service/outline/service_test.go server/internal/metrics/metrics.go
git commit -m "feat(outline): Service.Preview with 2-stage repair retry + cost recording (Plan 11A §7.1)"
```

---

### Task 18: `service/outline/resolver_impl.go` 实现 OutlineResolver

**Files:**
- Create: `server/internal/service/outline/resolver_impl.go`
- Create: `server/internal/service/outline/resolver_impl_test.go`

- [ ] **Step 1: 写 ResolverImpl**

```go
// server/internal/service/outline/resolver_impl.go
package outline

import (
	"context"
	"errors"

	"github.com/aibao/server/internal/service/outlinecontract"
)

type ResolverImpl struct {
	cache  *Cache
	events *EventStore
}

func NewResolver(cache *Cache, events *EventStore) *ResolverImpl {
	return &ResolverImpl{cache: cache, events: events}
}

// Resolve enforces user_id + child_id + outline_id triple ownership (spec §5.2).
// Also checks outline_events: if latest outcome is terminal (accepted/refreshed/expired),
// return ErrOutlineExpired (preventing replay attacks).
func (r *ResolverImpl) Resolve(ctx context.Context, outlineID string, userID, childID int64) (*outlinecontract.Outline, error) {
	co, err := r.cache.Get(ctx, outlineID)
	if errors.Is(err, ErrCacheMiss) {
		return nil, outlinecontract.ErrOutlineExpired
	}
	if err != nil {
		return nil, err
	}
	if co.UserID != userID || co.ChildID != childID {
		return nil, outlinecontract.ErrOutlineForbidden
	}
	latest, err := r.events.LatestOutcome(ctx, outlineID)
	if err != nil {
		return nil, err
	}
	if latest != OutcomePending {
		// already accepted (replay) / refreshed (stale) / expired
		return nil, outlinecontract.ErrOutlineExpired
	}
	out := co.Outline
	return &out, nil
}

// 编译期断言实现接口
var _ outlinecontract.OutlineResolver = (*ResolverImpl)(nil)
```

- [ ] **Step 2: 写 test**

```go
// server/internal/service/outline/resolver_impl_test.go
package outline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
)

func TestResolver_OK(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{})
	rdb := startTestRedis(t)
	cache := outline.NewCache(rdb); events := outline.NewEventStore(db)
	r := outline.NewResolver(cache, events)

	co := outline.NewCachedOutline(outlinecontract.Outline{OutlineID: "ol_x", Title: "t"}, 42, 7, "p")
	cache.Set(context.Background(), co)
	events.Append(context.Background(), model.OutlineEvent{OutlineID: "ol_x", UserID: 42, ChildIDHash: "h", Outcome: outline.OutcomePending, OutlineGroupID: "g"})

	out, err := r.Resolve(context.Background(), "ol_x", 42, 7)
	if err != nil { t.Fatalf("resolve: %v", err) }
	if out.Title != "t" { t.Errorf("title: %s", out.Title) }
}

func TestResolver_OwnershipMismatch_User(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{})
	rdb := startTestRedis(t)
	cache := outline.NewCache(rdb); events := outline.NewEventStore(db)
	r := outline.NewResolver(cache, events)
	cache.Set(context.Background(), outline.NewCachedOutline(outlinecontract.Outline{OutlineID: "ol_y"}, 42, 7, ""))
	events.Append(context.Background(), model.OutlineEvent{OutlineID: "ol_y", UserID: 42, Outcome: outline.OutcomePending, OutlineGroupID: "g", ChildIDHash: "h"})
	_, err := r.Resolve(context.Background(), "ol_y", 99, 7) // wrong user
	if !errors.Is(err, outlinecontract.ErrOutlineForbidden) {
		t.Fatalf("want Forbidden, got %v", err)
	}
}

func TestResolver_AlreadyAccepted_Replay(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{})
	rdb := startTestRedis(t)
	cache := outline.NewCache(rdb); events := outline.NewEventStore(db)
	r := outline.NewResolver(cache, events)
	cache.Set(context.Background(), outline.NewCachedOutline(outlinecontract.Outline{OutlineID: "ol_z"}, 42, 7, ""))
	events.Append(context.Background(), model.OutlineEvent{OutlineID: "ol_z", UserID: 42, Outcome: outline.OutcomeAccepted, OutlineGroupID: "g", ChildIDHash: "h"})
	_, err := r.Resolve(context.Background(), "ol_z", 42, 7)
	if !errors.Is(err, outlinecontract.ErrOutlineExpired) {
		t.Fatalf("want Expired (replay defense), got %v", err)
	}
}

func TestResolver_CacheMiss(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{})
	rdb := startTestRedis(t)
	r := outline.NewResolver(outline.NewCache(rdb), outline.NewEventStore(db))
	_, err := r.Resolve(context.Background(), "ol_nonexistent", 1, 1)
	if !errors.Is(err, outlinecontract.ErrOutlineExpired) {
		t.Fatalf("want Expired (cache miss), got %v", err)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd server && go test ./internal/service/outline/... -v -run TestResolver`
Expected: PASS (4 tests)

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/outline/resolver_impl.go server/internal/service/outline/resolver_impl_test.go
git commit -m "feat(outline): ResolverImpl with triple-ownership + replay defense (Plan 11A §5.2)"
```

---

### Task 19: API handler — `POST /api/v1/outlines/preview`

**Files:**
- Create: `server/internal/api/outline.go`
- Modify: `server/internal/router.go`（注册路由）
- Modify: `server/internal/pkg/errors/errors.go`（如缺 SafetyRejected/LLMFailed/ConflictingModes，加之）

- [ ] **Step 1: 加 apperr 错误 code**

```go
// server/internal/pkg/errors/errors.go (附加)
var (
	ErrSafetyRejected    = &AppError{Code: "safety_rejected", HTTPStatus: 422}
	ErrLLMFailed         = &AppError{Code: "llm_failed", HTTPStatus: 500}
	ErrOutlineExpired    = &AppError{Code: "outline_expired", HTTPStatus: 410}
	ErrConflictingModes  = &AppError{Code: "conflicting_modes", HTTPStatus: 400}
	ErrOutlineForbidden  = &AppError{Code: "forbidden", HTTPStatus: 403}
)

func SafetyRejected(reason, category string) error {
	return &AppError{Code: "safety_rejected", HTTPStatus: 422, Message: reason, Extra: map[string]any{"category": category}}
}
func LLMFailed(cause error) error {
	return &AppError{Code: "llm_failed", HTTPStatus: 500, Message: cause.Error()}
}
```

> 假设 AppError 已有 Code/HTTPStatus/Message/Extra 字段；若 Extra 不存在则加之。

- [ ] **Step 2: 写 outline handler**

```go
// server/internal/api/outline.go
package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/gin-gonic/gin"
)

type OutlineHandler struct {
	svc       *outline.Service
	cache     *outline.Cache
	events    *outline.EventStore
	childRepo repository.ChildRepository
}

func NewOutlineHandler(svc *outline.Service, cache *outline.Cache, events *outline.EventStore, childRepo repository.ChildRepository) *OutlineHandler {
	return &OutlineHandler{svc: svc, cache: cache, events: events, childRepo: childRepo}
}

type previewReq struct {
	ChildID     int64  `json:"child_id" binding:"required"`
	Prompt      string `json:"prompt" binding:"required,min=1,max=200"`
	DurationMin int    `json:"duration_min" binding:"required,oneof=3 5 8"`
}

type previewResp struct {
	OutlineID string                  `json:"outline_id"`
	Outline   outlinecontract.Outline `json:"outline"`
	ExpiresAt time.Time               `json:"expires_at"`
}

// POST /api/v1/outlines/preview
func (h *OutlineHandler) Preview(c *gin.Context) {
	var req previewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.WriteJSON(c, apperr.BadRequest(err.Error()))
		return
	}
	userID := c.MustGet("user_id").(int64)

	// fetch child profile
	child, err := h.childRepo.FindByID(c.Request.Context(), req.ChildID)
	if err != nil || child.UserID != userID {
		apperr.WriteJSON(c, apperr.Forbidden("child not yours"))
		return
	}

	res, err := h.svc.Preview(c.Request.Context(), outline.PreviewInput{
		UserID:        userID,
		ChildID:       req.ChildID,
		ChildNickname: child.Nickname,
		ChildAge:      child.Age,
		ChildFears:    child.Fears(), // helper on Child model
		IPBlacklist:   safetyConfig.IPBlacklist, // wire from config
		IPWhitelist:   safetyConfig.IPWhitelist,
		Prompt:        req.Prompt,
		DurationMin:   req.DurationMin,
	})
	if err != nil {
		apperr.WriteJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, previewResp{
		OutlineID: res.OutlineID, Outline: res.Outline, ExpiresAt: res.ExpiresAt,
	})
}
```

> `apperr.WriteJSON` / `apperr.BadRequest` / `apperr.Forbidden` 假设 Plan 1/2 已有。

- [ ] **Step 3: 注册路由 + 限流**

```go
// server/internal/router.go (摘录)
outlineH := api.NewOutlineHandler(outlineSvc, outlineCache, outlineEvents, childRepo)

// outline 端点共享桶 5/min
outlineGroup := r.Group("/api/v1/outlines", middleware.RateLimitPerUser("outline_bucket", 5, time.Minute))
outlineGroup.Use(jwtAuth)
outlineGroup.POST("/preview", outlineH.Preview)
outlineGroup.POST("/:id/refresh", outlineH.Refresh)  // Task 20
```

- [ ] **Step 4: 集成测试（真 PG + Redis + Mock LLM）**

```bash
cd server && go test ./internal/api/... -v -run TestOutlinePreview
```

写一个 minimal API test:

```go
// server/internal/api/outline_test.go (skeleton — 完整版按现有 api test pattern 写)
func TestOutlinePreview_Happy(t *testing.T) {
	// 拉起 testcontainers PG+Redis，wire Service with fakeLLM
	// HTTP POST /api/v1/outlines/preview with valid body
	// expect 200 + outline_id in response
}
func TestOutlinePreview_InvalidDuration(t *testing.T) {
	// duration_min=99 → 400 bad_request
}
func TestOutlinePreview_NotYourChild(t *testing.T) {
	// child_id 不属于当前 user → 403 forbidden
}
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/api/outline.go server/internal/api/outline_test.go server/internal/router.go server/internal/pkg/errors/
git commit -m "feat(api): POST /outlines/preview with 5/min rate limit + child ownership (Plan 11A §6.1)"
```

---

### Task 20: API handler — `POST /api/v1/outlines/:id/refresh`

**Files:**
- Modify: `server/internal/api/outline.go`（加 Refresh method）

- [ ] **Step 1: 写 Refresh handler**

```go
// 追加到 server/internal/api/outline.go
type refreshReq struct {
	// no body fields — outline_id from URL; same prompt/child as parent
}

// POST /api/v1/outlines/:id/refresh
func (h *OutlineHandler) Refresh(c *gin.Context) {
	parentID := c.Param("id")
	userID := c.MustGet("user_id").(int64)

	// fetch parent from cache
	parent, err := h.cache.Get(c.Request.Context(), parentID)
	if errors.Is(err, outline.ErrCacheMiss) {
		apperr.WriteJSON(c, apperr.NotFound("outline not found or expired"))
		return
	}
	if err != nil {
		apperr.WriteJSON(c, apperr.Internal(err))
		return
	}
	if parent.UserID != userID {
		apperr.WriteJSON(c, apperr.Forbidden("not your outline"))
		return
	}

	// invalidate parent + write refreshed event
	_ = h.cache.Invalidate(c.Request.Context(), parentID)
	_ = h.events.Append(c.Request.Context(), model.OutlineEvent{
		OutlineID: parentID, OutlineGroupID: parent.OutlineGroupID,
		UserID: userID, ChildIDHash: /* hasher */ "",
		Outcome: outline.OutcomeRefreshed,
		OutlinePromptVersion: parent.OutlinePromptVersion,
		DurationMin: parent.DurationMin, TraceID: traceid.From(c.Request.Context()),
	})

	// regenerate (new outline_id, same group)
	child, _ := h.childRepo.FindByID(c.Request.Context(), parent.ChildID)
	res, err := h.svc.Preview(c.Request.Context(), outline.PreviewInput{
		UserID: userID, ChildID: parent.ChildID,
		ChildNickname: child.Nickname, ChildAge: child.Age, ChildFears: child.Fears(),
		Prompt: parent.PromptText, DurationMin: parent.DurationMin,
		ParentOutlineID: parentID,
	})
	if err != nil {
		apperr.WriteJSON(c, err)
		return
	}
	c.JSON(200, previewResp{OutlineID: res.OutlineID, Outline: res.Outline, ExpiresAt: res.ExpiresAt})
}
```

> `child.Fears()` 需要在 Child model 加 helper（如未存在）：从 `profile` JSONB 解 `[[bootstrap-fears]]`。

- [ ] **Step 2: Test**

```go
func TestOutlineRefresh_Happy(t *testing.T) {
	// 先 POST preview 拿 parent outline_id
	// 再 POST /outlines/{parent}/refresh
	// expect 200 + 新 outline_id != parent + outline_events 中 parent 已 refreshed
}
func TestOutlineRefresh_NotYours(t *testing.T) {
	// 用 user2 token 刷 user1 的 outline → 403
}
func TestOutlineRefresh_NotFound(t *testing.T) {
	// 不存在的 id → 404
}
```

- [ ] **Step 3: 运行测试**

Run: `cd server && go test ./internal/api/... -v -run TestOutlineRefresh`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/api/outline.go server/internal/api/outline_test.go
git commit -m "feat(api): POST /outlines/:id/refresh — invalidate parent + same group new variant (Plan 11A §6.2)"
```

---

### Task 21: 修改 `/stories/generate` — outline_id + 互斥校验

**Files:**
- Modify: `server/internal/api/story.go`
- Modify: `server/internal/service/story/orchestrator.go`（注入 OutlineResolver；step 0 HydrateFromOutline）

- [ ] **Step 1: api/story.go 加 outline_id + storyline_id 互斥校验**

```go
// server/internal/api/story.go (摘录: handler 内)
type generateReq struct {
	ChildID     int64   `json:"child_id" binding:"required"`
	OutlineID   string  `json:"outline_id"`           // Plan 11A: 新增
	StorylineID *int64  `json:"storyline_id"`         // Plan 8: 已有
	OutlineOverrides *outlineOverrides `json:"outline_overrides"` // Plan 11A
	// 旧字段（兼容期内仍接受）
	DurationMin int    `json:"duration_min"`
	Style       string `json:"style"`
	Topic       string `json:"topic"`
	Prompt      string `json:"prompt"`
}

type outlineOverrides struct {
	Style            string   `json:"style"`
	Themes           []string `json:"themes"`
	EducationalValue string   `json:"educational_value"`
}

func (h *StoryHandler) Generate(c *gin.Context) {
	var req generateReq
	if err := c.ShouldBindJSON(&req); err != nil { apperr.WriteJSON(c, apperr.BadRequest(err.Error())); return }

	// 互斥校验：outline_id + storyline_id 不能同时存在 (Plan 11A §6.6/§10.1)
	if req.OutlineID != "" && req.StorylineID != nil {
		apperr.WriteJSON(c, &apperr.AppError{Code: "conflicting_modes", HTTPStatus: 400, Message: "outline_id and storyline_id are mutually exclusive"})
		return
	}

	// outline_overrides 白名单校验（Plan 11A §6.3）
	if req.OutlineOverrides != nil && req.OutlineID == "" {
		apperr.WriteJSON(c, apperr.BadRequest("outline_overrides requires outline_id"))
		return
	}

	// 调 orchestrator (Generate 内部决定走哪条路径)
	res, err := h.orch.Generate(c.Request.Context(), story.GenerateInput{
		UserID:           c.MustGet("user_id").(int64),
		ChildID:          req.ChildID,
		OutlineID:        req.OutlineID,
		StorylineID:      req.StorylineID,
		OutlineOverrides: toServiceOverrides(req.OutlineOverrides),
		// 旧字段
		DurationMin: req.DurationMin, Style: req.Style, Topic: req.Topic, Prompt: req.Prompt,
	})
	// ...
}
```

- [ ] **Step 2: Orchestrator step 0 — HydrateFromOutline**

```go
// server/internal/service/story/orchestrator.go (摘录)
type Orchestrator struct {
	// ... existing ...
	outlineResolver outlinecontract.OutlineResolver
	outlineEvents   *outline.EventStore // for marking accepted
	recorder        *cost.Recorder
	idHasher        *idhash.Hasher
}

func (o *Orchestrator) Generate(ctx context.Context, in GenerateInput) (*GenerateResult, error) {
	// Step 0a: storyline + outline 互斥（双重防御，handler 已查一次）
	if in.OutlineID != "" && in.StorylineID != nil {
		return nil, apperr.AppErrorFrom("conflicting_modes", 400, "")
	}

	// Step 0b: HydrateFromOutline (Plan 11A)
	if in.OutlineID != "" {
		ol, err := o.outlineResolver.Resolve(ctx, in.OutlineID, in.UserID, in.ChildID)
		if err != nil {
			switch {
			case errors.Is(err, outlinecontract.ErrOutlineExpired):
				return nil, apperr.AppErrorFrom("outline_expired", 410, "")
			case errors.Is(err, outlinecontract.ErrOutlineForbidden):
				return nil, apperr.AppErrorFrom("forbidden", 403, "")
			default:
				return nil, apperr.Internal(err)
			}
		}
		// apply outline overrides (whitelist only: style/themes/educational_value)
		applyOverrides(ol, in.OutlineOverrides)

		in.Style = ol.Style
		in.Topic = strings.Join(ol.Themes, ",")
		in.DurationMin = ol.DurationMin
		in.SceneSeed = ol.SceneSeed
		in.TitleHint = ol.Title
		in.SynopsisHint = ol.Synopsis
		in.EducationalValueHint = ol.EducationalValue
		in.OutlineGroupID = ol.OutlineGroupID
		in.OutlinePromptVersion = ol.OutlinePromptVersion

		// Mark accepted (after successful resolve, before LLM call)
		childHash := o.idHasher.Hash("child", in.ChildID)
		_ = o.outlineEvents.Append(ctx, model.OutlineEvent{
			OutlineID: in.OutlineID, OutlineGroupID: ol.OutlineGroupID,
			UserID: in.UserID, ChildIDHash: childHash,
			Outcome: outline.OutcomeAccepted, OutlinePromptVersion: ol.OutlinePromptVersion,
			DurationMin: ol.DurationMin, TraceID: traceid.From(ctx),
		})
		metrics.OutlineOutcomeTotal.WithLabelValues(outline.OutcomeAccepted).Inc()
	}

	// Step 1..5: existing pipeline (uses TitleHint/SynopsisHint/EducationalValueHint
	// injected into prompt builder — see Task 22)
	// ...
}

func applyOverrides(o *outlinecontract.Outline, ov *OutlineOverrides) {
	if ov == nil { return }
	if ov.Style != "" { o.Style = ov.Style }
	if len(ov.Themes) > 0 { o.Themes = ov.Themes }
	if ov.EducationalValue != "" { o.EducationalValue = ov.EducationalValue }
}
```

- [ ] **Step 3: 编译 + 单测**

```bash
cd server && go build ./... && go test ./internal/service/story/... ./internal/api/...
```

Expected: 通过。注意更新 Orchestrator 构造函数所有调用点。

- [ ] **Step 4: 写互斥 + 410 集成测试**

```go
// server/internal/api/story_test.go (附加)
func TestGenerate_OutlineAndStorylineConflict(t *testing.T) {
	// POST /stories/generate with both outline_id + storyline_id
	// expect 400 conflicting_modes
}
func TestGenerate_OutlineExpired(t *testing.T) {
	// POST /outlines/preview, wait until TTL expired (or manual cache.Invalidate)
	// POST /stories/generate with that outline_id
	// expect 410 outline_expired
}
func TestGenerate_OutlineOverridesWhitelist(t *testing.T) {
	// POST /stories/generate with outline_overrides containing duration_min=99
	// expect 400 bad_request (whitelist 排除 duration_min)
}
```

- [ ] **Step 5: Commit**

```bash
git add server/internal/api/story.go server/internal/service/story/orchestrator.go server/internal/api/story_test.go
git commit -m "feat(story): step 0 HydrateFromOutline + outline/storyline mutex + overrides whitelist (Plan 11A §6.6 §7.3 §10.1)"
```

---

### Task 22: 正文 prompt 注入 TitleHint/SynopsisHint/EducationalValueHint

**Files:**
- Modify: `server/internal/service/story/prompt/builder.go`（或现有 prompt 模板文件）
- Modify: `server/internal/service/story/orchestrator.go`（BuildInput 字段）

- [ ] **Step 1: 加 BuildInput 字段**

```go
// server/internal/service/story/prompt/builder.go (BuildInput struct 附加)
type BuildInput struct {
	// ... existing fields ...

	// Plan 11A: 大纲注入（仅当来自 outline 路径时填充）
	TitleHint            string
	SynopsisHint         string
	EducationalValueHint string
}
```

- [ ] **Step 2: 模板加大纲段（system prompt 末尾）**

```go
// system_prompt.tmpl (Go template，附加在末尾)
{{ if .TitleHint }}

## 本故事的预先设定（家长已确认）
- 标题：{{ .TitleHint }}
- 梗概：{{ .SynopsisHint }}
- 教育意义目标：{{ .EducationalValueHint }}
请把以上设定作为故事骨架展开为完整故事，**不要偏离梗概的主要情节走向**。
{{ end }}
```

- [ ] **Step 3: 单测**

```go
// server/internal/service/story/prompt/builder_test.go (附加)
func TestBuild_WithOutlineHints(t *testing.T) {
	in := BuildInput{
		// ... existing minimal fields ...
		TitleHint:            "小宇的星空冒险",
		SynopsisHint:         "小宇遇到爱宝，他们一起穿越到星空…",
		EducationalValueHint: "学到勇气与团队合作",
	}
	sys, _, _ := Build(in)
	if !strings.Contains(sys, "本故事的预先设定") {
		t.Errorf("expected outline hint section in system prompt")
	}
	if !strings.Contains(sys, "小宇的星空冒险") {
		t.Errorf("title not injected")
	}
}

func TestBuild_WithoutOutlineHints(t *testing.T) {
	in := BuildInput{
		// ... existing minimal fields, no TitleHint ...
	}
	sys, _, _ := Build(in)
	if strings.Contains(sys, "本故事的预先设定") {
		t.Errorf("outline hint section should NOT appear when TitleHint empty")
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd server && go test ./internal/service/story/prompt/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/story/prompt/
git commit -m "feat(prompt): inject outline TitleHint/SynopsisHint/EducationalValueHint into story system prompt (Plan 11A §7.3)"
```

---

### Task 23: Housekeeping — 兜底过期扫描 + 主动驱动

**Files:**
- Create: `server/internal/service/outline/housekeeping.go`
- Create: `server/internal/service/outline/housekeeping_test.go`
- Modify: `server/cmd/server/main.go`（启 goroutine）
- Modify: `server/internal/api/story.go` (List handler) + `server/internal/api/heartbeat.go`（注入 SweepUser hook）

- [ ] **Step 1: 写 Housekeeper**

```go
// server/internal/service/outline/housekeeping.go
package outline

import (
	"context"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
)

const (
	houseKeepInterval = 10 * time.Minute
	pendingThreshold  = 10 * time.Minute
	userSweepGrace    = 5*time.Minute + 30*time.Second // 略晚于 Redis TTL
	batchLimit        = 200
)

type Housekeeper struct {
	events *EventStore
}

func NewHousekeeper(events *EventStore) *Housekeeper {
	return &Housekeeper{events: events}
}

// Run blocks until ctx cancel; periodic full-table sweep (Plan 11A §5.5 兜底).
func (h *Housekeeper) Run(ctx context.Context) {
	tick := time.NewTicker(houseKeepInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.sweepAll(ctx)
		}
	}
}

func (h *Housekeeper) sweepAll(ctx context.Context) {
	threshold := time.Now().Add(-pendingThreshold)
	rows, err := h.events.ScanPendingOlderThan(ctx, threshold, nil, batchLimit)
	if err != nil {
		logger.From(ctx).Error("outline.housekeeping.scan_failed", "err", err)
		return
	}
	for _, row := range rows {
		_ = h.events.MarkExpiredIfPending(ctx, model.OutlineEvent{
			OutlineID: row.OutlineID, OutlineGroupID: row.OutlineGroupID,
			UserID: row.UserID, ChildIDHash: row.ChildIDHash,
			OutlinePromptVersion: row.OutlinePromptVersion,
			DurationMin: row.DurationMin, TraceID: row.TraceID,
		})
		metrics.OutlineOutcomeTotal.WithLabelValues(OutcomeExpired).Inc()
	}
	if len(rows) > 0 {
		logger.From(ctx).Info("outline.housekeeping.swept", "expired_count", len(rows))
	}
}

// SweepUser — 主动驱动（Plan 11A §5.5 A2）：用户进 /stories 或 /heartbeat 时调用
func (h *Housekeeper) SweepUser(ctx context.Context, userID int64) {
	threshold := time.Now().Add(-userSweepGrace)
	uid := userID
	rows, err := h.events.ScanPendingOlderThan(ctx, threshold, &uid, batchLimit)
	if err != nil {
		logger.From(ctx).Warn("outline.housekeeping.user_sweep_failed", "user_id", userID, "err", err)
		return
	}
	for _, row := range rows {
		_ = h.events.MarkExpiredIfPending(ctx, model.OutlineEvent{
			OutlineID: row.OutlineID, OutlineGroupID: row.OutlineGroupID,
			UserID: row.UserID, ChildIDHash: row.ChildIDHash,
			OutlinePromptVersion: row.OutlinePromptVersion,
			DurationMin: row.DurationMin, TraceID: row.TraceID,
		})
		metrics.OutlineOutcomeTotal.WithLabelValues(OutcomeExpired).Inc()
	}
}
```

- [ ] **Step 2: main.go 启动**

```go
// server/cmd/server/main.go
hk := outline.NewHousekeeper(outlineEvents)
go hk.Run(ctx)
// 同时 wire 给 storyHandler.housekeeper 和 heartbeatHandler.housekeeper
```

- [ ] **Step 3: handler hook**

```go
// server/internal/api/story.go (List 开头)
func (h *StoryHandler) List(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)
	h.housekeeper.SweepUser(c.Request.Context(), userID) // Plan 11A §5.5 A2
	// ... existing list logic
}

// server/internal/api/heartbeat.go (Get 开头同样调 SweepUser)
```

- [ ] **Step 4: 测试**

```go
// server/internal/service/outline/housekeeping_test.go
package outline_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/outline"
)

func TestHousekeeper_SweepUser_OnlyOlderThanGrace(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.OutlineEvent{})
	es := outline.NewEventStore(db)
	old := time.Now().Add(-6 * time.Minute) // 老于 grace
	fresh := time.Now().Add(-1 * time.Minute)
	db.Exec("INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')", old, "ol_old", "g1", int64(1), "h")
	db.Exec("INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')", fresh, "ol_new", "g2", int64(1), "h")
	hk := outline.NewHousekeeper(es)
	hk.SweepUser(context.Background(), 1)
	oldOut, _ := es.LatestOutcome(context.Background(), "ol_old")
	newOut, _ := es.LatestOutcome(context.Background(), "ol_new")
	if oldOut != "expired" { t.Errorf("ol_old should be expired, got %s", oldOut) }
	if newOut != "pending" { t.Errorf("ol_new should still be pending, got %s", newOut) }
}
```

- [ ] **Step 5: 运行**

Run: `cd server && go test ./internal/service/outline/... -v -run TestHousekeeper`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/internal/service/outline/housekeeping.go server/internal/service/outline/housekeeping_test.go server/cmd/server/main.go server/internal/api/story.go server/internal/api/heartbeat.go
git commit -m "feat(outline): housekeeping + on-demand SweepUser dual-path (Plan 11A §5.5 A2)"
```

---

### Task 24: deps-lint — Gateway 不依赖 service 编译期 enforce

**Files:**
- Create: `server/scripts/check_layering.sh`
- Modify: `server/Makefile`
- Modify: CI 配置（GitHub Actions 或 pre-push hook）

- [ ] **Step 1: 写 check 脚本**

```bash
#!/usr/bin/env bash
# server/scripts/check_layering.sh — Plan 11B §3.5
set -euo pipefail
violations=$(go list -deps ./internal/gateway/... 2>/dev/null \
    | grep -E "^github.com/aibao/server/internal/(service|repository|api)/" || true)
if [ -n "$violations" ]; then
    echo "FAIL: gateway depends on:"
    echo "$violations"
    exit 1
fi
echo "OK: layering check passed"
```

- [ ] **Step 2: Makefile**

```makefile
.PHONY: check-layering
check-layering:
	@bash scripts/check_layering.sh
```

- [ ] **Step 3: 本地跑**

Run: `cd server && chmod +x scripts/check_layering.sh && make check-layering`
Expected: `OK: layering check passed`

- [ ] **Step 4: CI 集成**（如有 GitHub Actions）

```yaml
- name: Layering check
  run: cd server && make check-layering
```

- [ ] **Step 5: Commit**

```bash
git add server/scripts/check_layering.sh server/Makefile
git commit -m "ci: enforce gateway not depending on service/repository/api (Plan 11B §3.5)"
```

---

## Sprint C — Flutter 客户端（Task 25-28）

> 后端冒烟全过后进。所有改动用 `feature_flag.outline_enabled` 控制灰度。

### Task 25: Feature flag + API client 方法

**Files:**
- Create: `app/lib/feature_flags.dart`
- Modify: `app/lib/api/api_client.dart`

- [ ] **Step 1: 写 feature_flags**

```dart
// app/lib/feature_flags.dart
class FeatureFlags {
  static const bool outlineEnabled = bool.fromEnvironment(
    'OUTLINE_ENABLED', defaultValue: true,
  );
}
```

紧急回滚命令：`flutter build apk --release --dart-define=OUTLINE_ENABLED=false ...`

- [ ] **Step 2: API client 方法**

```dart
// app/lib/api/api_client.dart 追加

class OutlineDto {
  final String title, synopsis, style, educationalValue;
  final List<String> themes;
  final int durationMin;
  OutlineDto({required this.title, required this.synopsis, required this.style, required this.educationalValue, required this.themes, required this.durationMin});
  factory OutlineDto.fromJson(Map<String, dynamic> j) => OutlineDto(
    title: j['title'] ?? '',
    synopsis: j['synopsis'] ?? '',
    style: j['style'] ?? '',
    educationalValue: j['educational_value'] ?? '',
    themes: List<String>.from(j['themes'] ?? []),
    durationMin: j['duration_min'] ?? 5,
  );
}

class OutlinePreviewResult {
  final String outlineId;
  final OutlineDto outline;
  final DateTime expiresAt;
  OutlinePreviewResult({required this.outlineId, required this.outline, required this.expiresAt});
  factory OutlinePreviewResult.fromJson(Map<String, dynamic> j) => OutlinePreviewResult(
    outlineId: j['outline_id'],
    outline: OutlineDto.fromJson(j['outline']),
    expiresAt: DateTime.parse(j['expires_at']),
  );
}

extension ApiClientOutline on ApiClient {
  Future<OutlinePreviewResult> previewOutline({required int childId, required String prompt, required int durationMin}) async {
    final r = await dio.post('/api/v1/outlines/preview', data: {
      'child_id': childId, 'prompt': prompt, 'duration_min': durationMin,
    });
    return OutlinePreviewResult.fromJson(r.data);
  }
  Future<OutlinePreviewResult> refreshOutline(String outlineId) async {
    final r = await dio.post('/api/v1/outlines/$outlineId/refresh');
    return OutlinePreviewResult.fromJson(r.data);
  }
  Future<int> generateStoryFromOutline({required int childId, required String outlineId, Map<String, dynamic>? overrides}) async {
    final r = await dio.post('/api/v1/stories/generate', data: {
      'child_id': childId, 'outline_id': outlineId,
      if (overrides != null) 'outline_overrides': overrides,
    });
    return r.data['story_id'] as int;
  }
}
```

- [ ] **Step 3: Compile + commit**

```bash
cd app && flutter analyze
git add app/lib/feature_flags.dart app/lib/api/api_client.dart
git commit -m "feat(app): outline API client + feature flag (Plan 11A §8.1)"
```

---

### Task 26: Riverpod outline provider

**Files:**
- Create: `app/lib/providers/outline_provider.dart`

- [ ] **Step 1: 写 provider**

```dart
// app/lib/providers/outline_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_client.dart';
import 'api_client_provider.dart';

class OutlinePreviewParams {
  final int childId;
  final String prompt;
  final int durationMin;
  const OutlinePreviewParams({required this.childId, required this.prompt, required this.durationMin});

  @override
  bool operator ==(Object o) => o is OutlinePreviewParams && o.childId == childId && o.prompt == prompt && o.durationMin == durationMin;
  @override
  int get hashCode => Object.hash(childId, prompt, durationMin);
}

final outlinePreviewProvider = FutureProvider.family<OutlinePreviewResult, OutlinePreviewParams>((ref, p) async {
  return await ref.watch(apiClientProvider).previewOutline(childId: p.childId, prompt: p.prompt, durationMin: p.durationMin);
});

final currentOutlineProvider = StateProvider<OutlinePreviewResult?>((ref) => null);
```

- [ ] **Step 2: Compile + commit**

```bash
cd app && flutter analyze
git add app/lib/providers/outline_provider.dart
git commit -m "feat(app): outline Riverpod providers (Plan 11A §8.4)"
```

---

### Task 27: generate_screen 重做（极简版）

**Files:**
- Modify: `app/lib/screens/generate_screen.dart`
- Create: `app/lib/screens/legacy_generate_screen.dart`（保留旧 UI 给 fallback）

- [ ] **Step 1: rename 旧 UI 为 legacy**

```bash
cd app && cp lib/screens/generate_screen.dart lib/screens/legacy_generate_screen.dart
# 在 legacy 文件里把 class 名改为 LegacyGenerateScreen
```

- [ ] **Step 2: 重写 generate_screen**

```dart
// app/lib/screens/generate_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../feature_flags.dart';
import '../providers/outline_provider.dart';
import '../providers/child_provider.dart';
import 'legacy_generate_screen.dart';

class GenerateScreen extends ConsumerStatefulWidget {
  const GenerateScreen({super.key});
  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  final _ctl = TextEditingController();
  int _duration = 5;
  bool _busy = false;
  String? _error;

  Future<void> _think() async {
    final prompt = _ctl.text.trim();
    if (prompt.isEmpty) { setState(() => _error = '说说今晚想听什么吧'); return; }
    final child = ref.read(childProvider).value;
    if (child == null) return;
    setState(() { _busy = true; _error = null; });
    try {
      final r = await ref.read(outlinePreviewProvider(
        OutlinePreviewParams(childId: child.id, prompt: prompt, durationMin: _duration),
      ).future);
      ref.read(currentOutlineProvider.notifier).state = r;
      if (mounted) context.go('/outline');
    } catch (e) {
      setState(() => _error = '让爱宝想想失败了：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!FeatureFlags.outlineEnabled) return const LegacyGenerateScreen();
    return Scaffold(
      appBar: AppBar(title: const Text('讲什么故事？')),
      body: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          TextField(controller: _ctl, maxLines: 3, decoration: const InputDecoration(hintText: '比如：跟奥特曼一起冒险', border: OutlineInputBorder())),
          const SizedBox(height: 24),
          const Text('故事时长', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          SegmentedButton<int>(
            segments: const [
              ButtonSegment(value: 3, label: Text('3 分钟')),
              ButtonSegment(value: 5, label: Text('5 分钟')),
              ButtonSegment(value: 8, label: Text('8 分钟')),
            ],
            selected: {_duration},
            onSelectionChanged: (s) => setState(() => _duration = s.first),
          ),
          const SizedBox(height: 32),
          if (_error != null) Padding(padding: const EdgeInsets.only(bottom: 12), child: Text(_error!, style: const TextStyle(color: Colors.red))),
          FilledButton(onPressed: _busy ? null : _think, child: _busy ? const CircularProgressIndicator() : const Text('让爱宝想想')),
        ]),
      ),
    );
  }
}
```

- [ ] **Step 3: Compile + commit**

```bash
cd app && flutter analyze
git add app/lib/screens/generate_screen.dart app/lib/screens/legacy_generate_screen.dart
git commit -m "feat(app): redesign generate_screen — prompt + duration only with legacy fallback (Plan 11A §8.1)"
```

---

### Task 28: outline_screen + 路由 + 真机冒烟

**Files:**
- Create: `app/lib/screens/outline_screen.dart`
- Modify: `app/lib/router.dart`

- [ ] **Step 1: 写 outline_screen**

```dart
// app/lib/screens/outline_screen.dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../api/api_client.dart';
import '../providers/api_client_provider.dart';
import '../providers/child_provider.dart';
import '../providers/outline_provider.dart';
import '../providers/story_list_provider.dart';

class OutlineScreen extends ConsumerStatefulWidget {
  const OutlineScreen({super.key});
  @override
  ConsumerState<OutlineScreen> createState() => _OutlineScreenState();
}

class _OutlineScreenState extends ConsumerState<OutlineScreen> {
  Timer? _timer;
  Duration _remaining = const Duration(minutes: 5);
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    final o = ref.read(currentOutlineProvider);
    if (o != null) {
      _remaining = o.expiresAt.difference(DateTime.now());
      _timer = Timer.periodic(const Duration(seconds: 1), (_) {
        final left = o.expiresAt.difference(DateTime.now());
        if (!mounted) return;
        setState(() => _remaining = left.isNegative ? Duration.zero : left);
        if (left.isNegative) _timer?.cancel();
      });
    }
  }

  @override
  void dispose() { _timer?.cancel(); super.dispose(); }

  Future<void> _start() async {
    final outline = ref.read(currentOutlineProvider);
    final child = ref.read(childProvider).value;
    if (outline == null || child == null) return;
    setState(() => _busy = true);
    try {
      final api = ref.read(apiClientProvider);
      final storyId = await api.generateStoryFromOutline(childId: child.id, outlineId: outline.outlineId);
      ref.invalidate(outlinePreviewProvider);
      ref.invalidate(storyListProvider);
      if (mounted) context.go('/player/$storyId');
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('生成失败：$e')));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _refresh() async {
    final o = ref.read(currentOutlineProvider);
    if (o == null) return;
    setState(() => _busy = true);
    try {
      final api = ref.read(apiClientProvider);
      final n = await api.refreshOutline(o.outlineId);
      ref.read(currentOutlineProvider.notifier).state = n;
      _remaining = n.expiresAt.difference(DateTime.now());
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('换个角度失败：$e')));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String _fmt(Duration d) {
    if (d.isNegative || d == Duration.zero) return '已过期';
    return '剩余 ${d.inMinutes}:${(d.inSeconds % 60).toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final outline = ref.watch(currentOutlineProvider);
    if (outline == null) {
      return Scaffold(appBar: AppBar(title: const Text('大纲')), body: const Center(child: Text('请先回上一步输入需求')));
    }
    final o = outline.outline;
    final expired = _remaining == Duration.zero;
    return Scaffold(
      appBar: AppBar(
        title: const Text('爱宝想到了这个…'),
        actions: [Center(child: Padding(padding: const EdgeInsets.only(right: 16), child: Text(_fmt(_remaining), style: TextStyle(color: expired ? Colors.red : Colors.grey[700]))))],
      ),
      body: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(o.title, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.bold)),
          const SizedBox(height: 16),
          const Text('📖 故事梗概', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 4),
          Text(o.synopsis),
          const SizedBox(height: 16),
          Wrap(spacing: 8, children: [
            Chip(label: Text('🎯 ${o.themes.join(' · ')}')),
            Chip(label: Text('🎨 ${o.style}')),
            Chip(label: Text('⏱ ${o.durationMin} 分钟')),
          ]),
          const SizedBox(height: 12),
          Text('教育意义：${o.educationalValue}', style: TextStyle(color: Colors.grey[700])),
          const Spacer(),
          FilledButton(
            onPressed: (_busy || expired) ? null : _start,
            child: _busy ? const CircularProgressIndicator() : Text(expired ? '已过期，请重新预览' : '开始生成'),
          ),
          const SizedBox(height: 8),
          OutlinedButton(onPressed: (_busy || expired) ? null : _refresh, child: const Text('换个角度')),
          const SizedBox(height: 8),
          TextButton(onPressed: () => context.go('/generate'), child: const Text('返回修改需求')),
        ]),
      ),
    );
  }
}
```

- [ ] **Step 2: 加路由**

```dart
// app/lib/router.dart 加 route
GoRoute(path: '/outline', builder: (_, __) => const OutlineScreen()),
```

- [ ] **Step 3: 真机冒烟（OPPO PJJ110）**

```bash
cd app && flutter run --release --dart-define=API_BASE=https://aibao.dhgames.com
```

人工验收（按 Plan 11A §9.3 验收标准）：
1. 登录 → home → 生成 → 输入"想听冒险" + 5min → 让爱宝想想
2. 2 秒内显示大纲卡，含 title/synopsis/themes/style/教育意义
3. 倒计时 5:00 → 4:59 → ...
4. 点"换个角度"→ 新大纲（title 不同），倒计时 reset
5. 点"开始生成" → 进 player，1-2 分钟后播放
6. 跑 5 个不同输入（"想听温柔的"/"明天考试加油"/"喜欢恐龙"/"小公主"/"科普海洋"）—— 4/5 大纲让人愿意点开始

- [ ] **Step 4: Commit**

```bash
cd app && flutter analyze
git add app/lib/screens/outline_screen.dart app/lib/router.dart
git commit -m "feat(app): outline_screen with countdown + 3 actions + smoke verified (Plan 11A §8.2)"
```

---

## Sprint D — 11B Full Build（Task 29-32）

> Thin Slice + 11A 上线稳定 1 周后开始。

### Task 29: `service/cost/Aggregator` 聚合查询

**Files:**
- Create: `server/internal/service/cost/aggregator.go`
- Create: `server/internal/service/cost/aggregator_test.go`

- [ ] **Step 1: 写 Aggregator**

```go
// server/internal/service/cost/aggregator.go
package cost

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Aggregator struct{ db *gorm.DB }

func NewAggregator(db *gorm.DB) *Aggregator { return &Aggregator{db: db} }

type OverallStats struct {
	TotalYuan         float64
	StoriesAccepted   int64
	OutlinesPreviewed int64
	OutlinesAccepted  int64
	OutlinesRefreshed int64
	OutlinesExpired   int64
}

func (a *Aggregator) Overall(ctx context.Context, since, until time.Time) (OverallStats, error) {
	var s OverallStats
	a.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(cost_yuan),0) FROM cost_events WHERE occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.TotalYuan)
	a.db.WithContext(ctx).Raw(`SELECT COUNT(DISTINCT story_id) FROM cost_events WHERE purpose='story' AND outcome='ok' AND story_id IS NOT NULL AND occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.StoriesAccepted)
	a.db.WithContext(ctx).Raw(`SELECT COUNT(DISTINCT outline_id) FROM outline_events WHERE occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.OutlinesPreviewed)
	a.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM outline_events WHERE outcome='accepted' AND occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.OutlinesAccepted)
	a.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM outline_events WHERE outcome='refreshed' AND occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.OutlinesRefreshed)
	a.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM outline_events WHERE outcome='expired' AND occurred_at >= ? AND occurred_at < ?`, since, until).Scan(&s.OutlinesExpired)
	return s, nil
}

type PurposeRow struct {
	Purpose  string  `gorm:"column:purpose"`
	CostYuan float64 `gorm:"column:cost_yuan"`
}

func (a *Aggregator) ByPurpose(ctx context.Context, since, until time.Time) ([]PurposeRow, error) {
	var rows []PurposeRow
	err := a.db.WithContext(ctx).Raw(`
SELECT purpose, SUM(cost_yuan) AS cost_yuan
FROM cost_events
WHERE occurred_at >= ? AND occurred_at < ?
GROUP BY purpose ORDER BY cost_yuan DESC`, since, until).Scan(&rows).Error
	return rows, err
}

type UserRow struct {
	UserIDHash string  `gorm:"column:user_id_hash"`
	Stories    int64   `gorm:"column:stories"`
	Outlines   int64   `gorm:"column:outlines"`
	TotalYuan  float64 `gorm:"column:total_yuan"`
}

func (a *Aggregator) TopUsers(ctx context.Context, since, until time.Time, limit int) ([]UserRow, error) {
	var rows []UserRow
	err := a.db.WithContext(ctx).Raw(`
SELECT
    child_id_hash AS user_id_hash,
    COUNT(*) FILTER (WHERE purpose='story') AS stories,
    COUNT(DISTINCT outline_id) FILTER (WHERE outline_id != '') AS outlines,
    SUM(cost_yuan) AS total_yuan
FROM cost_events
WHERE occurred_at >= ? AND occurred_at < ?
GROUP BY child_id_hash
ORDER BY total_yuan DESC LIMIT ?`, since, until, limit).Scan(&rows).Error
	return rows, err
}

// OutlineSaving 按 11B §3.4 full pipeline 公式
func (a *Aggregator) OutlineSaving(ctx context.Context, since, until time.Time) (float64, error) {
	var avgFullPipeline float64
	a.db.WithContext(ctx).Raw(`
WITH accepted AS (
    SELECT DISTINCT story_id FROM cost_events
    WHERE purpose='story' AND outcome='ok' AND story_id IS NOT NULL
      AND occurred_at >= ? AND occurred_at < ?
),
pipeline AS (
    SELECT story_id, SUM(cost_yuan) AS pipeline_cost FROM cost_events
    WHERE story_id IN (SELECT story_id FROM accepted)
    GROUP BY story_id
)
SELECT COALESCE(AVG(pipeline_cost), 0) FROM pipeline`, since, until).Scan(&avgFullPipeline)

	type rj struct {
		Count       int64
		ActualSpent float64
	}
	var r rj
	a.db.WithContext(ctx).Raw(`
WITH terminal AS (
    SELECT DISTINCT ON (outline_id) outline_id, outcome FROM outline_events
    WHERE occurred_at >= ? AND occurred_at < ?
    ORDER BY outline_id, occurred_at DESC, id DESC
),
rejected_outlines AS (
    SELECT outline_id FROM terminal WHERE outcome IN ('refreshed','expired')
)
SELECT
    COUNT(DISTINCT outline_id) AS count,
    COALESCE(SUM(cost_yuan), 0) AS actual_spent
FROM cost_events
WHERE outline_id IN (SELECT outline_id FROM rejected_outlines)`, since, until).Scan(&r)

	return float64(r.Count)*avgFullPipeline - r.ActualSpent, nil
}
```

- [ ] **Step 2: 测试**

```go
// server/internal/service/cost/aggregator_test.go
package cost_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/cost"
)

func TestAggregator_OutlineSaving_Formula(t *testing.T) {
	db := startTestPG(t); db.AutoMigrate(&model.CostEvent{}, &model.OutlineEvent{})
	ctx := context.Background()
	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)
	storyID := int64(1)

	// 1 accepted story 总 pipeline cost ≈ 1.00
	db.Create(&model.CostEvent{EventID: "t1:story:llm_call:1", OccurredAt: time.Now(), Purpose: "story", Outcome: "ok", Provider: "doubao", Model: "pro", CostYuan: 0.50, StoryID: &storyID, PriceVersion: "v"})
	db.Create(&model.CostEvent{EventID: "t1:tts:synthesize:1", OccurredAt: time.Now(), Purpose: "tts", Outcome: "ok", Provider: "minimax", CostYuan: 0.40, StoryID: &storyID, PriceVersion: "v"})
	db.Create(&model.CostEvent{EventID: "t1:storage_put:upload:1", OccurredAt: time.Now(), Purpose: "storage_put", Outcome: "ok", Provider: "cos", CostYuan: 0.10, StoryID: &storyID, PriceVersion: "v"})

	// 1 refreshed outline, cost 0.05
	rejectedID := "ol_rejected"
	db.Create(&model.OutlineEvent{OutlineID: rejectedID, OutlineGroupID: "g", UserID: 1, ChildIDHash: "h", Outcome: "refreshed", OccurredAt: time.Now()})
	db.Create(&model.CostEvent{EventID: "t2:outline:llm_call:1", OccurredAt: time.Now(), Purpose: "outline", Outcome: "ok", Provider: "doubao", Model: "lite", CostYuan: 0.05, OutlineID: rejectedID, PriceVersion: "v"})

	agg := cost.NewAggregator(db)
	saved, _ := agg.OutlineSaving(ctx, since, until)
	want := 1*1.00 - 0.05 // = 0.95
	if abs(saved-want) > 1e-6 {
		t.Errorf("saved want %.4f got %.4f", want, saved)
	}
}

func abs(x float64) float64 { if x < 0 { return -x }; return x }
```

- [ ] **Step 3: 运行 + commit**

```bash
cd server && go test ./internal/service/cost/... -v -run TestAggregator
git add server/internal/service/cost/aggregator.go server/internal/service/cost/aggregator_test.go
git commit -m "feat(cost): Aggregator with Overall/ByPurpose/TopUsers/OutlineSaving (Plan 11B §3.4)"
```

---

### Task 30: `cmd/cost-report` CLI

**Files:**
- Create: `server/cmd/cost-report/main.go`
- Modify: `server/Makefile`

- [ ] **Step 1: 写 CLI**

```go
// server/cmd/cost-report/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/service/cost"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	since := flag.String("since", "last_7d", "YYYY-MM-DD | last_7d | last_30d | last_month")
	until := flag.String("until", "now", "YYYY-MM-DD | now")
	by := flag.String("by", "overall", "overall|purpose|user")
	limit := flag.Int("limit", 10, "limit for --by=user")
	flag.Parse()

	s, u := parseDates(*since, *until)
	cfg := config.MustLoad()
	db, err := gorm.Open(postgres.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil { fmt.Fprintln(os.Stderr, "db:", err); os.Exit(1) }
	agg := cost.NewAggregator(db)
	ctx := context.Background()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0); defer w.Flush()
	fmt.Fprintf(w, "Period: %s to %s\n\n", s.Format("2006-01-02"), u.Format("2006-01-02"))

	switch *by {
	case "overall":
		st, _ := agg.Overall(ctx, s, u)
		saved, _ := agg.OutlineSaving(ctx, s, u)
		fmt.Fprintln(w, "=== Overall ===")
		fmt.Fprintf(w, "Total spent:\t¥%.2f\n", st.TotalYuan)
		fmt.Fprintf(w, "Stories accepted:\t%d\n", st.StoriesAccepted)
		fmt.Fprintf(w, "Outlines previewed:\t%d\n", st.OutlinesPreviewed)
		fmt.Fprintf(w, "  accepted:\t%d\n", st.OutlinesAccepted)
		fmt.Fprintf(w, "  refreshed:\t%d\n", st.OutlinesRefreshed)
		fmt.Fprintf(w, "  expired:\t%d\n", st.OutlinesExpired)
		fmt.Fprintf(w, "\nOutline saving (full-pipeline formula):\t¥%.2f\n", saved)
	case "purpose":
		rows, _ := agg.ByPurpose(ctx, s, u)
		fmt.Fprintln(w, "=== By Purpose ==="); fmt.Fprintln(w, "purpose\tcost")
		for _, r := range rows { fmt.Fprintf(w, "%s\t¥%.4f\n", r.Purpose, r.CostYuan) }
	case "user":
		rows, _ := agg.TopUsers(ctx, s, u, *limit)
		fmt.Fprintln(w, "=== Top Users (HMAC-hashed) ==="); fmt.Fprintln(w, "user_hash\tstories\toutlines\ttotal")
		for _, r := range rows { fmt.Fprintf(w, "%s\t%d\t%d\t¥%.4f\n", r.UserIDHash, r.Stories, r.Outlines, r.TotalYuan) }
	default:
		fmt.Fprintln(os.Stderr, "unknown --by:", *by); os.Exit(2)
	}
}

func parseDates(since, until string) (time.Time, time.Time) {
	now := time.Now()
	var s, u time.Time
	switch since {
	case "last_7d":    s = now.Add(-7*24*time.Hour)
	case "last_30d":   s = now.Add(-30*24*time.Hour)
	case "last_month": s = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
	default:
		t, err := time.Parse("2006-01-02", since); if err != nil { fmt.Fprintln(os.Stderr, "bad --since:", err); os.Exit(2) }
		s = t
	}
	if until == "now" { u = now } else {
		t, err := time.Parse("2006-01-02", until); if err != nil { fmt.Fprintln(os.Stderr, "bad --until:", err); os.Exit(2) }
		u = t
	}
	return s, u
}
```

- [ ] **Step 2: Makefile**

```makefile
.PHONY: cost-report
cost-report:
	@go build -o bin/cost-report ./cmd/cost-report && ./bin/cost-report $(ARGS)
```

- [ ] **Step 3: 编译 + 跑**

```bash
cd server && go build -o bin/cost-report ./cmd/cost-report
bin/cost-report --since=last_7d --by=overall
bin/cost-report --since=last_7d --by=purpose
bin/cost-report --since=last_7d --by=user --limit=5
```

Expected: 三个报表正确输出。

- [ ] **Step 4: Commit**

```bash
git add server/cmd/cost-report/ server/Makefile
git commit -m "feat(cost-report): CLI with overall/purpose/user (Plan 11B §7.1)"
```

---

### Task 31: 真链路对账验收（上线第 1 周后）

**Files:**
- Create: `docs/devlog/<date>-plan-11-cost-reconciliation-week1.md`

- [ ] **Step 1: 拉一周报表**

```bash
bin/cost-report --since=$(date -d '7 days ago' +%Y-%m-%d) --by=purpose
```

- [ ] **Step 2: 拉 provider 后台数据**

- 火山引擎账单（豆包 pro + lite 各 model 用量 / 元）
- Minimax 后台账单（t2a-v2 字数 / 元）
- 腾讯云 COS 控制台用量统计（PUT 次数 + 流量 GB / 元）

- [ ] **Step 3: 三方对账表（写入 devlog）**

```markdown
| 项目        | 我方报表 | provider 账单 | 误差 % | 通过 |
|------------|----------|---------------|--------|------|
| doubao-pro |  ¥X.XX   |  ¥Y.YY        | <10%   | ✅   |
| ...        |  ...     |  ...          | ...    | ...  |
```

- [ ] **Step 4: 误差 >10% 走 11B §9.2 排查清单 + 修正后再对账**

- [ ] **Step 5: 写 devlog + commit**

```bash
git add docs/devlog/...-plan-11-cost-reconciliation-week1.md
git commit -m "docs: Plan 11B cost reconciliation week 1 — all providers within 10%"
```

---

### Task 32: Plan 11 收官 — MEMORY + CLAUDE.md

**Files:**
- Modify: `MEMORY.md`（决策时间线 + 关键技术教训）
- Modify: `CLAUDE.md`（标记 Plan 11 完成）
- Create: `docs/devlog/<date>-plan-11-close-out.md`

- [ ] **Step 1: 更新 MEMORY.md 决策时间线**

```
- **<date>** — Plan 11 完成：AI 大纲预览（B 模式）替代手填表单 + 成本可观测（11B Thin Slice 同 sprint 落地 + Full Build 1 周后跟）。引入 service/outlinecontract/ 中立合约包解决 service/story↔outline 双向依赖；HMAC-SHA256 + 12 hex idhash 替换裸 SHA256；cost_events event_id 业务语义 {trace_id}:{purpose}:{stage}:{attempt}；outline_events append-only 模式；Gateway 不依赖 service（CI enforce）。Codex 三轮 review 通过。一周对账误差 X%。用户反馈 outline 接受率 X%。
```

- [ ] **Step 2: CLAUDE.md 标记 Plan 11 完成**

- [ ] **Step 3: 写收官 devlog**

包含：上线时间线、Codex review 经验、关键技术教训（spec 经多轮 review 才稳定、事件表 vs 状态表的选择、cost 双轨数据源等）

- [ ] **Step 4: Commit**

```bash
git add MEMORY.md CLAUDE.md docs/devlog/<date>-plan-11-close-out.md
git commit -m "docs(plan11): close out — outline B-mode + cost observability live, Codex 3-round reviewed"
```

---

## Self-Review

**Spec coverage check**：
- ✅ 11A §5.1 LLM JSON schema + repair retry → Task 15
- ✅ 11A §5.2 Redis 票据 + ownership + 410 → Task 13, 18
- ✅ 11A §5.3 OutlineSafetyCheck → Task 16
- ✅ 11A §5.4 outline_prompt_version → Task 15 (常量) + 17/18 (传递)
- ✅ 11A §5.5 outline_events append-only + 双路径 expired → Task 5, 14, 23
- ✅ 11A §6.1-6.7 API 契约 + 错误码 + 互斥 → Task 19, 20, 21
- ✅ 11A §7.5 OutlineResolver 独立包 → Task 12, 18
- ✅ 11A §10.1 storyline 兼容 → Task 21
- ✅ 11B §3.1 Gateway 不调 Recorder → Task 10, 24 (CI enforce)
- ✅ 11B §3.2 PG 是事实源 → 全文体现
- ✅ 11B §3.3 异步 Flusher + 幂等 → Task 8, 9
- ✅ 11B §3.4 outline 省钱公式 → Task 29
- ✅ 11B §5.1 cost_events schema + event_id → Task 6, 7
- ✅ 11B §5.1.1 event_id 业务语义 → Task 8 (validator) + 各业务方填
- ✅ 11B §5.2 PriceBook + 不 hot-reload → Task 3
- ✅ 11B §6.3 HMAC + 12 hex + domain separation → Task 2
- ✅ 11B §8.1 Thin Slice 同 sprint → Task 1-11 编排
- ✅ 11B §8.2 Full Build 后跟 → Task 29-32

**Placeholder scan**：
- Task 1 `<填入实际数字>` 是 Task 自身要做的校对（有意），不算 plan 失败
- 无 TBD / TODO / "Add appropriate error handling" 等空话

**Type consistency check**：
- `outlinecontract.Outline` (Task 12) → 17/18/21 一致
- `RawOutline` (Task 15) → 16/17 一致
- `RecordInput` (Task 8) → 11/17 一致
- `OutlineEvent` / `CostEvent` (Task 7) → 14/29 一致
- `OutcomePending/Accepted/Refreshed/Expired` 常量 (Task 14) → 17/18/23 一致

**Execution notes**：
- Task 1-11 = 11B Thin Slice（**必须先于 19+ 上线**）
- Task 12-23 = 11A 后端
- Task 24 = CI enforce（Thin Slice 完成后立即加，防回归）
- Task 25-28 = Flutter（后端冒烟全过后）
- Task 29-32 = Full Build（Thin Slice + 11A 上线 1 周后）

---

## 执行方式选择

**Plan 完成并保存到 `docs/superpowers/plans/2026-05-25-plan-11-ai-outline-and-cost-observability.md`。两种执行方式：**

1. **Subagent-Driven（推荐）** —— 每个 task 分发新 subagent；task 间审查；迭代快；本会话主线只负责派遣 + 校验

2. **Inline Execution** —— 本会话直接执行；按检查点（建议每个 Sprint 一个）批量推进；上下文连贯但累积压力大

**选哪种？**

---

## 附录 A — 实施期偏差日志（Sprint A+B+C 累积，2026-05-25 ~ 2026-05-28）

> 本节是 implementation phase 沉淀，**写于 28/32 task 完成后**。
> Plan 写作时凭印象假设的"项目 API 长什么样"，实际实施时 subagent 发现多处不一致。
> 这里**逐条记录正确的 API**，供未来 Plan 11C+ / Sprint D 后续 task / 类似 plan 重用 — 不再重复踩同样的坑。

### 后端（Go）

| Plan 写法 | 项目实际 | 影响 task | 说明 |
|---|---|---|---|
| `promauto.NewCounterVec(...)` 全局 var | `metrics.Business` struct + 注入 + 显式 `reg.MustRegister(...)` | Task 8 (Recorder) | Plan 1 立的注入模式；所有业务 metric 走 Business struct，不用 promauto |
| `logger.From(ctx)` | `logger.FromCtx(ctx)` | Task 8 (Recorder) | 包 `internal/pkg/logger`，Plan 1 写的 |
| `apperr.SafetyRejected(reason, cat)` / `apperr.LLMFailed(err)` / `apperr.Internal(err)` helper | 没有 helper —— 用 `apperr.New(apperr.CodeXxx, reason, userMsg)` 直接构造 | Task 17 (Service.Preview) | apperr 是 Code enum + AppError struct + `New/Wrap` 构造函数，没 SafetyRejected helper |
| `apperr.WriteJSON(c, err)` | `RespondError(c, *AppError)` | Task 19 (preview handler) | api 包内导出函数 |
| `c.MustGet("user_id").(int64)` | `userctx.FromContext(c.Request.Context())` returns `(int64, bool)` | Task 19 (preview handler) | userctx 包路径是 `internal/api/userctx`（不是 `internal/userctx`） |
| `safety.NewMatcherFromWords([]string)` / `matcher.FirstHit(input)` | `safety.NewKeywordMatcher([]string) *KeywordMatcher` / `Matcher.FindFirst(input) (string, bool)` | Task 16 (OutlineSafetyCheck) | Plan 3 写的 API |
| `safety.NewPreCheckerFromMatcher(m)` / `pre.Check(ctx, prompt, blacklist)` | `safety.NewPreChecker(rs *RuleSet, intent IntentProvider)` / `pre.Check(ctx, PreCheckInput{UserPrompt, ChildFearList, MaxPromptRunes}) PreCheckOutput{Pass, RejectReason, MatchedCategory, NormalizedPrompt, ...}` | Task 17 (Service.Preview) | PreChecker **要求** IntentProvider 非 nil（precheck.go:119 调用 `p.intent.Classify`）—— 测试需要 `safety.NoopIntentProvider{}` |
| `safety.RuleSet` 的 `IPWhitelist []IPEntry` / `IPBlacklist []IPEntry` | `IPWhitelist map[string]string` / `IPBlacklist []string` | Task 17 (Service.Preview test fixture) | grep 实际 struct field 类型 |
| `viper.GetViper()` 全局 | 项目 viper 实例是局部 —— 加 `config.LoadWithViper` 返回 `(*Config, *viper.Viper, error)` | Task 11 (main.go wire) | 保留 `Load` 作 thin wrapper，仅 main.go 用 LoadWithViper |
| `Summarizer.Summarize(ctx, text)` / `ChapterHook.Extract(ctx, text)` 纯文本 | `*ForStory` 平行方法接 `(ctx, text, childID, storyID, userID, traceHex)` — 老接口零 ID 包装保留兼容 | Task 11 (cost wire) | Recorder 需要 StoryID/ChildIDHash，原 method 拿不到 |
| `traceid.From(ctx)` 返 `(string, ok)` 直接拼 event_id | `traceid.FromContext` 返 `tr-<8hex>` —— event_id 正则 `^[a-f0-9]{8,}` 拒绝 `tr-` 前缀；要 `strings.TrimPrefix(id, "tr-")` | Task 11 / 17 (event_id 拼接) | 加 helper `traceIDFromCtx(ctx) string` |
| Plan 中 `cachedOutline` 小写（包内私有） | `CachedOutline` 大写 + `NewCachedOutline` 构造函数 | Task 13 (Cache) | 同包测试 + Service.Preview 都要构造，私有访问不便 |
| `INSERT ... WHERE NOT EXISTS WHERE outline_id = $2`（PG 风格） | GORM `db.Exec(sql, args...)` 用 `?` 占位 + 不能复用同一占位 —— `outline_id` 出现两次需传两次 | Task 14 (MarkExpiredIfPending) | 行为等价 |
| `OutlineEventAppender` interface 由 service/outline 定义并导入 | 由 **consumer 包 (service/story)** 定义 narrow interface —— 与项目 `StoryRepo` / `ChildRepo` / `Budget` 同款 duck-typing 习惯 | Task 21 (orchestrator step 0) | Go interface satisfaction 隐式；keep story 不 import outline 的关键 |
| 测试 fixture `测试标题` 4 runes + 55 runes synopsis | parser 边界 5-16 / 60-160 —— fixture 必须 ≥5 / ≥60 | Task 15 (parser test) | 边界是产品决策；fixture 收紧到 `测试标题甲` + synopsis 加 5 字 |
| `outline.OutcomeAccepted` 常量直接 import | story 包用 `"accepted"` 字符串字面量 —— 不能反向 import outline | Task 21 (step 0) | spec §7.5 N5 layering 守门 |
| `metrics.OutlineOutcomeTotal.WithLabelValues(...)` 全局 | `biz *metrics.Business`（nil-safe）注入 + `biz.OutlineOutcomeTotal.WithLabelValues(...)` | Task 17 / 23 | 同 promauto 偏差，all metrics 走 Business struct |

### Flutter（Dart）

| Plan 写法 | 项目实际 | 影响 task | 说明 |
|---|---|---|---|
| `app/lib/providers/outline_provider.dart` + `extension ApiClientOutline on ApiClient` | `app/lib/state/outline_state.dart` + method on `ApiClient` class —— with `_dio` private field + `_ensureSuccess(r)` | Task 25 / 26 | 沿用 heartbeat_state / child_state / story_list_state 的目录 + 命名模式 |
| `dio.post(...)` | `_dio.post(...)` + `_ensureSuccess(r)` after | Task 25 (api method) | dio 是 ApiClient 私有字段 |
| `DurationChips(value: 5, onChanged: ...)` | `DurationChips(selected: 5, onChanged: ...)` | Task 27 (generate_screen) | widget API 实际 |
| `ref.read(childProvider).value` | `ref.read(childProvider).valueOrNull`（AsyncValue API） | Task 27 / 28 | Riverpod 2.x AsyncValue 标准 |
| GenerateScreen 没考虑 storyline 续集模式 | Plan 9b GenerateScreen 接 `final int? storylineId` 续集模式必须保留 —— spec §10.1 续集与 outline 互斥 | Task 27 | 重做时三分支：续集 / flag off / 默认（新极简） |
| handler 注入用构造函数加参数 | `WithHousekeeper(hk)` chained setter —— 现有测试 fixture 零适配 | Task 23 (housekeeping hook) | 同 builder pattern；nil-safe field 守门 |

### 其他

| Plan 假设 | 项目实际 | 影响 |
|---|---|---|
| GitHub Actions `.github/workflows/...` | 项目无 `.github/workflows/`，CI 走 Makefile + 本地 lint | Task 24 (deps-lint) — `make check-layering` |
| Subagent 派遣"永远可用" | Anthropic API 在 Sprint A 后段 + 个别 task 出现 529 Overloaded / ConnectionRefused | Task 5/6/7/8/16 inline 完成 |

### Subagent vs Inline 实测对比（28 task 样本）

- **22 task by subagent** — 平均 35-50k token，2-5 分钟，task 内部 6-30 次 tool call；隔离上下文不污染主会话
- **6 task inline** — 主要发生在 529 拥塞 + 风险极低的 task（migration、纯 model、metrics 注册）
- **2 处需要 subagent 二次派遣** — Task 15 测试 fixture 边界冲突（subagent 正确停下来报告，第二次派遣给决策）

**结论**：Subagent-Driven 模式在 32-task 规模下**主会话上下文压力可控**，质量没下降。下次类似 plan 推荐沿用。

### Task 29-32 起步注意

- Aggregator (Task 29) 用 SQL DISTINCT ON 拿 outline 最新生命周期 —— **不要** UPDATE
- CLI (Task 30) 用 tabwriter；DSN 来源是 `config.MustLoad()` —— 走 `LoadWithViper` 或简化版
- 对账 (Task 31) 需要 Aggregator + CLI 上线 + 真实数据跑一周，**强制依赖运营动作**
- 收官 (Task 32) 写 devlog 时引用本附录
