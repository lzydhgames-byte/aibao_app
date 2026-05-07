# Plan 2：用户认证 + 孩子档案 实现规划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Plan 1 基础设施上实现"手机号验证码登录"+"孩子档案 CRUD"两条 API 链路，让用户能完成"注册→创建孩子→修改孩子档案"的端到端流程，并通过 JWT 保护所有非公开接口。

**Architecture:** 严格三层（api / service / repository）。SMS 通过 Gateway 抽象层（一期仅 Mock 实现，固定码 `123456`，腾讯实现接口预留）。手机号 hash + 加密双存（hash 用于查询、加密用于以后真发短信）。JWT 用 HS256，access token 24h、refresh token 7d。孩子档案表用 `UNIQUE(user_id)` 强制一期单孩子。所有写库 + JWT signing key 走环境变量，不入 git。

**Tech Stack:**
- Go 1.24+ + Gin + GORM + PostgreSQL
- `github.com/golang-jwt/jwt/v5` —— JWT 签发与校验
- 标准库 `crypto/aes` + `crypto/cipher` —— 手机号加密
- 复用 Plan 1 已有：viper config、slog logger、AppError、metrics、Recover/TraceID/Logger/Metrics 中间件、health/ready/router

**前置阅读：**
- 产品 spec：[2026-04-28-aibao-design.md](../specs/2026-04-28-aibao-design.md)（第 3.4 节 USER.md 双层；第 5.1 注册流程）
- 技术架构：[2026-04-28-aibao-tech-architecture.md](../specs/2026-04-28-aibao-tech-architecture.md)
  - 第 5.1 users / children 表设计
  - 第 5.4 数据合规（手机号 hash + 加密；child_id_hash 出现在日志）
  - 第 13.2 JWT、密码学

**完成验收（Definition of Done）：**

1. `go build ./...` + `go test ./...` 全过；service+pkg 层覆盖率 ≥ 70%
2. `make migrate-up` 执行新增迁移 `000002_users_and_children.up.sql`，建出 users + children 表
3. `make run-dev` 启动后能完成完整流程（用 curl 演示）：
   - `POST /api/v1/auth/sms/send {"phone":"13800138000"}` → 200，开发模式日志中出现固定码 `123456`
   - `POST /api/v1/auth/login_or_register {"phone":"13800138000","code":"123456","nickname":"妈妈"}` → 200，返回 access/refresh token + user 信息
   - `GET /api/v1/me` 带 `Authorization: Bearer <access>` → 200，返回当前用户信息
   - `POST /api/v1/children {"nickname":"小宇","gender":"boy","birthday":"2020-08-15"}` → 201，返回 child 信息
   - 第二次 POST 同一 user 创建 child → 409，提示已存在
   - `GET /api/v1/children` → 200，返回当前 user 的孩子（只可能 0 或 1 个）
   - `PATCH /api/v1/children/{id}` 部分字段更新 → 200
4. 没带 Authorization 访问 `/api/v1/me` → 401
5. JWT 过期或被篡改 → 401
6. 验证码错误 / 验证码已用 / 短信发送频次超限 → 各自语义化的错误码
7. 日志中**绝不**出现明文手机号、明文 JWT secret；手机号显示为 `138****8000`，user/child 用 hash
8. 路由按 spec 第 13 章规范挂在 `/api/v1/...` 下

---

## 范围决策记录

- **SMS Provider 一期仅 Mock**：固定验证码 `123456`，Gateway 抽象层为腾讯 SMS 预留实现位
- **注册采用两步式 + 自动注册**：`POST /auth/sms/send` 发码 → `POST /auth/login_or_register`（已注册登录、未注册创建）
- **孩子档案 Plan 2 仅做基础 CRUD**：昵称/性别/年龄；BOOTSTRAP 相遇仪式留给后续 Plan
- **一期单孩子约束**：DB 层 `UNIQUE(user_id)` 强制；二期开放多孩子时只需删约束 + 前端切换 UI

---

## File Structure

### 数据迁移

| 文件 | 职责 |
|---|---|
| `server/migrations/000002_users_and_children.up.sql` | 创建 users / children 表，加索引和 UNIQUE 约束 |
| `server/migrations/000002_users_and_children.down.sql` | 删除两表 |

### 配置扩展

| 文件 | 修改 |
|---|---|
| `server/internal/pkg/config/config.go` | 增加 `Auth`、`SMS`、`Crypto` 三个子配置 |
| `server/config/config.dev.yaml` | 增加对应的 dev 默认值（密钥占位由 env 注入） |
| `server/config/config.yaml.example` | 同上，但带 prod 提示注释 |

### 认证核心（pkg）

| 文件 | 职责 |
|---|---|
| `server/internal/pkg/jwtauth/jwt.go` | JWT 签发与解析（HS256） |
| `server/internal/pkg/jwtauth/jwt_test.go` | 测试 |
| `server/internal/pkg/phonecrypt/phonecrypt.go` | 手机号 AES-GCM 加密、解密 |
| `server/internal/pkg/phonecrypt/phonecrypt_test.go` | 测试 |
| `server/internal/pkg/safehash/safehash.go` | 已存在；本 Plan 不改 |

### Gateway 层（SMS）

| 文件 | 职责 |
|---|---|
| `server/internal/gateway/sms/sms.go` | `Sender` 接口定义 |
| `server/internal/gateway/sms/mock.go` | Mock 实现（固定 `123456`，仅打日志） |
| `server/internal/gateway/sms/mock_test.go` | 测试 |

### 数据模型

| 文件 | 职责 |
|---|---|
| `server/internal/model/user.go` | User 结构体（含 GORM tag） |
| `server/internal/model/child.go` | Child 结构体 |

### Repository 层

| 文件 | 职责 |
|---|---|
| `server/internal/repository/user_repo.go` | UserRepo 接口 + GORM 实现：CreateOrGet、FindByPhoneHash、FindByID |
| `server/internal/repository/user_repo_test.go` | 集成测试（testcontainers） |
| `server/internal/repository/child_repo.go` | ChildRepo 接口 + GORM 实现：Create、FindByUserID、Update、FindByID |
| `server/internal/repository/child_repo_test.go` | 集成测试 |
| `server/internal/repository/migrate.go` | `RunMigrations(db, dir)` —— 启动时自动跑迁移（main.go 用） |

### Service 层

| 文件 | 职责 |
|---|---|
| `server/internal/service/auth/codestore.go` | 验证码存储接口（Redis 实现） |
| `server/internal/service/auth/codestore_redis.go` | Redis 实现 |
| `server/internal/service/auth/codestore_test.go` | 集成测试 |
| `server/internal/service/auth/auth.go` | Service 主体：SendSMS、LoginOrRegister、IssueTokens、ValidateAccess |
| `server/internal/service/auth/auth_test.go` | 单元测试（mock repo + mock sms + mock codestore） |
| `server/internal/service/child/child.go` | Service：Create、Get、Update（强制 UNIQUE） |
| `server/internal/service/child/child_test.go` | 单元测试 |

### API 层

| 文件 | 职责 |
|---|---|
| `server/internal/api/auth.go` | `/api/v1/auth/sms/send`、`/api/v1/auth/login_or_register` handler |
| `server/internal/api/auth_test.go` | handler 测试 |
| `server/internal/api/me.go` | `/api/v1/me` handler |
| `server/internal/api/me_test.go` | 测试 |
| `server/internal/api/child.go` | `/api/v1/children` handler（POST / GET / PATCH） |
| `server/internal/api/child_test.go` | 测试 |
| `server/internal/api/middleware/auth.go` | `JWTAuth(secret)` middleware：解析 Authorization header，把 user_id 注入 context |
| `server/internal/api/middleware/auth_test.go` | 测试 |
| `server/internal/api/router.go` | 修改：注入 service 依赖、挂载 v1 路由组 + JWT middleware |
| `server/internal/api/errs.go` | `RespondError(c, err)` —— 把 AppError 转成统一 JSON 响应 |
| `server/internal/api/errs_test.go` | 测试 |
| `server/internal/api/userctx/userctx.go` | `WithUserID` / `FromContext` 帮助函数（避免循环依赖） |

### main.go

| 文件 | 修改 |
|---|---|
| `server/cmd/server/main.go` | 增加：`RunMigrations` 调用、构造 SMS、CodeStore、Repos、Services；router 注入 |

---

## 依赖与版本约定

`go.mod` 新增：

```
require (
    github.com/golang-jwt/jwt/v5 v5.2.1
)
```

注：手机号加密用 Go 标准库 `crypto/aes` + `crypto/cipher`，无新依赖。

---

## 数据模型字段约定（先定好，多个 Task 引用）

### users 表

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigserial PK | |
| `phone_hash` | varchar(64) UNIQUE NOT NULL | safehash.HashString 后的手机号 |
| `phone_encrypted` | bytea NOT NULL | AES-GCM 加密原文（含 12 字节 nonce 前缀） |
| `nickname` | varchar(50) NOT NULL | 默认 "家长"，注册时家长可填 |
| `subscription_tier` | varchar(20) NOT NULL DEFAULT 'free' | free / pro |
| `created_at` | timestamptz NOT NULL DEFAULT NOW() | |
| `updated_at` | timestamptz NOT NULL DEFAULT NOW() | |

索引：`UNIQUE(phone_hash)`

### children 表

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigserial PK | |
| `user_id` | bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE | |
| `nickname` | varchar(50) NOT NULL | |
| `gender` | varchar(10) NOT NULL | "boy" / "girl" / "unspecified" |
| `birthday` | date NOT NULL | |
| `profile` | jsonb NOT NULL DEFAULT '{}'::jsonb | 喜好/害怕/家人——本 Plan 暂留空，BOOTSTRAP plan 填充 |
| `created_at` | timestamptz NOT NULL DEFAULT NOW() | |
| `updated_at` | timestamptz NOT NULL DEFAULT NOW() | |

约束：
- `UNIQUE(user_id)` —— 一期单孩子
- 索引：`children(user_id)` —— 由 UNIQUE 自动建

---

## API 形态（先定好契约）

所有路径前缀 `/api/v1`。请求/响应 body 都是 JSON。错误响应统一 `{"reason":"...","user_msg":"..."}`。

### POST `/api/v1/auth/sms/send`
**Request:** `{"phone":"13800138000"}`
**Response 200:** `{"sent":true}`（dev 环境额外日志输出 `code=123456`）
**错误：** 400 phone_invalid（格式不对）；429 sms_rate_limited（同手机号 60s 内已发过）

### POST `/api/v1/auth/login_or_register`
**Request:** `{"phone":"13800138000","code":"123456","nickname":"妈妈"}`
- `nickname` 可选，未提供时默认 "家长"
**Response 200:**
```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user": {
    "id": 1,
    "nickname": "妈妈",
    "subscription_tier": "free"
  }
}
```
**错误：** 400 phone_invalid / code_invalid（格式）；401 code_mismatch；401 code_expired

### GET `/api/v1/me`
带 `Authorization: Bearer <access>`。
**Response 200:** `{"id":1,"nickname":"妈妈","subscription_tier":"free"}`
**错误：** 401 unauthorized（缺 header / 无效 token / 过期）

