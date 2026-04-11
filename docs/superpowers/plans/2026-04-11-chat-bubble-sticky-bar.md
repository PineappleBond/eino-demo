# Chat UI Bubble & Sticky Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign chat message bubbles into conversation-style layout and add a sticky bar that pins the user's last message to the top when it scrolls out of viewport.

**Architecture:** Rewrite `MessageBubble` for right/left bubble alignment, create `LastUserMessageStickyBar` component using `IntersectionObserver`, integrate into `MessageList`.

**Tech Stack:** React, Ant Design (existing), CSS variables, IntersectionObserver API

---

### Task 1: Add CSS for conversation-style bubbles

**Files:**
- Modify: `web/src/app/globals.css`

- [ ] **Step 1: Replace existing message CSS and add new classes**

In `globals.css`, replace the existing `.message-group`, `.message-sender`, `.message-content` CSS block (approximately lines 422-468) and the `.chat-messages` block with the following. Keep the `@keyframes slideUp` and `.fade-in`/`.slide-up` classes that come before.

Replace from `.chat-messages {` through `.message-content { ... }` (all message-related CSS up to `.message-content code`) with:

```css
/* ═══ Chat Messages ═══ */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 0;
}

.chat-inner {
  margin: 0 auto;
  padding: 0 32px;
  width: 100%;
}

/* ─── User Message (right-aligned, purple bubble) ─── */

.message-user {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
  animation: slideUp 0.3s ease;
}

.message-user .message-bubble {
  background: var(--accent);
  color: white;
  padding: 10px 14px;
  border-radius: 14px 14px 4px 14px;
  max-width: 70%;
  box-shadow: 0 2px 8px rgba(108, 92, 231, 0.3);
  line-height: 1.65;
  font-size: 14px;
}

.message-user .message-bubble-text {
  white-space: pre-wrap;
  word-break: break-word;
}

.message-user .message-bubble-text code {
  font-family: var(--font-mono);
  background: rgba(255, 255, 255, 0.15);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 13px;
  color: white;
}

.message-user .message-bubble-text pre {
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-md);
  padding: 14px 16px;
  margin: 8px 0;
  overflow-x: auto;
  font-family: var(--font-mono);
  font-size: 12.5px;
  line-height: 1.6;
  color: white;
}

.message-user .message-time {
  font-size: 10px;
  opacity: 0.6;
  margin-top: 4px;
  text-align: right;
  color: white;
}

.message-user .message-tool-calls {
  margin-top: 8px;
}

/* ─── Assistant Message (left-aligned, dark bubble with avatar) ─── */

.message-assistant {
  display: flex;
  justify-content: flex-start;
  margin-bottom: 16px;
  animation: slideUp 0.3s ease;
}

.message-assistant .message-avatar-wrapper {
  display: flex;
  gap: 8px;
  max-width: 80%;
}

.message-assistant .message-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  flex-shrink: 0;
}

.message-assistant .message-content-area {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.message-assistant .message-sender-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.message-assistant .message-bubble {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  padding: 10px 14px;
  border-radius: 4px 14px 14px 14px;
  max-width: 100%;
  line-height: 1.65;
  font-size: 14px;
}

.message-assistant .message-bubble code {
  font-family: var(--font-mono);
  background: var(--bg-primary);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 13px;
  color: var(--text-primary);
}

.message-assistant .message-bubble pre {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: 14px 16px;
  margin: 8px 0;
  overflow-x: auto;
  font-family: var(--font-mono);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--text-primary);
}

.message-assistant .message-time {
  font-size: 10px;
  color: var(--text-tertiary);
  margin-top: 4px;
}

.message-assistant .message-tool-calls {
  margin-top: 8px;
}

/* ─── Sticky Bar ─── */

.message-sticky-bar {
  position: sticky;
  top: 0;
  z-index: 100;
  padding: 8px 32px;
  background: linear-gradient(180deg, var(--bg-primary) 0%, color-mix(in srgb, var(--bg-primary) 95%, transparent) 70%, transparent);
}

.message-sticky-bar .message-sticky-bubble {
  background: var(--accent);
  color: white;
  padding: 8px 12px;
  border-radius: 14px 14px 4px 14px;
  max-width: 500px;
  margin-left: auto;
  font-size: 12px;
  line-height: 1.5;
  box-shadow: 0 4px 20px rgba(108, 92, 231, 0.35);
}

.message-sticky-bar .message-sticky-content {
  position: relative;
}

.message-sticky-bar .message-sticky-collapsed {
  max-height: 100px;
  overflow: hidden;
  position: relative;
}

.message-sticky-bar .message-sticky-gradient {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 60px;
  background: linear-gradient(transparent, var(--accent));
  display: flex;
  align-items: flex-end;
  justify-content: center;
  padding-bottom: 8px;
}

.message-sticky-bar .message-sticky-expand-btn {
  font-size: 12px;
  cursor: pointer;
  background: rgba(0, 0, 0, 0.25);
  padding: 4px 14px;
  border-radius: 14px;
  font-weight: 500;
  color: white;
  border: none;
  user-select: none;
}

.message-sticky-bar .message-sticky-expanded {
  max-height: 250px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.message-sticky-bar .message-sticky-action-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 6px;
  padding-top: 4px;
  border-top: 1px solid rgba(255, 255, 255, 0.15);
  font-size: 10px;
  opacity: 0.7;
}

.message-sticky-bar .message-sticky-close-btn {
  cursor: pointer;
  font-size: 14px;
  background: none;
  border: none;
  color: white;
  opacity: 0.7;
  padding: 0 2px;
  line-height: 1;
}

.message-sticky-bar .message-sticky-close-btn:hover {
  opacity: 1;
}

.message-sticky-bar .message-sticky-back-btn {
  cursor: pointer;
  background: none;
  border: none;
  color: white;
  opacity: 0.7;
  font-size: 10px;
  padding: 0;
}

.message-sticky-bar .message-sticky-back-btn:hover {
  opacity: 1;
}
```

