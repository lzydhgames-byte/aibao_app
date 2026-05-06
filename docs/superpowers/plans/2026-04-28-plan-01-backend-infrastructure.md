# Plan 1：后端基础设施 实现规划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建爱宝后端 Go 项目骨架，使服务能稳定启动、接受 HTTP 请求、连通 PostgreSQL/Redis、执行数据库迁移、暴露健康检查与 Metrics 端点、按规范输出 JSON 结构化日志，并通过 traceId 串联请求链路。完成后 Plan 2-9 可以基于此骨架增量开发。

**Architecture:** 单体 Go 应用 (`aibao-server`)；标准三层架构（api / service / repository）+ Gateway 抽象层；配置外置（YAML + 环境变量）；slog 结构化日志；Prometheus 客户端暴露 `/metrics`；PG 通过 GORM 访问；Redis 通过 go-redis；schema 通过 golang-migrate 管理；HTTP 框架使用 Gin；进程通过 systemd 启动（systemd 单元配置在本 Plan 末尾，但部署相关脚本属 Plan 9）。

**Tech Stack:**
- Go 1.22+（实际使用 1.24.2）
- Gin（HTTP 框架）
- GORM（ORM）
- go-redis/v9（Redis 客户端）
- golang-migrate/v4（数据库迁移）
- log/slog（标准库结构化日志）
- prometheus/client_golang（指标）
- viper（配置加载）
- testify（单元测试）
- testcontainers-go（集成测试用真实 PG/Redis）

**前置阅读：**
- 产品设计：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)（重点：第 3 章项目结构、第 10 章日志与可观测性、第 12 章部署）

**完成验收（Definition of Done）：**
1. `go build ./...` 编译通过
2. `go test ./...` 全部测试通过，service+pkg 层覆盖率 ≥ 70%
3. `make run-dev` 能启动服务并打印 INFO 级别启动日志
4. `curl localhost:8080/health` 返回 200
5. `curl localhost:8080/ready` 在 PG 与 Redis 正常时返回 200，关掉任意一个返回 503
6. `curl localhost:8080/metrics` 返回 Prometheus 格式指标，至少含 `http_request_duration_seconds`
7. 任何 HTTP 请求都在日志中出现 trace_id 字段，且同一请求多条日志 trace_id 一致
8. `make migrate-up` 能创建一张验证表（本 Plan 仅落地骨架表 `schema_migrations`，业务表留给 Plan 2+）
9. 优雅关停：发送 SIGTERM 后进程在合理时间内退出，且无在途请求被强杀

---

## File Structure

下面列出本 Plan 创建/修改的所有文件。每个文件单一职责，文件之间依赖方向单一（api → service → repository；所有层 → pkg）。

### 项目根

| 文件 | 职责 |
|---|---|
| `go.mod` / `go.sum` | Go 模块定义 |
| `Makefile` | 常用命令封装（build / run-dev / test / migrate-up / migrate-down / lint） |
| `.gitignore` | 忽略二进制、日志、IDE、敏感配置 |
| `.golangci.yml` | linter 配置 |
| `README.md` | 简明启动说明 |

### `cmd/server/`

| 文件 | 职责 |
|---|---|
| `main.go` | 程序入口：加载配置 → 初始化 logger/db/redis → 注册路由 → 启动 server → 优雅关停 |

### `internal/pkg/`（基础设施工具包）

| 文件 | 职责 |
|---|---|
| `config/config.go` | 配置结构体定义 + viper 加载 + 环境变量覆盖 |
| `config/config_test.go` | 配置加载测试 |
| `logger/logger.go` | slog 封装（JSON、文件输出、按天切割、级别控制） |
| `logger/logger_test.go` | 日志格式与脱敏测试 |
| `logger/sanitize.go` | 敏感字段脱敏函数（手机号、孩子姓名等） |
| `logger/sanitize_test.go` | 脱敏测试 |
| `traceid/traceid.go` | traceId 生成、context 存取 |
| `traceid/traceid_test.go` | traceId 唯一性、context 传递测试 |
| `errors/errors.go` | 统一错误类型（含 HTTP 状态码、面向用户消息） |
| `errors/errors_test.go` | 错误类型测试 |
| `safehash/safehash.go` | 敏感字段稳定 hash（SHA256 + 配置盐） |
| `safehash/safehash_test.go` | hash 测试 |

### `internal/api/middleware/`（HTTP 中间件）

| 文件 | 职责 |
|---|---|
| `recover.go` | panic 兜底，转 500，记录堆栈 |
| `recover_test.go` | panic 不让进程崩 |
| `traceid.go` | 生成 traceId 注入 context + 响应头 |
| `traceid_test.go` | traceId 流转测试 |
| `logger.go` | 请求开始/结束日志，含状态码、耗时 |
| `logger_test.go` | 日志字段完整性 |
| `metrics.go` | HTTP 请求指标采集（histogram + counter） |
| `metrics_test.go` | 指标采集正确性 |

### `internal/api/`（HTTP 入口）

| 文件 | 职责 |
|---|---|
| `router.go` | 注册路由（health/ready/metrics + 挂载 middleware） |
| `health.go` | `/health` `/ready` handler |
| `health_test.go` | health/ready 行为测试 |

### `internal/repository/`（数据访问层）

| 文件 | 职责 |
|---|---|
| `db.go` | GORM 初始化（连接池、参数、健康检查） |
| `db_test.go` | DB 连接测试（用 testcontainers） |
| `redis_client.go` | Redis 客户端初始化 + 健康检查 |
| `redis_client_test.go` | Redis 连接测试 |

### `internal/metrics/`（指标定义）

| 文件 | 职责 |
|---|---|
| `metrics.go` | 全局指标定义（http_request_duration_seconds 等） |
| `metrics_test.go` | 指标注册测试 |

### `migrations/`

| 文件 | 职责 |
|---|---|
| `000001_init.up.sql` | 初始迁移：仅创建 `schema_migrations` 表（由 migrate 自动管理）+ 一张占位 `infra_check` 表用于验证迁移流程 |
| `000001_init.down.sql` | 反向迁移 |

### `config/`

| 文件 | 职责 |
|---|---|
| `config.dev.yaml` | 开发环境配置（无敏感信息） |
| `config.yaml.example` | 配置模板（入 git，真实生产配置不入 git） |

### `.github/`（暂留空，CI/CD 待 Plan 9）

无文件本期创建。

---

## 依赖与版本约定

写入 `go.mod`，锁定主版本：

