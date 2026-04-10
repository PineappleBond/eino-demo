'use client';

import { useEffect, useRef, useState } from 'react';
import { Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';

export function MessageList({ messages, isStreaming, messageContextMenuItems }: {
  messages: Message[];
  isStreaming?: boolean;
  messageContextMenuItems?: (msg: Message) => MenuProps['items'];
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [userAtBottom, setUserAtBottom] = useState(true);

  // Track whether user is scrolled near the bottom
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const handleScroll = () => {
      const threshold = 80; // pixels from bottom
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
        {messages.map((msg) => {
          const items = messageContextMenuItems?.(msg);
          const bubble = <MessageBubble key={msg.id} message={msg} />;
          if (items && items.length > 0) {
            return (
              <Dropdown key={msg.id} menu={{ items }} trigger={['contextMenu']}>
                {bubble}
              </Dropdown>
            );
          }
          return bubble;
        })}
      </div>
    </div>
  );
}
