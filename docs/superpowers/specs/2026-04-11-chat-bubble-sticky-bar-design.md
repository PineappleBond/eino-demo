# Chat UI Bubble & Sticky Bar Design

## Overview

Redesign the chat message bubbles for clear user/assistant visual distinction, and add a sticky bar that pins the user's last message to the top when it scrolls out of viewport.

## Problem

Current chat messages all align left with identical styling — only the avatar color (purple vs dark) differs between user and assistant messages. This makes it hard to quickly scan a conversation and tell who said what. Additionally, when the user scrolls up to read older messages, their own last message disappears from view.

## Scope

- Message bubble layout: conversation-style (user right, assistant left)
- Sticky bar: appears when user's last message scrolls out of viewport
- Collapsible content in sticky bar: max-height constraint with expand/collapse toggle
- No changes to backend, API, WebSocket, or streaming logic

## Architecture

### Components

```
page.tsx (ConvChatPage)
  └── MessageList
        ├── MessageBubble (for each message, layout changes)
        └── LastUserMessageStickyBar (new component, renders conditionally)
```

### File Changes

| File | Change |
|------|--------|
| `web/src/components/chat/MessageBubble.tsx` | Rewrite to conversation-style bubbles |
| `web/src/components/chat/MessageList.tsx` | Add `LastUserMessageStickyBar`, ref to last user message DOM node |
| `web/src/components/chat/LastUserMessageStickyBar.tsx` | **New** — sticky bar component |
| `web/src/app/globals.css` | Add `.message-user`, `.message-assistant`, `.message-sticky-*` CSS classes |

### No New Dependencies

All functionality uses browser-native APIs (`IntersectionObserver`, CSS).

---

## Feature 1: Conversation-Style Message Bubbles