- [ ] **Step 2: Verify no syntax errors**

Run: `cd web && npx stylelint src/app/globals.css` (if stylelint is configured) or just verify the Next.js dev server compiles without CSS errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/app/globals.css
git commit -m "feat(ui): add conversation-style bubble CSS and sticky bar styles"
```

---

### Task 2: Add i18n keys for sticky bar

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add sticky bar keys to `en.json`**

In `web/src/locales/en.json`, inside the `"chat"` object, add these keys after `"assistant"`:

```json
"expandAll": "Expand all",
"collapse": "Collapse",
"backToMessage": "Back to message",
```

- [ ] **Step 2: Add sticky bar keys to `zh.json`**

In `web/src/locales/zh.json`, inside the `"chat"` object, add these keys after `"assistant"`:

```json
"expandAll": "展开全部",
"collapse": "折叠",
"backToMessage": "回到消息",
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat(ui): add i18n keys for sticky bar"
```

---

### Task 3: Rewrite MessageBubble for conversation-style layout

**Files:**
- Modify: `web/src/components/chat/MessageBubble.tsx`
- Test: `web/src/components/chat/MessageList.tsx` (existing, no changes needed — same props contract)

- [ ] **Step 1: Rewrite `MessageBubble.tsx` with conversation-style layout**

Replace the entire content of `web/src/components/chat/MessageBubble.tsx` with:

```tsx
'use client';

import { Avatar } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { useTranslations } from 'next-intl';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

