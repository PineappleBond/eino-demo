# Backend Logging Enhancement Design

## Problem

Many handler and service files have zero or minimal logging. When something goes wrong, developers must attach a debugger to trace issues. Production troubleshooting is nearly impossible.

## Scope

Three layers, all missing files:

| Layer | Status | Target |
|-------|--------|--------|
| Handler (HTTP entry) | 5 files with zero/minimal logs | Log entry params, errors, key outcomes |
| Service (business logic) | 4 files with zero/minimal logs | Log DB ops, state changes, failures |
| Eino Runner (AI execution) | 1 file with zero logs | Log entry/exit, config, failures |

**Already sufficient** (no changes): `chat.go`, `cron.go`, `compression.go`, `rootrunner_callbacks.go`, `ws/server.go`, `ws/manager.go`

## Logging Conventions

### Levels

| Level | When to use |
|-------|-------------|
| `Info` | Normal flow: entry/exit of important operations, state changes |
| `Debug` | Verbose detail: intermediate steps, large payloads (truncated) |
| `Warn` | Recoverable issues: fallback used, degraded mode, near-limit |
| `Error` | Failures: DB errors, assignment failures, panics recovered |

### Context Fields (standard across all files)

- `user_id`: `zap.String("user_id", userID.String())` — per-user isolation
- `conversation_id`: `zap.String("conv", conversationID.String())` — chat context
- `project_id`: `zap.String("project_id", projectID.String())` — project context
- Always use structured fields, never string interpolation in log messages

### Pattern

```go
// Entry: log what we're about to do
s.log.Info("creating conversation",
    zap.String("user_id", userID.String()),
    zap.String("project_id", projectID.String()),
)

// Error: log what failed and why
if err != nil {
    s.log.Error("create conversation failed",
        zap.String("user_id", userID.String()),
        zap.Error(err),
    )
    return nil, err
}

// Exit: log outcome for important mutations
s.log.Info("conversation created",
    zap.String("conv_id", conv.ID.String()),
    zap.String("user_id", userID.String()),
)
```

## File-by-File Plan

### 1. Handler Layer (5 files)

All handlers already have access to `log` via the DI module or closure. For handler files that don't receive `log` as a parameter, it must be threaded through the registration function.

#### `handler/conversation.go` (0 → ~12 logs)
- Each endpoint: log request params at Info on entry
- Each error path: log at Error with context
- Successful mutations: log at Info with result ID
- No logger currently injected — need to add `log *zap.Logger` to `RegisterConversationRoutes`

#### `handler/project.go` (0 → ~10 logs)
- Same pattern as conversation
- No logger currently injected — need to add parameter

#### `handler/template.go` (0 → ~6 logs)
- List/get/create: log entry and errors
- No logger currently injected — need to add parameter

#### `handler/settings.go` (0 → ~6 logs)
- Get/update: log entry and errors
- No logger currently injected — need to add parameter

#### `handler/user.go` (1 → ~5 logs)
- Already has logger for `/updates` endpoint
- Add to `GetMe` handler entry/error
- Add to `/updates` entry point

### 2. Service Layer (4 files)

#### `service/conversation.go` (1 → ~15 logs)
- `ListConversations`: log query params
- `CompleteCreateConversation`: log creation, project lookup, failure
- `CompleteDeleteConversation`: log deletion
- `CompleteRenameConversation`: log rename with old/new title
- `UpdateStatus`: log status change with old→new
- `CompactConversation`: already has partial logs, fill gaps

#### `service/project.go` (1 → ~12 logs)
- `ListProjects`: log count
- `GetProject`: log lookup failure
- `UpdateProject`: log field changes
- `CompleteDeleteProject`: log deletion with cascading effects

#### `service/template.go` (0 → ~6 logs)
- `ListTemplates`: log count (Debug)
- `GetTemplate`: log lookup failure
- `CompleteCreateProjectFromTemplate`: log template used, project created

#### `service/user.go` (0 → ~3 logs)
- `GetMe`: log lookup failure (Debug is fine for common path)

### 3. Eino Layer (1 file)

#### `eino/runner/rootrunner.go` (0 → ~8 logs)
- `NewRootRunner`: log model tier, tool count, agent count, enabled features
- `renderRootPrompt`: log failure at Error
- `NewRootRunner` error paths: log each step that can fail
- Config needs a `Log *zap.Logger` field

### 4. DI Wiring

#### `di/module.go`
- Pass `log` to all `Register*Routes` calls that currently don't receive it
- Pass `log` to `NewRootRunner` via config

## What NOT to Log

- Request/response bodies in full (too verbose, may contain sensitive data)
- Passwords, tokens, API keys (none exist in this codebase, but guard against future)
- Every single DB query (noise — only log on error or important mutations)
- Successful reads that are expected to be common (e.g., ListConversations success)

## Testing

- Start server, hit each endpoint, verify logs appear in stdout
- Verify error paths produce Error-level logs
- Verify log format is consistent (structured JSON via zap.NewProduction)