### POST `/api/v1/children`
带 Authorization。
**Request:** `{"nickname":"小宇","gender":"boy","birthday":"2020-08-15"}`
**Response 201:**
```json
{ "id": 1, "user_id": 1, "nickname": "小宇", "gender": "boy", "birthday": "2020-08-15", "profile": {} }
```
**错误：** 400 invalid_argument（字段缺失/格式错）；409 child_already_exists（一期单孩子）

### GET `/api/v1/children`
带 Authorization。
**Response 200:** `{"items":[{...}]}`（items 长度 0 或 1）

### PATCH `/api/v1/children/{id}`
带 Authorization。所有字段可选。
**Request:** `{"nickname":"小宇宙","birthday":"2020-08-15"}`
**Response 200:** 更新后的 child
**错误：** 404 child_not_found；403 permission_denied（孩子不属于当前用户）

---

# Tasks

## Task 0：迁移文件 `000002_users_and_children`

**Files:**
- Create: `server/migrations/000002_users_and_children.up.sql`
- Create: `server/migrations/000002_users_and_children.down.sql`

- [ ] **Step 0.1：创建 up SQL**

`server/migrations/000002_users_and_children.up.sql`：

```sql
CREATE TABLE IF NOT EXISTS users (
    id                 BIGSERIAL PRIMARY KEY,
    phone_hash         VARCHAR(64)  NOT NULL UNIQUE,
    phone_encrypted    BYTEA        NOT NULL,
    nickname           VARCHAR(50)  NOT NULL,
    subscription_tier  VARCHAR(20)  NOT NULL DEFAULT 'free',
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS children (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname    VARCHAR(50)  NOT NULL,
    gender      VARCHAR(10)  NOT NULL,
    birthday    DATE         NOT NULL,
    profile     JSONB        NOT NULL DEFAULT '{}'::JSONB,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT children_user_unique UNIQUE (user_id)
);
```

- [ ] **Step 0.2：创建 down SQL**

`server/migrations/000002_users_and_children.down.sql`：

```sql
DROP TABLE IF EXISTS children;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 0.3：手动跑一次验证（可选，需要 Docker）**

```bash
cd server
make migrate-up
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\d users"
docker exec aibao-postgres-dev psql -U aibao -d aibao -c "\d children"
```
Expected: 两表存在，children 有 `children_user_unique` UNIQUE 约束。

- [ ] **Step 0.4：commit**

```bash
git add server/migrations/000002_users_and_children.up.sql \
        server/migrations/000002_users_and_children.down.sql
git commit -m "feat(db): users and children tables with UNIQUE(user_id)"
```

---

## Task 1：扩展配置（Auth / SMS / Crypto）

**Files:**
- Modify: `server/internal/pkg/config/config.go`
- Modify: `server/internal/pkg/config/config_test.go`
- Modify: `server/config/config.dev.yaml`
- Modify: `server/config/config.yaml.example`

- [ ] **Step 1.1：修改 `config.go` 增加新结构体**

在 `Config` 中追加：

```go
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	SMS      SMSConfig      `mapstructure:"sms"`
	Crypto   CryptoConfig   `mapstructure:"crypto"`
}

// AuthConfig holds JWT signing parameters.
type AuthConfig struct {
	JWTSecret             string `mapstructure:"jwt_secret"`              // env AIBAO_AUTH_JWT_SECRET
	AccessTTLMinutes      int    `mapstructure:"access_ttl_minutes"`      // 24h = 1440
	RefreshTTLMinutes     int    `mapstructure:"refresh_ttl_minutes"`     // 7d  = 10080
}

// SMSConfig holds SMS provider parameters. In MVP we only support "mock".
type SMSConfig struct {
	Provider          string `mapstructure:"provider"`             // "mock" / "tencent" (future)
	CodeTTLSeconds    int    `mapstructure:"code_ttl_seconds"`     // verification code lifetime
	ResendCooldownSec int    `mapstructure:"resend_cooldown_sec"`  // per-phone send rate limit
}

// CryptoConfig holds at-rest encryption parameters.
type CryptoConfig struct {
	// PhoneAESKey is a 32-byte (hex-encoded 64-char) key for AES-256-GCM
	// encryption of phone numbers. From env AIBAO_CRYPTO_PHONE_AES_KEY.
	PhoneAESKey string `mapstructure:"phone_aes_key"`
	// SafehashSalt is the salt used for safehash.HashString of phone, child id,
	// etc. From env AIBAO_CRYPTO_SAFEHASH_SALT.
	SafehashSalt string `mapstructure:"safehash_salt"`
}
```

更新 `validate(c *Config, path string) error` 末尾增加新字段校验：

```go
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("config %s: auth.jwt_secret is required (set AIBAO_AUTH_JWT_SECRET)", path)
	}
	if c.Auth.AccessTTLMinutes == 0 {
		c.Auth.AccessTTLMinutes = 24 * 60
	}
	if c.Auth.RefreshTTLMinutes == 0 {
		c.Auth.RefreshTTLMinutes = 7 * 24 * 60
	}
	if c.SMS.Provider == "" {
		c.SMS.Provider = "mock"
	}
	if c.SMS.CodeTTLSeconds == 0 {
		c.SMS.CodeTTLSeconds = 300
	}
	if c.SMS.ResendCooldownSec == 0 {
		c.SMS.ResendCooldownSec = 60
	}
	if c.Crypto.PhoneAESKey == "" {
		return fmt.Errorf("config %s: crypto.phone_aes_key is required (set AIBAO_CRYPTO_PHONE_AES_KEY, 64 hex chars)", path)
	}
	if c.Crypto.SafehashSalt == "" {
		return fmt.Errorf("config %s: crypto.safehash_salt is required (set AIBAO_CRYPTO_SAFEHASH_SALT)", path)
	}
```

并把 `Load(path)` 中的 `BindEnv` 列表追加：

```go
	binds := []string{
		"server.port", "server.log_dir", "server.log_level",
		"postgres.host", "postgres.port", "postgres.database",
		"postgres.user", "postgres.password", "postgres.sslmode",
		"redis.addr", "redis.password", "redis.db",
		"auth.jwt_secret", "auth.access_ttl_minutes", "auth.refresh_ttl_minutes",
		"sms.provider", "sms.code_ttl_seconds", "sms.resend_cooldown_sec",
		"crypto.phone_aes_key", "crypto.safehash_salt",
	}
	for _, k := range binds {
		_ = v.BindEnv(k)
	}
```

如果之前的 BindEnv 列表写法不同，按当前文件实际情况扩展，保持等价行为。

- [ ] **Step 1.2：扩展测试**

在 `config_test.go` 的 `writeValidConfig` helper 中，把 yaml 内容扩展为：

```yaml
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
auth:
  jwt_secret: dev-secret
  access_ttl_minutes: 1440
  refresh_ttl_minutes: 10080
sms:
  provider: mock
  code_ttl_seconds: 300
  resend_cooldown_sec: 60
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: dev-salt
```

新增测试：

```go
func TestLoad_AuthAndSMSDefaults(t *testing.T) {
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
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 1440, cfg.Auth.AccessTTLMinutes)
	assert.Equal(t, 10080, cfg.Auth.RefreshTTLMinutes)
	assert.Equal(t, "mock", cfg.SMS.Provider)
	assert.Equal(t, 300, cfg.SMS.CodeTTLSeconds)
	assert.Equal(t, 60, cfg.SMS.ResendCooldownSec)
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
`), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth.jwt_secret")
}
```

更新原有 `TestLoad_FromFile` / `TestLoad_EnvOverride`（如果它们 unmarshal 后断言了字段），让它们能在 yaml 里多了 auth/sms/crypto 块时仍通过——通过 `writeValidConfig` 已统一就 OK。

- [ ] **Step 1.3：跑测试**

```bash
cd server && go test -count=1 ./internal/pkg/config/ -v
```
Expected: 全过（含新增 2 个用例）。

- [ ] **Step 1.4：更新 dev yaml**

`server/config/config.dev.yaml` 末尾追加：

```yaml

auth:
  access_ttl_minutes: 1440
  refresh_ttl_minutes: 10080
  # jwt_secret: from env AIBAO_AUTH_JWT_SECRET

sms:
  provider: mock
  code_ttl_seconds: 300
  resend_cooldown_sec: 60

crypto:
  # phone_aes_key: from env AIBAO_CRYPTO_PHONE_AES_KEY (64 hex chars = 32 bytes)
  # safehash_salt: from env AIBAO_CRYPTO_SAFEHASH_SALT
```

`server/config/config.yaml.example` 同步追加，注释里写明 prod 的 secret 必须用 `openssl rand -hex 32` 生成。

- [ ] **Step 1.5：commit**

```bash
git add server/internal/pkg/config server/config
git commit -m "feat(config): auth/sms/crypto config blocks with env binding"
```

---

## Task 2：JWT 包

**Files:**
- Create: `server/internal/pkg/jwtauth/jwt.go`
- Create: `server/internal/pkg/jwtauth/jwt_test.go`

- [ ] **Step 2.1：写测试**

`server/internal/pkg/jwtauth/jwt_test.go`：

```go
package jwtauth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueAndParseAccess_RoundTrip(t *testing.T) {
	m := New("secret-x", time.Hour, 7*24*time.Hour)
	tok, err := m.IssueAccess(42)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	claims, err := m.ParseAccess(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "access", claims.Type)
}

func TestParseAccess_RejectsRefreshToken(t *testing.T) {
	m := New("secret-x", time.Hour, 7*24*time.Hour)
	tok, err := m.IssueRefresh(42)
	require.NoError(t, err)

	_, err = m.ParseAccess(tok)
	assert.Error(t, err, "refresh token should not be accepted by ParseAccess")
}

func TestParseAccess_RejectsBadSignature(t *testing.T) {
	a := New("secret-a", time.Hour, time.Hour)
	b := New("secret-b", time.Hour, time.Hour)
	tok, _ := a.IssueAccess(1)
	_, err := b.ParseAccess(tok)
	assert.Error(t, err)
}

func TestParseAccess_RejectsExpired(t *testing.T) {
	m := New("secret-x", -time.Minute, time.Hour) // already-expired
	tok, _ := m.IssueAccess(1)
	_, err := m.ParseAccess(tok)
	assert.Error(t, err)
}

func TestParseAccess_RejectsMalformed(t *testing.T) {
	m := New("secret-x", time.Hour, time.Hour)
	_, err := m.ParseAccess("not-a-jwt")
	assert.Error(t, err)
}
```

- [ ] **Step 2.2：跑确认 FAIL**

```bash
go test ./internal/pkg/jwtauth/ -v
```
Expected: FAIL（包不存在）

- [ ] **Step 2.3：实现 `jwt.go`**

```go
// Package jwtauth issues and validates HS256 JWTs for the auth flow.
// Two token types are issued: access (short-lived) and refresh (long-lived).
// The Type claim distinguishes them so an attacker cannot present a refresh
// token where an access token is expected.
package jwtauth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Manager issues and validates JWTs.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// New constructs a Manager with the given HMAC secret and TTLs.
func New(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// Claims is the JWT claim set we use.
type Claims struct {
	UserID int64  `json:"uid"`
	Type   string `json:"typ"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// IssueAccess issues a short-lived access token for the given user.
