# CLAUDE.md — Project Constraints

## Source Code References

| Package         | Import Path                     | Local Path             |
|-----------------|---------------------------------|------------------------|
| Eino core       | `github.com/cloudwego/eino`     | `downloads/eino/`      |
| Eino extensions | `github.com/cloudwego/eino-ext` | `downloads/eino-ext/`  |
| WebSocket       | `github.com/coder/websocket`    | `downloads/websocket/` |
| Pretext         | `@chenglou/pretext`             | `downloads/pretext/`   |

When working with Eino or eino-ext code, read source from `downloads/` directory instead of searching online.

## References

| Document                                                   | Purpose                                             |
|------------------------------------------------------------|-----------------------------------------------------|
| [REQUIREMENTS.md](REQUIREMENTS.md)                         | Feature requirements, user stories, demo scope      |
| [docs/API.md](docs/API.md)                                 | API rules, Update type, communication flow diagrams |
| [docs/DATABASE_POSTGRESQL.md](docs/DATABASE_POSTGRESQL.md) | Backend-Database schema, ER diagrams                |
| [docs/DATABASE_INDEXEDDB.md](docs/DATABASE_INDEXEDDB.md)   | Frontend-Database schema, ER diagrams               |

## Tech Stack

- **Backend**: Go 1.26+, Gin, `github.com/coder/websocket` (shared port)
- **AI**: Eino core + eino-ext
- **DI**: `go.uber.org/fx`
- **DB**: GORM + PostgreSQL + AutoMigrate
- **Cache**: Redis (per-user `seq` allocation via `INCR`, connection state, heartbeat)
- **Logging**: `go.uber.org/zap`
- **Config**: minimal `flag` + `os.Getenv`, dynamic updates via DB
- **Templates**: `embed.FS` + `text/template` (Jinja-style `{{.Name}}` syntax)
- **Frontend**: Next.js + React + TypeScript + Ant Design + `openapi-typescript` + `next-intl` + `@chenglou/pretext`

## Available Skills — Invoke Before Relevant Work

| Skill                            | Trigger                                                |
|----------------------------------|--------------------------------------------------------|
| `eino-dev`                       | **Always** invoke before writing any Eino-related code |
| `eino-ext-using`                 | Before integrating any eino-ext component              |
| `golang-pro`                     | Before building Go backend architecture                |
| `golang-patterns`                | Before writing idiomatic Go code                       |
| `golang-testing`                 | Before writing tests                                   |
| `frontend-design`                | Before building frontend UI                            |
| `react-performance-optimization` | Before optimizing React/Next.js                        |
| `api-design-principles`          | Before designing APIs or WebSocket protocols           |
| `requesting-code-review`         | After completing any task                              |
| `systematic-debugging`           | On any bug or unexpected behavior                      |
| `brainstorming`                  | Before any creative or architectural decision          |
| `verification-before-completion` | Before claiming a task is done                         |

## Rules

### General

- Must keep CLAUDE.md as constraints only. Requirements go in [REQUIREMENTS.md](REQUIREMENTS.md). API specs go
  in [docs/API.md](docs/API.md).
- Must invoke relevant skills before writing code. No exceptions.
- Must never reinvent what third-party libraries already solve. Search npm, GitHub, and ecosystem first. Custom code is
  the last resort.
- All demo templates are integrated into the server and displayed/executed via the web UI. No standalone template
  binaries.

### Architecture

- Must use `uber-go/fx` for dependency injection. All components wired via Fx modules.
- Must keep config minimal: `flag` for startup params, `os.Getenv` for secrets, runtime config in DB.
- Must use GORM AutoMigrate for schema management. No raw migration files unless absolutely necessary.
- Must use `go.uber.org/zap` for logging. Structured logs, no `fmt.Println`.
- Must use `embed.FS` for prompt templates and static text. Template syntax: `text/template` (`{{.Name}}`).
- Must change dir `cd server` to execute `go` command. `server/go.mod` is the backend module; root may hold
  workspace-level config.

### Authentication

- Must use demo-mode auth: any non-empty `Authorization: Bearer <token>` header is accepted. No real auth
  implementation.
- Must resolve the token to a user UUID via `FirstOrCreate` in auth middleware: if the token is new, create a user row and return the UUID. The UUID is used as `user_id` for per-user isolation.
- Must apply auth middleware to all Go backend `/api/` routes. WebSocket auth via query param (`?token=`).
- Must never store passwords or real credentials in code or DB.

