'use client';

import { useEffect, useRef } from 'react';
import { Message } from '@/lib/api';
import { MessageBubble } from './MessageBubble';

export function MessageList({ messages, isStreaming, onMessageContextMenu }: {
  messages: Message[];
  isStreaming?: boolean;
  onMessageContextMenu?: (e: React.MouseEvent, msg: Message) => void;
}) {
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
      className="chat-messages"
      style={{ width: '100%' }}
    >
      <div className="chat-inner" style={{ width: '100%' }}>
        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} onContextMenu={onMessageContextMenu} />
        ))}
      </div>
    </div>
  );
}
