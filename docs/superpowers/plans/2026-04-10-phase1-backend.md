# Phase 1 Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete Go backend for Phase 1 — Gin HTTP server + WebSocket + Fx DI + GORM + Eino integration — delivering all Phase 1 API endpoints and real-time messaging.

**Architecture:** Service-first, dual-consumer. All business logic lives in `internal/service/`. HTTP handlers are thin (decode → call service → encode). WebSocket pushes use the same service methods. Eino Tools wrap service methods via `utils.InferTool`.

**Tech Stack:** Go 1.26, Gin, uber-go/fx, GORM + PostgreSQL, go-redis, zap, coder/websocket, cloudwego/eino, oapi-codegen

---

## File Map

| File | Responsibility |
|------|---------------|
| `server/cmd/server/main.go` | Entry point: flag parsing, fx.New().Run() |
| `server/internal/config/config.go` | Startup config from flags + env vars |
| `server/internal/di/module.go` | Fx providers: DB, Redis, services, handlers, router |
| `server/internal/di/logger.go` | Zap logger provider |
| `server/internal/db/db.go` | GORM connection, AutoMigrate |
| `server/internal/db/pgvector.go` | pgvector extension init |
| `server/internal/model/base.go` | BaseModel with ID, CreatedAt, UpdatedAt |
| `server/internal/model/user.go` | User model |
| `server/internal/model/project.go` | Project model |
| `server/internal/model/conversation.go` | Conversation + conversation member models |
| `server/internal/model/agent.go` | Agent + agent relationship models |
| `server/internal/model/message.go` | Message model |
| `server/internal/model/user_update.go` | UserUpdate (global seq log) |
| `server/internal/model/settings.go` | Settings model |
| `server/internal/service/user.go` | User service: GetOrCreate, GetMe |
| `server/internal/service/project.go` | Project service: list, create from template, get, delete |
| `server/internal/service/conversation.go` | Conversation service: list, create, get, delete |
| `server/internal/service/template.go` | Template registry + listing |
| `server/internal/service/chat.go` | Chat service: send message, stop, Eino agent execution |
| `server/internal/service/settings.go` | Settings service: get, update |
| `server/internal/handler/http.go` | Gin router setup, middleware, error helpers |
| `server/internal/handler/user.go` | GET /users/me |
| `server/internal/handler/template.go` | GET /templates, GET /templates/:id, POST /templates/:id/projects |
| `server/internal/handler/project.go` | CRUD projects |
| `server/internal/handler/conversation.go` | CRUD conversations, messages, stop |
| `server/internal/handler/settings.go` | GET/PUT /settings |
| `server/internal/eino/model.go` | LLM model factory (3 tiers) |
| `server/internal/eino/tools/registry.go` | Tool registration |
| `server/internal/eino/tools/weather.go` | Example weather tool |
| `server/internal/eino/agents/basic.go` | Template 01 Basic Agent builder |
| `server/internal/templates/registry.go` | Template registry |
| `server/internal/templates/01_basic_agent.go` | Template 01 definition |
| `server/internal/ws/protocol.go` | WS frame types, marshal/unmarshal |
| `server/internal/ws/manager.go` | Per-user connection manager + seq queue |
| `server/internal/ws/server.go` | WS upgrade handler |
| `openapi/generate.sh` | Type generation script (already exists) |
| `docker-compose.yml` | PostgreSQL + Redis |
| `.env.example` | Environment variable template |

---

### Task 1: Infrastructure — docker-compose, .env.example, generate.sh

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`
- Verify: `openapi/generate.sh` (already exists)

- [ ] **Step 1: Create docker-compose.yml**

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: eino_user
      POSTGRES_PASSWORD: eino_pass
      POSTGRES_DB: eino_demo
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  postgres_data:
```

- [ ] **Step 2: Create .env.example**

```bash
# OpenAI-compatible model config (3 tiers, each with independent BASE_URL + API_KEY + MODEL)
MODEL_HAIKU_BASE_URL=https://api.openai.com/v1
MODEL_HAIKU_API_KEY=sk-your-key-here
MODEL_HAIKU_MODEL=gpt-4o-mini

MODEL_SONNET_BASE_URL=https://api.openai.com/v1
MODEL_SONNET_API_KEY=sk-your-key-here
MODEL_SONNET_MODEL=gpt-4o

MODEL_OPUS_BASE_URL=https://api.openai.com/v1
MODEL_OPUS_API_KEY=sk-your-key-here
MODEL_OPUS_MODEL=o1

# PostgreSQL
DATABASE_URL=postgres://eino_user:eino_pass@localhost:5432/eino_demo?sslmode=disable

# Redis
REDIS_ADDR=localhost:6379

# Server
SERVER_PORT=8080
```

- [ ] **Step 3: Copy to .env and start Docker**

```bash
cp .env.example .env
docker compose up -d
```

Expected: PostgreSQL and Redis containers running.

- [ ] **Step 4: Run generate.sh to produce types**

```bash
./openapi/generate.sh
```

Expected: `server/internal/types/types.go` and `web/src/types/api.d.ts` generated.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml .env.example server/internal/types/types.go web/src/types/api.d.ts
git commit -m "feat: add docker-compose, env template, and generate initial types from OpenAPI spec"
```

---

### Task 2: go.mod dependencies + config

**Files:**
- Modify: `server/go.mod`
- Create: `server/internal/config/config.go`

- [ ] **Step 1: Update server/go.mod with all Phase 1 dependencies**

```go
module github.com/PineappleBond/eino-demo/server

go 1.26

require (
	github.com/cloudwego/eino v0.8.0
	github.com/cloudwego/eino-ext v0.1.15
	github.com/coder/websocket v1.8.13
	github.com/gin-gonic/gin v1.10.0
	github.com/google/uuid v1.6.0
	github.com/oapi-codegen/runtime v1.4.0
	github.com/redis/go-redis/v9 v9.7.0
	go.uber.org/fx v1.23.0
	go.uber.org/zap v1.27.0
	gorm.io/driver/postgres v1.5.11
	gorm.io/gorm v1.25.12
)
```

Run `cd server && go mod tidy` to resolve transitive deps.

- [ ] **Step 2: Create server/internal/config/config.go**

```go
package config

import (
	"flag"
	"os"
)

// ModelConfig holds one tier of model configuration.
type ModelConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Config holds all startup configuration.
type Config struct {
	ServerPort string
	DatabaseURL string
	RedisAddr string
	Models map[string]ModelConfig // "haiku", "sonnet", "opus"
}

// Load reads configuration from flags and environment variables.
func Load() *Config {
	port := flag.String("port", "8080", "HTTP server port")
	flag.Parse()

	// Override port from env if set
	if p := os.Getenv("SERVER_PORT"); p != "" {
		port = &p
	}

	return &Config{
		ServerPort:  *port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisAddr:   envOr("REDIS_ADDR", "localhost:6379"),
		Models: map[string]ModelConfig{
			"haiku": {
				BaseURL: envOr("MODEL_HAIKU_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_HAIKU_API_KEY"),
				Model:   envOr("MODEL_HAIKU_MODEL", "gpt-4o-mini"),
			},
			"sonnet": {
				BaseURL: envOr("MODEL_SONNET_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_SONNET_API_KEY"),
				Model:   envOr("MODEL_SONNET_MODEL", "gpt-4o"),
			},
			"opus": {
				BaseURL: envOr("MODEL_OPUS_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_OPUS_API_KEY"),
				Model:   envOr("MODEL_OPUS_MODEL", "o1"),
			},
		},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("config: required env var " + key + " is not set")
	}
	return v
}
```

- [ ] **Step 3: Commit**

```bash
git add server/go.mod server/go.sum server/internal/config/config.go
git commit -m "feat: add project dependencies and configuration with 3-tier model support"
```

---

### Task 3: Logger + DB + pgvector + GORM Models

**Files:**
- Create: `server/internal/di/logger.go`
- Create: `server/internal/db/db.go`
- Create: `server/internal/db/pgvector.go`
- Create: `server/internal/model/base.go`
- Create: `server/internal/model/user.go`
- Create: `server/internal/model/project.go`
- Create: `server/internal/model/conversation.go`
- Create: `server/internal/model/agent.go`
- Create: `server/internal/model/message.go`
- Create: `server/internal/model/user_update.go`
- Create: `server/internal/model/settings.go`

- [ ] **Step 1: Create server/internal/di/logger.go**

```go
package di

import (
	"go.uber.org/zap"
)

// ProvideLogger creates a production Zap logger.
func ProvideLogger() *zap.Logger {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("di: failed to create logger: " + err.Error())
	}
	return logger
}
```

- [ ] **Step 2: Create server/internal/db/db.go**

```go
package db

import (
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// ProvideDB opens a GORM connection and runs AutoMigrate.
func ProvideDB(databaseURL string, log *zap.Logger) *gorm.DB {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Discard, // we log errors ourselves
	})
	if err != nil {
		log.Fatal("db: failed to connect", zap.Error(err))
	}

	// Run pgvector extension first (before AutoMigrate creates tables that use it)
	if err := setupPGVector(db); err != nil {
		log.Fatal("db: failed to setup pgvector", zap.Error(err))
	}

	// AutoMigrate all models
	if err := db.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.Conversation{},
		&model.ConversationMember{},
		&model.Agent{},
		&model.AgentRelationship{},
		&model.Message{},
		&model.UserUpdate{},
		&model.Settings{},
	); err != nil {
		log.Fatal("db: AutoMigrate failed", zap.Error(err))
	}

	log.Info("db: connected and migrated")
	return db
}
```

- [ ] **Step 3: Create server/internal/db/pgvector.go**

```go
package db

