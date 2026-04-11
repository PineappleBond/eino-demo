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

          if (items && items.length > 0) {
            if (isLastUser) {
              return (
                <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
                  <div ref={lastUserMsgRef}>{bubble}</div>
                </Dropdown>
              );
            }
            return (
              <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
                {bubble}
              </Dropdown>
            );
          }

          if (isLastUser) {
            return (
              <div key={msg.id} ref={lastUserMsgRef}>
                {bubble}
              </div>
            );
          }

          return bubble;
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
