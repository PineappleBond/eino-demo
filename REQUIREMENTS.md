# REQUIREMENTS.md

## Goal

An interactive educational demo for the [Eino](https://github.com/cloudwego/eino) framework, targeting both Go developers (how to integrate Eino) and AI developers (multi-agent orchestration patterns). Users learn by interacting with pre-built Project templates directly in the web UI.

**Target Users**:

- Go developers wanting to learn Eino integration patterns
- AI developers wanting to learn multi-agent orchestration

**Success Criteria**:

- A user can complete their first AI conversation within 5 minutes of opening the app
- Each Project template is independently runnable and its source code is readable/educational
- 12 demo templates cover Eino's full capability range

## Core Concepts

| Concept | Description |
| --- | --- |
| **Project** | 1 scenario + multiple Agents + multiple Tools + multiple Conversations. The basic unit of user interaction. |
| **Conversation** | A single conversation session within a Project. When context exceeds the threshold, history is compressed via LLM Summary and a new Conversation is created carrying the summary forward. |
| **Template** | A pre-built Project blueprint containing Agent config, Tool list, and initial prompt. Users create Projects from templates. |
| **Update** | Unified event format for both WebSocket push and HTTP responses. `seq > 0` = persisted, `seq = 0` = ephemeral (streaming tokens, thinking). |

## Functional Requirements

### 1. User Management

- Users can create an account (demo mode: token value IS the user_id)
- Users can view their own profile info

### 2. Project Management

- Users can browse a list of pre-built Project templates
- Users can create a Project from a template
- Users can view Project details: Agent configs, available Tools, description
- Users can delete their own Projects

### 3. Conversation Management

- Users can start a new Conversation within a Project
- Users can view the list of Conversations within a Project
- **Context compression**: When token count exceeds the threshold, the system invokes an LLM to summarize history, closes the current Conversation, and creates a new one carrying the summary
- Users can delete their own Conversations

### 4. Chat

- Users can send text messages
- Users receive AI responses in real-time via WebSocket (streaming + complete message)
- Tool calls are displayed as interactive cards showing: name, input args, result, status, duration
- Users can stop/interrupt an active streaming response
- AI responses are rendered with Markdown + syntax-highlighted code blocks
- Streaming text uses cursor animation and pretext-based layout measurement

### 5. Model Selection

- Three model tiers available: `haiku` (lightweight), `sonnet` (general), `opus` (complex reasoning)
- Users can switch the active model tier
- Settings page explains each tier's use case

### 6. Settings

- Model tier selection (haiku / sonnet / opus)
- Language toggle (en / zh)
- Theme toggle (light / dark)

### 7. Message Queue

- Per-user `seq` queue with PostgreSQL persistence
- Zero message loss for offline users
- WebSocket reconnection with `last_seq` resume
- HTTP polling fallback with exponential backoff

### 8. Demo Project Templates

| # | Template | Demonstrates |
| --- | --- | --- |
| 01 | Basic Agent | ChatModelAgent, Tools, ReAct loop |
| 02 | Chain Workflow | Prompt → Model → Parse composition |
| 03 | Graph Branches | Conditional routing, parallel nodes |
| 04 | Workflow Composition | Field mappings, declarative dependencies |
| 05 | RAG Pipeline | Loader → Embedding → Indexer → Retriever → QA |
| 06 | Tool Workshop | MCP, Web Search, HTTP, custom InferTool |
| 07 | Deep Agent | Sub-agent delegation |
| 08 | Plan & Execute | Plan-then-execute pattern |
| 09 | Supervisor | Multi-agent supervision |
| 10 | Interrupt & Resume | Human-in-the-loop with checkpoint |
| 11 | Callbacks & Tracing | Langfuse / LangSmith integration |
| 12 | Full App | All features integrated |

## Phase Delivery

### Phase 1 — MVP: Core Chat + Basic Agent

**Must have**:

- Project skeleton (Gin + WS shared port + Fx DI + GORM AutoMigrate)
- User management (demo auth: token = user_id)
- Project template system (create from template)
- Project list and detail pages
- Conversation list within a Project
- Basic chat: send message → Agent (ChatModelAgent + Tools) → streaming reply
- WS push + seq queue (Redis INCR)
- Frontend chat UI (Ant Design): streaming render, stop button, Tool call cards
- IndexedDB persistence + UpdateDispatcher singleton + `applyUpdates()` pipeline
- Template 01 (Basic Agent) fully interactive
- Model tier switching (haiku / sonnet / opus)
- Settings page (model, language, theme)

### Phase 2 — Multi-Agent + Frontend Polish

**Must have**:

- Templates 02-04 (Chain / Graph / Workflow)
- Templates 07-09 (DeepAgent / PlanExecute / Supervisor)
- Template 10 (Interrupt & Resume)
- Template 11 (Callbacks & Tracing)
- Context compression (LLM Summary)
- Frontend polish: i18n (en/zh via next-intl), dark mode, pretext streaming animation
- Conversation archived UI (read-only view, compacting/loading states)

### Phase 3 — RAG + Complete Templates

**Must have**:

- Template 05 (RAG Pipeline) with pgvector
- Template 06 (Tool Workshop)
- Template 12 (Full App integration)
- Callbacks visualization panel
- Full test coverage (live endpoint tests + unit + integration)

## Data Model (Summary)

See [docs/DATABASE.md](docs/DATABASE.md) for full schema and ER diagrams.

| Table | Key Fields |
| --- | --- |
| **users** | `id`, `name`, `created_at` |
| **projects** | `id`, `user_id`, `template_id`, `name`, `config (JSONB)`, `created_at`, `updated_at` |
| **conversations** | `id`, `project_id`, `user_id`, `title`, `summary (text)`, `created_at`, `updated_at` |
| **messages** | `id`, `conversation_id`, `seq`, `role`, `content`, `metadata (JSONB)`, `created_at` |
| **checkpoints** | `id`, `conversation_id`, `agent_state`, `created_at` |
| **settings** | `id`, `user_id`, `model_tier`, `locale`, `theme`, `updated_at` |

## Non-Functional Requirements

- Each template's source code is readable as educational material
- Phase 1: single model provider (OpenAI-compatible) sufficient; multi-provider wiring in Phase 3
- Responsive UI: desktop + tablet
- All errors return `{ "error": { "code": "ERROR_CODE", "message": "human readable" } }` shape
- No hardcoded credentials; all secrets via environment variables
- See [CLAUDE.md](CLAUDE.md) for development constraints and coding standards