func (m *Manager) IssueAccess(userID int64) (string, error) {
	return m.issue(userID, "access", m.accessTTL)
}

// IssueRefresh issues a long-lived refresh token for the given user.
func (m *Manager) IssueRefresh(userID int64) (string, error) {
	return m.issue(userID, "refresh", m.refreshTTL)
}

func (m *Manager) issue(userID int64, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// ParseAccess validates the token and returns its claims, but only if Type=="access".
func (m *Manager) ParseAccess(s string) (*Claims, error) {
	return m.parse(s, "access")
}

// ParseRefresh validates the token and returns its claims, but only if Type=="refresh".
func (m *Manager) ParseRefresh(s string) (*Claims, error) {
	return m.parse(s, "refresh")
}

func (m *Manager) parse(s, requiredType string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(s, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Type != requiredType {
		return nil, fmt.Errorf("expected %s token, got %s", requiredType, claims.Type)
	}
	return claims, nil
}
```

添加依赖：

```bash
cd server && GOPROXY=https://goproxy.cn,direct go get github.com/golang-jwt/jwt/v5
```

- [ ] **Step 2.4：跑确认 PASS**

```bash
go test ./internal/pkg/jwtauth/ -v
```
Expected: 5/5 PASS。

- [ ] **Step 2.5：lint**

```bash
golangci-lint run ./internal/pkg/jwtauth/...
```
Expected: 0 issues。

- [ ] **Step 2.6：commit**

```bash
git add server/internal/pkg/jwtauth server/go.mod server/go.sum
git commit -m "feat(jwtauth): HS256 jwt issuer with access/refresh type guard"
```

---

## Task 3：手机号加密包（phonecrypt）

**Files:**
- Create: `server/internal/pkg/phonecrypt/phonecrypt.go`
- Create: `server/internal/pkg/phonecrypt/phonecrypt_test.go`

- [ ] **Step 3.1：写测试**

`server/internal/pkg/phonecrypt/phonecrypt_test.go`：

```go
package phonecrypt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRoundTrip(t *testing.T) {
	c, err := New(testKeyHex)
	require.NoError(t, err)

	enc, err := c.Encrypt("13800138000")
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "13800138000", dec)
}

func TestEncrypt_DifferentNonceEachCall(t *testing.T) {
	c, _ := New(testKeyHex)
	a, _ := c.Encrypt("13800138000")
	b, _ := c.Encrypt("13800138000")
	assert.NotEqual(t, a, b, "AES-GCM with random nonce must produce different ciphertexts")
}

func TestNew_RejectsShortKey(t *testing.T) {
	_, err := New("deadbeef")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "32 bytes")
}

func TestNew_RejectsBadHex(t *testing.T) {
	_, err := New("not-hex-and-too-shortzzz" + strings.Repeat("z", 40))
	require.Error(t, err)
}

func TestDecrypt_RejectsTampered(t *testing.T) {
	c, _ := New(testKeyHex)
	enc, _ := c.Encrypt("13800138000")
	enc[len(enc)-1] ^= 0x01
	_, err := c.Decrypt(enc)
	assert.Error(t, err)
}
```

- [ ] **Step 3.2：跑确认 FAIL**

```bash
go test ./internal/pkg/phonecrypt/ -v
```

- [ ] **Step 3.3：实现**

`server/internal/pkg/phonecrypt/phonecrypt.go`：

```go
// Package phonecrypt provides AES-256-GCM encryption for phone numbers stored
// at rest. Plaintext is needed only when sending real SMS; queries always use
// the safehash representation. Each call to Encrypt produces a fresh random
// nonce, prefixed to the ciphertext so Decrypt can recover it.
package phonecrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Cipher encrypts/decrypts strings with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New constructs a Cipher from a 64-char hex key (32 bytes / 256 bits).
func New(keyHex string) (*Cipher, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("phone aes key must be 32 bytes (got %d)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns a byte slice of the form: nonce || ciphertext || tag.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read random nonce: %w", err)
	}
	out := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return out, nil
}

// Decrypt parses a value produced by Encrypt and returns the plaintext.
func (c *Cipher) Decrypt(blob []byte) (string, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns+c.aead.Overhead() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("aes-gcm open: %w", err)
	}
	return string(pt), nil
}
```

- [ ] **Step 3.4：跑确认 PASS**

```bash
go test ./internal/pkg/phonecrypt/ -v
```
Expected: 5/5 PASS。

- [ ] **Step 3.5：commit**

```bash
golangci-lint run ./internal/pkg/phonecrypt/...
git add server/internal/pkg/phonecrypt
git commit -m "feat(phonecrypt): aes-256-gcm round trip with random nonce"
```

---

## Task 4：SMS Gateway（接口 + Mock 实现）

**Files:**
- Create: `server/internal/gateway/sms/sms.go`
- Create: `server/internal/gateway/sms/mock.go`
- Create: `server/internal/gateway/sms/mock_test.go`

- [ ] **Step 4.1：写测试**

`server/internal/gateway/sms/mock_test.go`：

```go
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/pkg/logger"
)

func TestMockSender_SendsFixedCodeAndLogs(t *testing.T) {
	var buf bytes.Buffer
	logger.SetDefault(logger.NewWithWriter(&buf, "debug"))

	m := NewMock()
	require.Equal(t, "123456", m.FixedCode())

	err := m.SendCode(context.Background(), "13800138000", "123456")
	require.NoError(t, err)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &entry))
	assert.Equal(t, "sms.mock.send", entry["msg"])
	// phone must be masked
	assert.Equal(t, "138****8000", entry["phone"])
	// code present so the dev can read it
	assert.Equal(t, "123456", entry["code"])
}

func TestMockSender_ImplementsSenderInterface(t *testing.T) {
	var s Sender = NewMock()
	assert.NotNil(t, s)
}
```

- [ ] **Step 4.2：实现接口 `sms.go`**

```go
// Package sms abstracts SMS providers behind a Sender interface. The MVP only
// ships a mock implementation that logs the message and uses a fixed code; a
// future Tencent Cloud SMS implementation will plug in via the same interface.
package sms

import "context"

// Sender sends a verification code to a phone number.
type Sender interface {
	SendCode(ctx context.Context, phone, code string) error
}
```

- [ ] **Step 4.3：实现 Mock**

`server/internal/gateway/sms/mock.go`：

```go
package sms

import (
	"context"

	"github.com/aibao/server/internal/pkg/logger"
)

// MockSender is the dev/test SMS sender. It logs the (phone, code) pair and
// always reports success. Use FixedCode() to learn the constant code that the
// auth service should expect when SMS.Provider == "mock".
type MockSender struct{}

// NewMock constructs a MockSender.
func NewMock() *MockSender { return &MockSender{} }

// FixedCode is the verification code that the mock provider always uses.
const fixedCode = "123456"

// FixedCode returns the constant verification code emitted by NewMock.
func (m *MockSender) FixedCode() string { return fixedCode }

// SendCode logs the message and returns nil. Phone is masked in the log.
func (m *MockSender) SendCode(ctx context.Context, phone, code string) error {
	logger.FromCtx(ctx).Info("sms.mock.send",
		"phone", logger.MaskPhone(phone),
		"code", code,
	)
	return nil
}
```

- [ ] **Step 4.4：跑确认 PASS**

```bash
go test ./internal/gateway/sms/ -v
```

- [ ] **Step 4.5：lint + commit**

```bash
golangci-lint run ./internal/gateway/sms/...
git add server/internal/gateway/sms
git commit -m "feat(sms): sender interface + dev mock that logs and returns 123456"
```

---

## Task 5：data model（User / Child）

**Files:**
- Create: `server/internal/model/user.go`
- Create: `server/internal/model/child.go`

> 仅声明结构体；GORM 行为由 repo 层控制。无单元测试（model 是数据载体）。

- [ ] **Step 5.1：实现 `user.go`**

```go
// Package model holds GORM-tagged data structs that mirror the database tables.
// Table-name conventions and JSON keys match the API contract documented in
// the Plan 2 spec.
package model

import "time"

// User maps to the `users` table.
type User struct {
	ID               int64     `gorm:"primaryKey;column:id" json:"id"`
	PhoneHash        string    `gorm:"column:phone_hash;uniqueIndex" json:"-"`
	PhoneEncrypted   []byte    `gorm:"column:phone_encrypted" json:"-"`
	Nickname         string    `gorm:"column:nickname" json:"nickname"`
	SubscriptionTier string    `gorm:"column:subscription_tier" json:"subscription_tier"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName returns the SQL table name for User. Required because GORM's
// default pluralization uses "users" already, but we make it explicit.
func (User) TableName() string { return "users" }
```

- [ ] **Step 5.2：实现 `child.go`**

```go
package model

import "time"

// Child maps to the `children` table.
type Child struct {
	ID        int64     `gorm:"primaryKey;column:id" json:"id"`
	UserID    int64     `gorm:"column:user_id;uniqueIndex:children_user_unique" json:"user_id"`
	Nickname  string    `gorm:"column:nickname" json:"nickname"`
	Gender    string    `gorm:"column:gender" json:"gender"`
	Birthday  time.Time `gorm:"column:birthday;type:date" json:"birthday"`
	Profile   []byte    `gorm:"column:profile;type:jsonb" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName returns the SQL table name for Child.
func (Child) TableName() string { return "children" }
```

- [ ] **Step 5.3：编译 & commit**

```bash
go build ./...
git add server/internal/model
git rm -f server/internal/model/.gitkeep   # 如果之前有
git commit -m "feat(model): user and child structs with gorm tags"
```

如果 `internal/model/` 不存在则先 `mkdir -p server/internal/model`。

---

## Task 6：UserRepo

**Files:**
- Create: `server/internal/repository/user_repo.go`
- Create: `server/internal/repository/user_repo_test.go`

- [ ] **Step 6.1：写集成测试**

`server/internal/repository/user_repo_test.go`：

```go
//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

func setupUserRepo(t *testing.T) (UserRepo, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	return NewUserRepo(db), func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestUserRepo_CreateOrGet_New(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	u, created, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:        "h_aaa",
		PhoneEncrypted:   []byte{1, 2, 3},
		Nickname:         "妈妈",
		SubscriptionTier: "free",
	})
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotZero(t, u.ID)
	assert.Equal(t, "妈妈", u.Nickname)
}

func TestUserRepo_CreateOrGet_Existing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	first, _, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:      "h_bbb",
		PhoneEncrypted: []byte{4, 5, 6},
		Nickname:       "first",
	})
	require.NoError(t, err)

	second, created, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:      "h_bbb",
		PhoneEncrypted: []byte{99, 99, 99},
		Nickname:       "second", // should be ignored, original kept
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "first", second.Nickname)
}