```
go 1.22

require (
    github.com/gin-gonic/gin v1.10.0
    github.com/spf13/viper v1.19.0
    gorm.io/gorm v1.25.12
    gorm.io/driver/postgres v1.5.9
    github.com/redis/go-redis/v9 v9.7.0
    github.com/golang-migrate/migrate/v4 v4.18.1
    github.com/prometheus/client_golang v1.20.5
    github.com/google/uuid v1.6.0
    github.com/stretchr/testify v1.9.0
    github.com/testcontainers/testcontainers-go v0.34.0
    github.com/testcontainers/testcontainers-go/modules/postgres v0.34.0
    github.com/testcontainers/testcontainers-go/modules/redis v0.34.0
    gopkg.in/natefinch/lumberjack.v2 v2.2.1
)
```

> 🎓 **测试注解**：testcontainers-go 会用 Docker 起真实 PG/Redis 跑集成测试。开发机需要安装 Docker。如果开发环境没有 Docker，可以用 build tag `-tags=integration` 跳过这类测试。本 Plan 全部集成测试都带 `//go:build integration`。

---

## Task 0：环境准备（一次性，已部分完成）

> 这一节记录"开始写代码前必须就绪的一切"。如果将来换电脑/新人加入，照此节跑一遍即可恢复同样的开发环境。

### 0.1 检查 Go 与 Docker
```bash
go version            # 期望 1.22+
docker --version
docker compose version
```

### 0.2 配置 git 全局身份（每台电脑只做一次）
```bash
git config --global user.name "lzy"
git config --global user.email "332803710@qq.com"
git config --global init.defaultBranch main
```

### 0.3 在仓库根目录初始化 git 仓库（已完成）
```bash
cd f:/claud/aibao_app
git init
```

### 0.4 安装 Go 工具（已完成）
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
migrate -version
golangci-lint --version
```

### 0.5 启动本地依赖（PG 16 + Redis 7，每天开发前执行）
仓库根目录已存在 `docker-compose.dev.yml`。启动：
```bash
docker compose -f docker-compose.dev.yml up -d
```
验证：
```bash
docker exec aibao-postgres-dev pg_isready -U aibao -d aibao   # accepting connections
docker exec aibao-redis-dev redis-cli ping                    # PONG
```
停止（不丢数据）：`docker compose -f docker-compose.dev.yml down`
彻底清空：`docker compose -f docker-compose.dev.yml down -v`

### 0.6 项目布局确认
```
f:/claud/aibao_app/             ← Git 仓库根（已 init）
├── .gitignore                  ← 已创建
├── docker-compose.dev.yml      ← 已创建
├── docs/                       ← 已存在
├── server/                     ← 后端 Go 代码（Task 1 起在此目录工作）
└── client/                     ← Flutter 前端（Plan 8 创建）
```

### 0.7 基线 commit
本 Task 结束时把 Task 0 的产物提交（由本 Plan 执行流程在 Task 0 末尾统一处理，不在此重复）。

---

## Task 1：项目初始化与目录骨架

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `.golangci.yml`
- Create: `README.md`
- Create: 各空目录的 `.gitkeep`

- [ ] **Step 1.1：初始化 Go module**

工作目录在 `f:/claud/aibao_app/server/`（建议把 Go 项目放在仓库子目录 `server/`，与 Flutter 子目录 `client/` 区分）。

```bash
mkdir -p server && cd server
go mod init github.com/zylili/aibao-server
```

- [ ] **Step 1.2：创建 `.gitignore`**

```
# Binaries
/bin/
/aibao-server
*.exe
*.test
*.out

# Logs
/logs/
*.log

# IDE
.idea/
.vscode/
*.swp

# Sensitive config
config.prod.yaml
config.local.yaml
.env
.env.*

# OS
.DS_Store
Thumbs.db
```

- [ ] **Step 1.3：创建 `Makefile`**

```makefile
.PHONY: build run-dev test test-integration lint migrate-up migrate-down clean

GO := go
BINARY := bin/aibao-server
PKG := ./...

CONFIG_DEV := config/config.dev.yaml
MIGRATE_DIR := migrations
DB_URL_DEV := postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable

build:
	$(GO) build -o $(BINARY) ./cmd/server

run-dev: build
	AIBAO_CONFIG=$(CONFIG_DEV) ./$(BINARY)

test:
	$(GO) test -race -count=1 -cover $(PKG)

test-integration:
	$(GO) test -race -count=1 -tags=integration $(PKG)

lint:
	golangci-lint run

migrate-up:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" up

migrate-down:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" down 1

clean:
	rm -rf bin logs
```

- [ ] **Step 1.4：创建 `.golangci.yml`**

```yaml
run:
  timeout: 3m
  tests: true

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - revive

linters-settings:
  revive:
    rules:
      - name: exported
        disabled: false
```

- [ ] **Step 1.5：创建空目录骨架**

```bash
mkdir -p cmd/server
mkdir -p internal/api/middleware
mkdir -p internal/repository
mkdir -p internal/metrics
mkdir -p internal/pkg/config
mkdir -p internal/pkg/logger
mkdir -p internal/pkg/traceid
mkdir -p internal/pkg/errors
mkdir -p internal/pkg/safehash
mkdir -p migrations
mkdir -p config

touch cmd/server/.gitkeep \
      internal/api/middleware/.gitkeep \
      internal/repository/.gitkeep \
      internal/metrics/.gitkeep \
      internal/pkg/config/.gitkeep \
      internal/pkg/logger/.gitkeep \
      internal/pkg/traceid/.gitkeep \
      internal/pkg/errors/.gitkeep \
      internal/pkg/safehash/.gitkeep \
      migrations/.gitkeep \
      config/.gitkeep
```

- [ ] **Step 1.6：创建 `README.md`（最小可启动说明）**

```markdown
# aibao-server

爱宝后端服务。

## 本地开发

前置：Go 1.22+、Docker（用于 testcontainers 集成测试）、PostgreSQL 16、Redis 7、`migrate` CLI（`go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`）。

启动 PG/Redis（自行搭建或用 docker 起本地）后：

    make migrate-up
    make run-dev

健康检查：

    curl localhost:8080/health
    curl localhost:8080/ready
    curl localhost:8080/metrics

## 测试

    make test                # 单测
    make test-integration    # 集成测试（需要 Docker）
    make lint
```

- [ ] **Step 1.7：提交**

```bash
git add server/.gitignore server/Makefile server/.golangci.yml server/README.md \
        server/go.mod \
        server/cmd server/internal server/migrations server/config
