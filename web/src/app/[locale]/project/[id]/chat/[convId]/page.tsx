'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams } from 'next/navigation';
import { Spin, Result, App } from 'antd';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ChatInput } from '@/components/chat/ChatInput';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';

export default function ConvChatPage() {
  const params = useParams();
  const convId = params.convId as string;
  const { message } = App.useApp();
  const t = useTranslations('chat');

  const [messages, setMessages] = useState<MessageType[]>([]);
  const [streamingContent, setStreamingContent] = useState<string | null>(null);
  const [isStreaming, setIsStreaming] = useState(false);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const streamingMsgIdRef = useRef<string | null>(null);

  // Handle streaming updates from WebSocket
  const handleStreamingUpdate = useCallback((update: Update) => {
    switch (update.type) {
      case 'message.new': {
        const payload = update.payload as Record<string, unknown>;
        setMessages((prev) => [
          ...prev,
          {
            id: (payload.id as string) || `stream-${Date.now()}`,
            conversation_id: convId,
            seq: update.seq,
            sender_role: 'assistant',
            sender_id: '',
            content: '',
            reason_content: '',
            metadata: {},
            finish_reason: null,
            error_message: null,
            duration_ms: null,
            token_prompt: 0,
            token_completion: 0,
            created_at: new Date().toISOString(),
          },
        ]);
        setStreamingContent('');
        setIsStreaming(true);
        break;
      }
      case 'message.delta': {
        const payload = update.payload as Record<string, unknown>;
        const delta = (payload.content as string) || '';
        setStreamingContent((prev) => (prev || '') + delta);
        break;
      }
      case 'message.done': {
        setIsStreaming(false);
        setStreamingContent(null);
        api.get<MessageType[]>(`/conversations/${convId}/messages`)
          .then(setMessages)
          .catch(() => {});
        break;
      }
      case 'message.stop': {
        setIsStreaming(false);
        setStreamingContent(null);
        setSending(false);
        break;
      }
      case 'message.error': {
        const payload = update.payload as Record<string, unknown>;
        message.error((payload.error as string) || 'Stream error');
        setIsStreaming(false);
        setSending(false);
        break;
      }
    }
  }, [convId, message]);

  useSubscribe(`conv:${convId}`, handleStreamingUpdate);

  // Initial load
  useEffect(() => {
    api.get<MessageType[]>(`/conversations/${convId}/messages`)
      .then(setMessages)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [convId, message]);

  const handleSend = useCallback(async (content: string) => {
    setSending(true);
    const tempMsg: MessageType = {
      id: `temp-${Date.now()}`,
      conversation_id: convId,
      seq: 0,
      sender_role: 'user',
      sender_id: '',
      content,
      reason_content: '',
      metadata: {},
      finish_reason: null,
      error_message: null,
      duration_ms: null,
      token_prompt: 0,
      token_completion: 0,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempMsg]);

    try {
      await api.post(`/conversations/${convId}/messages`, { content });
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to send');
      setMessages((prev) => prev.filter((m) => m.id !== tempMsg.id));
      setSending(false);
    }
  }, [convId, message]);

  const handleStop = useCallback(async () => {
    try {
      await api.post(`/conversations/${convId}/stop`);
      message.info('Stopped');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to stop');
    } finally {
      setSending(false);
    }
  }, [convId, message]);

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  // Merge streaming content into the last message for display
  const displayMessages = [...messages];
  if (isStreaming && streamingContent !== null) {
    const lastMsg = displayMessages[displayMessages.length - 1];
    if (lastMsg && lastMsg.sender_role === 'assistant') {
      displayMessages[displayMessages.length - 1] = { ...lastMsg, content: streamingContent };
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {displayMessages.length === 0 && !isStreaming ? (
        <Result
          subTitle={t('noMessages')}
          style={{
            flex: 1,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            color: 'var(--text-secondary)',
          }}
        />
      ) : (
        <MessageList messages={displayMessages} isStreaming={isStreaming} />
      )}
      <ChatInput
        onSend={handleSend}
        onStop={handleStop}
        isLoading={isStreaming || sending}
      />
    </div>
  );
}
