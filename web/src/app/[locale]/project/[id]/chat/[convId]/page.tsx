'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams } from 'next/navigation';
import { Spin, Result, App } from 'antd';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ChatInput } from '@/components/chat/ChatInput';
import { ConvInfoPanel } from '@/components/chat/ConvInfoPanel';
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
  const [showConvInfo, setShowConvInfo] = useState(false);
  const [contextMenu, setContextMenu] = useState<{
    x: number; y: number; msg: MessageType;
  } | null>(null);
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

  const handleMessageContextMenu = useCallback((e: React.MouseEvent, msg: MessageType) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, msg });
  }, []);

  const closeContextMenu = useCallback(() => setContextMenu(null), []);

  useEffect(() => {
    if (contextMenu) {
      const handler = () => closeContextMenu();
      document.addEventListener('click', handler);
      return () => document.removeEventListener('click', handler);
    }
  }, [contextMenu, closeContextMenu]);

  const handleCopyMessage = useCallback((msg: MessageType) => {
    navigator.clipboard?.writeText(msg.content);
    closeContextMenu();
  }, [closeContextMenu]);

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

  // Stats for ConvInfoPanel
  const stats = messages.reduce((acc, m) => ({
    messages: acc.messages + 1,
    tokenPrompt: acc.tokenPrompt + (m.token_prompt || 0),
    tokenCompletion: acc.tokenCompletion + (m.token_completion || 0),
    toolCalls: acc.toolCalls + ((m.metadata?.tool_calls as Array<unknown> | undefined)?.length || 0),
  }), { messages: 0, tokenPrompt: 0, tokenCompletion: 0, toolCalls: 0 });

  return (
    <div style={{ display: 'flex', flexDirection: 'row', height: '100%' }}>
      {/* Main chat area */}
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%', flex: 1, minWidth: 0, position: 'relative' }}>
        {/* Info toggle button */}
        <button
          className="btn"
          style={{
            position: 'absolute',
            right: 12,
            top: 12,
            zIndex: 10,
            padding: '4px 8px',
            fontSize: 14,
            background: 'var(--bg-tertiary)',
            border: '1px solid var(--border-subtle)',
            color: 'var(--text-tertiary)',
            opacity: 0.5,
          }}
          title={showConvInfo ? 'Hide conversation info' : 'Show conversation info'}
          onClick={() => setShowConvInfo(!showConvInfo)}
        >
          ℹ
        </button>

        {displayMessages.length === 0 && !isStreaming ? (
          <Result
            subTitle={t('noMessages')}
            style={{ flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center' }}
          />
        ) : (
          <MessageList messages={displayMessages} isStreaming={isStreaming} onMessageContextMenu={handleMessageContextMenu} />
        )}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          isLoading={isStreaming || sending}
        />

        {/* Message Context Menu */}
        {contextMenu && (
          <div
            className="context-menu"
            style={{ left: contextMenu.x, top: contextMenu.y }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="context-menu-item" onClick={() => handleCopyMessage(contextMenu.msg)}>
              📋 Copy
            </div>
            <div className="context-menu-divider" />
            <div className="context-menu-item" onClick={() => {
              // TODO: Reply
              closeContextMenu();
            }}>
              ↩ Reply
            </div>
            <div className="context-menu-divider" />
            <div className="context-menu-item" onClick={() => {
              // TODO: Create branch conversation
              closeContextMenu();
            }}>
              🌿 Create Branch Conversation
            </div>
          </div>
        )}
      </div>

      {/* ConvInfo Panel */}
      {showConvInfo && (
        <ConvInfoPanel
          onClose={() => setShowConvInfo(false)}
          stats={stats}
          model="Sonnet"
        />
      )}
    </div>
  );
}