### Error Response Convention

- Must use uniform HTTP error shape: `{ "error": { "code": "ERROR_CODE", "message": "human readable" } }`.
- Must use standard HTTP status codes: 400 (bad request), 401 (unauth), 404 (not found), 500 (internal).
- Must never leak stack traces or internal details in error responses.

### Service-First, Dual-Consumer Architecture

- Must treat `internal/service/` as the single source of truth for all business logic.
- Must expose every server capability as both an HTTP endpoint AND an Eino Tool.
- Must use `utils.InferTool` to wrap service methods as Tools. The service method signature becomes the Tool's JSON
  schema automatically.
- HTTP handlers must be thin: decode request → call service method → encode response. No business logic in handlers.
- Tools must delegate to the same service methods that HTTP handlers call. One implementation, two consumers.
- Must inject `user_id` (from auth context) into both handler and tool calls for per-user isolation.

#### Data Flow

```text
HTTP Client  ──→ handler (decode) ──→ service (logic) ──→ db/ws/eino
Agent/LLM    ──→ InferTool (auto-schema) ──→ service (logic) ──→ db/ws/eino
WebSocket    ──→ ws/server.go ──→ service (logic) ──→ db/ws/eino
```

### Eino

- Must use `components/` packages from eino-ext. Never use internal `libs/acl/` directly.
- Must prefer `adk.NewChatModelAgent` + `adk.NewRunner` for agent execution.
- Must handle both `Invoke` and `Stream` paradigms in every example.
- Must support three model tiers: `haiku`, `sonnet`, `opus`.

### Tool Design (Eino)

- Must define all server capabilities as Tools using `utils.InferTool`. Examples:
    - Business tools: `create_conversation`, `send_message`, `list_conversations`, `update_settings`, `query_messages`,
      `run_example`, `get_model_info`
    - Demo tools (educational only): `weather`, `web_search`, `http_request` — for teaching how to build custom tools
- Must structure tool input/output as typed Go structs. `InferTool` auto-generates JSON schema.
- Must give tools clear names and descriptions. These are shown to the LLM for tool selection.
- Must keep tool implementations short: validate input → call service → return result.
- Must return typed response structs from tools, not raw strings. Enables LLM understanding of structured data.

### Go Code

- Must follow `gofmt` and `go vet`. Zero warnings.
- Must write table-driven tests for all logic.
- Must use integration tests with real PostgreSQL and real LLM provider for message queue, RAG, and agent execution.
- Must write comments that explain **why**, not **what**.
- Must handle errors explicitly. No naked returns, no ignored errors.

### Testing Workflow

- Must use local Docker Compose (`deploy/dependencies/dev/docker-compose.yaml`) for all infrastructure dependencies (PostgreSQL, Redis, etc.). Never use mocks or external services.
- Must start dependencies via `docker compose -f deploy/dependencies/dev/docker-compose.yaml up -d` before testing.
- Must run the actual server and test live endpoints. Never rely solely on mocked unit tests.
- Must test in this order:
    1. Start the backend server (`go run ./server/cmd/server/main.go`)
    2. Use `curl` commands or bash scripts to hit each endpoint
    3. Examine the actual HTTP response (status code, body shape, field names)
    4. Judge pass/fail by comparing observed behavior against expected behavior from [docs/API.md](docs/API.md) (must be
       written before testing)
    5. Fix any failures, restart, re-test
- Must verify WebSocket by connecting and sending/receiving at least one frame.
- Must not hardcode expected response bodies in tests. The agent must read the response, understand its structure, and
  decide if it looks correct.
- Must test error paths too: send invalid payloads, missing auth, bad params — verify proper error responses.
- Must test the full flow: HTTP endpoint → service → DB → response. Not just the service layer in isolation.
- Must document the test commands used and their output in commit messages or PR descriptions.

### WebSocket & Messaging

- Must share port between Gin HTTP and WebSocket.
- Must implement per-user `seq` queue via Redis `INCR("seq:{user_id}")` for assignment, with PostgreSQL `user_updates` table for persistence and offline recovery.
- Must guarantee zero message loss for offline users.
- Must implement exponential backoff polling as fallback.
- Must batch server pushes as `[]Update` arrays.

### Frontend

- Must prefer third-party libraries over custom implementations. Search npm/ecosystem first. Never reinvent UI
  components or utilities that exist.