export function MessageBubble({ message }: { message: Message }) {
  const t = useTranslations('chat');
  const isUser = message.sender_role === 'user';

  const timeStr = message.created_at
    ? new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '';

  const toolCalls = (message.metadata?.tool_calls as Array<{
    name: string;
    input: Record<string, unknown>;
    output: string;
    status: string;
  }> | undefined);
  const hasToolCalls = toolCalls && Array.isArray(toolCalls) && toolCalls.length > 0;

  if (isUser) {
    return (
      <div className="message-user">
        <div className="message-bubble">
          <div className="message-bubble-text" style={{ whiteSpace: 'pre-wrap' }}>
            {message.content}
          </div>
          {timeStr && <div className="message-time">{timeStr}</div>}
          {hasToolCalls && (
            <div className="message-tool-calls">
              {toolCalls.map((tc, i) => (
                <ToolCallCard
                  key={i}
                  name={tc.name}
                  input={tc.input}
                  output={tc.output}
                  status={tc.status as 'running' | 'done' | 'error'}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="message-assistant">
      <div className="message-avatar-wrapper">
        <Avatar
          size={28}
          className="message-avatar"
          icon={<RobotOutlined />}
          style={{ fontSize: 14, background: 'var(--bg-elevated)', color: 'var(--text-secondary)' }}
        />
        <div className="message-content-area">
          <span className="message-sender-name">{t('assistant')}</span>
          <div className="message-bubble">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeHighlight]}
              components={{
                code: ({ children, ...props }) => {
                  const isBlock = props.className?.includes('language-') || String(children).includes('\n');
                  if (isBlock) {
                    return (
                      <pre style={{ margin: '8px 0' }}>
                        <code {...props}>{children}</code>
                      </pre>
                    );
                  }
                  return <code {...props}>{children}</code>;
                },
                pre: ({ children }) => <>{children}</>,
              }}
            >
              {message.content}
            </ReactMarkdown>
          </div>
          {timeStr && <div className="message-time">{timeStr}</div>}
          {hasToolCalls && (
            <div className="message-tool-calls">
              {toolCalls.map((tc, i) => (
                <ToolCallCard
                  key={i}
                  name={tc.name}
                  input={tc.input}
                  output={tc.output}
                  status={tc.status as 'running' | 'done' | 'error'}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
```

Key changes from current:
- Removed the old `.message-group`, `.message-sender`, `.message-content` layout
- User messages: right-aligned purple bubble, no avatar, timestamp inside
- Assistant messages: left-aligned with avatar on left, sender name above, timestamp below
- ToolCallCard rendering preserved, moved into the appropriate bubble
- Removed `UserOutlined` import (user messages don't show avatar)

- [ ] **Step 2: Commit**

```bash
git add web/src/components/chat/MessageBubble.tsx
git commit -m "feat(ui): rewrite message bubbles as conversation-style layout"
```

---

### Task 4: Create LastUserMessageStickyBar component

**Files:**
- Create: `web/src/components/chat/LastUserMessageStickyBar.tsx`

- [ ] **Step 1: Create the sticky bar component**

Create `web/src/components/chat/LastUserMessageStickyBar.tsx` with the following content:

```tsx
'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { Message } from '@/lib/api';

interface LastUserMessageStickyBarProps {
  lastUserMessage: Message;
  messageRef: React.RefObject<HTMLDivElement | null>;
  containerRef: React.RefObject<HTMLDivElement | null>;
}

export function LastUserMessageStickyBar({
  lastUserMessage,
  messageRef,
  containerRef,
}: LastUserMessageStickyBarProps) {
  const t = useTranslations('chat');
  const [isIntersecting, setIsIntersecting] = useState(true);
  const [isExpanded, setIsExpanded] = useState(false);
  const [isManuallyClosed, setIsManuallyClosed] = useState(false);
  const [needsExpand, setNeedsExpand] = useState(false);

  // IntersectionObserver: detect when the message scrolls out of view
  useEffect(() => {
    const target = messageRef.current;
    const root = containerRef.current;
    if (!target || !root) return;
    if (typeof IntersectionObserver === 'undefined') return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        setIsIntersecting(entry.isIntersecting);
        // Reset manual close when message becomes visible again
        if (entry.isIntersecting) {
          setIsManuallyClosed(false);
        }
      },
      { root, threshold: 0 }
    );

    observer.observe(target);
    return () => observer.disconnect();
  }, [messageRef, containerRef]);

  // Detect if content overflows the collapsed height
  const contentRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    setNeedsExpand(el.scrollHeight > (isExpanded ? 250 : 100));
  }, [lastUserMessage.content, isExpanded]);

  const handleBackToMessage = useCallback(() => {
    messageRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }, [messageRef]);

  const handleClose = useCallback(() => {
    setIsManuallyClosed(true);
  }, []);

  const handleToggleExpand = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  // Don't render if message is visible or manually closed
  if (isIntersecting || isManuallyClosed) return null;

  const timeStr = lastUserMessage.created_at
    ? new Date(lastUserMessage.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '';

  return (
    <div className="message-sticky-bar">
      <div className="message-sticky-bubble">
        <div className="message-sticky-content">
          <div
            ref={contentRef}
            className={isExpanded ? 'message-sticky-expanded' : 'message-sticky-collapsed'}
            style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
          >
            {lastUserMessage.content}
            {!isExpanded && needsExpand && (
              <div className="message-sticky-gradient">
                <button className="message-sticky-expand-btn" onClick={handleToggleExpand}>
                  {t('expandAll')} ▼
                </button>
              </div>
            )}
          </div>
        </div>
        {isExpanded && needsExpand && (
          <div style={{ textAlign: 'right', marginTop: 4 }}>
            <button className="message-sticky-close-btn" onClick={handleToggleExpand} style={{ fontSize: 10 }}>
              ▲ {t('collapse')}
            </button>
          </div>
        )}
        <div className="message-sticky-action-row">
          <span>👤 {t('you')} · {timeStr}</span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="message-sticky-back-btn" onClick={handleBackToMessage}>
              ↩ {t('backToMessage')}
            </button>
            <button className="message-sticky-close-btn" onClick={handleClose}>
              ✕
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/chat/LastUserMessageStickyBar.tsx
git commit -m "feat(ui): add LastUserMessageStickyBar component"
```

---

### Task 5: Integrate sticky bar into MessageList

**Files:**
- Modify: `web/src/components/chat/MessageList.tsx`

- [ ] **Step 1: Add ref, last user detection, and sticky bar rendering**

Replace the entire content of `web/src/components/chat/MessageList.tsx` with:

```tsx
'use client';

import { useEffect, useRef, useState } from 'react';
import { Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';
import { LastUserMessageStickyBar } from './LastUserMessageStickyBar';

export function MessageList({ messages, isStreaming, messageContextMenuItems }: {
  messages: Message[];
  isStreaming?: boolean;
  messageContextMenuItems?: (msg: Message) => MenuProps['items'];
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const lastUserMsgRef = useRef<HTMLDivElement>(null);
  const [userAtBottom, setUserAtBottom] = useState(true);

  // Find the last user message index
  const lastUserMsgIdx = messages
    .map((m, i) => (m.sender_role === 'user' ? i : -1))
    .filter((i) => i >= 0)
    .pop() ?? -1;

  // Track whether user is scrolled near the bottom
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const handleScroll = () => {
      const threshold = 80;
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
      setUserAtBottom(atBottom);
    };
    el.addEventListener('scroll', handleScroll, { passive: true });
    return () => el.removeEventListener('scroll', handleScroll);
  }, []);

  // Auto-scroll only when user is at the bottom
  useEffect(() => {
    if (containerRef.current && userAtBottom) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [messages, userAtBottom]);

  if (messages.length === 0) {
    return null;
  }

  return (
    <div
      ref={containerRef}
      className="chat-messages"
      style={{ width: '100%' }}
    >
      <div className="chat-inner" style={{ width: '100%' }}>
        {messages.map((msg, idx) => {
          const isLastUser = idx === lastUserMsgIdx;
          const items = messageContextMenuItems?.(msg);
          const bubble = <MessageBubble key={msg.id} message={msg} />;
          const wrapped = isLastUser ? (
            <div key={msg.id} ref={lastUserMsgRef}>
              {bubble}
            </div>
          ) : bubble;

          if (items && items.length > 0 && !isLastUser) {
            return (
              <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
                {bubble}
              </Dropdown>
            );
          }
          if (items && items.length > 0 && isLastUser) {
            return (
              <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
                <div ref={lastUserMsgRef}>{bubble}</div>
              </Dropdown>
            );
          }
          return wrapped;
        })}
      </div>
      {lastUserMsgIdx >= 0 && (
        <LastUserMessageStickyBar
          lastUserMessage={messages[lastUserMsgIdx]}
          messageRef={lastUserMsgRef}
          containerRef={containerRef}
        />
      )}
    </div>
  );
}
```

Key changes from current:
- Added `lastUserMsgRef` to track the DOM node of the last user message
- Added `lastUserMsgIdx` computation
- The last user message's wrapper div gets the `ref`
- Context menu wrapping handles the last user message case separately to preserve the ref
- `LastUserMessageStickyBar` rendered inside the scrollable container (so `position: sticky` works relative to it)

- [ ] **Step 2: Commit**

```bash
git add web/src/components/chat/MessageList.tsx
git commit -m "feat(ui): integrate sticky bar into MessageList"
```

---

### Task 6: Manual testing and visual verification

**Files:** No code changes

- [ ] **Step 1: Start the dev environment**

Ensure Docker Compose is running:
```bash
docker compose -f deploy/dependencies/dev/docker-compose.yaml ps
```
If not running:
```bash
docker compose -f deploy/dependencies/dev/docker-compose.yaml up -d
```

- [ ] **Step 2: Start the backend server**

```bash
go run ./server/cmd/server/main.go
```

- [ ] **Step 3: Start the frontend dev server**

```bash
cd web && npm run dev
```

- [ ] **Step 4: Visual verification checklist**

Open the browser to `http://localhost:3000` (or your frontend dev port) and verify:

1. **User messages right-aligned**: Send a message — it should appear as a purple bubble on the right
2. **Assistant messages left-aligned**: The response should appear as a dark bubble on the left with a robot avatar
3. **Timestamps**: User timestamps inside bubble (bottom-right), assistant timestamps below bubble (left)
4. **Sticky bar appears**: After sending a user message, scroll up until the message is no longer visible — the sticky bar should appear at the top
5. **Sticky bar disappears**: Scroll back to bottom — the sticky bar should disappear
6. **Long message collapse**: Send a long message (>100px), scroll up — the sticky bar should show gradient + "展开全部" button
7. **Expand/collapse**: Click "展开全部" — content expands, "▲ 折叠" appears. Click it — collapses back
8. **Close button**: Click ✕ — sticky bar closes and stays closed even after scrolling up/down
9. **Back to message**: Click "↩ 回到消息" — scrolls to the original user message
10. **Theme support**: Switch between light and dark themes — colors should adapt correctly

- [ ] **Step 5: Commit any fixes from testing**

```bash
git add -A
git commit -m "fix(ui): address visual issues from manual testing"
```