import "gorm.io/gorm"

// setupPGVector creates the vector extension if it doesn't exist.
// GORM cannot create extensions, so this runs raw SQL before AutoMigrate.
func setupPGVector(db *gorm.DB) error {
	return db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
}
```

- [ ] **Step 4: Create server/internal/model/base.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// BaseModel provides common fields for all models.
type BaseModel struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
}
```

- [ ] **Step 5: Create server/internal/model/user.go**

```go
package model

// User represents an authenticated user.
// Auth token is NOT stored — middleware maps token → user UUID via FirstOrCreate.
type User struct {
	BaseModel
	Name string `gorm:"type:varchar(255);not null;default:''"`
}

func (User) TableName() string { return "users" }
```

- [ ] **Step 6: Create server/internal/model/project.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// Project is a user-created project from a template.
type Project struct {
	BaseModel
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index"`
	TemplateID string         `gorm:"type:varchar(32);not null"`
	Name       string         `gorm:"type:varchar(255);not null;default:''"`
	Config     map[string]any `gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedAt  time.Time      `gorm:"not null;autoUpdateTime"`
}

func (Project) TableName() string { return "projects" }
```

- [ ] **Step 7: Create server/internal/model/conversation.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// Conversation represents a single chat session within a project.
type Conversation struct {
	BaseModel
	ProjectID           uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID              uuid.UUID `gorm:"type:uuid;not null;index"`
	Title               string    `gorm:"type:varchar(512);not null;default:''"`
	Summary             string    `gorm:"type:text;not null;default:''"`
	Status              string    `gorm:"type:varchar(20);not null;default:'active';index:idx_user_status"`
	LastMessagePreview  string    `gorm:"type:text"`
	MessageCount        int       `gorm:"not null;default:0"`
	LatestMessageSeq    int64     `gorm:"not null;default:0"`
	TokenPrompt         int64     `gorm:"not null;default:0"`
	TokenCompletion     int64     `gorm:"not null;default:0"`
	MemberCount         int       `gorm:"not null;default:0"`
	ParentConversationID *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt           time.Time `gorm:"not null;autoUpdateTime;index"`
}

func (Conversation) TableName() string { return "conversations" }

// ConversationMember tracks who/what agents are in a conversation.
type ConversationMember struct {
	BaseModel
	ConversationID uuid.UUID `gorm:"type:uuid;not null;index"`
	MemberType     string    `gorm:"type:varchar(10);not null"` // "user" or "agent"
	MemberID       string    `gorm:"type:varchar(255);not null"`
	MemberName     string    `gorm:"type:varchar(255);not null;default:''"`
	IsOwner        bool      `gorm:"not null;default:false"`
}

func (ConversationMember) TableName() string { return "conversation_members" }
```

- [ ] **Step 8: Create server/internal/model/agent.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// Agent is an AI agent definition within a project.
type Agent struct {
	BaseModel
	ProjectID    uuid.UUID      `gorm:"type:uuid;not null;index"`
	AgentKey     string         `gorm:"type:varchar(64);not null"`
	AgentName    string         `gorm:"type:varchar(255);not null"`
	Description  string         `gorm:"type:text;not null;default:''"`
	Avatar       string         `gorm:"type:varchar(255);not null;default:''"`
	SystemPrompt string         `gorm:"type:text;not null;default:''"`
	Config       map[string]any `gorm:"type:jsonb;not null;default:'{}'"`
	SortOrder    int            `gorm:"not null;default:0"`
}

func (Agent) TableName() string { return "agents" }

// AgentRelationship defines parent-child links between agents.
type AgentRelationship struct {
	BaseModel
	ProjectID   uuid.UUID `gorm:"type:uuid;not null;index"`
	ParentID    uuid.UUID `gorm:"type:uuid;not null;index"`
	ChildID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Relationship string   `gorm:"type:varchar(64);not null;default:'delegates'"`
	Description string    `gorm:"type:text;not null;default:''"`
	SortOrder   int       `gorm:"not null;default:0"`
}

func (AgentRelationship) TableName() string { return "agent_relationships" }
```

- [ ] **Step 9: Create server/internal/model/message.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// Message is a single message in a conversation.
type Message struct {
	BaseModel
	ConversationID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	Seq             int64          `gorm:"not null"`
	SenderRole      string         `gorm:"type:varchar(20);not null"`
	SenderID        string         `gorm:"type:varchar(255);not null"`
	Content         string         `gorm:"type:text;not null;default:''"`
	ReasonContent   string         `gorm:"type:text;not null;default:''"`
	ReplyToSeq      *int64         `gorm:""`
	MentionedMembers []string      `gorm:"type:text[]"`
	Metadata        map[string]any `gorm:"type:jsonb;not null;default:'{}'"`
	FinishReason    *string        `gorm:"type:varchar(20)"`
	ErrorMessage    *string        `gorm:"type:text"`
	DurationMs      *int           `gorm:""`
	TokenPrompt     int64          `gorm:"not null;default:0"`
	TokenCompletion int64          `gorm:"not null;default:0"`
}

func (Message) TableName() string { return "messages" }
```

- [ ] **Step 10: Create server/internal/model/user_update.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// UserUpdate is the global Update event log for seq-based real-time sync.
type UserUpdate struct {
	BaseModel
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index:idx_user_seq"`
	Seq       int64          `gorm:"not null;index:idx_user_seq"`
	Type      string         `gorm:"type:varchar(40);not null"`
	Payload   map[string]any `gorm:"type:jsonb;not null"`
	CreatedAt time.Time      `gorm:"not null"`
}

func (UserUpdate) TableName() string { return "user_updates" }
```

- [ ] **Step 11: Create server/internal/model/settings.go**

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

// Settings stores user preferences.
type Settings struct {
	BaseModel
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	ModelTier string    `gorm:"type:varchar(10);not null;default:'sonnet'"`
	Locale    string    `gorm:"type:varchar(5);not null;default:'en'"`
	Theme     string    `gorm:"type:varchar(10);not null;default:'light'"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

func (Settings) TableName() string { return "settings" }
```

- [ ] **Step 12: Commit**

```bash
git add server/internal/di/logger.go server/internal/db/ server/internal/model/
git commit -m "feat: add GORM models, DB connection, and pgvector setup"
```

---

### Task 4: Fx DI module + main.go

**Files:**
- Create: `server/internal/di/module.go`
- Create: `server/cmd/server/main.go`

- [ ] **Step 1: Create server/internal/di/module.go**

```go
package di

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"github.com/redis/go-redis/v9"

	"github.com/PineappleBond/eino-demo/server/internal/config"
	"github.com/PineappleBond/eino-demo/server/internal/db"
)

// Module wires all dependencies for the application.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		ProvideLogger,
		db.ProvideDB,
		ProvideRedis,
	),
	// Handler modules — register Gin routes
	fx.Invoke(RegisterRoutes),
)

// ProvideRedis creates a Redis client.
func ProvideRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	return rdb
}

// RegisterRoutes sets up all Gin routes.
func RegisterRoutes(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	rdb *redis.Client,
) {
	// TODO: will be filled in later tasks
	log.Info("routes registered (stub)")
}
```

- [ ] **Step 2: Create server/cmd/server/main.go**

```go
package main

import (
	"go.uber.org/fx"

	"github.com/PineappleBond/eino-demo/server/internal/di"
)

func main() {
	app := fx.New(
		di.Module,
	)
	app.Run()
}
```

- [ ] **Step 3: Verify the project compiles**

```bash
cd server && go build ./cmd/server/
```

Expected: successful build (may fail at runtime without DB/Redis).

- [ ] **Step 4: Commit**

```bash
git add server/internal/di/module.go server/cmd/server/main.go
git commit -m "feat: add Fx DI module and main entry point"
```

---

### Task 5: Auth middleware + HTTP handler helpers

**Files:**
- Create: `server/internal/handler/http.go`

- [ ] **Step 1: Create server/internal/handler/http.go**

This file sets up the Gin router, auth middleware, and shared error response helpers.

```go
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/config"
	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// NewRouter creates and configures the Gin engine with all routes.
func NewRouter(
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(log))

	api := r.Group("/api/v1")
	api.Use(authMiddleware(db))

	// Routes will be registered by individual handlers here.
	// For now, register a health check.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}

// authMiddleware extracts user_id from Bearer token via FirstOrCreate.
func authMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "UNAUTHORIZED", "message": "missing or empty Authorization header"},
			})
			return
		}

		// Demo mode: use token as a lookup key to find or create user.
		// The token itself is NOT stored — we generate a UUID.
		var user model.User
		result := db.Where("id = ?::uuid", token).First(&user)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// Token is not a UUID — try parsing it as the user's auth token.
				// In demo mode, the token IS the lookup. Create a user with a random UUID ID,
				// but we need a way to map the token back. For simplicity in demo mode,
				// we'll use the token hash as a deterministic UUID-like key.
				// Actually, the simplest approach: use token as a seed for a deterministic user.
				// Since tokens are arbitrary strings in demo mode, we FirstOrCreate by a
				// derived key. For now, generate a UUID from the token.
				userID := uuid.NewSHA1(uuid.Nil, []byte(token))
				user = model.User{ID: userID, Name: ""}
				db.FirstOrCreate(&user, model.User{ID: userID})
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{"code": "INTERNAL_ERROR", "message": "database error"},
				})
				return
			}
		}

		c.Set("user_id", user.ID)
		c.Next()
	}
}