git commit -m "chore: bootstrap aibao-server project skeleton"
```

---

## Task 2：配置加载（config）

**Files:**
- Create: `server/internal/pkg/config/config.go`
- Create: `server/internal/pkg/config/config_test.go`
- Create: `server/config/config.dev.yaml`
- Create: `server/config/config.yaml.example`

> **决策依据**：技术架构 12.4 节定义了完整 yaml 结构。本 Plan 只落地基础设施需要的字段（server / postgres / redis / log）。其它字段（llm / tts 等）留待对应 Plan 加入，避免未使用字段污染骨架。

- [ ] **Step 2.1：写失败测试 `config_test.go`**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
  log_level: info
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "/tmp/aibao", cfg.Server.LogDir)
	assert.Equal(t, "info", cfg.Server.LogLevel)
	assert.Equal(t, "127.0.0.1", cfg.Postgres.Host)
	assert.Equal(t, "127.0.0.1:6379", cfg.Redis.Addr)
}

func TestLoad_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
  log_level: info
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
`), 0o600))

	t.Setenv("AIBAO_POSTGRES_PASSWORD", "secret")
	t.Setenv("AIBAO_SERVER_PORT", "9090")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Postgres.Password)
	assert.Equal(t, 9090, cfg.Server.Port, "env should override file")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/file.yaml")
	assert.Error(t, err)
}

func TestLoad_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`server:
  port: 8080
`), 0o600))
	_, err := Load(path)
	assert.Error(t, err, "missing postgres host should fail")
}
```

- [ ] **Step 2.2：运行测试，确认失败**

```bash
cd server && go test ./internal/pkg/config/ -run TestLoad -v
```
Expected: FAIL with `package config: no Go files` 或 `undefined: Load`

- [ ] **Step 2.3：实现 `config.go`**

```go
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	LogDir   string `mapstructure:"log_dir"`
	LogLevel string `mapstructure:"log_level"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"` // from env AIBAO_POSTGRES_PASSWORD
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"` // from env AIBAO_REDIS_PASSWORD
	DB       int    `mapstructure:"db"`
}

// Load reads config from file and overlays env vars (prefix AIBAO_).
// Env naming: AIBAO_SERVER_PORT, AIBAO_POSTGRES_PASSWORD, etc.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("AIBAO")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(envReplacer{})

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

type envReplacer struct{}

func (envReplacer) Replace(s string) string {
	// viper splits keys by ".", env uses "_". e.g. "postgres.host" -> "POSTGRES_HOST"
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			c = '_'
		}
		out = append(out, c)
	}
	return string(out)
}

func validate(c *Config) error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
	}
	if c.Postgres.Host == "" {
		return fmt.Errorf("postgres.host is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	return nil
}
```

- [ ] **Step 2.4：运行测试，确认通过**

```bash
go test ./internal/pkg/config/ -run TestLoad -v
```
Expected: PASS（4 个用例）

- [ ] **Step 2.5：创建 `config/config.dev.yaml`**

```yaml
server:
  port: 8080
  log_dir: ./logs
  log_level: debug

postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
  sslmode: disable
  # password: from env AIBAO_POSTGRES_PASSWORD

redis:
  addr: 127.0.0.1:6379
  db: 0
  # password: from env AIBAO_REDIS_PASSWORD
```

- [ ] **Step 2.6：创建 `config/config.yaml.example`**

内容与 `config.dev.yaml` 相同（生产环境拷贝后修改）。注释里写明 prod 必须改 log_level=info、log_dir=/var/log/aibao。

- [ ] **Step 2.7：提交**

```bash
git add server/internal/pkg/config server/config
git commit -m "feat(config): load yaml with env override and validation"
```

---

## Task 3：traceId 生成与 context 传递

**Files:**
- Create: `server/internal/pkg/traceid/traceid.go`
- Create: `server/internal/pkg/traceid/traceid_test.go`

- [ ] **Step 3.1：写测试 `traceid_test.go`**

```go
package traceid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_Unique(t *testing.T) {
	a := New()
	b := New()
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b)
	assert.True(t, len(a) >= 10)
}

func TestContext_Roundtrip(t *testing.T) {
	ctx := WithID(context.Background(), "tr-abc")
	got, ok := FromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "tr-abc", got)
}

func TestContext_Missing(t *testing.T) {
	got, ok := FromContext(context.Background())
	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestEnsure_GeneratesWhenMissing(t *testing.T) {
	ctx, id := Ensure(context.Background())
	assert.NotEmpty(t, id)
	got, _ := FromContext(ctx)
	assert.Equal(t, id, got)
}

func TestEnsure_KeepsExisting(t *testing.T) {
	ctx := WithID(context.Background(), "tr-existing")
	ctx, id := Ensure(ctx)
	assert.Equal(t, "tr-existing", id)
}
```

- [ ] **Step 3.2：运行测试确认失败**

```bash
go test ./internal/pkg/traceid/ -v
```
Expected: FAIL（包不存在）

- [ ] **Step 3.3：实现 `traceid.go`**

```go
package traceid

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

const Header = "X-Trace-Id"

// New returns a new trace id like "tr-<8-char>" (truncated UUID for log brevity).
func New() string {
	return "tr-" + uuid.NewString()[0:8]
}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// Ensure returns a context with a trace id, generating one if absent.
func Ensure(ctx context.Context) (context.Context, string) {
	if id, ok := FromContext(ctx); ok {
		return ctx, id
	}
	id := New()
	return WithID(ctx, id), id
}
```

- [ ] **Step 3.4：运行测试确认通过**

```bash
go test ./internal/pkg/traceid/ -v
```
Expected: PASS（5 用例）

- [ ] **Step 3.5：提交**

```bash
git add server/internal/pkg/traceid
git commit -m "feat(traceid): context-based trace id with uuid generator"
```

---

## Task 4：敏感字段稳定 hash（safehash）

**Files:**
- Create: `server/internal/pkg/safehash/safehash.go`
- Create: `server/internal/pkg/safehash/safehash_test.go`

> **目的**：日志/错误上报中需要 child_id_hash、phone_hash 等"既可关联又不可还原"的标识。SHA256(salt + value)，盐从配置读。

- [ ] **Step 4.1：写测试**

```go
package safehash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash_Stable(t *testing.T) {
	h := New("test-salt")
	a := h.HashString("13800138000")
	b := h.HashString("13800138000")
	assert.Equal(t, a, b, "same input must produce same hash")
}

func TestHash_DifferentSaltDifferentResult(t *testing.T) {
	a := New("salt1").HashString("x")
	b := New("salt2").HashString("x")
	assert.NotEqual(t, a, b)
}

func TestHash_Prefix(t *testing.T) {
	h := New("salt")
	got := h.HashString("foo")
	assert.True(t, len(got) > 4)
	assert.Equal(t, "h_", got[:2])
}

func TestHash_EmptyInput(t *testing.T) {
	h := New("salt")
	assert.Equal(t, "", h.HashString(""), "empty input returns empty string")
}

func TestNew_RequiresSalt(t *testing.T) {
	defer func() { recover() }()
	_ = New("")
	t.Errorf("expected panic on empty salt")
}
```

- [ ] **Step 4.2：运行测试确认失败**

```bash
go test ./internal/pkg/safehash/ -v
```
Expected: FAIL

- [ ] **Step 4.3：实现 `safehash.go`**

```go
package safehash

import (
	"crypto/sha256"
	"encoding/hex"
)

type Hasher struct {
	salt string
}

// New panics if salt is empty - safehash without salt is meaningless.
func New(salt string) *Hasher {
	if salt == "" {
		panic("safehash: salt must not be empty")
	}
	return &Hasher{salt: salt}
}

// HashString returns "h_<first-12-hex-chars>" or empty string for empty input.
func (h *Hasher) HashString(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(h.salt + ":" + s))
	return "h_" + hex.EncodeToString(sum[:])[:12]
}
```

- [ ] **Step 4.4：运行测试确认通过**

```bash
go test ./internal/pkg/safehash/ -v
```
Expected: PASS（5 用例）

- [ ] **Step 4.5：提交**

```bash
git add server/internal/pkg/safehash
git commit -m "feat(safehash): stable salted hash for log-safe identifiers"
```

---

## Task 5：日志脱敏函数（sanitize）

**Files:**
- Create: `server/internal/pkg/logger/sanitize.go`
- Create: `server/internal/pkg/logger/sanitize_test.go`

- [ ] **Step 5.1：写测试**

```go
package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "138****8000", MaskPhone("13800138000"))
	assert.Equal(t, "***", MaskPhone("123"))
	assert.Equal(t, "", MaskPhone(""))
	assert.Equal(t, "138****8000", MaskPhone("+8613800138000"), "should normalize country code prefix")
}