### User Messages
- Right-aligned (`justify-content: flex-end`)
- Purple background (`--accent` = `#6c5ce7`)
- White text
- Border-radius: `14px 14px 4px 14px` (flat bottom-left for tail effect)
- Max-width: 70% of container
- Box-shadow: subtle purple glow
- Timestamp: small, semi-transparent, right-aligned inside bubble
- No avatar (self-evident as user's own messages)
- Tool calls: displayed below the bubble (same as current)

### Assistant Messages
- Left-aligned (`justify-content: flex-start`)
- Dark background (`--bg-elevated`)
- Normal text color (`--text-primary`)
- Border: 1px solid `--border-subtle`
- Border-radius: `4px 14px 14px 14px` (flat top-left for tail effect)
- Max-width: 75% of container
- Avatar on left side (28px circle, `RobotOutlined`)
- Sender name above bubble ("Assistant")
- Timestamp: small, below bubble, left-aligned
- Tool calls: displayed below the content (same as current)

### CSS Classes (new in `globals.css`)

```css
.message-user {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.message-user .message-bubble {
  background: var(--accent);
  color: white;
  padding: 10px 14px;
  border-radius: 14px 14px 4px 14px;
  max-width: 70%;
  box-shadow: 0 2px 8px rgba(108, 92, 231, 0.3);
}

.message-user .message-time {
  font-size: 10px;
  opacity: 0.6;
  margin-top: 4px;
  text-align: right;
}

.message-assistant {
  display: flex;
  justify-content: flex-start;
  margin-bottom: 16px;
}

.message-assistant .message-bubble {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  padding: 10px 14px;
  border-radius: 4px 14px 14px 14px;
  max-width: 75%;
}

.message-assistant .message-avatar-wrapper {
  display: flex;
  gap: 8px;
  max-width: 80%;
}

.message-assistant .message-time {
  font-size: 10px;
  color: var(--text-tertiary);
  margin-top: 4px;
}
```

### MessageBubble.tsx Changes

The component receives `message: Message`. It determines `isUser` from `message.sender_role === 'user'` and applies the appropriate layout.

Key structural change:
- **User messages**: wrapper div with `message-user` class → purple bubble inside
- **Assistant messages**: wrapper div with `message-assistant` class → avatar + bubble
- The content rendering logic (ReactMarkdown for assistant, plain text for user, tool calls) remains the same
- Remove the old `.message-group`, `.message-sender`, `.message-content` layout structure

---

## Feature 2: Last User Message Sticky Bar

### New Component: `LastUserMessageStickyBar`

**Location**: `web/src/components/chat/LastUserMessageStickyBar.tsx`

**Props**:
```typescript
interface LastUserMessageStickyBarProps {
  lastUserMessage: Message | null;
  messageRef: React.RefObject<HTMLDivElement | null>;
}
```

**Behavior**:
1. On mount, create an `IntersectionObserver` with `root: containerRef.current` (the chat messages scroll container) and `threshold: 0`
2. Observe `messageRef.current` (the last user message's wrapper div)
3. When `isIntersecting === false` (message completely scrolled out of viewport/container):
   - Render the sticky bar
4. When `isIntersecting === true` (at least 1px of the message is visible):
   - Do not render anything
5. On unmount, disconnect the observer

**Sticky Bar Rendering**:
- Container: `position: sticky; top: 0; z-index: 100;`
- Background gradient: `linear-gradient(180deg, var(--bg-primary) 0%, var(--bg-primary)f5 70%, transparent)` for smooth blend
- Content: same purple bubble style as user messages
- Max-width: 500px (narrower than chat messages for bar-like appearance)

**Collapsible Content**:
- **Collapsed state** (default): `max-height: 100px; overflow: hidden`
  - If content exceeds 100px, show gradient overlay + "展开全部 ▼" button at bottom
  - Gradient: `linear-gradient(transparent, var(--accent))`
- **Expanded state**: `max-height: 250px; overflow-y: auto`
  - Show "▲ 折叠" button
- Toggle via local state: `isExpanded`

**Action Row** (below content, inside bubble):
- Left: "👤 你 · HH:MM"
- Right: "↩ 回到消息" (calls `messageRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })`) + "✕" (closes sticky bar)

**Close Behavior**:
- Clicking ✕ sets a `isManuallyClosed` state to `true`, preventing re-render even if message scrolls back into view and out again
- Scrolling back to the message (message becomes visible again) resets `isManuallyClosed` to `false`

---

## MessageList.tsx Integration

Add the following to `MessageList`:

1. Find the last user message index: `const lastUserMsgIdx = messages.map((m, i) => m.sender_role === 'user' ? i : -1).filter(i => i >= 0).pop() ?? -1;`

2. Pass a `ref` callback to the last user message's wrapper div in the render loop:
```tsx
{messages.map((msg, idx) => {
  const isLastUser = idx === lastUserMsgIdx;
  const bubble = (
    <div
      key={msg.id}
      ref={isLastUser ? lastUserMsgRef : undefined}
      data-is-last-user={isLastUser || undefined}
    >
      <MessageBubble message={msg} />
    </div>
  );
  // ... context menu wrapping
})}
```

3. Render `LastUserMessageStickyBar` at the bottom of the messages container:
```tsx
{lastUserMsgIdx >= 0 && (
  <LastUserMessageStickyBar
    lastUserMessage={messages[lastUserMsgIdx]}
    messageRef={lastUserMsgRef}
  />
)}
```

The sticky bar is rendered inside the scrollable container so `position: sticky` works relative to the container.

---

## Error Handling

- `IntersectionObserver` may not be available in very old browsers — guard with `if (typeof IntersectionObserver === 'undefined')` and skip the feature
- `messageRef.current` may be null on initial render — observer setup should happen in a `useEffect` that depends on `messageRef.current`
- Empty message content — render empty bubble gracefully

## Testing

- Manual testing via browser:
  1. Verify user messages appear right-aligned, assistant messages left-aligned
  2. Send a user message, scroll up — verify sticky bar appears
  3. Scroll back down — verify sticky bar disappears
  4. Send a long user message, scroll up — verify collapse/expand works
  5. Click ✕ to close sticky bar, scroll up/down — verify it stays closed
  6. Verify "回到消息" scrolls to the original message
- Test in both light and dark themes
