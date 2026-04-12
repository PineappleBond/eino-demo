'use client';

import { useEffect, useRef, useState, useCallback, memo } from 'react';
import { Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';
import { LastUserMessageStickyBar } from './LastUserMessageStickyBar';

const MessageListInner = memo(function MessageList({ messages, isStreaming, messageContextMenuItems }: {
  messages: Message[];
  isStreaming?: boolean;
  messageContextMenuItems?: (msg: Message) => MenuProps['items'];
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const [userAtBottom, setUserAtBottom] = useState(true);

  // Find the last user message index in a single pass
  let lastUserMsgIdx = -1;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].sender_role === 'user') {
      lastUserMsgIdx = i;
      break;
    }
  }
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
    handleScroll(); // set correct value on mount
    el.addEventListener('scroll', handleScroll, { passive: true });
    return () => el.removeEventListener('scroll', handleScroll);
  }, []);

  // Auto-scroll only when user is at the bottom
  // Use rAF so scrollHeight reflects painted DOM, not stale layout
  useEffect(() => {
    if (!userAtBottom) return;
    requestAnimationFrame(() => {
      containerRef.current?.scrollTo({ top: containerRef.current.scrollHeight });
    });
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
});

export const MessageList = MessageListInner;