func TestRedactPromptText(t *testing.T) {
	in := "想听一个奥特曼打怪兽的睡前故事"
	out := RedactPromptText(in)
	assert.NotContains(t, out, "奥特曼")
	assert.Contains(t, out, "len=") // expose length only
}

func TestRedactPromptText_Empty(t *testing.T) {
	assert.Equal(t, "len=0", RedactPromptText(""))
}
```

- [ ] **Step 5.2：实现 `sanitize.go`**

```go
package logger

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaskPhone masks middle 4 digits of a Chinese mobile phone number.
// Returns "138****8000" for "13800138000" and "+8613800138000".
func MaskPhone(phone string) string {
	p := strings.TrimPrefix(phone, "+86")
	if p == "" {
		return ""
	}
	if len(p) < 7 {
		return "***"
	}
	return p[:3] + "****" + p[len(p)-4:]
}

// RedactPromptText keeps only the rune length, never the content.
func RedactPromptText(s string) string {
	return fmt.Sprintf("len=%d", utf8.RuneCountInString(s))
}
```

- [ ] **Step 5.3：运行测试确认通过**

```bash
go test ./internal/pkg/logger/ -v
```
Expected: PASS（4 用例）

- [ ] **Step 5.4：提交**

```bash
git add server/internal/pkg/logger/sanitize.go server/internal/pkg/logger/sanitize_test.go
git commit -m "feat(logger): phone masking and prompt redaction helpers"
```

---

## Task 6：slog logger 封装

**Files:**
- Create: `server/internal/pkg/logger/logger.go`
- Create: `server/internal/pkg/logger/logger_test.go`

> **设计要点**：
> 1. 输出 JSON 单行（technical-architecture 第 10.1 节规定）
> 2. 同时写到文件（lumberjack 滚动）+ stderr
> 3. 自动从 context 提取 trace_id 注入每条日志
> 4. 提供 `FromCtx(ctx)` 取出含 trace_id 的 logger

- [ ] **Step 6.1：写测试**

```go
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zylili/aibao-server/internal/pkg/traceid"
)

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "debug")
	lg.Info("hello", "k", "v")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "v", entry["k"])
	assert.NotEmpty(t, entry["time"])
}

func TestFromCtx_InjectsTraceID(t *testing.T) {
	var buf bytes.Buffer
	base := NewWithWriter(&buf, "debug")
	SetDefault(base)

	ctx := traceid.WithID(context.Background(), "tr-xyz")
	FromCtx(ctx).Info("evt")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "tr-xyz", entry["trace_id"])
}

func TestFromCtx_NoTraceID(t *testing.T) {
	var buf bytes.Buffer
	base := NewWithWriter(&buf, "debug")
	SetDefault(base)

	FromCtx(context.Background()).Info("evt")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	_, has := entry["trace_id"]
	assert.False(t, has, "no trace_id when ctx has none")
}

func TestNewFromConfig_FileOutput(t *testing.T) {
	dir := t.TempDir()
	lg, closer, err := NewFromConfig(dir, "info")
	require.NoError(t, err)
	defer closer()

	lg.Info("hello")

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "log file should be created under log_dir")
}

func TestLevel_FilteredCorrectly(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "warn")
	lg.Debug("nope")
	lg.Info("nope")
	lg.Warn("yep")
	assert.NotContains(t, buf.String(), "nope")
	assert.Contains(t, buf.String(), "yep")
}
```

- [ ] **Step 6.2：实现 `logger.go`**

```go
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/zylili/aibao-server/internal/pkg/traceid"
)

var (
	defaultMu sync.RWMutex
	def       *slog.Logger = slog.Default()
)

// NewWithWriter creates a JSON slog.Logger that writes to w at the given level.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(h)
}

// NewFromConfig creates a logger writing to <dir>/app.log with daily rotation
// (lumberjack: 100MB max size, 14-day retention) and also stderr for dev visibility.
// Returns the logger and a closer to flush on shutdown.
func NewFromConfig(logDir, level string) (*slog.Logger, func() error, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("mkdir log dir: %w", err)
	}

	rot := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "app.log"),
		MaxSize:    100, // MB
		MaxAge:     14,  // days
		MaxBackups: 14,
		Compress:   true,
	}

	mw := io.MultiWriter(rot, os.Stderr)
	lg := NewWithWriter(mw, level)

	return lg, rot.Close, nil
}

func SetDefault(l *slog.Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	def = l
}

func Default() *slog.Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return def
}