// getUserID extracts the authenticated user's UUID from context.
func getUserID(c *gin.Context) uuid.UUID {
	id, _ := c.Get("user_id")
	return id.(uuid.UUID)
}

// requestLogger logs each request's method, path, and status.
func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}

// respondJSON sends a JSON response.
func respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// respondError sends a uniform error response.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}
```

- [ ] **Step 2: Update module.go to wire the router**

Modify `server/internal/di/module.go` — replace the stub `RegisterRoutes`:

```go
// RegisterRoutes sets up all Gin routes and starts the server.
func RegisterRoutes(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	rdb *redis.Client,
) {
	r := handler.NewRouter(cfg, log, db)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr := ":" + cfg.ServerPort
			log.Info("server starting", zap.String("addr", addr))
			go func() {
				if err := r.Run(addr); err != nil {
					log.Error("server error", zap.Error(err))
				}
			}()
			return nil
		},
	})
}
```

Add `"context"` to imports.

- [ ] **Step 3: Commit**

```bash
git add server/internal/handler/http.go server/internal/di/module.go
git commit -m "feat: add auth middleware, Gin router setup, and server lifecycle"
```

---

### Task 6: User service + handler + Settings service + handler

**Files:**
- Create: `server/internal/service/user.go`
- Create: `server/internal/service/settings.go`
- Create: `server/internal/handler/user.go`
- Create: `server/internal/handler/settings.go`

- [ ] **Step 1: Create server/internal/service/user.go**

```go
package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// UserService handles user-related business logic.
type UserService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewUserService creates a UserService.
func NewUserService(db *gorm.DB, log *zap.Logger) *UserService {
	return &UserService{db: db, log: log}
}

// GetMe returns the user profile with settings.
func (s *UserService) GetMe(ctx context.Context, userID uuid.UUID) (*model.User, *model.Settings, error) {
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return nil, nil, err
	}

	var settings model.Settings
	s.db.WithContext(ctx).Where("user_id = ?", userID).FirstOrCreate(&settings, model.Settings{
		UserID:    userID,
		ModelTier: "sonnet",
		Locale:    "en",
		Theme:     "light",
	})

	return &user, &settings, nil
}
```

- [ ] **Step 2: Create server/internal/service/settings.go**

```go
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// SettingsService handles user settings logic.
type SettingsService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewSettingsService creates a SettingsService.
func NewSettingsService(db *gorm.DB, log *zap.Logger) *SettingsService {
	return &SettingsService{db: db, log: log}
}

// GetSettings returns user settings.
func (s *SettingsService) GetSettings(ctx context.Context, userID uuid.UUID) (*model.Settings, error) {
	var settings model.Settings
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateSettingsRequest holds the fields to update (zero values mean "don't change").
type UpdateSettingsRequest struct {
	ModelTier *string `json:"model_tier,omitempty"`
	Locale    *string `json:"locale,omitempty"`
	Theme     *string `json:"theme,omitempty"`
}

// UpdateSettings updates only the provided fields.
func (s *SettingsService) UpdateSettings(ctx context.Context, userID uuid.UUID, req UpdateSettingsRequest) (*model.Settings, error) {
	var settings model.Settings
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.ModelTier != nil {
		updates["model_tier"] = *req.ModelTier
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.Theme != nil {
		updates["theme"] = *req.Theme
	}
	updates["updated_at"] = time.Now()

	if len(updates) > 1 { // more than just updated_at
		s.db.WithContext(ctx).Model(&settings).Updates(updates)
		s.db.WithContext(ctx).Where("user_id = ?", userID).First(&settings)
	}

	return &settings, nil
}
```

- [ ] **Step 3: Create server/internal/handler/user.go**

```go
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo/server/internal/service"
)

// RegisterUserRoutes registers GET /users/me.
func RegisterUserRoutes(api *gin.RouterGroup, svc *service.UserService) {
	api.GET("/users/me", func(c *gin.Context) {
		userID := getUserID(c)
		user, settings, err := svc.GetMe(c.Request.Context(), userID)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch user")
			return
		}
		respondJSON(c, 200, gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"created_at": user.CreatedAt,
			"settings": gin.H{
				"model_tier": settings.ModelTier,
				"locale":     settings.Locale,
				"theme":      settings.Theme,
				"updated_at": settings.UpdatedAt,
			},
		})
	})
}
```

- [ ] **Step 4: Create server/internal/handler/settings.go**

```go
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo/server/internal/service"
)

// RegisterSettingsRoutes registers GET /settings and PUT /settings.
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService) {
	api.GET("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		settings, err := svc.GetSettings(c.Request.Context(), userID)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch settings")
			return
		}
		respondJSON(c, 200, gin.H{
			"model_tier": settings.ModelTier,
			"locale":     settings.Locale,
			"theme":      settings.Theme,
			"updated_at": settings.UpdatedAt,
		})
	})

	api.PUT("/settings", func(c *gin.Context) {
		userID := getUserID(c)
		var req service.UpdateSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		settings, err := svc.UpdateSettings(c.Request.Context(), userID, req)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to update settings")
			return
		}
		respondJSON(c, 200, gin.H{
			"model_tier": settings.ModelTier,
			"locale":     settings.Locale,
			"theme":      settings.Theme,
			"updated_at": settings.UpdatedAt,
		})
	})
}
```

- [ ] **Step 5: Wire user and settings routes in http.go**

In `handler.NewRouter`, after `api.Use(authMiddleware(db))`, add:

```go
	userSvc := service.NewUserService(db, log)
	settingsSvc := service.NewSettingsService(db, log)

	RegisterUserRoutes(api, userSvc)
	RegisterSettingsRoutes(api, settingsSvc)
```

Add import for `"github.com/PineappleBond/eino-demo/server/internal/service"`.

- [ ] **Step 6: Commit**

```bash
git add server/internal/service/user.go server/internal/service/settings.go server/internal/handler/user.go server/internal/handler/settings.go
git commit -m "feat: add user and settings services with HTTP endpoints"
```

---

### Task 7: Template registry + Template 01 + Template service + handler

**Files:**
- Create: `server/internal/templates/registry.go`
- Create: `server/internal/templates/01_basic_agent.go`
- Create: `server/internal/service/template.go`
- Create: `server/internal/handler/template.go`

- [ ] **Step 1: Create server/internal/templates/registry.go**

```go
package templates

import "sort"

// TemplateInfo is the public metadata for a template.
type TemplateInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Difficulty  string   `json:"difficulty"` // beginner, intermediate, advanced
}

// TemplateDetail includes full agent/tool config for the detail view.
type TemplateDetail struct {
	TemplateInfo
	Agents        []TemplateAgentInfo `json:"agents"`
	Tools         []TemplateToolInfo  `json:"tools"`
	InitialPrompt string              `json:"initial_prompt"`
}

// TemplateAgentInfo describes an agent in a template.
type TemplateAgentInfo struct {
	AgentKey     string `json:"agent_key"`
	AgentName    string `json:"agent_name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
}

// TemplateToolInfo describes a tool in a template.
type TemplateToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Registry holds all registered templates.
var Registry = make(map[string]TemplateDetail)

// Register adds a template to the registry.
func Register(t TemplateDetail) {
	Registry[t.ID] = t
}

// List returns all template metadata sorted by ID.
func List() []TemplateInfo {
	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]TemplateInfo, 0, len(ids))
	for _, id := range ids {
		result = append(result, Registry[id].TemplateInfo)
	}
	return result
}

// Get returns a template by ID.
func Get(id string) (TemplateDetail, bool) {
	t, ok := Registry[id]
	return t, ok
}
```

- [ ] **Step 2: Create server/internal/templates/01_basic_agent.go**

```go
package templates

func init() {
	Register(TemplateDetail{
		TemplateInfo: TemplateInfo{
			ID:          "01",
			Name:        "Basic Agent",
			Description: "A single ChatModelAgent with tools and ReAct loop. The simplest Eino integration pattern.",
			Tags:        []string{"agent", "tools", "react"},
			Difficulty:  "beginner",
		},
		Agents: []TemplateAgentInfo{
			{
				AgentKey:     "basic",
				AgentName:    "Basic Agent",
				Description:  "A chat agent with tool support and ReAct loop.",
				SystemPrompt: "You are a helpful assistant. Use tools when appropriate to answer questions.",
			},
		},
		Tools: []TemplateToolInfo{
			{Name: "weather", Description: "Get current weather for a location"},
		},
		InitialPrompt: "Hello! I'm a basic agent. I can answer questions and use tools. Try asking me about the weather!",
	})
}
```

- [ ] **Step 3: Create server/internal/service/template.go**

```go
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
	"github.com/PineappleBond/eino-demo/server/internal/templates"
)

// TemplateService handles template listing and project creation.
type TemplateService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewTemplateService creates a TemplateService.
func NewTemplateService(db *gorm.DB, log *zap.Logger) *TemplateService {
	return &TemplateService{db: db, log: log}
}

// ListTemplates returns all template metadata.
func (s *TemplateService) ListTemplates() []templates.TemplateInfo {
	return templates.List()
}

// GetTemplate returns full template detail.
func (s *TemplateService) GetTemplate(id string) (templates.TemplateDetail, error) {
	t, ok := templates.Get(id)
	if !ok {
		return t, fmt.Errorf("template %q not found", id)
	}
	return t, nil
}

// CreateProjectFromTemplate creates a project, agents, and agent relationships.
func (s *TemplateService) CreateProjectFromTemplate(ctx context.Context, userID uuid.UUID, templateID string, name string, config map[string]any) (*model.Project, error) {
	t, ok := templates.Get(templateID)
	if !ok {
		return nil, fmt.Errorf("template %q not found", templateID)
	}

	if name == "" {
		name = t.Name
	}
	if config == nil {
		config = map[string]any{}
	}

	project := model.Project{
		UserID:     userID,
		TemplateID: templateID,
		Name:       name,
		Config:     config,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		// Create agents from template
		agentIDMap := make(map[string]uuid.UUID) // agent_key → agent UUID
		for _, agentInfo := range t.Agents {
			agent := model.Agent{
				ProjectID:    project.ID,
				AgentKey:     agentInfo.AgentKey,
				AgentName:    agentInfo.AgentName,
				Description:  agentInfo.Description,
				SystemPrompt: agentInfo.SystemPrompt,
				Config:       map[string]any{"model_tier": "sonnet"},
			}
			if err := tx.Create(&agent).Error; err != nil {
				return err
			}
			agentIDMap[agentInfo.AgentKey] = agent.ID
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &project, nil
}
```

- [ ] **Step 4: Create server/internal/handler/template.go**

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo/server/internal/service"
)