func TestUserRepo_FindByID_Missing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	_, err := repo.FindByID(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepo_FindByID_Existing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	u, _, _ := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash: "h_ccc", PhoneEncrypted: []byte{7}, Nickname: "n",
	})

	got, err := repo.FindByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.WithinDuration(t, time.Now(), got.CreatedAt, time.Minute)
}
```

> 集成测试用 `autoMigrateForTest(db)` —— 这是 Task 8 提供的 helper，自动跑迁移。

- [ ] **Step 6.2：实现 `user_repo.go`**

```go
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// ErrNotFound is returned when a row lookup yields no result.
var ErrNotFound = errors.New("not found")

// UserRepo is the data-access surface the auth service depends on.
type UserRepo interface {
	// CreateOrGet inserts u when no row with the same PhoneHash exists,
	// otherwise loads the existing row. The returned bool is true on creation.
	CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error)

	// FindByID returns the user with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

type userRepo struct {
	db *gorm.DB
}

// NewUserRepo returns a GORM-backed UserRepo.
func NewUserRepo(db *gorm.DB) UserRepo { return &userRepo{db: db} }

func (r *userRepo) CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error) {
	tx := r.db.WithContext(ctx)

	var existing model.User
	err := tx.Where("phone_hash = ?", u.PhoneHash).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// not found, create
	if err := tx.Create(u).Error; err != nil {
		// Could be a race — another concurrent insert won. Re-fetch.
		var existing2 model.User
		if e2 := tx.Where("phone_hash = ?", u.PhoneHash).First(&existing2).Error; e2 == nil {
			return &existing2, false, nil
		}
		return nil, false, err
	}
	return u, true, nil
}

func (r *userRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 6.3：跑集成测试（Task 8 完成后才能跑通）**

集成测试需要 `autoMigrateForTest`，跳过 Task 8 之前不跑。先确认编译过：

```bash
go build ./...
```

- [ ] **Step 6.4：commit**

```bash
git add server/internal/repository/user_repo.go server/internal/repository/user_repo_test.go
git commit -m "feat(repo): user repo with CreateOrGet idempotent insert"
```

---

## Task 7：ChildRepo

**Files:**
- Create: `server/internal/repository/child_repo.go`
- Create: `server/internal/repository/child_repo_test.go`

- [ ] **Step 7.1：写集成测试**

```go
//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

func setupChildRepo(t *testing.T) (UserRepo, ChildRepo, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	return NewUserRepo(db), NewChildRepo(db), func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func makeUser(t *testing.T, repo UserRepo, hash string) *model.User {
	t.Helper()
	u, _, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash: hash, PhoneEncrypted: []byte{1}, Nickname: "x",
	})
	require.NoError(t, err)
	return u
}

func TestChildRepo_Create_AndFindByUserID(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_a")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	c := &model.Child{
		UserID:   u.ID,
		Nickname: "小宇",
		Gender:   "boy",
		Birthday: bday,
		Profile:  []byte(`{}`),
	}
	require.NoError(t, crepo.Create(context.Background(), c))
	assert.NotZero(t, c.ID)

	got, err := crepo.FindByUserID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, "小宇", got.Nickname)
}

func TestChildRepo_Create_RejectsDuplicateForSameUser(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_b")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")

	require.NoError(t, crepo.Create(context.Background(), &model.Child{
		UserID: u.ID, Nickname: "first", Gender: "boy", Birthday: bday, Profile: []byte(`{}`),
	}))
	err := crepo.Create(context.Background(), &model.Child{
		UserID: u.ID, Nickname: "second", Gender: "girl", Birthday: bday, Profile: []byte(`{}`),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAlreadyExists), "expected ErrAlreadyExists, got %v", err)
}

func TestChildRepo_FindByUserID_NotFound(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_c")
	_, err := crepo.FindByUserID(context.Background(), u.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestChildRepo_FindByID_AndUpdate(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_d")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	c := &model.Child{UserID: u.ID, Nickname: "n", Gender: "boy", Birthday: bday, Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	c.Nickname = "n2"
	require.NoError(t, crepo.Update(context.Background(), c))

	got, err := crepo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "n2", got.Nickname)
}
```

- [ ] **Step 7.2：实现**

`server/internal/repository/child_repo.go`：

```go
package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// ErrAlreadyExists is returned when an INSERT violates a UNIQUE constraint.
var ErrAlreadyExists = errors.New("already exists")

// ChildRepo is the data-access surface the child service depends on.
type ChildRepo interface {
	Create(ctx context.Context, c *model.Child) error
	FindByUserID(ctx context.Context, userID int64) (*model.Child, error)
	FindByID(ctx context.Context, id int64) (*model.Child, error)
	Update(ctx context.Context, c *model.Child) error
}

type childRepo struct {
	db *gorm.DB
}

// NewChildRepo returns a GORM-backed ChildRepo.
func NewChildRepo(db *gorm.DB) ChildRepo { return &childRepo{db: db} }

func (r *childRepo) Create(ctx context.Context, c *model.Child) error {
	err := r.db.WithContext(ctx).Create(c).Error
	if err == nil {
		return nil
	}
	// PG unique-violation error contains "duplicate key" / "unique constraint"
	if isUniqueViolation(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *childRepo) FindByUserID(ctx context.Context, userID int64) (*model.Child, error) {
	var c model.Child
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *childRepo) FindByID(ctx context.Context, id int64) (*model.Child, error) {
	var c model.Child
	err := r.db.WithContext(ctx).First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *childRepo) Update(ctx context.Context, c *model.Child) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func isUniqueViolation(err error) bool {
	// We don't depend on lib/pq error codes — match by message substring,
	// which is robust across pgx and lib/pq drivers.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
```

- [ ] **Step 7.3：编译 & commit**

```bash
go build ./...
git add server/internal/repository/child_repo.go server/internal/repository/child_repo_test.go
git commit -m "feat(repo): child repo with UNIQUE-violation translation"
```

---

## Task 8：迁移自动应用 helper（启动期 + 测试期）

**Files:**
- Create: `server/internal/repository/migrate.go`
- Create: `server/internal/repository/migrate_helper_test.go`

> 目的：（1）main.go 启动时自动跑 `migrate up`，避免手动忘记；（2）集成测试期 `autoMigrateForTest` helper 用 GORM 的 AutoMigrate 跑 schema（不依赖 migrate CLI 也不依赖文件路径，更快）。

- [ ] **Step 8.1：实现 `migrate.go`**

```go
package repository

import (
	"errors"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// RunMigrations applies any pending SQL migrations from migrationsDir to the
// supplied database. It is idempotent — re-running with no pending migrations
// is a no-op.
func RunMigrations(db *gorm.DB, migrationsDir string) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// autoMigrateForTest creates schema for User/Child by GORM's AutoMigrate.
// Used by integration tests so they don't depend on the migrate CLI.
// Production uses RunMigrations() with the SQL files instead.
func autoMigrateForTest(db *gorm.DB) error {
	return db.AutoMigrate(&model.User{}, &model.Child{})
}
```

需要新依赖：

```bash
GOPROXY=https://goproxy.cn,direct go get github.com/golang-migrate/migrate/v4
```

- [ ] **Step 8.2：跑现有 user_repo_test 与 child_repo_test 验证集成测试现在能通过**

```bash
cd server && go test -count=1 -tags=integration ./internal/repository/ -v
```
Expected: 含 Task 6/7 写的所有用例都过（11+ 个测试）。

如果失败：检查 `autoMigrateForTest` 是否真的为两个 model 建出了 UNIQUE 约束——GORM AutoMigrate 会读 model 的 `uniqueIndex` tag。

- [ ] **Step 8.3：commit**

```bash
git add server/internal/repository/migrate.go server/go.mod server/go.sum
git commit -m "feat(repo): RunMigrations(file://) and AutoMigrate test helper"
```

---

## Task 9：CodeStore（验证码存 Redis）

**Files:**
- Create: `server/internal/service/auth/codestore.go`
- Create: `server/internal/service/auth/codestore_redis.go`
- Create: `server/internal/service/auth/codestore_test.go`

> CodeStore 提供两个能力：① 存验证码（key=phone_hash，TTL=5min）② 速率限制（key=phone_hash，TTL=60s，重发冷却）。

- [ ] **Step 9.1：定义接口 `codestore.go`**

```go
// Package auth implements the SMS-code-based login/register flow.
package auth

import (
	"context"
	"errors"
	"time"
)

// ErrCooldown is returned when a new code is requested within the resend
// cooldown window.
var ErrCooldown = errors.New("resend cooldown")

// ErrCodeNotFound is returned when no code is stored for the phone (expired or never sent).
var ErrCodeNotFound = errors.New("code not found")

// CodeStore stores SMS verification codes with TTL and per-phone resend cooldown.
type CodeStore interface {
	// Save persists code under phoneHash with the given codeTTL. If a
	// previous Save happened within cooldown, returns ErrCooldown without
	// overwriting the existing code.
	Save(ctx context.Context, phoneHash, code string, codeTTL, cooldown time.Duration) error

	// Take fetches and atomically deletes the code for phoneHash. Returns
	// ErrCodeNotFound if absent or already taken.
	Take(ctx context.Context, phoneHash string) (string, error)
}
```

- [ ] **Step 9.2：实现 Redis 版 `codestore_redis.go`**

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisCodeStore returns a CodeStore backed by go-redis.
func NewRedisCodeStore(c *redis.Client) CodeStore { return &redisStore{c: c} }

type redisStore struct {
	c *redis.Client
}

func codeKey(phoneHash string) string  { return "auth:code:" + phoneHash }
func cooldownKey(phoneHash string) string { return "auth:cd:" + phoneHash }

func (s *redisStore) Save(ctx context.Context, phoneHash, code string, codeTTL, cooldown time.Duration) error {
	// First check & set cooldown atomically. If cooldown key already exists, refuse.
	ok, err := s.c.SetNX(ctx, cooldownKey(phoneHash), "1", cooldown).Result()
	if err != nil {
		return fmt.Errorf("set cooldown: %w", err)
	}
	if !ok {
		return ErrCooldown
	}
	if err := s.c.Set(ctx, codeKey(phoneHash), code, codeTTL).Err(); err != nil {
		return fmt.Errorf("set code: %w", err)
	}
	return nil
}

func (s *redisStore) Take(ctx context.Context, phoneHash string) (string, error) {
	got, err := s.c.GetDel(ctx, codeKey(phoneHash)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return got, nil
}
```

- [ ] **Step 9.3：写集成测试**

`server/internal/service/auth/codestore_test.go`：

```go
//go:build integration

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	rdb "github.com/redis/go-redis/v9"
)

func startRedis(t *testing.T) *rdb.Client {
	t.Helper()
	ctx := context.Background()
	c, err := redis.Run(ctx, "redis:7-alpine",
		tc.WithWaitStrategy(wait.ForListeningPort("6379/tcp").WithStartupTimeout(15*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "6379/tcp")
	return rdb.NewClient(&rdb.Options{Addr: host + ":" + port.Port()})
}

func TestCodeStore_SaveAndTake(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_a", "123456", time.Minute, 100*time.Millisecond))

	code, err := store.Take(context.Background(), "h_a")
	require.NoError(t, err)
	assert.Equal(t, "123456", code)
}

func TestCodeStore_TakeIsOneShot(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_b", "123456", time.Minute, 100*time.Millisecond))

	_, err := store.Take(context.Background(), "h_b")
	require.NoError(t, err)
	_, err = store.Take(context.Background(), "h_b")
	assert.True(t, errors.Is(err, ErrCodeNotFound))
}

func TestCodeStore_Cooldown(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_c", "123456", time.Minute, time.Second))
	err := store.Save(context.Background(), "h_c", "999999", time.Minute, time.Second)
	assert.True(t, errors.Is(err, ErrCooldown))
}
```

- [ ] **Step 9.4：跑测试**

```bash
go test -count=1 -tags=integration ./internal/service/auth/ -v
```

- [ ] **Step 9.5：commit**

```bash
git add server/internal/service/auth/codestore.go \
        server/internal/service/auth/codestore_redis.go \
        server/internal/service/auth/codestore_test.go
git commit -m "feat(auth): redis-backed code store with cooldown"
```

---

## Task 10：Auth Service

**Files:**
- Create: `server/internal/service/auth/auth.go`
- Create: `server/internal/service/auth/auth_test.go`

- [ ] **Step 10.1：写测试（mock 全部依赖）**

`server/internal/service/auth/auth_test.go`：

```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/model"
)

type fakeUserRepo struct {
	byHash map[string]*model.User
	nextID int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byHash: map[string]*model.User{}, nextID: 1}
}

func (f *fakeUserRepo) CreateOrGet(_ context.Context, u *model.User) (*model.User, bool, error) {
	if existing, ok := f.byHash[u.PhoneHash]; ok {
		return existing, false, nil
	}
	u.ID = f.nextID
	f.nextID++
	f.byHash[u.PhoneHash] = u
	return u, true, nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id int64) (*model.User, error) {
	for _, u := range f.byHash {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

type fakeCodeStore struct {
	saved    map[string]string
	cooldown bool
}

func (f *fakeCodeStore) Save(_ context.Context, ph, code string, _, _ time.Duration) error {
	if f.cooldown {
		return ErrCooldown
	}
	if f.saved == nil {
		f.saved = map[string]string{}
	}
	f.saved[ph] = code
	return nil
}
func (f *fakeCodeStore) Take(_ context.Context, ph string) (string, error) {
	c, ok := f.saved[ph]
	if !ok {
		return "", ErrCodeNotFound
	}
	delete(f.saved, ph)
	return c, nil
}

type fakeSMS struct {
	sent      bool
	lastPhone string
	lastCode  string
}

func (f *fakeSMS) SendCode(_ context.Context, phone, code string) error {
	f.sent = true
	f.lastPhone = phone
	f.lastCode = code
	return nil
}

type fakePhoneCipher struct{}

func (fakePhoneCipher) Encrypt(s string) ([]byte, error) { return []byte("enc:" + s), nil }
func (fakePhoneCipher) Decrypt(b []byte) (string, error) { return string(b)[4:], nil }

func newSvc(t *testing.T) (*Service, *fakeUserRepo, *fakeCodeStore, *fakeSMS) {
	t.Helper()
	repo := newFakeUserRepo()
	cs := &fakeCodeStore{}
	sms := &fakeSMS{}
	jwt := jwtauth.New("secret-x", time.Hour, 7*24*time.Hour)
	hasher := safehash.New("salt")
	svc := New(Deps{
		Users:        repo,
		CodeStore:    cs,
		SMS:          sms,
		JWT:          jwt,
		PhoneCipher:  fakePhoneCipher{},
		Hasher:       hasher,
		FixedDevCode: "123456",
		CodeTTL:      5 * time.Minute,
		Cooldown:     time.Minute,
	})
	return svc, repo, cs, sms
}

func TestSendSMS_HappyPath(t *testing.T) {
	svc, _, cs, sms := newSvc(t)
	err := svc.SendSMS(context.Background(), "13800138000")
	require.NoError(t, err)
	assert.True(t, sms.sent)
	assert.Equal(t, "13800138000", sms.lastPhone)
	assert.Equal(t, "123456", sms.lastCode)
	// stored under the safehash of the phone
	require.Len(t, cs.saved, 1)
}

func TestSendSMS_RejectsBadPhone(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	err := svc.SendSMS(context.Background(), "abc")
	require.Error(t, err)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
}

func TestSendSMS_Cooldown(t *testing.T) {
	svc, _, cs, _ := newSvc(t)
	cs.cooldown = true
	err := svc.SendSMS(context.Background(), "13800138000")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeRateLimited, ae.Code)
}

func TestLoginOrRegister_NewUser(t *testing.T) {
	svc, repo, cs, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))

	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")
	require.NoError(t, err)
	assert.NotEmpty(t, out.AccessToken)
	assert.NotEmpty(t, out.RefreshToken)
	assert.Equal(t, "妈妈", out.User.Nickname)
	assert.Len(t, repo.byHash, 1, "exactly one user")
	assert.Empty(t, cs.saved, "code consumed")
}