// FromCtx returns the default logger pre-populated with trace_id (if present).
func FromCtx(ctx context.Context) *slog.Logger {
	lg := Default()
	if id, ok := traceid.FromContext(ctx); ok {
		return lg.With("trace_id", id)
	}
	return lg
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 6.3：运行测试确认通过**

```bash
go test ./internal/pkg/logger/ -v
```
Expected: PASS（5 + 之前 4 = 9 用例全过）

- [ ] **Step 6.4：提交**

```bash
git add server/internal/pkg/logger/logger.go server/internal/pkg/logger/logger_test.go
git commit -m "feat(logger): slog json with file rotation and trace_id from ctx"
```

---

## Task 7：统一错误类型（errors）

**Files:**
- Create: `server/internal/pkg/errors/errors.go`
- Create: `server/internal/pkg/errors/errors_test.go`

> **目的**：业务层抛 `errors.New(...)` 时携带 HTTP 状态码与面向用户的友好消息，由 api 层统一转 JSON 响应。避免业务层直接依赖 `gin.Context`。

- [ ] **Step 7.1：写测试**

```go
package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_FieldsSet(t *testing.T) {
	e := New(CodeNotFound, "child_not_found", "未找到孩子档案")
	assert.Equal(t, CodeNotFound, e.Code)
	assert.Equal(t, "child_not_found", e.Reason)
	assert.Equal(t, "未找到孩子档案", e.UserMsg)
	assert.Equal(t, http.StatusNotFound, e.HTTPStatus())
}

func TestWrap_PreservesCause(t *testing.T) {
	cause := errors.New("db connection refused")
	e := Wrap(cause, CodeInternal, "db_error", "服务暂时不可用")
	assert.True(t, errors.Is(e, cause))
}

func TestAsAppError(t *testing.T) {
	e := New(CodeInvalidArgument, "bad_input", "参数错误")
	got, ok := AsAppError(e)
	assert.True(t, ok)
	assert.Equal(t, CodeInvalidArgument, got.Code)

	plain := errors.New("plain")
	_, ok = AsAppError(plain)
	assert.False(t, ok)
}

func TestHTTPStatus_AllCodes(t *testing.T) {
	cases := map[Code]int{
		CodeInvalidArgument: http.StatusBadRequest,
		CodeUnauthenticated: http.StatusUnauthorized,
		CodePermissionDenied: http.StatusForbidden,
		CodeNotFound:        http.StatusNotFound,
		CodeRateLimited:     http.StatusTooManyRequests,
		CodeBudgetExceeded:  http.StatusServiceUnavailable,
		CodeInternal:        http.StatusInternalServerError,
	}
	for c, want := range cases {
		assert.Equal(t, want, New(c, "x", "y").HTTPStatus(), "code=%v", c)
	}
}
```

- [ ] **Step 7.2：实现 `errors.go`**

```go
package errors

import (
	stderr "errors"
	"fmt"
	"net/http"
)

type Code int

const (
	CodeInvalidArgument Code = iota + 1
	CodeUnauthenticated
	CodePermissionDenied
	CodeNotFound
	CodeRateLimited
	CodeBudgetExceeded
	CodeInternal
)

type AppError struct {
	Code    Code
	Reason  string // machine-readable, e.g. "child_not_found"
	UserMsg string // user-facing, may be Chinese
	cause   error
}

func New(code Code, reason, userMsg string) *AppError {
	return &AppError{Code: code, Reason: reason, UserMsg: userMsg}
}

func Wrap(cause error, code Code, reason, userMsg string) *AppError {
	return &AppError{Code: code, Reason: reason, UserMsg: userMsg, cause: cause}
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Reason, e.UserMsg, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.UserMsg)
}

func (e *AppError) Unwrap() error { return e.cause }

func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeBudgetExceeded:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func AsAppError(err error) (*AppError, bool) {
	var e *AppError
	if stderr.As(err, &e) {
		return e, true
	}
	return nil, false
}
```

- [ ] **Step 7.3：运行测试确认通过**

```bash
go test ./internal/pkg/errors/ -v
```
Expected: PASS（4 用例）

- [ ] **Step 7.4：提交**

```bash
git add server/internal/pkg/errors
git commit -m "feat(errors): app error type with code, reason, user msg"
```

---

## Task 8：Metrics 全局注册

**Files:**
- Create: `server/internal/metrics/metrics.go`
- Create: `server/internal/metrics/metrics_test.go`

> **本期范围**：仅注册基础设施层指标 `http_request_duration_seconds`、`http_requests_total`。业务指标（story_generate_total 等）由对应 Plan 注册到同一 registry。

- [ ] **Step 8.1：写测试**

```go
package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPMetrics_Registered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)
	require.NotNil(t, m)

	m.HTTPRequests.WithLabelValues("/health", "200").Inc()
	m.HTTPDuration.WithLabelValues("/health", "200").Observe(0.012)

	got := testutil.CollectAndCount(m.HTTPRequests)
	assert.Equal(t, 1, got)

	mf, err := reg.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(mf))
	for _, f := range mf {
		names = append(names, f.GetName())
	}
	joined := strings.Join(names, ",")
	assert.Contains(t, joined, "http_requests_total")
	assert.Contains(t, joined, "http_request_duration_seconds")
}
```

- [ ] **Step 8.2：实现 `metrics.go`**

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
}

// New registers core HTTP metrics on the given registry.
// Pass prometheus.DefaultRegisterer in main.go.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total HTTP requests by path and status.",
			},
			[]string{"path", "status"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration by path and status.",
				Buckets: prometheus.ExponentialBuckets(0.005, 2, 12), // 5ms..~20s
			},
			[]string{"path", "status"},
		),
	}
	reg.MustRegister(m.HTTPRequests, m.HTTPDuration)
	return m
}
```

- [ ] **Step 8.3：运行测试确认通过**

```bash
go test ./internal/metrics/ -v
```
Expected: PASS

- [ ] **Step 8.4：提交**

```bash
git add server/internal/metrics
git commit -m "feat(metrics): register http request counter and duration histogram"
```

---

## Task 9：HTTP Middleware - recover

**Files:**
- Create: `server/internal/api/middleware/recover.go`
- Create: `server/internal/api/middleware/recover_test.go`

- [ ] **Step 9.1：写测试**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRecover_HandlesPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recover())
	r.GET("/boom", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal_error")
}

func TestRecover_PassesThroughNormal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recover())
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 9.2：实现 `recover.go`**

```go
package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/zylili/aibao-server/internal/pkg/logger"
)