// RegisterTemplateRoutes registers template endpoints.
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService) {
	api.GET("/templates", func(c *gin.Context) {
		list := svc.ListTemplates()
		respondJSON(c, 200, list)
	})

	api.GET("/templates/:id", func(c *gin.Context) {
		id := c.Param("id")
		t, err := svc.GetTemplate(id)
		if err != nil {
			respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		respondJSON(c, 200, t)
	})

	api.POST("/templates/:id/projects", func(c *gin.Context) {
		userID := getUserID(c)
		templateID := c.Param("id")

		var req struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		}
		c.ShouldBindJSON(&req)

		project, err := svc.CreateProjectFromTemplate(c.Request.Context(), userID, templateID, req.Name, req.Config)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		respondJSON(c, http.StatusCreated, gin.H{
			"id":           project.ID,
			"user_id":      project.UserID,
			"template_id":  project.TemplateID,
			"name":         project.Name,
			"config":       project.Config,
			"created_at":   project.CreatedAt,
			"updated_at":   project.UpdatedAt,
		})
	})
}
```

- [ ] **Step 5: Wire template routes in http.go**

Add to `NewRouter`:
```go
	templateSvc := service.NewTemplateService(db, log)
	RegisterTemplateRoutes(api, templateSvc)
```

- [ ] **Step 6: Commit**

```bash
git add server/internal/templates/ server/internal/service/template.go server/internal/handler/template.go
git commit -m "feat: add template registry, Template 01, and project creation from template"
```

---

### Task 8: Project service + handler

**Files:**
- Create: `server/internal/service/project.go`
- Create: `server/internal/handler/project.go`

- [ ] **Step 1: Create server/internal/service/project.go**

```go
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// ProjectService handles project CRUD operations.
type ProjectService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewProjectService creates a ProjectService.
func NewProjectService(db *gorm.DB, log *zap.Logger) *ProjectService {
	return &ProjectService{db: db, log: log}
}

// ListProjects returns user's projects sorted by updated_at desc.
func (s *ProjectService) ListProjects(ctx context.Context, userID uuid.UUID, limit int, cursor *time.Time) ([]model.Project, bool, error) {
	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if cursor != nil {
		q = q.Where("updated_at < ?", cursor)
	}
	q = q.Order("updated_at DESC").Limit(limit + 1)

	var projects []model.Project
	if err := q.Find(&projects).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(projects) > limit
	if hasMore {
		projects = projects[:limit]
	}
	return projects, hasMore, nil
}

// GetProject returns a project with its agents and relationships.
func (s *ProjectService) GetProject(ctx context.Context, userID, projectID uuid.UUID) (*model.Project, []model.Agent, []model.AgentRelationship, error) {
	var project model.Project
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, errors.New("project not found")
		}
		return nil, nil, nil, err
	}

	var agents []model.Agent
	s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("sort_order ASC").Find(&agents)

	var relationships []model.AgentRelationship
	s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("sort_order ASC").Find(&relationships)

	return &project, agents, relationships, nil
}

// DeleteProject removes a project and cascades all children.
func (s *ProjectService) DeleteProject(ctx context.Context, userID, projectID uuid.UUID) error {
	result := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", projectID, userID).Delete(&model.Project{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("project not found")
	}
	return nil
}
```

- [ ] **Step 2: Create server/internal/handler/project.go**

```go
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo/server/internal/service"
)

// RegisterProjectRoutes registers project endpoints.
func RegisterProjectRoutes(api *gin.RouterGroup, svc *service.ProjectService) {
	api.GET("/projects", func(c *gin.Context) {
		userID := getUserID(c)
		limit := 20
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}

		var cursor *time.Time
		if t := c.Query("cursor"); t != "" {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				cursor = &parsed
			}
		}

		projects, hasMore, err := svc.ListProjects(c.Request.Context(), userID, limit, cursor)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to list projects")
			return
		}

		result := make([]gin.H, 0, len(projects))
		for _, p := range projects {
			result = append(result, gin.H{
				"id":          p.ID,
				"user_id":     p.UserID,
				"template_id": p.TemplateID,
				"name":        p.Name,
				"config":      p.Config,
				"created_at":  p.CreatedAt,
				"updated_at":  p.UpdatedAt,
			})
		}

		resp := gin.H{
			"projects": result,
			"has_more": hasMore,
		}
		if hasMore && len(projects) > 0 {
			resp["next_cursor"] = projects[len(projects)-1].UpdatedAt.Format(time.RFC3339)
		}
		respondJSON(c, 200, resp)
	})

	api.GET("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid project ID")
			return
		}

		project, agents, relationships, err := svc.GetProject(c.Request.Context(), userID, projectID)
		if err != nil {
			respondError(c, 404, "NOT_FOUND", "project not found")
			return
		}

		respondJSON(c, 200, gin.H{
			"id":                  project.ID,
			"user_id":             project.UserID,
			"template_id":         project.TemplateID,
			"name":                project.Name,
			"config":              project.Config,
			"created_at":          project.CreatedAt,
			"updated_at":          project.UpdatedAt,
			"agents":              agents,
			"agent_relationships": relationships,
		})
	})

	api.DELETE("/projects/:id", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid project ID")
			return
		}

		if err := svc.DeleteProject(c.Request.Context(), userID, projectID); err != nil {
			respondError(c, 404, "NOT_FOUND", "project not found")
			return
		}
		c.Status(204)
	})
}
```

- [ ] **Step 3: Wire project routes in http.go**

Add to `NewRouter`:
```go
	projectSvc := service.NewProjectService(db, log)
	RegisterProjectRoutes(api, projectSvc)
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/project.go server/internal/handler/project.go
git commit -m "feat: add project list, detail, and delete endpoints"
```

---

### Task 9: Conversation service + handler

**Files:**
- Create: `server/internal/service/conversation.go`
- Create: `server/internal/handler/conversation.go`

- [ ] **Step 1: Create server/internal/service/conversation.go**

```go
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// ConversationService handles conversation CRUD operations.
type ConversationService struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewConversationService creates a ConversationService.
func NewConversationService(db *gorm.DB, log *zap.Logger) *ConversationService {
	return &ConversationService{db: db, log: log}
}

// ListConversations returns conversations for a project.
func (s *ConversationService) ListConversations(ctx context.Context, userID, projectID uuid.UUID, status string, limit int, cursor *time.Time) ([]model.Conversation, bool, error) {
	q := s.db.WithContext(ctx).Where("user_id = ? AND project_id = ?", userID, projectID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if cursor != nil {
		q = q.Where("updated_at < ?", cursor)
	}
	q = q.Order("updated_at DESC").Limit(limit + 1)

	var conversations []model.Conversation
	if err := q.Find(&conversations).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(conversations) > limit
	if hasMore {
		conversations = conversations[:limit]
	}
	return conversations, hasMore, nil
}

// CreateConversationRequest holds optional fields for creating a conversation.
type CreateConversationRequest struct {
	Title                string     `json:"title"`
	ParentConversationID *uuid.UUID `json:"parent_conversation_id"`
}

// CreateConversation creates a new conversation and adds the user as a member.
func (s *ConversationService) CreateConversation(ctx context.Context, userID, projectID uuid.UUID, req CreateConversationRequest) (*model.Conversation, error) {
	conv := model.Conversation{
		ProjectID:          projectID,
		UserID:             userID,
		Title:              req.Title,
		ParentConversationID: req.ParentConversationID,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conv).Error; err != nil {
			return err
		}

		// Add user as owner member
		member := model.ConversationMember{
			ConversationID: conv.ID,
			MemberType:     "user",
			MemberID:       userID.String(),
			MemberName:     "",
			IsOwner:        true,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		// Add agents as members
		var agents []model.Agent
		tx.Where("project_id = ?", projectID).Order("sort_order ASC").Find(&agents)
		for _, agent := range agents {
			agentMember := model.ConversationMember{
				ConversationID: conv.ID,
				MemberType:     "agent",
				MemberID:       agent.AgentKey,
				MemberName:     agent.AgentName,
			}
			tx.Create(&agentMember)
		}

		conv.MemberCount = 1 + len(agents)
		tx.Model(&conv).Update("member_count", conv.MemberCount)

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// GetConversation returns a single conversation.
func (s *ConversationService) GetConversation(ctx context.Context, userID, conversationID uuid.UUID) (*model.Conversation, error) {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("conversation not found")
		}
		return nil, err
	}
	return &conv, nil
}

// DeleteConversation removes a conversation and its messages.
func (s *ConversationService) DeleteConversation(ctx context.Context, userID, conversationID uuid.UUID) error {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("conversation not found")
		}
		return err
	}

	if conv.Status == "compacted" || conv.Status == "archived" {
		return errors.New("cannot delete conversation in terminal state")
	}

	result := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).Delete(&model.Conversation{})
	if result.RowsAffected == 0 {
		return errors.New("conversation not found")
	}
	return result.Error
}