func TestLoginOrRegister_ExistingUserKeepsNickname(t *testing.T) {
	svc, repo, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")
	require.NoError(t, err)

	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	require.NoError(t, err)
	assert.Equal(t, "妈妈", out.User.Nickname, "nickname not overwritten on second login")
	assert.Len(t, repo.byHash, 1)
}

func TestLoginOrRegister_DefaultNickname(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	require.NoError(t, err)
	assert.Equal(t, "家长", out.User.Nickname)
}

func TestLoginOrRegister_CodeMismatch(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "999999", "妈妈")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeUnauthenticated, ae.Code)
	assert.Equal(t, "code_mismatch", ae.Reason)
}

func TestLoginOrRegister_NoCodeStored(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeUnauthenticated, ae.Code)
	assert.Equal(t, "code_expired", ae.Reason)
}

func TestValidateAccess_HappyPath(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, _ := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")

	uid, err := svc.ValidateAccess(out.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, out.User.ID, uid)
}

func TestValidateAccess_BadToken(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.ValidateAccess("not-a-jwt")
	require.Error(t, err)
}
```

- [ ] **Step 10.2：实现 `auth.go`**

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/model"
)

// PhoneCipher abstracts phone-number encryption so the service can be tested
// without depending on real AES.
type PhoneCipher interface {
	Encrypt(plain string) ([]byte, error)
	Decrypt(blob []byte) (string, error)
}

// SMS is the minimal surface auth.Service needs from the SMS gateway.
type SMS interface {
	SendCode(ctx context.Context, phone, code string) error
}

// UserRepo is the minimal surface auth.Service needs from the user repository.
// Mirrors repository.UserRepo so the service can swap in a fake.
type UserRepo interface {
	CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error)
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

// Deps groups Service dependencies.
type Deps struct {
	Users        UserRepo
	CodeStore    CodeStore
	SMS          SMS
	JWT          *jwtauth.Manager
	PhoneCipher  PhoneCipher
	Hasher       *safehash.Hasher
	FixedDevCode string        // e.g. "123456" for mock provider
	CodeTTL      time.Duration // e.g. 5 min
	Cooldown     time.Duration // e.g. 60s
}

// Service is the auth service.
type Service struct {
	d Deps
}

// New constructs the Service.
func New(d Deps) *Service { return &Service{d: d} }

// LoginOutput is what LoginOrRegister returns to callers.
type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *model.User
}

const defaultNickname = "家长"

var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

func validatePhone(p string) bool {
	return phoneRe.MatchString(p)
}

// SendSMS issues a verification code for the phone.
func (s *Service) SendSMS(ctx context.Context, phone string) error {
	if !validatePhone(phone) {
		return apperr.New(apperr.CodeInvalidArgument, "phone_invalid", "手机号格式不正确")
	}
	hash := s.d.Hasher.HashString(phone)
	code := s.d.FixedDevCode
	if err := s.d.CodeStore.Save(ctx, hash, code, s.d.CodeTTL, s.d.Cooldown); err != nil {
		if errors.Is(err, ErrCooldown) {
			return apperr.New(apperr.CodeRateLimited, "sms_rate_limited", "请稍后再试")
		}
		return apperr.Wrap(err, apperr.CodeInternal, "code_save_failed", "短信发送失败")
	}
	if err := s.d.SMS.SendCode(ctx, phone, code); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "sms_send_failed", "短信发送失败")
	}
	logger.FromCtx(ctx).Info("auth.sms.sent", "phone", logger.MaskPhone(phone), "phone_hash", hash)
	return nil
}

// LoginOrRegister verifies code; returns access + refresh tokens for an
// existing or freshly created user.
func (s *Service) LoginOrRegister(ctx context.Context, phone, code, nickname string) (*LoginOutput, error) {
	if !validatePhone(phone) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "phone_invalid", "手机号格式不正确")
	}
	if strings.TrimSpace(code) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "code_invalid", "验证码不能为空")
	}
	hash := s.d.Hasher.HashString(phone)

	stored, err := s.d.CodeStore.Take(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrCodeNotFound) {
			return nil, apperr.New(apperr.CodeUnauthenticated, "code_expired", "验证码已过期，请重新获取")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "code_take_failed", "验证失败")
	}
	if stored != code {
		return nil, apperr.New(apperr.CodeUnauthenticated, "code_mismatch", "验证码错误")
	}

	enc, err := s.d.PhoneCipher.Encrypt(phone)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "phone_encrypt_failed", "服务暂时不可用")
	}

	chosenNickname := strings.TrimSpace(nickname)
	if chosenNickname == "" {
		chosenNickname = defaultNickname
	}

	u, _, err := s.d.Users.CreateOrGet(ctx, &model.User{
		PhoneHash:        hash,
		PhoneEncrypted:   enc,
		Nickname:         chosenNickname,
		SubscriptionTier: "free",
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "user_upsert_failed", "服务暂时不可用")
	}

	access, err := s.d.JWT.IssueAccess(u.ID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "jwt_issue_failed", "服务暂时不可用")
	}
	refresh, err := s.d.JWT.IssueRefresh(u.ID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "jwt_issue_failed", "服务暂时不可用")
	}

	logger.FromCtx(ctx).Info("auth.login_or_register",
		"user_id", u.ID,
		"phone_hash", hash,
		"new_user", u.CreatedAt.After(time.Now().Add(-5*time.Second)),
	)
	return &LoginOutput{AccessToken: access, RefreshToken: refresh, User: u}, nil
}

// ValidateAccess verifies an access token and returns its user id.
func (s *Service) ValidateAccess(tok string) (int64, error) {
	c, err := s.d.JWT.ParseAccess(tok)
	if err != nil {
		return 0, fmt.Errorf("parse access: %w", err)
	}
	return c.UserID, nil
}
```

- [ ] **Step 10.3：跑测试 + lint**

```bash
go test -count=1 ./internal/service/auth/ -v
golangci-lint run ./internal/service/auth/...
```
Expected: 9/9 PASS（仅 SendSMS / LoginOrRegister / ValidateAccess 子测）；集成测试因 build tag 不参与单测跑。

- [ ] **Step 10.4：commit**

```bash
git add server/internal/service/auth/auth.go server/internal/service/auth/auth_test.go
git commit -m "feat(auth): service for sms send / login_or_register / validate"
```

---

## Task 11：Child Service

