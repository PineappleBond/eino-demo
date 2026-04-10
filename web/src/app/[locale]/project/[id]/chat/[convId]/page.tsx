'use client';

import { useEffect, useState, useCallback } from 'react';
import { useParams } from 'next/navigation';
import { Layout, Spin, Result, message } from 'antd';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ChatInput } from '@/components/chat/ChatInput';
import { useTranslations } from 'next-intl';

const { Content } = Layout;

export default function ConvChatPage() {
  const params = useParams();
  const convId = params.convId as string;
  const [messages, setMessages] = useState<MessageType[]>([]);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const t = useTranslations('chat');

  useEffect(() => {
    api.get<MessageType[]>(`/conversations/${convId}/messages`)
      .then(setMessages)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [convId]);

  const handleSend = useCallback(async (content: string) => {
    setSending(true);
    // Optimistically add user message
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
    } finally {
      setSending(false);
    }
  }, [convId]);

  const handleStop = useCallback(async () => {
    try {
      await api.post(`/conversations/${convId}/stop`);
      message.info('Stopped');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to stop');
    } finally {
      setSending(false);
    }
  }, [convId]);

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  return (
    <Content style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 52px)' }}>
      {messages.length === 0 ? (
        <Result
          subTitle={t('noMessages')}
          style={{ flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center' }}
        />
      ) : (
        <MessageList messages={messages} />
      )}
      <ChatInput
        onSend={handleSend}
        onStop={handleStop}
        isLoading={sending}
      />
    </Content>
  );
}
