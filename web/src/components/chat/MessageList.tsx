'use client';

import { useEffect, useRef } from 'react';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';

export function MessageList({ messages }: { messages: Message[] }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [messages]);

  if (messages.length === 0) {
    return null;
  }

  return (
    <div
      ref={containerRef}
      style={{
        flex: 1,
        overflowY: 'auto',
        padding: '16px 0',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {messages.map((msg) => (
        <MessageBubble key={msg.id} message={msg} />
      ))}
    </div>
  );
}
