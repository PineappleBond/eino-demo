# Conversation Mode Design

## Summary

Add a `mode` field to the Conversation model to control how tool calls are permission-checked during agent execution. Four modes: `ask_before_edits`, `edit_automatically`, `bypass_permissions`, `plan_mode` (reserved).

## Context

The existing permission system uses a threshold + whitelist + AI safety evaluation approach. All conversations share this same behavior. This design adds conversation-level mode control so users can choose how strictly the system checks each tool call.

## Architecture

### Data Model Changes

**`server/internal/model/conversation.go`**
- Add `Mode string` field to `Conversation` struct with GORM tag:
  `gorm:"type:varchar(30);not null;default:'ask_before_edits'"`
- GORM AutoMigrate handles the schema change.

**`openapi/spec.yaml`**
- Add `mode` to `Conversation` schema with enum: `["ask_before_edits", "edit_automatically", "bypass_permissions", "plan_mode"]`
- Add optional `mode` to POST `/projects/{id}/conversations` requestBody
- PATCH `/conversations/{id}` requestBody accepts both `title` and `mode` (either or both)
- Run `openapi/generate.sh` to regenerate Go + TypeScript types

### Backend — Permission Middleware

**`server/internal/eino/permission/types.go`**
- Define `ConversationMode` type and four constants

**`server/internal/eino/permission/middleware.go`**
- `MiddlewareConfig` gains `Mode ConversationMode` field
- `checkPermission` adds a mode switch at the top:
  - `ask_before_edits`: Skip whitelist + safety eval, always interrupt for human approval
  - `edit_automatically`: Keep existing logic (whitelist → threshold → interrupt)
  - `bypass_permissions`: Return true immediately (no permission checking)
  - `plan_mode`: Reserved — treat as `bypass_permissions` for now (TODO)

**`server/internal/service/chat.go`**
- When creating `PermissionMW`, read `conversation.Mode` from the conversation record and pass to `MiddlewareConfig.Mode`

### Backend — Handler + Service

**`server/internal/service/conversation.go`**
- `CreateConversationRequest` gains optional `Mode` field (defaults to `ask_before_edits`)
- Add `UpdateModeRequest` struct with `Mode` field
- Add `CompleteUpdateMode` method (similar pattern to rename: verify ownership → update + user_update in transaction → push WS)

**`server/internal/handler/conversation.go`**
- PATCH `/conversations/:id` handler updated to also handle `mode` field alongside `title`

### Frontend

**`ConvInfoPanel.tsx`**
- Add mode display section below the model section
- Ant Design `Select` component with four options (Chinese labels)
- `plan_mode` shows as disabled with "开发中" badge

**`chat/[convId]/page.tsx`**
- Load conversation mode on initial fetch
- Add state for mode, update handler calls PATCH API
- Listen for `conversation.updated` updates that carry mode changes

### Event Flow

```
User selects mode → PATCH /conversations/:id { mode } →
DB update + user_update created → WS push conversation.updated →
Frontend applyUpdates → ConvInfoPanel re-renders with new mode →
Next agent run picks up new mode (new conversation uses new runner)
```

Note: Mode changes take effect on the next agent run, not mid-stream. The current streaming session continues with the mode it started with.

## Error Handling

- Invalid mode value → 400 with uniform error shape
- Mode change during active streaming → accepted, applied next run (not an error)
- Plan mode (reserved) → stored in DB but treated as bypass in middleware
