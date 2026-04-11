'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
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
  const messagesRef = useRef<HTMLDivElement>(null);
  const [userAtBottom, setUserAtBottom] = useState(true);

  // Find the last user message index
  const lastUserMsgIdx = messages
    .map((m, i) => (m.sender_role === 'user' ? i : -1))
    .filter((i) => i >= 0)
    .pop() ?? -1;
  const lastUserMessage = lastUserMsgIdx >= 0 ? messages[lastUserMsgIdx] : null;

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

  // Render messages with sticky bar inline after the last user message
  const renderMessages = () => {
    const result: React.ReactNode[] = [];
    for (let idx = 0; idx < messages.length; idx++) {
      const msg = messages[idx];
      const isLastUser = idx === lastUserMsgIdx;
      const items = messageContextMenuItems?.(msg);
      const bubble = <MessageBubble key={msg.id} message={msg} />;

      if (items && items.length > 0) {
        if (isLastUser) {
          result.push(
            <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
              <div>{bubble}</div>
            </Dropdown>
          );
        } else {
          result.push(
            <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
              {bubble}
            </Dropdown>
          );
        }
      } else {
        result.push(<div key={msg.id}>{bubble}</div>);
      }

      // Insert sticky bar RIGHT AFTER the last user message
      if (isLastUser && lastUserMessage) {
        result.push(
          <LastUserMessageStickyBar
            key="sticky-bar"
            lastUserMessage={lastUserMessage}
            lastUserMsgIdx={idx}
            containerRef={containerRef}
            messageContainerRef={messagesRef}
          />
        );
      }
    }
    return result;
  };

  return (
    <div
      ref={containerRef}
      className="chat-messages"
      style={{ width: '100%' }}
    >
      <div ref={messagesRef} className="chat-inner" style={{ width: '100%' }}>
        {renderMessages()}
      </div>
    </div>
  );
}