// GetMessages returns messages for a conversation.
func (s *ConversationService) GetMessages(ctx context.Context, conversationID uuid.UUID, afterSeq *int64, beforeSeq *int64, limit int, order string) ([]model.Message, bool, int64, error) {
	q := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)

	if afterSeq != nil {
		q = q.Where("seq > ?", *afterSeq)
		order = "asc" // sync always ascending
	}
	if beforeSeq != nil {
		q = q.Where("seq < ?", *beforeSeq)
	}
	if order == "" {
		order = "asc"
	}
	q = q.Order("seq " + order).Limit(limit + 1)

	var messages []model.Message
	if err := q.Find(&messages).Error; err != nil {
		return nil, false, 0, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		if order == "desc" {
			messages = messages[:limit]
		} else {
			messages = messages[len(messages)-limit:]
			hasMore = true
		}
	}

	// Get latest_message_seq
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Select("latest_message_seq").Where("id = ?", conversationID).First(&conv).Error; err != nil {
		return nil, false, 0, err
	}

	return messages, hasMore, conv.LatestMessageSeq, nil
}
```

- [ ] **Step 2: Create server/internal/handler/conversation.go**

```go
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PineappleBond/eino-demo/server/internal/service"
)

// RegisterConversationRoutes registers conversation endpoints.
func RegisterConversationRoutes(api *gin.RouterGroup, svc *service.ConversationService) {
	// List project conversations
	api.GET("/projects/:id/conversations", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid project ID")
			return
		}

		status := c.DefaultQuery("status", "active")
		limit := 50
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		var cursor *time.Time
		if t := c.Query("cursor"); t != "" {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				cursor = &parsed
			}
		}

		conversations, hasMore, err := svc.ListConversations(c.Request.Context(), userID, projectID, status, limit, cursor)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to list conversations")
			return
		}

		result := make([]gin.H, 0, len(conversations))
		for _, conv := range conversations {
			result = append(result, conversationToJSON(conv))
		}
		resp := gin.H{"conversations": result, "has_more": hasMore}
		if hasMore && len(conversations) > 0 {
			resp["next_cursor"] = conversations[len(conversations)-1].UpdatedAt.Format(time.RFC3339)
		}
		respondJSON(c, 200, resp)
	})

	// Create conversation
	api.POST("/projects/:id/conversations", func(c *gin.Context) {
		userID := getUserID(c)
		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid project ID")
			return
		}

		var req service.CreateConversationRequest
		c.ShouldBindJSON(&req)

		conv, err := svc.CreateConversation(c.Request.Context(), userID, projectID, req)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to create conversation")
			return
		}
		respondJSON(c, 201, conversationToJSON(conv))
	})

	// Get conversation detail
	api.GET("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		conv, err := svc.GetConversation(c.Request.Context(), userID, conversationID)
		if err != nil {
			respondError(c, 404, "NOT_FOUND", "conversation not found")
			return
		}
		respondJSON(c, 200, conversationToJSON(conv))
	})

	// Delete conversation
	api.DELETE("/conversations/:id", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		if err := svc.DeleteConversation(c.Request.Context(), userID, conversationID); err != nil {
			respondError(c, 404, "NOT_FOUND", err.Error())
			return
		}
		c.Status(204)
	})

	// Get messages
	api.GET("/conversations/:id/messages", func(c *gin.Context) {
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		limit := 50
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}

		var afterSeq, beforeSeq *int64
		if a := c.Query("after_seq"); a != "" {
			if v, err := strconv.ParseInt(a, 10, 64); err == nil {
				afterSeq = &v
			}
		}
		if b := c.Query("before_seq"); b != "" {
			if v, err := strconv.ParseInt(b, 10, 64); err == nil {
				beforeSeq = &v
			}
		}

		order := c.DefaultQuery("order", "asc")

		messages, hasMore, latestSeq, err := svc.GetMessages(c.Request.Context(), conversationID, afterSeq, beforeSeq, limit, order)
		if err != nil {
			respondError(c, 404, "NOT_FOUND", "conversation not found")
			return
		}

		result := make([]gin.H, 0, len(messages))
		for _, m := range messages {
			result = append(result, messageToJSON(m))
		}
		respondJSON(c, 200, gin.H{
			"messages":   result,
			"has_more":   hasMore,
			"latest_seq": latestSeq,
		})
	})
}

func conversationToJSON(conv model.Conversation) gin.H {
	return gin.H{
		"id":                     conv.ID,
		"project_id":             conv.ProjectID,
		"user_id":                conv.UserID,
		"title":                  conv.Title,
		"summary":                conv.Summary,
		"status":                 conv.Status,
		"last_message_preview":   conv.LastMessagePreview,
		"message_count":          conv.MessageCount,
		"latest_message_seq":     conv.LatestMessageSeq,
		"token_prompt":           conv.TokenPrompt,
		"token_completion":       conv.TokenCompletion,
		"member_count":           conv.MemberCount,
		"parent_conversation_id": conv.ParentConversationID,
		"created_at":             conv.CreatedAt,
		"updated_at":             conv.UpdatedAt,
	}
}

func messageToJSON(m model.Message) gin.H {
	result := gin.H{
		"id":              m.ID,
		"conversation_id":   m.ConversationID,
		"seq":               m.Seq,
		"sender_role":       m.SenderRole,
		"sender_id":         m.SenderID,
		"content":           m.Content,
		"reason_content":    m.ReasonContent,
		"metadata":          m.Metadata,
		"token_prompt":      m.TokenPrompt,
		"token_completion":  m.TokenCompletion,
		"created_at":        m.CreatedAt,
	}
	if m.ReplyToSeq != nil {
		result["reply_to_seq"] = *m.ReplyToSeq
	}
	if m.FinishReason != nil {
		result["finish_reason"] = *m.FinishReason
	}
	if m.ErrorMessage != nil {
		result["error_message"] = *m.ErrorMessage
	}
	if m.DurationMs != nil {
		result["duration_ms"] = *m.DurationMs
	}
	if m.MentionedMembers != nil {
		result["mentioned_members"] = m.MentionedMembers
	}
	return result
}
```

- [ ] **Step 3: Wire conversation routes in http.go**

Add to `NewRouter`:
```go
	convSvc := service.NewConversationService(db, log)
	RegisterConversationRoutes(api, convSvc)
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/service/conversation.go server/internal/handler/conversation.go
git commit -m "feat: add conversation CRUD and message listing endpoints"
```

---

### Task 10: WebSocket layer — protocol, manager, server

**Files:**
- Create: `server/internal/ws/protocol.go`
- Create: `server/internal/ws/manager.go`
- Create: `server/internal/ws/server.go`

- [ ] **Step 1: Create server/internal/ws/protocol.go**

```go
package ws

import "encoding/json"

// FrameType identifies WebSocket frame types.
type FrameType string

const (
	FrameConnected FrameType = "connected"
	FrameUpdates   FrameType = "updates"
	FramePing      FrameType = "ping"
)

// ServerFrame is a server→client WebSocket envelope.
type ServerFrame struct {
	Type    FrameType `json:"type"`
	Payload any       `json:"payload"`
}

// ClientFrame is a client→server WebSocket envelope.
type ClientFrame struct {
	Type    FrameType      `json:"type"`
	Payload map[string]any `json:"payload"`
}

// ConnectedPayload is sent when a client connects.
type ConnectedPayload struct {
	UserID     string `json:"user_id"`
	ServerTime string `json:"server_time"`
	MaxSeq     int64  `json:"max_seq"`
}

// MarshalJSON encodes a ServerFrame to JSON.
func (f ServerFrame) MarshalJSON() ([]byte, error) {
	type Alias ServerFrame
	return json.Marshal(Alias(f))
}