func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.FromCtx(c.Request.Context()).Error(
					"http.panic",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", c.FullPath(),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"reason":   "internal_error",
					"user_msg": "服务暂时不可用，请稍后再试",
				})
			}
		}()
		c.Next()
	}
}
```

- [ ] **Step 9.3：运行测试确认通过**

```bash
go test ./internal/api/middleware/ -run TestRecover -v
```
Expected: PASS

- [ ] **Step 9.4：提交**

```bash
git add server/internal/api/middleware/recover.go server/internal/api/middleware/recover_test.go
git commit -m "feat(middleware): recover from panic and respond 500"
```

---

## Task 10：HTTP Middleware - traceId

**Files:**
- Create: `server/internal/api/middleware/traceid.go`
- Create: `server/internal/api/middleware/traceid_test.go`

- [ ] **Step 10.1：写测试**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/zylili/aibao-server/internal/pkg/traceid"
)

func TestTraceID_GeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		id, _ := traceid.FromContext(c.Request.Context())
		seen = id
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	assert.NotEmpty(t, seen)
	assert.Equal(t, seen, rec.Header().Get("X-Trace-Id"))
}

func TestTraceID_HonorsIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		id, _ := traceid.FromContext(c.Request.Context())
		seen = id
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Trace-Id", "tr-incoming")
	r.ServeHTTP(rec, req)

	assert.Equal(t, "tr-incoming", seen)
	assert.Equal(t, "tr-incoming", rec.Header().Get("X-Trace-Id"))
}
```

- [ ] **Step 10.2：实现 `traceid.go`**

```go
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/zylili/aibao-server/internal/pkg/traceid"
)

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		incoming := c.GetHeader(traceid.Header)
		var id string
		if incoming != "" {
			id = incoming
		} else {
			id = traceid.New()
		}
		ctx := traceid.WithID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(traceid.Header, id)
		c.Next()
	}
}
```

- [ ] **Step 10.3：运行测试确认通过**

```bash
go test ./internal/api/middleware/ -run TestTraceID -v
```
Expected: PASS

- [ ] **Step 10.4：提交**

```bash
git add server/internal/api/middleware/traceid.go server/internal/api/middleware/traceid_test.go
git commit -m "feat(middleware): trace id from header or freshly generated"
```

---

## Task 11：HTTP Middleware - logger

**Files:**
- Create: `server/internal/api/middleware/logger.go`
- Create: `server/internal/api/middleware/logger_test.go`

- [ ] **Step 11.1：写测试**

```go
package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zylili/aibao-server/internal/pkg/logger"
)

func TestLogger_LogsStartAndEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger.SetDefault(logger.NewWithWriter(&buf, "debug"))

	r := gin.New()
	r.Use(TraceID(), Logger())
	r.GET("/x", func(c *gin.Context) { c.Status(204) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2)

	var start, end map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &start))
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &end))

	assert.Equal(t, "http.request.start", start["msg"])
	assert.Equal(t, "http.request.done", end["msg"])
	assert.Equal(t, start["trace_id"], end["trace_id"])
	assert.Equal(t, float64(204), end["status"])
	assert.NotNil(t, end["duration_ms"])
}
```

- [ ] **Step 11.2：实现 `logger.go`**

```go
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zylili/aibao-server/internal/pkg/logger"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		lg := logger.FromCtx(c.Request.Context())
		lg.Info("http.request.start",
			"method", c.Request.Method,
			"path", c.FullPath(),
		)
		c.Next()
		lg.Info("http.request.done",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
```

- [ ] **Step 11.3：运行测试确认通过**

```bash
go test ./internal/api/middleware/ -run TestLogger -v
```
Expected: PASS

- [ ] **Step 11.4：提交**

```bash
git add server/internal/api/middleware/logger.go server/internal/api/middleware/logger_test.go
git commit -m "feat(middleware): per-request structured access log"
```

---

## Task 12：HTTP Middleware - metrics

**Files:**
- Create: `server/internal/api/middleware/metrics.go`
- Create: `server/internal/api/middleware/metrics_test.go`

- [ ] **Step 12.1：写测试**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zylili/aibao-server/internal/metrics"
)

func TestMetrics_RecordsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	r := gin.New()
	r.Use(Metrics(m))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	mf, err := reg.Gather()
	require.NoError(t, err)
	var seen bool
	for _, f := range mf {
		if f.GetName() == "http_requests_total" {
			for _, met := range f.GetMetric() {
				if met.GetCounter().GetValue() > 0 {
					seen = true
				}
			}
		}
	}
	assert.True(t, seen, "expected counter to be incremented")
}
```

- [ ] **Step 12.2：实现 `metrics.go`**

```go
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zylili/aibao-server/internal/metrics"
)

func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.HTTPRequests.WithLabelValues(path, status).Inc()
		m.HTTPDuration.WithLabelValues(path, status).Observe(time.Since(start).Seconds())
	}
}
```

- [ ] **Step 12.3：运行测试确认通过**

```bash
go test ./internal/api/middleware/ -run TestMetrics -v
```
Expected: PASS

- [ ] **Step 12.4：提交**

```bash
git add server/internal/api/middleware/metrics.go server/internal/api/middleware/metrics_test.go
git commit -m "feat(middleware): record http metrics per request"
```

---

## Task 13：DB 连接（GORM + PostgreSQL）

**Files:**
- Create: `server/internal/repository/db.go`
- Create: `server/internal/repository/db_test.go`

- [ ] **Step 13.1：写测试（集成测试，build tag）**

```go
//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/zylili/aibao-server/internal/pkg/config"
)

func startPG(t *testing.T) (*postgres.PostgresContainer, config.PostgresConfig) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("aibao"),
		postgres.WithUsername("aibao"),
		postgres.WithPassword("aibao"),
		tc.WithWaitStrategyAndDeadline(30*time.Second),
	)
	require.NoError(t, err)
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432/tcp")
	return pg, config.PostgresConfig{
		Host:     host,
		Port:     port.Int(),
		Database: "aibao",
		User:     "aibao",
		Password: "aibao",
		SSLMode:  "disable",
	}
}

func TestNewDB_Connects(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()

	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer Close(db)

	assert.NoError(t, Ping(context.Background(), db))
}

func TestNewDB_BadHost(t *testing.T) {
	cfg := config.PostgresConfig{Host: "127.0.0.1", Port: 1, Database: "x", User: "x", SSLMode: "disable"}
	_, err := NewDB(cfg)
	assert.Error(t, err)
}
```

- [ ] **Step 13.2：实现 `db.go`**

```go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/zylili/aibao-server/internal/pkg/config"
)

func NewDB(cfg config.PostgresConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, sslMode(cfg.SSLMode),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("pg ping: %w", err)
	}
	return db, nil
}

func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func Close(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func sslMode(s string) string {
	if s == "" {
		return "disable"
	}
	return s
}

var _ = sql.ErrNoRows // silence unused import in some builds
```

- [ ] **Step 13.3：运行集成测试**

```bash
go test -tags=integration ./internal/repository/ -run TestNewDB -v
```
Expected: PASS（需要 Docker）

- [ ] **Step 13.4：提交**

```bash
git add server/internal/repository/db.go server/internal/repository/db_test.go
git commit -m "feat(repo): postgres connection via gorm with pool tuning"
```

---

## Task 14：Redis 客户端

**Files:**
- Create: `server/internal/repository/redis_client.go`
- Create: `server/internal/repository/redis_client_test.go`

- [ ] **Step 14.1：写集成测试**

```go
//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/zylili/aibao-server/internal/pkg/config"
)