**Files:**
- Create: `server/internal/service/child/child.go`
- Create: `server/internal/service/child/child_test.go`

- [ ] **Step 11.1：写测试**

```go
package child

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

type fakeChildRepo struct {
	byID     map[int64]*model.Child
	byUser   map[int64]int64 // user_id -> child_id
	nextID   int64
	createOK bool
}

func newFakeChildRepo() *fakeChildRepo {
	return &fakeChildRepo{
		byID:     map[int64]*model.Child{},
		byUser:   map[int64]int64{},
		nextID:   1,
		createOK: true,
	}
}

func (r *fakeChildRepo) Create(_ context.Context, c *model.Child) error {
	if _, ok := r.byUser[c.UserID]; ok {
		return repository.ErrAlreadyExists
	}
	c.ID = r.nextID
	r.nextID++
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c.ID
	return nil
}

func (r *fakeChildRepo) FindByUserID(_ context.Context, userID int64) (*model.Child, error) {
	id, ok := r.byUser[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *fakeChildRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	c, ok := r.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

func (r *fakeChildRepo) Update(_ context.Context, c *model.Child) error {
	r.byID[c.ID] = c
	return nil
}

func newSvc() (*Service, *fakeChildRepo) {
	r := newFakeChildRepo()
	return New(r), r
}

func bday(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return d
}

func TestCreate_HappyPath(t *testing.T) {
	svc, _ := newSvc()
	c, err := svc.Create(context.Background(), 1, CreateInput{
		Nickname: "小宇", Gender: "boy", Birthday: bday(t, "2020-08-15"),
	})
	require.NoError(t, err)
	assert.Equal(t, "小宇", c.Nickname)
	assert.NotZero(t, c.ID)
}

func TestCreate_RejectsDuplicate(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "a", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 1, CreateInput{Nickname: "b", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
	assert.Equal(t, "child_already_exists", ae.Reason)
}

func TestCreate_RejectsInvalidGender(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "a", Gender: "alien", Birthday: bday(t, "2020-08-15")})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_gender", ae.Reason)
}

func TestCreate_RejectsBlankNickname(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "  ", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_nickname", ae.Reason)
}

func TestList_EmptyAndOne(t *testing.T) {
	svc, _ := newSvc()
	got, err := svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 0)

	_, _ = svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	got, err = svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestUpdate_OnlyOwnerCanUpdate(t *testing.T) {
	svc, _ := newSvc()
	c, _ := svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})

	newName := "n2"
	_, err := svc.Update(context.Background(), 999 /* not owner */, c.ID, UpdateInput{Nickname: &newName})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodePermissionDenied, ae.Code)
}

func TestUpdate_HappyPath(t *testing.T) {
	svc, _ := newSvc()
	c, _ := svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})

	newName := "n2"
	got, err := svc.Update(context.Background(), 1, c.ID, UpdateInput{Nickname: &newName})
	require.NoError(t, err)
	assert.Equal(t, "n2", got.Nickname)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _ := newSvc()
	newName := "x"
	_, err := svc.Update(context.Background(), 1, 9999, UpdateInput{Nickname: &newName})
	require.Error(t, err)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeNotFound, ae.Code)
}
```

- [ ] **Step 11.2：实现 `child.go`**

```go
// Package child implements the child-profile CRUD service.
package child

import (
	"context"
	"errors"
	"strings"
	"time"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

// Service implements the child-profile CRUD logic.
type Service struct {
	repo repository.ChildRepo
}

// New constructs a Service.
func New(r repository.ChildRepo) *Service { return &Service{repo: r} }

// CreateInput is the user-facing input for Create.
type CreateInput struct {
	Nickname string
	Gender   string
	Birthday time.Time
}

// UpdateInput holds optional fields for Update. nil means "don't change".
type UpdateInput struct {
	Nickname *string
	Gender   *string
	Birthday *time.Time
}

var validGenders = map[string]bool{"boy": true, "girl": true, "unspecified": true}

// Create inserts a new child for userID.
func (s *Service) Create(ctx context.Context, userID int64, in CreateInput) (*model.Child, error) {
	nick := strings.TrimSpace(in.Nickname)
	if nick == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_nickname", "孩子昵称不能为空")
	}
	if !validGenders[in.Gender] {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_gender", "性别必须是 boy / girl / unspecified")
	}
	if in.Birthday.IsZero() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_birthday", "生日不能为空")
	}
	c := &model.Child{
		UserID:   userID,
		Nickname: nick,
		Gender:   in.Gender,
		Birthday: in.Birthday,
		Profile:  []byte(`{}`),
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "child_already_exists", "您已经创建过孩子档案")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_create_failed", "服务暂时不可用")
	}
	return c, nil
}

// ListByUser returns the user's child as a one- or zero-element slice.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]*model.Child, error) {
	c, err := s.repo.FindByUserID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return []*model.Child{}, nil
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_list_failed", "服务暂时不可用")
	}
	return []*model.Child{c}, nil
}

// Update mutates fields of an existing child belonging to userID.
func (s *Service) Update(ctx context.Context, userID, childID int64, in UpdateInput) (*model.Child, error) {
	c, err := s.repo.FindByID(ctx, childID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_load_failed", "服务暂时不可用")
	}
	if c.UserID != userID {
		return nil, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权修改该孩子档案")
	}
	if in.Nickname != nil {
		nick := strings.TrimSpace(*in.Nickname)
		if nick == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_nickname", "孩子昵称不能为空")
		}
		c.Nickname = nick
	}
	if in.Gender != nil {
		if !validGenders[*in.Gender] {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_gender", "性别必须是 boy / girl / unspecified")
		}
		c.Gender = *in.Gender
	}
	if in.Birthday != nil {
		c.Birthday = *in.Birthday
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_update_failed", "服务暂时不可用")
	}
	return c, nil
}
```

- [ ] **Step 11.3：跑测试**

```bash
go test -count=1 ./internal/service/child/ -v
```
Expected: 全过。

- [ ] **Step 11.4：lint + commit**

```bash
golangci-lint run ./internal/service/child/...
git add server/internal/service/child
git commit -m "feat(child): create / list / update child profile service"
```

---

## Task 12：错误响应 helper + userctx

**Files:**
- Create: `server/internal/api/userctx/userctx.go`
- Create: `server/internal/api/errs.go`
- Create: `server/internal/api/errs_test.go`

> userctx 单独成包是为了避免 `api` ↔ `middleware` 循环依赖：middleware 依赖 userctx 写入 user_id，handler 依赖 userctx 读出。

- [ ] **Step 12.1：实现 `userctx.go`**

```go
// Package userctx stores the authenticated user id in request context.
// It exists in a tiny dedicated package to avoid api ↔ middleware import cycles.
package userctx

import "context"

type ctxKey struct{}

// WithUserID returns ctx carrying the given user id.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext extracts the user id, returning ok=false when absent.
func FromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(ctxKey{}).(int64)
	if !ok || v == 0 {
		return 0, false
	}
	return v, true
}
```

- [ ] **Step 12.2：实现 `errs.go`**

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/logger"
)

// RespondError translates err into a JSON response. AppError errors are mapped
// to their declared HTTP status; everything else becomes 500 internal_error.
func RespondError(c *gin.Context, err error) {
	if ae, ok := apperr.AsAppError(err); ok {
		c.AbortWithStatusJSON(ae.HTTPStatus(), gin.H{
			"reason":   ae.Reason,
			"user_msg": ae.UserMsg,
		})
		return
	}
	logger.FromCtx(c.Request.Context()).Error("api.unexpected_error", "err", err.Error())
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"reason":   "internal_error",
		"user_msg": "服务暂时不可用，请稍后再试",
	})
}
```

- [ ] **Step 12.3：写测试 `errs_test.go`**

```go
package api

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	apperr "github.com/aibao/server/internal/pkg/errors"
)

func TestRespondError_AppError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	RespondError(c, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到孩子档案"))

	assert.Equal(t, 404, rec.Code)
	assert.Contains(t, rec.Body.String(), "child_not_found")
	assert.Contains(t, rec.Body.String(), "未找到孩子档案")
}

func TestRespondError_PlainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	RespondError(c, errors.New("boom"))

	assert.Equal(t, 500, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal_error")
}
```

- [ ] **Step 12.4：跑测试**

```bash
go test -count=1 ./internal/api/ -run TestRespondError -v
```

- [ ] **Step 12.5：commit**

```bash
git add server/internal/api/userctx server/internal/api/errs.go server/internal/api/errs_test.go
git commit -m "feat(api): RespondError + userctx package"
```

---

## Task 13：JWTAuth Middleware

**Files:**
- Create: `server/internal/api/middleware/auth.go`
- Create: `server/internal/api/middleware/auth_test.go`

- [ ] **Step 13.1：写测试**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

func newTestMgr() *jwtauth.Manager {
	return jwtauth.New("secret-x", time.Hour, time.Hour)
}

func TestJWTAuth_AcceptsValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := newTestMgr()
	tok, err := mgr.IssueAccess(42)
	require.NoError(t, err)

	r := gin.New()
	r.Use(JWTAuth(mgr))

	var seen int64
	r.GET("/x", func(c *gin.Context) {
		uid, _ := userctx.FromContext(c.Request.Context())
		seen = uid
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, int64(42), seen)
}

func TestJWTAuth_RejectsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestJWTAuth_RejectsBadToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestJWTAuth_RejectsWrongScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	tok, _ := newTestMgr().IssueAccess(1)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Token "+tok) // wrong scheme
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}
```

- [ ] **Step 13.2：实现 `auth.go`**

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

// JWTAuth requires a valid Bearer access token. On success, the user id is
// attached to the request context via userctx.WithUserID.
func JWTAuth(mgr *jwtauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"reason":   "unauthorized",
				"user_msg": "请先登录",
			})
			return
		}
		tok := strings.TrimPrefix(auth, prefix)
		claims, err := mgr.ParseAccess(tok)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"reason":   "unauthorized",
				"user_msg": "登录已过期，请重新登录",
			})
			return
		}
		ctx := userctx.WithUserID(c.Request.Context(), claims.UserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
```

- [ ] **Step 13.3：跑测试 + commit**

```bash
go test -count=1 ./internal/api/middleware/ -run TestJWTAuth -v
golangci-lint run ./internal/api/middleware/...
git add server/internal/api/middleware/auth.go server/internal/api/middleware/auth_test.go
git commit -m "feat(middleware): jwt auth middleware injecting user_id into ctx"
```

---

## Task 14：Auth handlers

**Files:**
- Create: `server/internal/api/auth.go`
- Create: `server/internal/api/auth_test.go`

- [ ] **Step 14.1：实现 `auth.go`**

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/service/auth"
)

// AuthHandler exposes the SMS / login_or_register endpoints.
type AuthHandler struct {
	svc *auth.Service
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(svc *auth.Service) *AuthHandler { return &AuthHandler{svc: svc} }

// RegisterRoutes attaches /auth/* routes under the supplied router group.
func (h *AuthHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/auth/sms/send", h.smsSend)
	g.POST("/auth/login_or_register", h.loginOrRegister)
}

type smsSendReq struct {
	Phone string `json:"phone" binding:"required"`
}

func (h *AuthHandler) smsSend(c *gin.Context) {
	var req smsSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	if err := h.svc.SendSMS(c.Request.Context(), req.Phone); err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true})
}

type loginOrRegisterReq struct {
	Phone    string `json:"phone" binding:"required"`
	Code     string `json:"code" binding:"required"`
	Nickname string `json:"nickname"` // optional
}

func (h *AuthHandler) loginOrRegister(c *gin.Context) {
	var req loginOrRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	out, err := h.svc.LoginOrRegister(c.Request.Context(), req.Phone, req.Code, req.Nickname)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  out.AccessToken,
		"refresh_token": out.RefreshToken,
		"user": gin.H{
			"id":                out.User.ID,
			"nickname":          out.User.Nickname,
			"subscription_tier": out.User.SubscriptionTier,
		},
	})
}
```

- [ ] **Step 14.2：写测试**

`server/internal/api/auth_test.go`：

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/service/auth"
)