// Update represents a single Update event (matches the OpenAPI Update schema).
type Update struct {
	Seq     int64          `json:"seq"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}
```

- [ ] **Step 2: Create server/internal/ws/manager.go**

```go
package ws

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Manager handles per-user WebSocket connections and seq-based update delivery.
type Manager struct {
	mu      sync.RWMutex
	conns   map[uuid.UUID][]*wsConn // user_id → connections
	rdb     *redis.Client
	log     *zap.Logger
}

type wsConn struct {
	conn   *websocket.Conn
	send   chan []byte
	userID uuid.UUID
}

// NewManager creates a connection manager.
func NewManager(rdb *redis.Client, log *zap.Logger) *Manager {
	return &Manager{
		conns: make(map[uuid.UUID][]*wsConn),
		rdb:   rdb,
		log:   log,
	}
}

// AddConnection registers a new WebSocket for a user and starts read/write loops.
func (m *Manager) AddConnection(ctx context.Context, userID uuid.UUID, conn *websocket.Conn) {
	wc := &wsConn{
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}

	m.mu.Lock()
	m.conns[userID] = append(m.conns[userID], wc)
	m.mu.Unlock()

	// Heartbeat timeout: 1 minute
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)

	go m.writeLoop(ctx, wc, cancel)
	go m.readLoop(ctx, wc, cancel)
}

// PushToUserConnections broadcasts an Update to all connections for a user.
func (m *Manager) PushToUserConnections(userID uuid.UUID, update Update) {
	data, err := json.Marshal(ServerFrame{
		Type: FrameUpdates,
		Payload: []Update{update},
	})
	if err != nil {
		m.log.Error("ws: marshal failed", zap.Error(err))
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, wc := range m.conns[userID] {
		select {
		case wc.send <- data:
		default:
			m.log.Warn("ws: send channel full, dropping message")
		}
	}
}

// PushBatchToUserConnections broadcasts a batch of Updates to all connections for a user.
func (m *Manager) PushBatchToUserConnections(userID uuid.UUID, updates []Update) {
	if len(updates) == 0 {
		return
	}
	data, err := json.Marshal(ServerFrame{
		Type:    FrameUpdates,
		Payload: updates,
	})
	if err != nil {
		m.log.Error("ws: marshal batch failed", zap.Error(err))
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, wc := range m.conns[userID] {
		select {
		case wc.send <- data:
		default:
			m.log.Warn("ws: send channel full, dropping batch")
		}
	}
}

// NextSeq assigns a monotonically increasing seq via Redis INCR.
func (m *Manager) NextSeq(ctx context.Context, userID uuid.UUID) (int64, error) {
	return m.rdb.Incr(ctx, "seq:"+userID.String()).Result()
}

// writeLoop sends frames to the client.
func (m *Manager) writeLoop(ctx context.Context, wc *wsConn, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-wc.send:
			if err := wc.conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// readLoop handles incoming client frames (only ping).
func (m *Manager) readLoop(ctx context.Context, wc *wsConn, cancel context.CancelFunc) {
	defer cancel()
	for {
		_, msg, err := wc.conn.Read(ctx)
		if err != nil {
			return
		}

		var frame ClientFrame
		if err := json.Unmarshal(msg, &frame); err != nil {
			continue
		}

		if frame.Type == FramePing {
			// Reset the timeout — connection is alive
			// We rely on the parent context's timeout for eviction
		}
	}
}

// RemoveConnection removes a connection when it closes.
func (m *Manager) RemoveConnection(userID uuid.UUID, wc *wsConn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conns := m.conns[userID]
	for i, c := range conns {
		if c == wc {
			m.conns[userID] = append(conns[:i], conns[i+1:]...)
			if len(m.conns[userID]) == 0 {
				delete(m.conns, userID)
			}
			return
		}
	}
}
```

Add `import "encoding/json"` to the imports.

- [ ] **Step 3: Create server/internal/ws/server.go**

```go
package ws

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/model"
)

// WSHandler handles WebSocket upgrades.
type WSHandler struct {
	manager *Manager
	db      *gorm.DB
	log     *zap.Logger
}

// NewWSHandler creates a WebSocket handler.
func NewWSHandler(manager *Manager, db *gorm.DB, log *zap.Logger) *WSHandler {
	return &WSHandler{manager: manager, db: db, log: log}
}

// RegisterWSRoutes registers the /ws endpoint on the root router (not /api/v1).
func RegisterWSRoutes(r *gin.Engine, manager *Manager, db *gorm.DB, log *zap.Logger) {
	handler := NewWSHandler(manager, db, log)
	r.GET("/ws", gin.WrapH(handler))
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Resolve token to user UUID (same logic as auth middleware)
	userID := uuid.NewSHA1(uuid.Nil, []byte(token))
	var user model.User
	h.db.FirstOrCreate(&user, model.User{ID: userID})

	lastSeqStr := r.URL.Query().Get("last_seq")
	var lastSeq int64
	if lastSeqStr != "" {
		lastSeq, _ = strconv.ParseInt(lastSeqStr, 10, 64)
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.log.Error("ws: accept failed", zap.Error(err))
		return
	}

	ctx := context.Background()

	// Send connected frame
	maxSeq, _ := h.manager.rdb.Get(ctx, "seq:"+user.ID.String()).Int64()
	connectedData := ServerFrame{
		Type: FrameConnected,
		Payload: ConnectedPayload{
			UserID:     user.ID.String(),
			ServerTime: time.Now().UTC().Format(time.RFC3339),
			MaxSeq:     maxSeq,
		},
	}
	data, _ := json.Marshal(connectedData)
	conn.Write(ctx, websocket.MessageText, data)

	h.manager.AddConnection(ctx, user.ID, conn)
}
```

Add `import "encoding/json"` to the imports.

- [ ] **Step 4: Wire WebSocket in module.go**

Add to `server/internal/di/module.go` — in `Provide` section:
```go
	fx.Provide(ws.NewManager),
```

Add to `RegisterRoutes`:
```go
	wsManager := ws.NewManager(rdb, log)
	ws.RegisterWSRoutes(r, wsManager, db, log)
```

- [ ] **Step 5: Commit**

```bash
git add server/internal/ws/
git commit -m "feat: add WebSocket connection manager, protocol, and /ws endpoint"
```

---

### Task 11: Eino model factory + Basic Agent + Tool registry

**Files:**
- Create: `server/internal/eino/model.go`
- Create: `server/internal/eino/tools/registry.go`
- Create: `server/internal/eino/tools/weather.go`
- Create: `server/internal/eino/agents/basic.go`

- [ ] **Step 1: Create server/internal/eino/model.go**

```go
package eino

import (
	"context"

	"github.com/PineappleBond/eino-demo/server/internal/config"
)

// ModelProvider creates LLM chat model instances.
type ModelProvider struct {
	cfg *config.Config
}

// NewModelProvider creates a model provider from config.
func NewModelProvider(cfg *config.Config) *ModelProvider {
	return &ModelProvider{cfg: cfg}
}

// GetModel returns the LLM model instance for the given tier.
// In Phase 1, we return model config only — actual Eino model creation
// happens in the chat service. This keeps eino-ext as an optional dependency
// that's initialized only when needed.
func (p *ModelProvider) GetModel(tier string) config.ModelConfig {
	if mc, ok := p.cfg.Models[tier]; ok {
		return mc
	}
	return p.cfg.Models["sonnet"] // fallback
}
```

- [ ] **Step 2: Create server/internal/eino/tools/registry.go**

```go
package tools

import (
	"github.com/PineappleBond/eino-demo/server/internal/config"
)

// ToolRegistry holds all registered Eino tools.
type ToolRegistry struct {
	cfg *config.Config
}

// NewToolRegistry creates a tool registry.
func NewToolRegistry(cfg *config.Config) *ToolRegistry {
	return &ToolRegistry{cfg: cfg}
}

// RegisterWeatherTool creates and registers the weather InferTool.
func (r *ToolRegistry) RegisterWeatherTool() *WeatherTool {
	return NewWeatherTool()
}
```

- [ ] **Step 3: Create server/internal/eino/tools/weather.go**

```go
package tools

// WeatherInput is the input schema for the weather tool.
type WeatherInput struct {
	Location string `json:"location" jsonschema_description:"City or location name"`
}

// WeatherOutput is the output schema for the weather tool.
type WeatherOutput struct {
	Location    string `json:"location"`
	Temperature int    `json:"temperature"`
	Condition   string `json:"condition"`
}

// WeatherTool is a mock weather tool for demo purposes.
type WeatherTool struct{}

// NewWeatherTool creates a weather tool.
func NewWeatherTool() *WeatherTool {
	return &WeatherTool{}
}

// Run returns mock weather data for the given location.
func (t *WeatherTool) Run(input WeatherInput) WeatherOutput {
	return WeatherOutput{
		Location:    input.Location,
		Temperature: 22,
		Condition:   "Sunny",
	}
}
```

- [ ] **Step 4: Create server/internal/eino/agents/basic.go**

```go
package agents

import (
	"context"

	"github.com/PineappleBond/eino-demo/server/internal/config"
	"github.com/PineappleBond/eino-demo/server/internal/eino/tools"
)

// BasicAgentBuilder builds a Basic Agent for Template 01.
type BasicAgentBuilder struct {
	cfg    *config.Config
	tools  *tools.ToolRegistry
}

// NewBasicAgentBuilder creates a builder for the basic agent.
func NewBasicAgentBuilder(cfg *config.Config, toolRegistry *tools.ToolRegistry) *BasicAgentBuilder {
	return &BasicAgentBuilder{cfg: cfg, tools: toolRegistry}
}

// Build returns the agent configuration for a given model tier.
func (b *BasicAgentBuilder) Build(modelTier string) AgentConfig {
	mc := b.cfg.Models[modelTier]
	return AgentConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
		SystemPrompt: "You are a helpful assistant. Use tools when appropriate to answer questions.",
		ToolNames: []string{"weather"},
	}
}

// AgentConfig holds the runtime config for an agent.
type AgentConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	SystemPrompt string
	ToolNames    []string
}
```

- [ ] **Step 5: Commit**

```bash
git add server/internal/eino/
git commit -m "feat: add Eino model provider, tool registry, weather tool, and basic agent builder"
```

---

### Task 12: Chat service + handler (send message + stop)

**Files:**
- Create: `server/internal/service/chat.go`
- Modify: `server/internal/handler/conversation.go` (add send_message + stop routes)

- [ ] **Step 1: Create server/internal/service/chat.go**

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/eino"
	"github.com/PineappleBond/eino-demo/server/internal/model"
	"github.com/PineappleBond/eino-demo/server/internal/ws"
)

// ChatService handles message sending and AI response generation.
type ChatService struct {
	db       *gorm.DB
	log      *zap.Logger
	wsMgr    *ws.Manager
	models   *eino.ModelProvider
}

// NewChatService creates a ChatService.
func NewChatService(db *gorm.DB, log *zap.Logger, wsMgr *ws.Manager, models *eino.ModelProvider) *ChatService {
	return &ChatService{db: db, log: log, wsMgr: wsMgr, models: models}
}

