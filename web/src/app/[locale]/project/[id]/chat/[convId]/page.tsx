'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams } from 'next/navigation';
import { Spin, Result, message } from 'antd';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ChatInput } from '@/components/chat/ChatInput';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';

export default function ConvChatPage() {
  const params = useParams();
  const convId = params.convId as string;
  const [messages, setMessages] = useState<MessageType[]>([]);
  const [streamingContent, setStreamingContent] = useState<string | null>(null);
  const [isStreaming, setIsStreaming] = useState(false);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const t = useTranslations('chat');
  const streamingMsgIdRef = useRef<string | null>(null);
  const [messageApi, contextHolder] = message.useMessage();

  const handleStreamingUpdate = useCallback((update: Update) => {
    switch (update.type) {
      case 'message.new': {
        const payload = update.payload as Record<string, unknown>;
        const msgId = payload.id as string | undefined;
        setMessages((prev) => [
          ...prev,
          {
            id: msgId || `stream-${Date.now()}`,
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
        streamingMsgIdRef.current = msgId || null;
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
        streamingMsgIdRef.current = null;
        setStreamingContent(null);
        api.get<MessageType[]>(`/conversations/${convId}/messages`)
          .then(setMessages)
          .catch(() => {});
        break;
      }
      case 'message.stop': {
        setIsStreaming(false);
        streamingMsgIdRef.current = null;
        setSending(false);
        break;
      }
      case 'message.error': {
        const payload = update.payload as Record<string, unknown>;
        messageApi.error((payload.error as string) || 'Stream error');
        setIsStreaming(false);
        streamingMsgIdRef.current = null;
        setSending(false);
        break;
      }
    }
  }, [convId, messageApi]);

  useSubscribe(`conv:${convId}`, handleStreamingUpdate);

  useEffect(() => {
    api.get<MessageType[]>(`/conversations/${convId}/messages`)
      .then(setMessages)
      .catch((err) => messageApi.error(err.message))
      .finally(() => setLoading(false));
  }, [convId, messageApi]);

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
      messageApi.error(err instanceof Error ? err.message : 'Failed to send');
      setMessages((prev) => prev.filter((m) => m.id !== tempMsg.id));
      setSending(false);
    }
  }, [convId, messageApi]);

  const handleStop = useCallback(async () => {
    try {
      await api.post(`/conversations/${convId}/stop`);
      messageApi.info('Stopped');
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to stop');
    } finally {
      setSending(false);
    }
  }, [convId, messageApi]);

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  const displayMessages = [...messages];
  if (isStreaming && streamingContent !== null) {
    const lastMsg = displayMessages[displayMessages.length - 1];
    if (lastMsg && lastMsg.sender_role === 'assistant') {
      displayMessages[displayMessages.length - 1] = { ...lastMsg, content: streamingContent };
    }
  }

  return (
    <>
      {contextHolder}
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        {displayMessages.length === 0 && !isStreaming ? (
          <Result
            subTitle={t('noMessages')}
            style={{ flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center' }}
          />
        ) : (
          <MessageList messages={displayMessages} isStreaming={isStreaming} />
        )}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          isLoading={sending || isStreaming}
        />
      </div>
    </>
  );
}
