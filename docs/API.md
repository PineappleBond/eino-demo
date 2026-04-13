# API Final Specification

> **Status**: Final
> **Version**: v1
> **Base URL**: `http(s)://host:port`
> **API Prefix**: `/api/v1`
> **OpenAPI Source**: `openapi/spec.yaml`
> **Generated Types**: `web/src/types/api.d.ts`, `server/internal/types/types.go` (via `openapi/generate.sh`)

---

## Table of Contents

1. [Core Principles](#core-principles)
2. [Design Rules](#design-rules)
3. [Authentication](#authentication)
4. [Error Response Shape](#error-response-shape)
5. [The Update Type](#the-update-type)
6. [Frontend Architecture](#frontend-architecture)
7. [Communication Flows](#communication-flows)

---

## Core Principles

- **HTTP** = client-initiated requests. All resource CRUD and user actions go through HTTP.
- **WebSocket** = server-initiated pushes. Server pushes streaming updates, notifications, and events to the client.
- **Client MUST NOT send any WebSocket frame except `ping` (heartbeat).** All client actions (send message, stop, reconnect) are HTTP endpoints.
- The same `Update` type flows through both WS pushes and HTTP polling responses. One `applyUpdates()` pipeline on the frontend.
- Single entry point: `applyUpdates(updates: Update[])` -- ALL incoming updates (WS push or HTTP polling) go through this.
- `seq > 0` → persist to IndexedDB first, then notify subscribers.
- `seq = 0` → ephemeral (streaming tokens, thinking). NOT persisted. Final `seq > 0` message supersedes.

---

## Design Rules

### Path Style

- RESTful resource paths (`/api/v1/projects`, `/api/v1/conversations`).
- Action-oriented operations use verb suffixes: `POST /api/v1/conversations/{id}/messages`, `POST /api/v1/conversations/{id}/stop`.
- RPC-style verbs in paths (`/project.create`) are forbidden.

### send_message Strategy

- `POST /api/v1/conversations/{id}/messages` returns synchronously with `{ conversation_id, message_id, seq }` for the user's message only.
- The AI response (streaming tokens, tool calls, final message) flows exclusively through WebSocket pushes as `[]Update` arrays.
- If WS is disconnected, client falls back to HTTP polling endpoint.

### Offline Polling

- Returns `{ updates: [...], max_seq, has_more }` — the server fills missing seq numbers with `empty` updates.
- Client passes `updates` array into `applyUpdates()`. The `empty` updates are skipped by the subscriber notifier.

### WebSocket Frame Envelope

- Single envelope type -- all frames share `{ "type": string, "payload": object }`.
- Server → Client frames: `updates` (batched `[]Update`), `connected` (WS handshake), and future types.
- Client → Server frames: `ping` (heartbeat) only.
- No client message, stop, or reconnect frames via WebSocket. Those are HTTP endpoints.

### Type Generation

```bash
./openapi/generate.sh
```

Generates both Go (`server/internal/types/types.go`) and TypeScript (`web/src/types/api.d.ts`) types from `openapi/spec.yaml`.

**All API and WebSocket payloads must reference the generated types.** Never hand-write field names that exist in the spec — this is the only way to guarantee field-name consistency between backend and frontend.

---

## Authentication

- All `/api/v1/` endpoints require `Authorization: Bearer <token>`.
- In demo mode, `<token>` value IS the `user_id`. Any non-empty token is accepted.
- WebSocket auth uses query param: `ws://host/ws?token=<token>&last_seq=<number>`.
- Missing or empty token: `401 Unauthorized`.

---

## Error Response Shape

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "human readable description"
  }
}
```

| HTTP Status | Code | When |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | Malformed JSON, missing required fields, invalid enum values |
| 400 | `NOT_FOUND` | Referenced resource (project, conversation) does not exist |
| 401 | `UNAUTHORIZED` | Missing or empty Authorization header |
| 404 | `NOT_FOUND` | Endpoint path does not match any route |
| 500 | `INTERNAL_ERROR` | Unexpected server error |

No stack traces or internal details are ever leaked.

### UI Treatment

| Error Code | UI Treatment |
| --- | --- |
| `INVALID_REQUEST` | `message.error(message)` -- user input error |
| `NOT_FOUND` | `Result` page with "not found" + back button, or `message.warning` for list ops |
| `UNAUTHORIZED` | Redirect to login, clear token |
| `INTERNAL_ERROR` | `message.error('Something went wrong. Please try again.')` |

---

## The Update Type

Every server-to-client event (both WS pushes and HTTP polling responses) is an `Update`:

```json
{
  "seq": 0,
  "type": "message.new",
  "payload": {}
}
```

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `seq` | number | yes | `> 0` = persisted (stored in DB, replayable on reconnect, stored in IndexedDB). `0` = ephemeral (streaming token, thinking chunk), NOT persisted, NOT replayable. |
| `type` | string | yes | Discriminator. See Update Types below. |
| `payload` | object | yes | Type-specific entity. Frontend extracts entity fields (e.g. `conversation_id`) from payload to derive topic. |

### Update Types

| `type` | `seq > 0` | `seq = 0` | Payload Shape |
| --- | --- | --- | --- |
| `message.new` | Message persisted (user or AI) | -- | payload contains conversation context + `sender`, `content`, etc. |
| `message.delta` | -- | Streaming text token appended | payload contains conversation context + `delta` |
| `message.done` | Final message with complete content | -- | payload contains conversation context + `content`, `metadata` |
| `message.tool_call` | Tool call record persisted | -- | payload contains conversation context + tool fields |
| `message.thinking` | -- | Thinking/reasoning token | payload contains conversation context + `delta` |
| `message.error` | Error event persisted | -- | payload contains conversation context + error fields |
| `message.stop` | Stream interrupted by user | -- | payload contains conversation context + partial `content` |
| `conversation.created` | Conversation created | -- | payload contains `conversation_id`, `project_id` |
| `conversation.deleted` | Conversation deleted | -- | payload contains `conversation_id` |
| `conversation.compacting` | Compression started | -- | payload contains `conversation_id` |
| `conversation.compacted` | Compression completed | -- | payload contains `old_conv_id`, `new_conv_id` |
| `conversation.archived` | Conversation archived | -- | payload contains `conversation_id` |
| `project.created` | Project created | -- | payload contains `project_id`, `template_id` |
| `project.deleted` | Project deleted | -- | payload contains `project_id` |
| `settings.changed` | Settings updated | -- | payload contains `model_tier`, `locale`, or `theme` |
| `empty` | `> 0` | -- | `{}` (seq gap filler) |

### TypeScript Discriminated Union

```typescript
type Update = MessageNewUpdate | MessageDeltaUpdate | MessageDoneUpdate
  | MessageToolCallUpdate | MessageThinkingUpdate | MessageErrorUpdate | MessageStopUpdate
  | ConversationCreatedUpdate | ConversationDeletedUpdate
  | ConversationCompactingUpdate | ConversationCompactedUpdate | ConversationArchivedUpdate
  | ProjectCreatedUpdate | ProjectDeletedUpdate
  | SettingsChangedUpdate
  | EmptyUpdate;
```

### Seq Assignment

- Server assigns monotonically increasing `seq` per user via Redis `INCR` command.
- `seq = 0` = server assigns zero for ephemeral events (streaming tokens).
- **Gap filling**: `INCR` runs before DB write. If the DB write fails, the seq is not rolled back, creating gaps in the sequence. Before returning `[]Update` to client (HTTP response or polling), server fills missing seq numbers with `empty` updates so the frontend's seq continuity check always passes. For example: client sends `last_seq=100`, server max_seq=200. Server queries `seq > 100`, expects 101–200. Actual DB rows may be sparse (e.g. 102, 105, 106…200). Server inserts `empty` updates for every missing seq (101, 103, 104, 107…) so the client receives a continuous range.

---

## Frontend Architecture

### UpdateDispatcher (singleton)

- `applyUpdates(updates: Update[])` -- single entry point for all updates. Thread-safe: concurrent calls are serialized via a mutex/queue.
- Partitions into `persistable (seq > 0)` and `ephemeral (seq = 0)`.
- For persistable: validates seq continuity against `latest_seq` cursor. If gap detected, triggers HTTP pull for the
  missing range and discards the current batch. If continuous, updates the `latest_seq` cursor.
- For ephemeral: no IndexedDB write, no seq check.
- **IndexedDB strategy**: `applyUpdates()` does NOT write to a dedicated `updates` store. Only `latest_seq` (a single
  integer in the `settings` store) tracks the global sync cursor. If missed during disconnect, `latest_seq` + HTTP pull
  recovers missing entities. Other entities (messages, conversations, projects) are stored in separate IndexedDB tables
  via HTTP sync after subscribers are notified.

### Topic Routing

- Derived from route context: `/chat/{convId}` → `conv:{convId}`.
- System-level updates (no conversation entity in payload) → `system` topic.
- Backend has no topic concept. Frontend extracts entity fields from `payload` to derive topic.

### Connection Lifecycle

- WS connects once on auth. App-global, never disconnect on page nav.
- On reconnect, server actively closes (kicks) all old connections for this user before adding the new one. Heartbeat expiry is per-connection (1 minute from last heartbeat).
- On reconnect, HTTP polling endpoint fills gaps until WS is stable.
- All client actions (send, stop, reconnect) via HTTP endpoints. WS is receive-only.

### `applyUpdates()` Flow

```text
1. Partition: persistable (seq > 0) vs ephemeral (seq = 0)
2. For persistable: check seq continuity with local latest_seq
   - If gap detected: trigger HTTP pull for missing range (via onGapDetected callback), discard current batch
   - If continuous: update latest_seq cursor in IndexedDB settings store
3. For ephemeral: skip IndexedDB
4. For each update (skip "empty" gap-fillers): extract entity from payload → derive topic
5. Notify active subscribers for that topic
```

### IndexedDB Schema

- Store name: `settings` — contains `latest_seq` cursor (single integer).
- Additional stores (populated via HTTP sync, not by `applyUpdates()`): `messages`, `conversations`, `projects`, `users`.
- Chat page flush: queries IndexedDB `messages` store for the current conversation. Missing data triggers HTTP sync, which updates local tables and then re-renders.

### Flow

```text
source → applyUpdates → (seq>0: update latest_seq cursor) → (skip empty updates) → notify subscribers → component re-render
```

---

## Communication Flows

### Flow 1: Create Conversation

```mermaid
sequenceDiagram
    participant C as Client (Browser)
    participant S as Server (Gin)
    participant Auth as Auth Middleware
    participant Seq as Seq Service (INCR)
    participant DB as PostgreSQL
    participant WS as WS Connection Manager
    participant IDB as IndexedDB

    rect rgb(240, 248, 255)
    Note over C,IDB: Step 1-6: HTTP Create Conversation
    C->>S: POST /api/v1/projects/{id}/conversations { title? }
    S->>Auth: Auth middleware extracts user_id from Bearer token
    Auth-->>S: user_id
    S->>Seq: Redis INCR (user_id) → seq
    Seq-->>S: seq (number)
    S->>DB: INSERT conversation + INSERT user_update (type=conversation.created, payload={conv_id, ...}, seq)
    DB-->>S: OK
    S->>WS: pushToUserConnections(user_update)
    WS-->>WS: broadcast { seq, type: "conversation.created", payload: { conv_id, ... } }
    S-->>C: 201 { id, project_id, title, created_at }
    end

    rect rgb(255, 250, 240)
    Note over C,IDB: Step 7-9: Client processes HTTP response
    C->>C: applyUpdates([{ seq, type: "conversation.created", payload: { conv_id, ... } }])
    C->>C: seq > 0? Check continuity with local maxSeq
    alt seq gap detected
        C->>C: abort, HTTP pull for missing range
    else continuous
        C->>IDB: store Update to IndexedDB
    end
    C->>C: extract conv_id from payload → derive topic = conv:{conv_id}
    C->>C: HTTP sync: fetch new conversation entity from server
    C->>C: render: conversation list updated, navigate to /chat/{conv_id}
    end

    rect rgb(240, 255, 240)
    Note over C,IDB: Step 10-13: Client receives WS push (same update)
    WS->>C: push user_update via WebSocket
    C->>C: applyUpdates([{ ...same update... }])
    C->>C: seq > 0? Try IndexedDB upsert
    Note over C: seq conflict → upsert ignored (same key)
    C->>C: extract conv_id from payload → is current conversation the new one?
    Note over C: if not on this conversation → ignored
    C->>C: HTTP sync: refresh conversation list
    end
```

---

### Flow 2: Send Message

```mermaid
sequenceDiagram
    participant C as Client (Browser)
    participant S as Server (Gin)
    participant Auth as Auth Middleware
    participant Seq as Seq Service (INCR)
    participant DB as PostgreSQL
    participant Agent as Eino Agent
    participant WS as WS Connection Manager
    participant IDB as IndexedDB

    rect rgb(240, 248, 255)
    Note over C,IDB: Step 1-7: HTTP Send Message
    C->>S: POST /api/v1/conversations/{conv_id}/messages { content }
    S->>Auth: Auth middleware extracts user_id
    Auth-->>S: user_id
    S->>Seq: Redis INCR (user_id) → seq
    Seq-->>S: seq (number)
    S->>DB: INSERT user_message + INSERT user_update<br/>(type=message.new, payload={role:user, content, ...}, seq)
    DB-->>S: OK
    S->>WS: pushToUserConnections(user_update)
    WS-->>WS: broadcast user message update
    S-->>C: 200 { conversation_id, message_id, seq }
    end

    rect rgb(255, 250, 240)
    Note over C,IDB: Step 8-10: Client processes HTTP response
    C->>C: applyUpdates([{ seq, type: "message.new", payload: { conversation_id, role: "user", content, ... } }])
    C->>C: seq > 0? Check continuity
    alt seq gap detected
        C->>C: abort, HTTP pull for missing range
    else continuous
        C->>IDB: store Update to IndexedDB
    end
    C->>C: extract conversation_id from payload → derive topic = conv:{conversation_id}
    C->>C: render: message appears in chat, streaming indicator shows
    end

    rect rgb(240, 255, 240)
    Note over C,IDB: Step 11-14: Client receives WS push (same user message update)
    WS->>C: push user_message update via WebSocket
    C->>C: applyUpdates([{ ...same update... }])
    C->>C: seq conflict → IndexedDB upsert ignored
    Note over C: if already rendered from HTTP response → no-op
    end

    rect rgb(255, 240, 245)
    Note over C,IDB: Step 15-20: Server Agent streams AI response
    Agent-->>S: streaming tokens (seq=0)
    S->>Seq: assign seq=0
    S->>WS: pushToUserConnections({ seq:0, type: "message.delta", payload: { conversation_id, delta } })
    WS->>C: push delta via WebSocket
    C->>C: applyUpdates([{ seq:0, ... }])
    Note over C: seq=0 → ephemeral, NOT stored in IndexedDB
    C->>C: extract conversation_id from payload → derive topic = conv:{conversation_id}
    C->>C: append delta to streaming text component (pretext)
    end

    rect rgb(255, 255, 224)
    Note over C,IDB: Step 21-26: AI response complete
    Agent-->>S: final message
    S->>Seq: Redis INCR (user_id) → seq
    Seq-->>S: seq (number)
    S->>DB: INSERT ai_message + INSERT user_update<br/>(type=message.done, payload={conversation_id, role:assistant, content, metadata}, seq)
    DB-->>S: OK
    S->>WS: pushToUserConnections(message.done update)
    WS->>C: push message.done via WebSocket
    C->>C: applyUpdates([{ seq, type: "message.done", payload: { conversation_id, role, content, metadata } }])
    C->>IDB: store Update to IndexedDB
    C->>C: extract conversation_id from payload → replace streaming text with final message, render tool call cards
    end
```

---

### Flow 3: WebSocket Connection Lifecycle

```mermaid
sequenceDiagram
    participant C as Client (Browser)
    participant S as Server (Gin)
    participant Auth as Auth Middleware
    participant Conn as Connection Manager

    rect rgb(240, 248, 255)
    Note over C,Conn: Connection
    C->>S: WS connect: ws://host/ws?token=<token>&last_seq=<number>
    S->>Auth: Validate Bearer token from query param
    alt token empty/missing
        S-->>C: close 401
    else valid
        Auth-->>S: user_id
        S->>Conn: addConnection(user_id, ws_conn)
        Note over Conn: allows multiple connections per user
        S->>C: { "type": "connected", "payload": { "user_id", "server_time", "max_seq" } }
    end
    end

    rect rgb(255, 250, 240)
    Note over C,Conn: Heartbeat
    Note over C: On "connected" received → start 30s heartbeat timer
    loop every 30 seconds
        C->>S: { "type": "ping", "payload": {} }
        S->>Conn: renewConnectionExpiry(user_id, ws_conn)
        Note over Conn: expiry: 1 minute from last heartbeat
    end
    end

    rect rgb(255, 240, 245)
    Note over C,Conn: Timeout / Disconnect
    Note over Conn: If no heartbeat for 1 minute → Conn.removeConnection()
    Conn->>S: close connection
    S->>C: close event (code 1001)
    C->>C: mark wsConnected = false
    C->>C: start exponential backoff reconnect (1s → 2s → 4s → 8s → 16s → 30s cap)
    end

    rect rgb(240, 255, 240)
    Note over C,Conn: Reconnection
    C->>S: WS reconnect: ws://host/ws?token=<token>&last_seq=<max_seq_in_IndexedDB>
    S->>Auth: Validate token
    Auth-->>S: user_id
    S->>Conn: kickAllOldConnections(user_id)
    S->>Conn: addConnection(user_id, ws_conn)
    S->>C: { "type": "connected", "payload": { "user_id", "server_time", "max_seq" } }
    C->>C: mark wsConnected = true, restart heartbeat timer
    Note over C: If max_seq from server > local maxSeq → start HTTP pull to fill gap
    end
```

---

### Flow 4: Client Pulls Updates (Polling Fallback)

```mermaid
sequenceDiagram
    participant C as Client (Browser)
    participant S as Server (Gin)
    participant DB as PostgreSQL
    participant IDB as IndexedDB
    participant Sub as Subscribers

    rect rgb(240, 248, 255)
    Note over C,Sub: Polling Trigger
    Note over C: Trigger 1: WS disconnected → start polling
    Note over C: Trigger 2: periodic timer → catch up missed updates
    Note over C: delay: 1s → 2s → 4s → ... → 30s (cap, exponential backoff)
    end

    rect rgb(255, 250, 240)
    Note over C,Sub: HTTP Pull Request
    C->>S: GET /api/v1/users/me/updates?last_seq=<local_latestSeq>
    alt last_seq == 0
        Note over S: Fresh client → return only the last Update
        S->>DB: SELECT latest user_update WHERE user_id = ?
    else last_seq > 0
        S->>DB: SELECT user_updates WHERE user_id = ? AND seq > last_seq ORDER BY seq ASC
        Note over S: Fill missing seq numbers with "empty" updates to ensure continuity
    end
    DB-->>S: []Update
    S-->>C: 200 { updates, max_seq, has_more }
    end

    rect rgb(255, 240, 245)
    Note over C,Sub: Client processes Updates
    C->>C: applyUpdates(updates)
    alt seq > 0 AND continuous with local latestSeq
        C->>C: update latest_seq cursor in settings store
        C->>C: for each update (skip "empty"): extract entity from payload → derive topic
        C->>Sub: notify subscribers(topic, update)
        C->>C: update local latestSeq = max(seq)
        Sub->>Sub: subscribers re-render
    else seq gap detected (not continuous)
        C->>C: abort this batch
        C->>C: trigger HTTP pull with last_seq = local_latestSeq to fill gap first
        Note over C: applyUpdates terminates for this batch, gap recovery happens via HTTP
    end
    end

    rect rgb(240, 255, 240)
    Note over C,Sub: Polling Stop Condition
    alt WS reconnected
        C->>C: stop polling, switch to WS-only updates
    else polling returns empty []
        C->>C: no new updates, continue polling with current delay
    end
    end
```

---

### Flow 5: applyUpdates() Complete Flow

```mermaid
flowchart TD
    A["applyUpdates(updates: Update[])"] --> B["Partition: persistable (seq > 0) vs ephemeral (seq = 0)"]
    B --> C{"persistable.length > 0?"}

    C -->|yes| D["Check seq continuity: min(persistable.seq) == localLatestSeq + 1?"]
    D -->|no, gap detected| E["ABORT: trigger HTTP pull for missing range via onGapDetected callback"]
    E --> F["applyUpdates terminates for this batch"]

    D -->|yes, continuous| G["Update latest_seq cursor in IndexedDB settings store"]
    G --> H["Process all updates (persistable + ephemeral)"]

    C -->|no, all ephemeral| H

    H --> I["For each update: skip 'empty' gap-fillers"]
    I --> J["Extract entity from payload → derive topic"]
    J --> K["Notify active subscribers for that topic"]
    K --> L["Subscribers handle update by type:"]

    L --> M["message.new → render message bubble"]
    L --> N["message.delta → append to streaming text (pretext)"]
    L --> O["message.tool_call → render/update tool call card"]
    L --> P["message.thinking → show thinking indicator"]
    L --> Q["message.error → show error banner"]
    L --> R["message.done → replace streaming text with final message"]
    L --> S["message.stop → stop streaming, show partial content"]
    L --> T["conversation.compacting → show 'compacting' loading, disable input"]
    L --> U["conversation.compacted → navigate to new_conv_id"]
    L --> V["conversation.archived → mark conversation read-only"]
    L --> W["project.created/deleted → refresh project list"]
    L --> X["settings.changed → update local settings, re-render UI"]
    L --> Y["empty → no-op (seq gap filler, discarded by subscriber)"]

    M --> Z["Done"]
    N --> Z
    O --> Z
    P --> Z
    Q --> Z
    R --> Z
    S --> Z
    T --> Z
    U --> Z
    V --> Z
    W --> Z
    X --> Z
    Y --> Z
    F --> Z
```

---

### Flow 6: Page Mount + Flush Pending Updates

```mermaid
sequenceDiagram
    participant Page as Chat Page (mounts)
    participant Hook as useSubscribe(conv:{id})
    participant Disp as UpdateDispatcher
    participant IDB as IndexedDB
    participant WS as WS Connection

    Page->>Hook: mount on /chat/{conv_id}
    Hook->>Disp: subscribe(topic = conv:{conv_id})
    Hook->>IDB: query messages store WHERE conversation_id = conv_id ORDER BY created_at ASC
    IDB-->>Hook: [local_messages]
    Hook->>S: HTTP GET /api/v1/conversations/{conv_id}/messages?after=<last_local_seq>
    S-->>Hook: [new_messages]
    Hook->>IDB: upsert new_messages into messages store
    Hook->>Disp: notify subscribers
    Disp->>Page: renders all messages (local + synced)

    Note over WS: WS connection stays alive globally
    alt WS pushes new update for this conv_id
        WS->>Disp: update arrives
        Disp->>Disp: seq > 0? → IndexedDB upsert (may conflict, ignored)
        Disp->>Hook: notify subscriber
        Hook->>Page: re-render with new update
    end

    Page->>Hook: navigate away, unmount
    Hook->>Disp: unsubscribe(topic)
    Note over Disp: Updates for this topic still persist to IndexedDB<br/>but no subscriber is notified until page re-mounts
```

---

## Summary of Server Push Types

| `type` | `seq` | Triggered By | Sent Via |
| --- | --- | --- | --- |
| `message.new` | `> 0` | Message persisted (user or AI) | WS + HTTP response |
| `message.delta` | `0` | Agent streaming token | WS only |
| `message.done` | `> 0` | Agent response complete | WS |
| `message.tool_call` | `> 0` | Tool execution result | WS |
| `message.thinking` | `0` | Model reasoning token | WS only |
| `message.error` | `> 0` | Agent or system error | WS |
| `message.stop` | `> 0` | User interrupted stream | WS |
| `conversation.created` | `> 0` | New conversation started | WS + HTTP response |
| `conversation.deleted` | `> 0` | Conversation deleted | WS |
| `conversation.compacting` | `> 0` | Context compression started | WS |
| `conversation.compacted` | `> 0` | Compression completed | WS |
| `conversation.archived` | `> 0` | Conversation archived | WS |
| `project.created` | `> 0` | Project created from template | WS + HTTP response |
| `project.deleted` | `> 0` | Project deleted | WS |
| `settings.changed` | `> 0` | Settings updated | WS |
| `empty` | `> 0` | Seq gap filler (INCR non-rollback) | WS + HTTP response |
| `connected` | -- | WS connection established | WS only (WSServerFrame, not Update) |