// SendMessageRequest holds the input for sending a message.
type SendMessageRequest struct {
	Content    string  `json:"content"`
	ReplyToSeq *int64  `json:"reply_to_seq"`
}

// SendMessage persists a user message and streams an AI response via WebSocket.
// Returns the message.new Update for the HTTP response.
func (s *ChatService) SendMessage(ctx context.Context, userID, conversationID uuid.UUID, req SendMessageRequest) (*ws.Update, error) {
	// Verify conversation exists and is active
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("conversation not found")
	}
	if conv.Status != "active" {
		return nil, fmt.Errorf("conversation is not active")
	}

	// Get user message seq for this conversation
	seq := conv.LatestMessageSeq + 1

	// Create user message
	msg := model.Message{
		ConversationID: conversationID,
		Seq:            seq,
		SenderRole:     "user",
		SenderID:       userID.String(),
		Content:        req.Content,
		ReplyToSeq:     req.ReplyToSeq,
	}
	if err := s.db.WithContext(ctx).Create(&msg).Error; err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	// Update conversation
	conv.LatestMessageSeq = seq
	conv.MessageCount++
	conv.LastMessagePreview = req.Content
	conv.UpdatedAt = time.Now()
	s.db.WithContext(ctx).Save(&conv)

	// Get user settings for model tier
	var settings model.Settings
	s.db.WithContext(ctx).Where("user_id = ?", userID).FirstOrCreate(&settings, model.Settings{
		UserID:    userID,
		ModelTier: "sonnet",
	})

	// Assign global seq for the Update event
	updateSeq, err := s.wsMgr.NextSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to assign seq: %w", err)
	}

	// Create Update
	update := ws.Update{
		Seq:  updateSeq,
		Type: "message.new",
		Payload: map[string]any{
			"conversation_id": conversationID.String(),
			"message": map[string]any{
				"id":              msg.ID,
				"conversation_id": msg.ConversationID.String(),
				"seq":             msg.Seq,
				"sender_role":     msg.SenderRole,
				"sender_id":       msg.SenderID,
				"content":         msg.Content,
				"reason_content":  msg.ReasonContent,
				"metadata":        msg.Metadata,
				"token_prompt":    msg.TokenPrompt,
				"token_completion": msg.TokenCompletion,
				"created_at":      msg.CreatedAt,
			},
		},
	}

	// Push to WebSocket (if client is connected, they'll receive the same update)
	s.wsMgr.PushToUserConnections(userID, update)

	// TODO: Stream AI response via WebSocket
	// This requires:
	// 1. Building the agent from Eino (adk.NewChatModelAgent or similar)
	// 2. Calling Stream() with the conversation history
	// 3. For each token: send message.delta (seq=0) via WS
	// 4. On completion: save message, create message.done Update, push via WS
	//
	// For Phase 1 MVP, this is the critical path. See eino-dev skill for implementation details.

	return &update, nil
}

// StopStreaming interrupts an active AI response.
func (s *ChatService) StopStreaming(ctx context.Context, userID, conversationID uuid.UUID) (*StopStreamingResponse, error) {
	// TODO: Implement cancellation of active agent stream.
	// Requires storing a cancel context per conversation in the chat service.
	// For now, return a not-implemented error.
	s.log.Info("stop: not yet implemented",
		zap.String("user_id", userID.String()),
		zap.String("conversation_id", conversationID.String()),
	)

	return &StopStreamingResponse{
		ConversationID: conversationID,
		StoppedAt:      time.Now(),
	}, nil
}

// StopStreamingResponse is the response for the stop endpoint.
type StopStreamingResponse struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	StoppedAt      time.Time `json:"stopped_at"`
}
```

- [ ] **Step 2: Add send_message and stop routes to conversation handler**

Add to `RegisterConversationRoutes` in `server/internal/handler/conversation.go`:

```go
	// Send message
	api.POST("/conversations/:id/messages", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		var req service.SendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid request body")
			return
		}

		update, err := svc.Chat.SendMessage(c.Request.Context(), userID, conversationID, req)
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		respondJSON(c, 200, []any{update})
	})

	// Stop streaming
	api.POST("/conversations/:id/stop", func(c *gin.Context) {
		userID := getUserID(c)
		conversationID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", "invalid conversation ID")
			return
		}

		resp, err := svc.Chat.StopStreaming(c.Request.Context(), userID, conversationID)
		if err != nil {
			respondError(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		respondJSON(c, 200, gin.H{
			"conversation_id": resp.ConversationID,
			"stopped_at":      resp.StoppedAt,
		})
	})
```

Note: This requires `Chat *service.ChatService` to be added as a field to `ConversationService`, or better, pass `ChatService` separately. Let's use a separate handler:

Actually, the cleanest approach: create a `RegisterChatRoutes` function that takes the ChatService. But the spec says send_message and stop are under `/conversations/:id/`. Let me add ChatService as a parameter:

```go
// RegisterConversationRoutes registers conversation endpoints.
func RegisterConversationRoutes(
	api *gin.RouterGroup,
	convSvc *service.ConversationService,
	chatSvc *service.ChatService,
) {
```

Then update all existing route handlers to use `convSvc` instead of `svc`, and new chat routes use `chatSvc`.

- [ ] **Step 3: Commit**

```bash
git add server/internal/service/chat.go server/internal/handler/conversation.go
git commit -m "feat: add send_message and stop endpoints with user message persistence"
```

---

### Task 12b: GET /api/v1/models endpoint

**Files:**
- Create: `server/internal/handler/models.go`

- [ ] **Step 1: Create server/internal/handler/models.go**

```go
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo/server/internal/config"
)

// RegisterModelRoutes registers GET /models.
func RegisterModelRoutes(api *gin.RouterGroup, cfg *config.Config) {
	api.GET("/models", func(c *gin.Context) {
		result := []gin.H{}
		for tier, mc := range cfg.Models {
			result = append(result, gin.H{
				"tier":  tier,
				"model": mc.Model,
			})
		}
		respondJSON(c, 200, result)
	})
}
```

- [ ] **Step 2: Wire in module.go**

Add to `RegisterAllRoutes`:
```go
handler.RegisterModelRoutes(api, cfg)
```

- [ ] **Step 3: Commit**

```bash
git add server/internal/handler/models.go
git commit -m "feat: add GET /models endpoint listing available model tiers"
```

---

### Task 13: Wire everything in module.go + updates polling endpoint

**Files:**
- Modify: `server/internal/di/module.go`
- Modify: `server/internal/handler/http.go`

- [ ] **Step 1: Create updates handler in handler/conversation.go (or new file)**

Add to `server/internal/handler/user.go`:

```go
// RegisterUpdateRoutes registers GET /users/me/updates.
func RegisterUpdateRoutes(api *gin.RouterGroup, db *gorm.DB) {
	api.GET("/users/me/updates", func(c *gin.Context) {
		userID := getUserID(c)
		lastSeqStr := c.Query("last_seq")
		if lastSeqStr == "" {
			respondError(c, 400, "INVALID_REQUEST", "last_seq is required")
			return
		}

		lastSeq, err := strconv.ParseInt(lastSeqStr, 10, 64)
		if err != nil || lastSeq < 0 {
			respondError(c, 400, "INVALID_REQUEST", "invalid last_seq")
			return
		}

		var updates []model.UserUpdate
		q := db.Where("user_id = ? AND seq > ?", userID, lastSeq).Order("seq ASC")
		if err := q.Find(&updates).Error; err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch updates")
			return
		}

		result := make([]gin.H, 0, len(updates))
		for _, u := range updates {
			result = append(result, gin.H{
				"seq":     u.Seq,
				"type":    u.Type,
				"payload": u.Payload,
			})
		}

		// Gap filling: check for missing seq numbers
		if len(result) > 0 {
			expectedStart := lastSeq + 1
			actualStart := result[0]["seq"].(int64)
			filled := make([]gin.H, 0)
			for seq := expectedStart; seq < actualStart; seq++ {
				filled = append(filled, gin.H{"seq": seq, "type": "empty", "payload": map[string]any{}})
			}
			result = append(filled, result...)
		}

		respondJSON(c, 200, result)
	})
}
```

Add `"github.com/PineappleBond/eino-demo/server/internal/model"` and `"github.com/PineappleBond/eino-demo/server/internal/config"` to imports.

- [ ] **Step 2: Update module.go with all service providers and route registrations**

Replace the full `module.go` content:

```go
package di

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo/server/internal/config"
	"github.com/PineappleBond/eino-demo/server/internal/db"
	"github.com/PineappleBond/eino-demo/server/internal/eino"
	einotools "github.com/PineappleBond/eino-demo/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo/server/internal/handler"
	"github.com/PineappleBond/eino-demo/server/internal/service"
	"github.com/PineappleBond/eino-demo/server/internal/ws"
)

// Module wires all dependencies.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		ProvideLogger,
		db.ProvideDB,
		ProvideRedis,
		ws.NewManager,
		eino.NewModelProvider,
		einotools.NewToolRegistry,
		service.NewUserService,
		service.NewSettingsService,
		service.NewTemplateService,
		service.NewProjectService,
		service.NewConversationService,
		service.NewChatService,
	),
	fx.Invoke(RegisterAllRoutes),
)

// ProvideRedis creates a Redis client.
func ProvideRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
}

