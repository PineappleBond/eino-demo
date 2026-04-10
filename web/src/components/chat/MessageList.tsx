'use client';

import { useEffect, useRef } from 'react';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';

export function MessageList({ messages, isStreaming }: { messages: Message[]; isStreaming?: boolean }) {
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
        padding: '24px 0',
      }}
    >
      <div style={{ maxWidth: 720, margin: '0 auto', padding: '0 24px' }}>
        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} />
        ))}
      </div>
    </div>
  );
}