- Must use Ant Design for all UI components (forms, tables, modals, layout, buttons). Do not hand-roll.
- Must use `next-intl` for i18n. Support `en` and `zh`. Locale files in `web/src/locales/`.
- Must support light/dark theme via Ant Design `ConfigProvider` + `theme` token. Persist user choice in localStorage.
- Must use `@chenglou/pretext` for streaming text layout measurement and cursor positioning.
- Must use `react-markdown` + `remark-gfm` + `rehype-highlight` for AI response rendering.
- Must use `react-syntax-highlighter` or `shiki` for code block highlighting.
- Must display tool calls as collapsible cards: name, input args, result, status, duration.
- Must implement stop/interrupt button during active streaming.
- Must use React Context + `useReducer` for state management. No Redux/Zustand unless proven insufficient.
- Must handle WebSocket reconnection with exponential backoff. On reconnect, send `last_seq` to resume from last
  received message. Server replays missed updates or signals fallback to HTTP polling.
- Server only closes stale connections on heartbeat timeout (1 minute from last ping). New connections are added alongside existing ones; old connections are NOT actively kicked on reconnect.
- Must display streaming token updates in real-time with cursor animation.
- Must generate Go and TypeScript types by running `openapi/generate.sh`. One `spec.yaml`, two codebases, types always
  in sync. Must NEVER hand-write request/response types that exist in the spec.

### Type Generation

- Must generate Go (`server/internal/types/types.go`) and TypeScript (`web/src/types/api.d.ts`) types by running
  `openapi/generate.sh`. One `spec.yaml`, two codebases, types always in sync.
- Must NEVER hand-write request/response types that exist in the spec.
- **Must reference the generated types** for all API and WebSocket payloads. Backend reads `server/internal/types/types.go`,
  frontend reads `web/src/types/api.d.ts`. This is the only way to guarantee field-name consistency between the two codebases.
- `Update.payload` is typed as `map[string]interface{}` / `Record<string, never>` in the generated types.
  When writing backend code that constructs Update payloads or frontend code that reads them, cross-check the actual
  field names used in `server/internal/eino/runner/rootrunner_callbacks.go` against the frontend parsing logic.
  If a payload field needs a concrete schema, add it to `openapi/spec.yaml` and regenerate.

### Frontend Update Event Flow

- Must use a single entry point `applyUpdates(updates: Update[])` for all incoming updates, regardless of source (
  WebSocket push or HTTP response body).
- Must persist entities (messages, conversations, projects) from Update payloads to their respective IndexedDB tables via HTTP sync **before** notifying subscribers. Like a real IM app.
- Must NOT persist Update events themselves. Only `latest_seq` in the `settings` table tracks the global sync cursor. If missed during disconnect, `latest_seq` + HTTP pull recovers missing entities.
- Must derive **topics** on the frontend from route context. Backend has no topic concept. Examples:
  - `/[locale]/chat/[convId]` → topic `conv:{convId}`
  - `/[locale]/chat/` (conversation list) → topic `project:{projectId}` or `system` (global)
  - Global (home, settings) → topic `system`
- Must implement `useSubscribe(topic: string)` hook: mounts subscribe, unmounts unsubscribe. Only active subscribers
  receive notifications.
- Must implement `UpdateDispatcher` (singleton): routes updates by topic, manages seq continuity, notifies active
  subscribers.
- IndexedDB entity tables (populated via HTTP sync, not by `applyUpdates()`): `messages`, `conversations`, `projects`, `users`, `settings`.
- Flow:
  `source → applyUpdates → dispatch by topic (skip empty updates) → notify subscribers → component triggers HTTP sync → persist entity to IndexedDB → re-render`.

### Commits

- Must use Conventional Commits in English.
- Must use English for all code comments and commit messages.

## Project Structure

<!-- This is the target architecture, not the current state. -->