// RegisterAllRoutes sets up all routes and starts the server.
func RegisterAllRoutes(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	rdb *redis.Client,
	wsMgr *ws.Manager,
	userSvc *service.UserService,
	settingsSvc *service.SettingsService,
	templateSvc *service.TemplateService,
	projectSvc *service.ProjectService,
	convSvc *service.ConversationService,
	chatSvc *service.ChatService,
	models *eino.ModelProvider,
	toolRegistry *einotools.ToolRegistry,
) {
	r := handler.NewRouter(cfg, log, db)
	api := r.Group("/api/v1")

	// Auth middleware is already applied in NewRouter via api.Use(authMiddleware(db))
	// We need to re-get the auth group. Let's refactor:
	// Actually, NewRouter returns the full engine. We need access to the auth group.
	// Let's change NewRouter to accept services.

	// Register all routes
	handler.RegisterUserRoutes(api, userSvc)
	handler.RegisterSettingsRoutes(api, settingsSvc)
	handler.RegisterTemplateRoutes(api, templateSvc)
	handler.RegisterProjectRoutes(api, projectSvc)
	handler.RegisterConversationRoutes(api, convSvc, chatSvc)
	handler.RegisterUpdateRoutes(api, db)

	// Register WebSocket on root
	ws.RegisterWSRoutes(r, wsMgr, db, log)

	// Register tools
	toolRegistry.RegisterWeatherTool()

	_ = models // used by chat service

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr := ":" + cfg.ServerPort
			log.Info("server starting", zap.String("addr", addr))
			go func() {
				if err := r.Run(addr); err != nil {
					log.Error("server error", zap.Error(err))
				}
			}()
			return nil
		},
	})
}
```

- [ ] **Step 3: Refactor handler/http.go** — remove route registration from NewRouter, keep only middleware setup:

The issue is that `NewRouter` currently creates services internally. We need to refactor it to return the engine AND the auth group so the DI module can register routes.

Replace `NewRouter` in `handler/http.go`:

```go
// RouterSetup holds the configured Gin engine and auth-protected group.
type RouterSetup struct {
	Engine *gin.Engine
	API    *gin.RouterGroup
}

// NewRouter creates the Gin engine with middleware.
func NewRouter(cfg *config.Config, log *zap.Logger, db *gorm.DB) *RouterSetup {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(log))

	api := r.Group("/api/v1")
	api.Use(authMiddleware(db))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return &RouterSetup{Engine: r, API: api}
}
```

Then update `module.go` to use the setup:

```go
	setup := handler.NewRouter(cfg, log, db)
	r := setup.Engine
	api := setup.API

	// Register all routes on api group
	handler.RegisterUserRoutes(api, userSvc)
	// ... rest
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/di/module.go server/internal/handler/http.go server/internal/handler/user.go
git commit -m "feat: wire all services in Fx module and add updates polling endpoint"
```

---

### Task 14: Live endpoint testing

**Files:**
- Create: `server/test/verify.sh` (optional convenience script)

- [ ] **Step 1: Start Docker containers**

```bash
docker compose up -d
```

- [ ] **Step 2: Set environment and start server**

```bash
export $(grep -v '^#' .env | xargs)
cd server && go run ./cmd/server/main.go
```

Expected: server starts on port 8080, AutoMigrate runs, no errors.

- [ ] **Step 3: Test health endpoint**

```bash
curl -s http://localhost:8080/health | python3 -m json.tool
```

Expected: `{"status": "ok"}`

- [ ] **Step 4: Test auth — GET /users/me with token**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/users/me | python3 -m json.tool
```

Expected: 200 with user profile including settings (model_tier: sonnet, locale: en, theme: light).

- [ ] **Step 5: Test templates — GET /api/v1/templates**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/templates | python3 -m json.tool
```

Expected: Array with Template 01 (Basic Agent).

- [ ] **Step 6: Test template detail — GET /api/v1/templates/01**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/templates/01 | python3 -m json.tool
```

Expected: TemplateDetail with agents, tools, initial_prompt.

- [ ] **Step 7: Test create project from template — POST /api/v1/templates/01/projects**

```bash
curl -s -X POST -H "Authorization: Bearer test-user-1" -H "Content-Type: application/json" -d '{"name": "My First Project"}' http://localhost:8080/api/v1/templates/01/projects | python3 -m json.tool
```

Expected: 201 with project object including id, user_id, template_id: "01".

Save the project_id for next tests.

- [ ] **Step 8: Test list projects — GET /api/v1/projects**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/projects | python3 -m json.tool
```

Expected: ProjectListResponse with at least one project.

- [ ] **Step 9: Test project detail — GET /api/v1/projects/:id**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/projects/<PROJECT_ID> | python3 -m json.tool
```

Expected: ProjectDetail with agents array.

- [ ] **Step 10: Test create conversation — POST /api/v1/projects/:id/conversations**

```bash
curl -s -X POST -H "Authorization: Bearer test-user-1" -H "Content-Type: application/json" -d '{"title": "Test Chat"}' http://localhost:8080/api/v1/projects/<PROJECT_ID>/conversations | python3 -m json.tool
```

Expected: 201 with conversation object.

Save the conversation_id.

- [ ] **Step 11: Test send message — POST /api/v1/conversations/:id/messages**

```bash
curl -s -X POST -H "Authorization: Bearer test-user-1" -H "Content-Type: application/json" -d '{"content": "Hello, world!"}' http://localhost:8080/api/v1/conversations/<CONVERSATION_ID>/messages | python3 -m json.tool
```

Expected: 200 with array of Update objects (at least one message.new).

- [ ] **Step 12: Test get messages — GET /api/v1/conversations/:id/messages**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/conversations/<CONVERSATION_ID>/messages | python3 -m json.tool
```

Expected: MessageListResponse with at least one user message.

- [ ] **Step 13: Test settings — GET /api/v1/settings**

```bash
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/settings | python3 -m json.tool
```

Expected: Settings object with model_tier, locale, theme.

- [ ] **Step 14: Test update settings — PUT /api/v1/settings**

```bash
curl -s -X PUT -H "Authorization: Bearer test-user-1" -H "Content-Type: application/json" -d '{"theme": "dark", "model_tier": "opus"}' http://localhost:8080/api/v1/settings | python3 -m json.tool
```

Expected: Updated settings.

- [ ] **Step 15: Test error paths**

```bash
# No auth token — should get 401
curl -s http://localhost:8080/api/v1/users/me | python3 -m json.tool

# Empty token — should get 401
curl -s -H "Authorization: Bearer " http://localhost:8080/api/v1/users/me | python3 -m json.tool

# Non-existent project — should get 404
curl -s -H "Authorization: Bearer test-user-1" http://localhost:8080/api/v1/projects/00000000-0000-0000-0000-000000000000 | python3 -m json.tool

# Bad request — send message with empty content
curl -s -X POST -H "Authorization: Bearer test-user-1" -H "Content-Type: application/json" -d '{"content": ""}' http://localhost:8080/api/v1/conversations/<CONVERSATION_ID>/messages | python3 -m json.tool
```

- [ ] **Step 16: Test WebSocket connection**

```bash
# Use websocat or similar tool
# brew install websocat
websocat "ws://localhost:8080/ws?token=test-user-1"
```

Expected: Receive `{"type":"connected","payload":{"user_id":"...","server_time":"...","max_seq":...}}`

Then send `{"type":"ping","payload":{}}` — connection should stay alive.

- [ ] **Step 17: Commit**

```bash
git add server/test/verify.sh 2>/dev/null; git commit -m "test: verify all Phase 1 endpoints with live HTTP and WebSocket tests"
```

---

### Task 15: Create UserUpdate on business operations + seq queue integration

**Files:**
- Modify: `server/internal/service/conversation.go` (add UserUpdate creation)
- Modify: `server/internal/service/settings.go` (add UserUpdate creation)

- [ ] **Step 1: Update conversation creation to create UserUpdate**

In `CreateConversation` in `server/internal/service/conversation.go`, after creating the conversation, assign a seq and create a UserUpdate:

```go
// After creating conv and members, inside the transaction:
updateSeq, _ := s.rdb.Incr(ctx, "seq:"+userID.String()).Result()
userUpdate := model.UserUpdate{
	UserID: userID,
	Seq:    updateSeq,
	Type:   "conversation.created",
	Payload: map[string]any{
		"conversation": map[string]any{
			"id":         conv.ID.String(),
			"project_id":  conv.ProjectID.String(),
			"user_id":     conv.UserID.String(),
			"title":       conv.Title,
			"status":      conv.Status,
			"created_at":  conv.CreatedAt,
			"updated_at":  conv.UpdatedAt,
		},
	},
}
tx.Create(&userUpdate)
```

This requires adding `rdb *redis.Client` to ConversationService constructor and injecting it.

- [ ] **Step 2: Update settings change to create UserUpdate**

In `UpdateSettings` in `server/internal/service/settings.go`, after saving settings:

```go
updateSeq, _ := s.rdb.Incr(ctx, "seq:"+userID.String()).Result()
userUpdate := model.UserUpdate{
	UserID: userID,
	Seq:    updateSeq,
	Type:   "settings.changed",
	Payload: map[string]any{
		"settings": map[string]any{
			"model_tier": settings.ModelTier,
			"locale":     settings.Locale,
			"theme":      settings.Theme,
			"updated_at": settings.UpdatedAt,
		},
	},
}
s.db.WithContext(ctx).Create(&userUpdate)
```

Then push via WS manager.

- [ ] **Step 3: Commit**

```bash
git add server/internal/service/conversation.go server/internal/service/settings.go server/internal/service/project.go
git commit -m "feat: integrate UserUpdate seq queue with business operations for real-time sync"
```