func startRedis(t *testing.T) (*redis.RedisContainer, config.RedisConfig) {
	t.Helper()
	ctx := context.Background()
	c, err := redis.Run(ctx, "redis:7-alpine",
		tc.WithWaitStrategyAndDeadline(15*time.Second),
	)
	require.NoError(t, err)
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "6379/tcp")
	return c, config.RedisConfig{Addr: host + ":" + port.Port()}
}

func TestNewRedis_PingPong(t *testing.T) {
	c, cfg := startRedis(t)
	defer func() { _ = c.Terminate(context.Background()) }()

	r, err := NewRedis(cfg)
	require.NoError(t, err)
	defer r.Close()

	assert.NoError(t, PingRedis(context.Background(), r))
}

func TestNewRedis_BadAddr(t *testing.T) {
	_, err := NewRedis(config.RedisConfig{Addr: "127.0.0.1:1"})
	assert.Error(t, err)
}
```

- [ ] **Step 14.2：实现 `redis_client.go`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/zylili/aibao-server/internal/pkg/config"
)

func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return c, nil
}

func PingRedis(ctx context.Context, c *redis.Client) error {
	return c.Ping(ctx).Err()
}
```

- [ ] **Step 14.3：运行集成测试**

```bash
go test -tags=integration ./internal/repository/ -run TestNewRedis -v
```
Expected: PASS

- [ ] **Step 14.4：提交**

```bash
git add server/internal/repository/redis_client.go server/internal/repository/redis_client_test.go
git commit -m "feat(repo): redis client with timeouts and ping check"
```

---

## Task 15：health / ready / metrics handler

**Files:**
- Create: `server/internal/api/health.go`
- Create: `server/internal/api/health_test.go`

- [ ] **Step 15.1：写测试**

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type fakeChecker struct{ err error }

func (f fakeChecker) Check(ctx context.Context) error { return f.err }

func TestHealth_AlwaysOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterHealth(r, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReady_OKWhenAllChecksPass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterHealth(r, fakeChecker{}, fakeChecker{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReady_503WhenAnyCheckFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterHealth(r, fakeChecker{err: errors.New("pg down")}, fakeChecker{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "pg")
}
```

- [ ] **Step 15.2：实现 `health.go`**

```go
package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Checker interface {
	Check(ctx context.Context) error
}

func RegisterHealth(r *gin.Engine, pg, redis Checker) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		ctx := c.Request.Context()
		problems := gin.H{}
		if pg != nil {
			if err := pg.Check(ctx); err != nil {
				problems["pg"] = err.Error()
			}
		}
		if redis != nil {
			if err := redis.Check(ctx); err != nil {
				problems["redis"] = err.Error()
			}
		}
		if len(problems) > 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "problems": problems})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}
```

- [ ] **Step 15.3：运行测试确认通过**

```bash
go test ./internal/api/ -v
```
Expected: PASS（3 用例）

- [ ] **Step 15.4：提交**

```bash
git add server/internal/api/health.go server/internal/api/health_test.go
git commit -m "feat(api): health and ready endpoints with pluggable checkers"
```

---

## Task 16：路由注册（router）

**Files:**
- Create: `server/internal/api/router.go`

> 此 task 仅装配，无新单元测试；冒烟测试在 Task 18 通过 main 启动验证。

- [ ] **Step 16.1：实现 `router.go`**

```go
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zylili/aibao-server/internal/api/middleware"
	"github.com/zylili/aibao-server/internal/metrics"
)

type RouterDeps struct {
	Metrics *metrics.Metrics
	Reg     *prometheus.Registry
	PG      Checker
	Redis   Checker
}