// reuse the in-process fakes from auth_test.go in the auth pkg by inlining
// minimal versions here — keeps the api test self-contained.

type fakeUserRepo struct{ created *model.User }

func (f *fakeUserRepo) CreateOrGet(_ context.Context, u *model.User) (*model.User, bool, error) {
	u.ID = 7
	f.created = u
	return u, true, nil
}
func (f *fakeUserRepo) FindByID(_ context.Context, id int64) (*model.User, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, errors.New("not found")
}

type fakeStore struct{ saved string }

func (f *fakeStore) Save(_ context.Context, _, c string, _, _ time.Duration) error {
	f.saved = c
	return nil
}
func (f *fakeStore) Take(_ context.Context, _ string) (string, error) {
	if f.saved == "" {
		return "", auth.ErrCodeNotFound
	}
	c := f.saved
	f.saved = ""
	return c, nil
}

type fakeSMS struct{}

func (fakeSMS) SendCode(_ context.Context, _, _ string) error { return nil }

type fakePC struct{}

func (fakePC) Encrypt(s string) ([]byte, error) { return []byte(s), nil }
func (fakePC) Decrypt(b []byte) (string, error) { return string(b), nil }

func setupAuth(t *testing.T) (*gin.Engine, *fakeUserRepo, *fakeStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &fakeUserRepo{}
	cs := &fakeStore{}
	jwt := jwtauth.New("s", time.Hour, time.Hour)
	svc := auth.New(auth.Deps{
		Users: repo, CodeStore: cs, SMS: fakeSMS{}, JWT: jwt,
		PhoneCipher: fakePC{}, Hasher: safehash.New("salt"),
		FixedDevCode: "123456", CodeTTL: time.Minute, Cooldown: time.Second,
	})
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewAuthHandler(svc).RegisterRoutes(v1)
	return r, repo, cs
}

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestSmsSend_OK(t *testing.T) {
	r, _, cs := setupAuth(t)
	rec := postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"})
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "123456", cs.saved)
}

func TestSmsSend_InvalidPhone(t *testing.T) {
	r, _, _ := setupAuth(t)
	rec := postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "abc"})
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "phone_invalid")
}

func TestLoginOrRegister_OK(t *testing.T) {
	r, _, _ := setupAuth(t)
	require.Equal(t, 200, postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"}).Code)

	rec := postJSON(r, "/api/v1/auth/login_or_register", map[string]string{
		"phone": "13800138000", "code": "123456", "nickname": "妈妈",
	})
	require.Equal(t, 200, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotEmpty(t, out["access_token"])
	assert.NotEmpty(t, out["refresh_token"])
	user := out["user"].(map[string]any)
	assert.Equal(t, "妈妈", user["nickname"])
}

func TestLoginOrRegister_BadCode(t *testing.T) {
	r, _, _ := setupAuth(t)
	require.Equal(t, 200, postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"}).Code)

	rec := postJSON(r, "/api/v1/auth/login_or_register", map[string]string{
		"phone": "13800138000", "code": "999999",
	})
	assert.Equal(t, 401, rec.Code)
	assert.Contains(t, rec.Body.String(), "code_mismatch")
}
```

> ⚠️ 上面的 `auth_test.go` 顶部需要 `import "context"` / `errors`，请在编写时补全。

- [ ] **Step 14.3：跑测试 + commit**

```bash
go test -count=1 ./internal/api/ -v
golangci-lint run ./internal/api/...
git add server/internal/api/auth.go server/internal/api/auth_test.go
git commit -m "feat(api): /auth/sms/send and /auth/login_or_register handlers"
```

---

## Task 15：me handler

**Files:**
- Create: `server/internal/api/me.go`
- Create: `server/internal/api/me_test.go`

- [ ] **Step 15.1：实现 `me.go`**

```go
package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/model"
)

// MeUserLookup is the surface MeHandler needs to load a user by id.
type MeUserLookup interface {
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

// MeHandler serves /me.
type MeHandler struct {
	users MeUserLookup
}

// NewMeHandler constructs a MeHandler.
func NewMeHandler(users MeUserLookup) *MeHandler { return &MeHandler{users: users} }

// RegisterRoutes mounts /me on the supplied authenticated group.
func (h *MeHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/me", h.me)
}

func (h *MeHandler) me(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		RespondError(c, apperr.Wrap(err, apperr.CodeNotFound, "user_not_found", "用户不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                u.ID,
		"nickname":          u.Nickname,
		"subscription_tier": u.SubscriptionTier,
	})
}
```

- [ ] **Step 15.2：写测试**

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
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
)

type meFakeUsers struct {
	byID map[int64]*model.User
}

func (f *meFakeUsers) FindByID(_ context.Context, id int64) (*model.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func TestMe_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &meFakeUsers{byID: map[int64]*model.User{
		7: {ID: 7, Nickname: "妈妈", SubscriptionTier: "free"},
	}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), 7))
		c.Next()
	})
	v1 := r.Group("/api/v1")
	NewMeHandler(users).RegisterRoutes(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "妈妈")
}

func TestMe_NoUserCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewMeHandler(&meFakeUsers{byID: map[int64]*model.User{}}).RegisterRoutes(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 15.3：跑 + commit**

```bash
go test -count=1 ./internal/api/ -run TestMe -v
git add server/internal/api/me.go server/internal/api/me_test.go
git commit -m "feat(api): /me endpoint for current user"
```

---

## Task 16：Child handlers

**Files:**
- Create: `server/internal/api/child.go`
- Create: `server/internal/api/child_test.go`

- [ ] **Step 16.1：实现 `child.go`**

```go
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/child"
)

// ChildHandler serves /children endpoints.
type ChildHandler struct {
	svc *child.Service
}

// NewChildHandler constructs a ChildHandler.
func NewChildHandler(svc *child.Service) *ChildHandler { return &ChildHandler{svc: svc} }

// RegisterRoutes mounts the /children routes on an authenticated group.
func (h *ChildHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/children", h.create)
	g.GET("/children", h.list)
	g.PATCH("/children/:id", h.update)
}

type createChildReq struct {
	Nickname string `json:"nickname" binding:"required"`
	Gender   string `json:"gender" binding:"required"`
	Birthday string `json:"birthday" binding:"required"` // YYYY-MM-DD
}

type updateChildReq struct {
	Nickname *string `json:"nickname,omitempty"`
	Gender   *string `json:"gender,omitempty"`
	Birthday *string `json:"birthday,omitempty"`
}

func (h *ChildHandler) requireUser(c *gin.Context) (int64, bool) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return 0, false
	}
	return uid, true
}

func (h *ChildHandler) create(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req createChildReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	bday, err := time.Parse("2006-01-02", req.Birthday)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_birthday", "user_msg": "生日格式应为 YYYY-MM-DD"})
		return
	}
	out, err := h.svc.Create(c.Request.Context(), uid, child.CreateInput{
		Nickname: req.Nickname, Gender: req.Gender, Birthday: bday,
	})
	if err != nil {
		// Translate child_already_exists -> 409
		if ae, ok := apperr.AsAppError(err); ok && ae.Reason == "child_already_exists" {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"reason": ae.Reason, "user_msg": ae.UserMsg})
			return
		}
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, childJSON(out))
}

func (h *ChildHandler) list(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	items, err := h.svc.ListByUser(c.Request.Context(), uid)
	if err != nil {
		RespondError(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, childJSON(it))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *ChildHandler) update(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_id", "user_msg": "id 不合法"})
		return
	}
	var req updateChildReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	in := child.UpdateInput{Nickname: req.Nickname, Gender: req.Gender}
	if req.Birthday != nil {
		t, err := time.Parse("2006-01-02", *req.Birthday)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_birthday", "user_msg": "生日格式应为 YYYY-MM-DD"})
			return
		}
		in.Birthday = &t
	}
	out, err := h.svc.Update(c.Request.Context(), uid, id, in)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, childJSON(out))
}

// childJSON shapes the JSON response for a Child object.
func childJSON(c *model.Child) gin.H {
	return gin.H{
		"id":       c.ID,
		"user_id":  c.UserID,
		"nickname": c.Nickname,
		"gender":   c.Gender,
		"birthday": c.Birthday.Format("2006-01-02"),
		"profile":  string(c.Profile), // jsonb stored as bytes; pass through as string
	}
}
```

- [ ] **Step 16.2：写测试**

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/child"
)

type childFakeRepo struct {
	byUser map[int64]*model.Child
	byID   map[int64]*model.Child
	next   int64
}

func newChildFakeRepo() *childFakeRepo {
	return &childFakeRepo{byUser: map[int64]*model.Child{}, byID: map[int64]*model.Child{}, next: 1}
}

func (r *childFakeRepo) Create(_ context.Context, c *model.Child) error {
	if _, ok := r.byUser[c.UserID]; ok {
		return repository.ErrAlreadyExists
	}
	c.ID = r.next
	r.next++
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c
	return nil
}
func (r *childFakeRepo) FindByUserID(_ context.Context, uid int64) (*model.Child, error) {
	if c, ok := r.byUser[uid]; ok {
		return c, nil
	}
	return nil, repository.ErrNotFound
}
func (r *childFakeRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	if c, ok := r.byID[id]; ok {
		return c, nil
	}
	return nil, repository.ErrNotFound
}
func (r *childFakeRepo) Update(_ context.Context, c *model.Child) error {
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c
	return nil
}

func setupChild(t *testing.T, asUser int64) (*gin.Engine, *childFakeRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newChildFakeRepo()
	svc := child.New(repo)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), asUser))
		c.Next()
	})
	v1 := r.Group("/api/v1")
	NewChildHandler(svc).RegisterRoutes(v1)
	return r, repo
}

func doJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	var req *http.Request
	if rd != nil {
		req = httptest.NewRequest(method, path, rd)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestChild_Create_OK(t *testing.T) {
	r, _ := setupChild(t, 7)
	rec := doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "小宇", "gender": "boy", "birthday": "2020-08-15",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "小宇")
	assert.Contains(t, rec.Body.String(), `"birthday":"2020-08-15"`)
}

func TestChild_Create_Conflict(t *testing.T) {
	r, _ := setupChild(t, 7)
	require.Equal(t, http.StatusCreated, doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "a", "gender": "boy", "birthday": "2020-08-15",
	}).Code)
	rec := doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "b", "gender": "girl", "birthday": "2020-08-15",
	})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "child_already_exists")
}

func TestChild_List_EmptyAndNonEmpty(t *testing.T) {
	r, _ := setupChild(t, 7)
	rec := doJSON(r, http.MethodGet, "/api/v1/children", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.Items)

	require.Equal(t, http.StatusCreated, doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	}).Code)
	rec = doJSON(r, http.MethodGet, "/api/v1/children", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Items, 1)
}

func TestChild_Update_OK(t *testing.T) {
	r, _ := setupChild(t, 7)
	doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	})
	rec := doJSON(r, http.MethodPatch, "/api/v1/children/1", map[string]string{"nickname": "n2"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "n2")
}

func TestChild_Update_Forbidden(t *testing.T) {
	r, _ := setupChild(t, 7)
	doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	})

	// switch user
	r2, _ := setupChild(t, 99)
	// route is per-engine; we need both to share repo. Simplest: only assert
	// that "user 99 cannot update child belonging to user 7" via direct lookup
	// — already covered in service-level test. So instead verify update on
	// non-existent id returns 404 here.
	rec := doJSON(r2, http.MethodPatch, "/api/v1/children/1", map[string]string{"nickname": "x"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 16.3：跑 + commit**

```bash
go test -count=1 ./internal/api/ -v
golangci-lint run ./internal/api/...
git add server/internal/api/child.go server/internal/api/child_test.go
git commit -m "feat(api): POST/GET/PATCH /children handlers"
```

---

## Task 17：Router 装配 + main.go 接入

**Files:**
- Modify: `server/internal/api/router.go`
- Modify: `server/cmd/server/main.go`

> 把 auth、me、child handler 挂上 v1 路由组，并把 JWT middleware 挂在受保护路由组上。

- [ ] **Step 17.1：修改 `router.go`**

```go
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aibao/server/internal/api/middleware"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