```text
.
│
├── .gitignore                 # Includes downloads/
│
├── go.work                     # Go workspace for root + server/
│
├── openapi/
│   └── spec.yaml               # OpenAPI spec (source of truth for both Go + TS types)
│
├── server/
│   ├── cmd/server/
│   │   └── main.go              # Entry point, flag parsing, fx.New().Run()
│   │
│   ├── internal/
│   │   ├── types/
│   │   │   └── types.go         # Generated by openapi/generate.sh. Never edit manually.
│   │   │
│   │   ├── config/
│   │   │   └── config.go        # Minimal config: flag + os.Getenv
│   │   │
│   │   ├── di/
│   │   │   ├── module.go        # Fx module wiring (DB, services, handlers)
│   │   │   └── logger.go        # Zap logger provider
│   │   │
│   │   ├── model/
│   │   │   ├── base.go          # GORM base model, common fields
│   │   │   ├── user.go          # User model
│   │   │   ├── project.go       # Project model
│   │   │   ├── conversation.go  # Conversation model
│   │   │   ├── conversation_member.go # Conversation membership
│   │   │   ├── agent.go         # Agent definition
│   │   │   ├── agent_relationship.go  # Agent delegation tree
│   │   │   ├── message.go       # Message + per-conversation seq
│   │   │   ├── user_update.go   # Global Update event log
│   │   │   ├── checkpoint.go    # Interrupt/resume checkpoint
│   │   │   └── settings.go      # Runtime settings (dynamic config)
│   │   │
│   │   ├── db/
│   │   │   ├── db.go            # GORM init, AutoMigrate
│   │   │   └── pgvector.go      # pgvector extension setup
│   │   │
│   │   ├── eino/
│   │   │   ├── model.go         # LLM model factory (haiku/sonnet/opus)
│   │   │   ├── model_router.go  # Smart model selection
│   │   │   │
│   │   │   ├── tools/
│   │   │   │   ├── registry.go  # Tool registration
│   │   │   │   ├── weather.go   # Example: weather tool
│   │   │   │   ├── search.go    # Example: web search
│   │   │   │   └── ...
│   │   │   │
│   │   │   ├── agents/
│   │   │   │   ├── registry.go  # Agent registration interface
│   │   │   │   ├── basic.go     # Basic ChatModelAgent
│   │   │   │   ├── deep.go      # DeepAgent
│   │   │   │   ├── supervisor.go# Supervisor
│   │   │   │   └── ...
│   │   │   │
│   │   │   ├── workflows/
│   │   │   │   ├── chain.go     # Chain examples
│   │   │   │   ├── graph.go     # Graph examples
│   │   │   │   └── workflow.go  # Workflow examples
│   │   │   │
│   │   │   └── prompts/
│   │   │       ├── embed.go     `//go:embed all:templates`
│   │   │       ├── loader.go    # Template loading from embed.FS
│   │   │       └── templates/   # Prompt templates (*.tmpl)
│   │   │
│   │   ├── templates/
│   │   │   ├── registry.go      # Template registration (id, name, desc, builder)
│   │   │   ├── 01_basic_agent.go
│   │   │   ├── 02_chain.go
│   │   │   ├── 03_graph.go
│   │   │   └── ...              # All template definitions
│   │   │
│   │   ├── service/
│   │   │   ├── conversation.go  # Conversation business logic
│   │   │   ├── chat.go          # Chat execution (invokes eino agents)
│   │   │   ├── template.go      # Template listing and project creation
│   │   │   └── settings.go      # Runtime settings management
│   │   │
│   │   ├── handler/
│   │   │   ├── http.go          # Gin router setup (shared port)
│   │   │   ├── user.go          # User endpoints
│   │   │   ├── conversation.go  # Conversation endpoints
│   │   │   ├── template.go      # Template list + project creation
│   │   │   └── settings.go      # Settings get/update
│   │   │
│   │   └── ws/
│   │       ├── server.go        # WebSocket upgrade handler
│   │       ├── manager.go       # Connection manager (per-user)
│   │       ├── queue.go         # Per-user seq queue
│   │       └── protocol.go      # Frame types, marshal/unmarshal
│   │
│   └── go.mod
│
├── web/
│   ├── src/
│   │   ├── app/                     # Next.js App Router
│   │   │   ├── [locale]/            # next-intl locale route
│   │   │   │   ├── page.tsx         # Home / template browser
│   │   │   │   ├── layout.tsx       # Providers: theme, i18n, auth
│   │   │   │   ├── chat/
│   │   │   │   │   ├── page.tsx       # Conversation list
│   │   │   │   │   └── [convId]/
│   │   │   │   │       └── page.tsx   # Chat interface
│   │   │   │   ├── settings/
│   │   │   │   │   └── page.tsx     # Dynamic config page
│   │   │   │   └── not-found.tsx
│   │   │   └── api/                 # Next.js API routes (if needed)
│   │   │
│   │   ├── components/              # Reusable UI (thin wrappers around Ant Design)
│   │   │   ├── chat/
│   │   │   │   ├── MessageList.tsx  # Scrollable message list
│   │   │   │   ├── MessageBubble.tsx # Single message (text + tool cards)
│   │   │   │   ├── ToolCallCard.tsx  # Collapsible tool call display
│   │   │   │   ├── StreamingText.tsx # pretext-based streaming
│   │   │   │   └── ChatInput.tsx     # Input + stop button
│   │   │   ├── template/
│   │   │   │   ├── TemplateCard.tsx  # Template preview card
│   │   │   │   └── TemplateList.tsx  # Browseable template grid
│   │   │   └── layout/
│   │   │       ├── Sidebar.tsx
│   │   │       ├── Header.tsx
│   │   │       └── ThemeSwitch.tsx
│   │   │
│   │   ├── hooks/
│   │   │   ├── useWebSocket.ts      # WS connection + reconnection
│   │   │   ├── useStream.ts         # Streaming text state
│   │   │   └── useTheme.ts          # Theme toggle
│   │   │
│   │   ├── lib/
│   │   │   ├── api.ts               # HTTP client (typed)
│   │   │   ├── utils.ts
│   │   │   └── updateDispatcher.ts  # applyUpdates(), topic routing, IndexedDB
│   │   │
│   │   ├── store/
│   │   │   ├── indexedDB.ts         # IndexedDB open/read/write helpers
│   │   │   └── topic.ts             # Topic derivation from route context
│   │   │
│   │   ├── providers/
│   │   │   ├── ThemeProvider.tsx     # Ant Design theme + dark/light
│   │   │   ├── WSProvider.tsx        # WebSocket connection, passes frames to applyUpdates
│   │   │   ├── UpdateProvider.tsx    # UpdateDispatcher context + flush on mount
│   │   │   └── ChatProvider.tsx      # Chat state + reducer
│   │   │
│   │   ├── locales/                  # next-intl messages
│   │   │   ├── en.json
│   │   │   └── zh.json
│   │   │
│   │   └── types/
│   │       └── api.d.ts             # Generated by openapi/generate.sh
│   │
│   ├── next.config.ts
│   ├── tsconfig.json
│   └── package.json
│
├── docs/
│   ├── API.md                   # API specification
│   └── DATABASE_POSTGRESQL.md              # Database schema
│
├── REQUIREMENTS.md              # Feature requirements
├── CLAUDE.md                    # This file — constraints
├── downloads/                   # Framework source (gitignored)
│
└── deploy/
│   └── dependencies/dev/        # Local dev environment (docker-compose, Grafana, Prometheus)
│       ├── docker-compose.yaml  # Services: PostgreSQL, Redis, Grafana, Prometheus
│       ├── .env                 # Dev environment variables
│       ├── .env.example         # Template for dev environment variables
│       ├── grafana/             # Grafana dashboards and provisioning
│       └── prometheus/          # Prometheus configuration
```

### Directory Rules

- `internal/types/types.go` — Generated by openapi/generate.sh. Never edit manually.
- `internal/eino/` — Eino-specific code only. Agents, tools, workflows, prompts.
- `internal/service/` — Business logic. Calls eino components, DB, WS. No HTTP/WebSocket logic.
- `internal/handler/` — HTTP handlers only. Thin, delegates to services.
- `internal/ws/` — WebSocket protocol and connection management only.
- `internal/templates/` — Template definitions registered as data, not standalone programs.
- `web/src/lib/updateDispatcher.ts` — Singleton. `applyUpdates()` entry point, topic routing, IndexedDB persistence.
- `web/src/store/` — IndexedDB helpers and topic derivation from route context.
- `web/src/providers/UpdateProvider.tsx` — Exposes UpdateDispatcher via context.
- `web/src/providers/WSProvider.tsx` — Manages WebSocket lifecycle, passes received frames to `applyUpdates`.
- `web/src/components/` — Thin wrappers around Ant Design. Never hand-roll buttons, modals, forms.
- `web/src/providers/` — React Context providers (theme, updates, chat).
- `web/src/locales/` — next-intl i18n messages (`en.json`, `zh.json`).
- `web/src/types/api.d.ts` — Generated by openapi/generate.sh. Never edit manually.
- `openapi/spec.yaml` — Single source of truth for API types. Run `openapi/generate.sh` to regenerate both Go and
  TypeScript.