func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(
		middleware.Recover(),
		middleware.TraceID(),
		middleware.Logger(),
		middleware.Metrics(deps.Metrics),
	)

	RegisterHealth(r, deps.PG, deps.Redis)

	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(deps.Reg, promhttp.HandlerOpts{})))

	return r
}
```

- [ ] **Step 16.2：编译检查**

```bash
go build ./...
```
Expected: 无错误

- [ ] **Step 16.3：提交**

```bash
git add server/internal/api/router.go
git commit -m "feat(api): assemble router with middleware and core endpoints"
```

---

## Task 17：数据库迁移 - 初始迁移

**Files:**
- Create: `server/migrations/000001_init.up.sql`
- Create: `server/migrations/000001_init.down.sql`

> 本期仅创建占位表 `infra_check`（用于验证迁移工具流通），业务表全部留给 Plan 2+。

- [ ] **Step 17.1：创建 `000001_init.up.sql`**

```sql
CREATE TABLE IF NOT EXISTS infra_check (
    id          BIGSERIAL PRIMARY KEY,
    note        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO infra_check (note) VALUES ('plan-01-bootstrap');
```

- [ ] **Step 17.2：创建 `000001_init.down.sql`**

```sql
DROP TABLE IF EXISTS infra_check;
```

- [ ] **Step 17.3：手动跑一次迁移验证**

前置：本机已起 PG（监听 5432，用户 `aibao`，库 `aibao`，密码 `aibao`），并已 `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`。

```bash
make migrate-up
```
然后用 `psql` 检查表存在：

```bash
psql "postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable" -c "SELECT note FROM infra_check;"
```
Expected: `plan-01-bootstrap`

- [ ] **Step 17.4：测试 `down`**

```bash
make migrate-down
psql "postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable" -c "\dt infra_check"
```
Expected: `Did not find any relation named "infra_check"`

随后再次 `make migrate-up` 把表重新建好（供 Task 18 使用）。

- [ ] **Step 17.5：提交**

```bash
git add server/migrations
git commit -m "feat(db): initial migration creating infra_check placeholder table"
```

---

## Task 18：main.go 启动与优雅关停

**Files:**
- Create: `server/cmd/server/main.go`

- [ ] **Step 18.1：实现 `main.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/zylili/aibao-server/internal/api"
	"github.com/zylili/aibao-server/internal/metrics"
	"github.com/zylili/aibao-server/internal/pkg/config"
	"github.com/zylili/aibao-server/internal/pkg/logger"
	"github.com/zylili/aibao-server/internal/repository"
	"gorm.io/gorm"

	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("AIBAO_CONFIG")
	if configPath == "" {
		configPath = "config/config.dev.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	lg, closeLog, err := logger.NewFromConfig(cfg.Server.LogDir, cfg.Server.LogLevel)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = closeLog() }()
	logger.SetDefault(lg)
	lg.Info("server.starting", "port", cfg.Server.Port, "log_level", cfg.Server.LogLevel)

	db, err := repository.NewDB(cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer repository.Close(db)

	rdb, err := repository.NewRedis(cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer rdb.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	m := metrics.New(reg)

	router := api.NewRouter(api.RouterDeps{
		Metrics: m,
		Reg:     reg,
		PG:      pgChecker{db: db},
		Redis:   redisChecker{c: rdb},
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("server.listen", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		lg.Info("server.shutdown.signal")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("server.shutdown.error", "err", err)
		return err
	}
	lg.Info("server.shutdown.done")
	return nil
}

type pgChecker struct{ db *gorm.DB }

func (p pgChecker) Check(ctx context.Context) error { return repository.Ping(ctx, p.db) }

type redisChecker struct{ c *redis.Client }

func (r redisChecker) Check(ctx context.Context) error { return repository.PingRedis(ctx, r.c) }
```

- [ ] **Step 18.2：编译**

```bash
go build -o bin/aibao-server ./cmd/server
```
Expected: 无错误

- [ ] **Step 18.3：本地启动冒烟（前置：PG 与 Redis 运行中，迁移已 up）**

```bash
make run-dev
```
Expected：终端输出 JSON 日志，含 `server.starting`、`server.listen`。新开一个终端：

```bash
curl -i localhost:8080/health
curl -i localhost:8080/ready
curl -s localhost:8080/metrics | head -50
```
Expected：
- `/health` → 200，`{"status":"ok"}`
- `/ready` → 200（PG/Redis 健康），关掉 Redis 后 → 503 含 `redis` 字段
- `/metrics` → 含 `http_requests_total`、`http_request_duration_seconds`、`go_*`、`process_*`

- [ ] **Step 18.4：验证 trace_id 贯穿**

```bash
curl -s -H "X-Trace-Id: tr-smoke" localhost:8080/health
grep tr-smoke logs/app.log
```
Expected：日志中有两条带 `"trace_id":"tr-smoke"` 的记录（`http.request.start` 与 `http.request.done`），且 `X-Trace-Id` 响应头为 `tr-smoke`。

- [ ] **Step 18.5：验证优雅关停**

```bash
# 在另一个终端发送 SIGTERM
kill -TERM $(pgrep aibao-server)
```
Expected：日志输出 `server.shutdown.signal` → `server.shutdown.done`，进程退出码 0。

- [ ] **Step 18.6：提交**

```bash
git add server/cmd/server/main.go
git commit -m "feat(server): main entrypoint with graceful shutdown"
```

---

## Task 19：覆盖率验证

- [ ] **Step 19.1：运行单元测试，输出覆盖率**

```bash
cd server
go test -count=1 -cover ./internal/pkg/... ./internal/api/... ./internal/metrics/...
```
Expected：
- 每个被测包覆盖率 ≥ 70%
- `internal/pkg/config` ≥ 80%
- `internal/pkg/traceid` ≥ 90%
- `internal/pkg/safehash` ≥ 90%
- `internal/pkg/errors` ≥ 80%
- `internal/pkg/logger` ≥ 70%
- `internal/api` ≥ 70%
- `internal/api/middleware` ≥ 80%
- `internal/metrics` ≥ 60%（注册类代码覆盖率天然偏低）

如有不达标的包，回到对应 Task 补充测试。

- [ ] **Step 19.2：运行集成测试**

```bash
go test -count=1 -tags=integration ./internal/repository/...
```
Expected：PASS（需 Docker）。

- [ ] **Step 19.3：运行 lint**

```bash
make lint
```
Expected：0 issues。

- [ ] **Step 19.4：提交（如果之前因补测试有改动）**

无改动则跳过。

---

## Task 20：systemd 单元模板（占位入仓）

**Files:**
- Create: `server/scripts/systemd/aibao-server.service.example`

> Plan 9 会真正写部署脚本，本期只把单元模板入仓避免 Plan 2+ 部署时回头补。

- [ ] **Step 20.1：创建文件**

```ini
[Unit]
Description=Aibao Server
After=network.target postgresql.service redis-server.service
Wants=postgresql.service redis-server.service

[Service]
Type=simple
User=aibao
Group=aibao
WorkingDirectory=/opt/aibao
EnvironmentFile=/etc/aibao/aibao.env
ExecStart=/opt/aibao/bin/aibao-server
Restart=on-failure
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=30
LimitNOFILE=65536
# Memory soft limit 400 MiB
Environment=GOMEMLIMIT=400MiB

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 20.2：提交**

```bash
mkdir -p server/scripts/systemd
git add server/scripts/systemd/aibao-server.service.example
git commit -m "chore: add systemd unit example for aibao-server"
```

---

## 完成验收清单

逐项勾选确认后，本 Plan 视为完成。

- [ ] `go build ./...` 通过
- [ ] `make test` 全部通过
- [ ] `make test-integration` 全部通过（需 Docker）
- [ ] `make lint` 0 issues
- [ ] service+pkg 层覆盖率 ≥ 70%
- [ ] `make run-dev` 启动后输出符合规范的 JSON 日志（含 server.starting / server.listen）
- [ ] `curl /health` → 200 OK
- [ ] `curl /ready` → 200（PG+Redis 健康），任意一个挂掉 → 503 且 body 指明哪个挂了
- [ ] `curl /metrics` → 包含 `http_requests_total`、`http_request_duration_seconds`
- [ ] 同一请求的多条日志 `trace_id` 一致；外部传入 `X-Trace-Id` 被尊重
- [ ] `make migrate-up` 创建 `infra_check` 表；`make migrate-down` 删除
- [ ] SIGTERM 触发优雅关停，日志包含 `server.shutdown.signal` 与 `server.shutdown.done`
- [ ] 提交粒度合理（每个 Task 至少一个 commit），无 WIP 留存

---

## 后续 Plan 衔接

下一份 plan（Plan 2：用户认证 + 孩子档案）将基于本 Plan 的骨架增量开发：
- 复用 `pkg/config`、`pkg/logger`、`pkg/errors`、`pkg/safehash`
- 新增 `service/auth`、`service/child`、`repository/user_repo.go`、`repository/child_repo.go`
- 新增 migrations `000002_users.up.sql`（users 表 + children 表 + UNIQUE(user_id) 约束）
- 在 router 中追加 `/api/v1/auth/*` 与 `/api/v1/children/*`

请审阅本 Plan，通过后我开始写 Plan 2。