// RouterDeps groups everything NewRouter needs from main.go.
type RouterDeps struct {
	Metrics *metrics.Metrics
	Reg     *prometheus.Registry
	PG      Checker
	Redis   Checker

	// Auth-related (Plan 2)
	JWT     *jwtauth.Manager
	Auth    *AuthHandler
	Me      *MeHandler
	Child   *ChildHandler
}

// NewRouter builds the gin.Engine.
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

	// Public v1 routes
	v1 := r.Group("/api/v1")
	if deps.Auth != nil {
		deps.Auth.RegisterRoutes(v1)
	}

	// Authenticated v1 routes
	if deps.JWT != nil {
		auth := r.Group("/api/v1")
		auth.Use(middleware.JWTAuth(deps.JWT))
		if deps.Me != nil {
			deps.Me.RegisterRoutes(auth)
		}
		if deps.Child != nil {
			deps.Child.RegisterRoutes(auth)
		}
	}

	return r
}
```

- [ ] **Step 17.2：修改 `main.go`** —— 在 `run()` 中合适位置（DB/Redis 之后、router 之前）增加：

```go
	// Auto-apply pending migrations
	if err := repository.RunMigrations(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Build domain dependencies
	hasher := safehash.New(cfg.Crypto.SafehashSalt)
	pcipher, err := phonecrypt.New(cfg.Crypto.PhoneAESKey)
	if err != nil {
		return fmt.Errorf("init phone cipher: %w", err)
	}
	jwtMgr := jwtauth.New(
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.AccessTTLMinutes)*time.Minute,
		time.Duration(cfg.Auth.RefreshTTLMinutes)*time.Minute,
	)

	userRepo := repository.NewUserRepo(db)
	childRepo := repository.NewChildRepo(db)
	codeStore := authsvc.NewRedisCodeStore(rdb)

	var smsSender authsvc.SMS
	switch cfg.SMS.Provider {
	case "mock", "":
		smsSender = sms.NewMock()
	default:
		return fmt.Errorf("unknown sms provider: %s", cfg.SMS.Provider)
	}

	authService := authsvc.New(authsvc.Deps{
		Users:        userRepo,
		CodeStore:    codeStore,
		SMS:          smsSender,
		JWT:          jwtMgr,
		PhoneCipher:  pcipher,
		Hasher:       hasher,
		FixedDevCode: sms.NewMock().FixedCode(), // safe even when provider!=mock
		CodeTTL:      time.Duration(cfg.SMS.CodeTTLSeconds) * time.Second,
		Cooldown:     time.Duration(cfg.SMS.ResendCooldownSec) * time.Second,
	})
	childService := childsvc.New(childRepo)

	authHandler := api.NewAuthHandler(authService)
	meHandler := api.NewMeHandler(userRepo)
	childHandler := api.NewChildHandler(childService)

	router := api.NewRouter(api.RouterDeps{
		Metrics: m,
		Reg:     reg,
		PG:      pgChecker{db: db},
		Redis:   redisChecker{c: rdb},
		JWT:     jwtMgr,
		Auth:    authHandler,
		Me:      meHandler,
		Child:   childHandler,
	})
```

并在 main.go 的 import 里加：

```go
	"github.com/aibao/server/internal/gateway/sms"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/phonecrypt"
	"github.com/aibao/server/internal/pkg/safehash"
	authsvc "github.com/aibao/server/internal/service/auth"
	childsvc "github.com/aibao/server/internal/service/child"
```

> 注意：`FixedDevCode` 当 provider 是非 mock 时不再有意义，但留默认值不影响——当真发短信时，service 会忽略它（service 实现里用的还是这个值；MVP 阶段只支持 mock，二期接入腾讯时再改造此处）。

- [ ] **Step 17.3：编译 + lint**

```bash
go build ./...
golangci-lint run ./...
```
Expected: 0 issues。

- [ ] **Step 17.4：commit**

```bash
git add server/internal/api/router.go server/cmd/server/main.go
git commit -m "feat(server): wire auth/me/child handlers into router and main"
```

---

## Task 18：完整覆盖率检查 + Makefile run-dev 注入更多 env

**Files:**
- Modify: `server/Makefile`

- [ ] **Step 18.1：Makefile run-dev 加 dev-only 密钥**

```makefile
run-dev: build
	AIBAO_CONFIG=$(CONFIG_DEV) \
	AIBAO_POSTGRES_PASSWORD=aibao \
	AIBAO_AUTH_JWT_SECRET=dev-jwt-secret-change-me \
	AIBAO_CRYPTO_PHONE_AES_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
	AIBAO_CRYPTO_SAFEHASH_SALT=dev-safehash-salt \
	./$(BINARY)
```

(make 多行命令需用 `\` 续行)

- [ ] **Step 18.2：跑全套测试 + 覆盖率**

```bash
cd server
go test -count=1 -cover ./internal/pkg/... ./internal/gateway/... ./internal/service/... ./internal/api/...
```
Expected: 全过；service+pkg 平均 ≥ 70%。

- [ ] **Step 18.3：跑集成测试（要 Docker）**

```bash
go test -count=1 -tags=integration ./internal/repository/... ./internal/service/auth/...
```
Expected: 全过。

- [ ] **Step 18.4：commit**

```bash
git add server/Makefile
git commit -m "chore(make): inject auth/crypto dev secrets into run-dev"
```

---

## Task 19：端到端 smoke（手动，需 Docker）

> 这一节不写自动化脚本，人工执行；执行后把 outcomes 落入 devlog。

- [ ] **Step 19.1：跑迁移**

```bash
make migrate-up
```

- [ ] **Step 19.2：起服务**

```bash
make run-dev
```
另开终端：

- [ ] **Step 19.3：发验证码**

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/sms/send \
  -H "Content-Type: application/json" \
  -d '{"phone":"13800138000"}'
```
Expected: 200，server 日志中含 `sms.mock.send code=123456`。

- [ ] **Step 19.4：登录**

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/login_or_register \
  -H "Content-Type: application/json" \
  -d '{"phone":"13800138000","code":"123456","nickname":"妈妈"}'
```
Expected: 200，body 含 `access_token` / `refresh_token` / `user.id`。把 access_token 复制下来。

- [ ] **Step 19.5：拉 me**

```bash
TOKEN=<上面的 access_token>
curl -i http://localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
```
Expected: 200，含 `"nickname":"妈妈"`。

- [ ] **Step 19.6：创建孩子**

```bash
curl -i -X POST http://localhost:8080/api/v1/children \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"nickname":"小宇","gender":"boy","birthday":"2020-08-15"}'
```
Expected: 201，含 `"id":1`、`"nickname":"小宇"`。

- [ ] **Step 19.7：再创建孩子（应失败）**

```bash
curl -i -X POST http://localhost:8080/api/v1/children \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"nickname":"小妹","gender":"girl","birthday":"2022-01-01"}'
```
Expected: 409，`reason=child_already_exists`。

- [ ] **Step 19.8：列表**

```bash
curl -i http://localhost:8080/api/v1/children -H "Authorization: Bearer $TOKEN"
```
Expected: 200，items 长度 1。

- [ ] **Step 19.9：更新孩子昵称**

```bash
curl -i -X PATCH http://localhost:8080/api/v1/children/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"nickname":"小宇宙"}'
```
Expected: 200，`"nickname":"小宇宙"`。

- [ ] **Step 19.10：未带 token 访问 /me**

```bash
curl -i http://localhost:8080/api/v1/me
```
Expected: 401。

如果以上每一步都符合预期，写一篇 devlog 记录"Plan 2 端到端冒烟通过"。

---

## 完成验收清单（Plan 2 整体）

逐项勾选确认后，本 Plan 视为完成。

- [ ] `go build ./...` 通过
- [ ] `make test`（无 tag）全部通过
- [ ] `make test-integration`（含 Docker）全部通过
- [ ] `make lint` 0 issues
- [ ] service+pkg 层覆盖率 ≥ 70%
- [ ] 迁移脚本通过 `make migrate-up` 创建 `users` + `children` 表，`children` 含 `UNIQUE(user_id)`
- [ ] `make run-dev` 启动后所有 19.x 冒烟步骤通过
- [ ] 没带 Authorization 访问 `/api/v1/me` → 401
- [ ] JWT 过期或被篡改 → 401
- [ ] 同一用户重复创建孩子 → 409
- [ ] 日志中**绝不**出现明文手机号、明文 JWT secret、明文孩子姓名（孩子姓名落业务返回 body 里没问题，**日志**里不能有）
- [ ] 提交粒度合理（每个 Task 至少一个 commit），无 WIP 留存

---

## 后续 Plan 衔接

Plan 3 起会大量使用本 Plan 的产物：
- `userctx.FromContext(ctx)` —— 业务 service 拿当前用户
- `repository.UserRepo` / `ChildRepo` —— 数据访问入口
- `apperr.AppError` —— 统一错误模型已在 Plan 1，Plan 2 已大量应用

下一份 plan（Plan 3：安全模块——前置预审 + 后置审核 + 强约束模板）将在没有 LLM Gateway 的前提下做规则部分，留 LLM 兜底接入到 Plan 4。
