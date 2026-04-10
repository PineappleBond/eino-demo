'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams } from 'next/navigation';
import { Spin, Result, App } from 'antd';
import type { MenuProps } from 'antd';
import {
  CopyOutlined,
  ForkOutlined,
} from '@ant-design/icons';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ChatInput } from '@/components/chat/ChatInput';
import { StreamingText } from '@/components/chat/StreamingText';
import { ConvInfoPanel } from '@/components/chat/ConvInfoPanel';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';
import { dispatcher } from '@/lib/updateDispatcher';

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
  const [members, setMembers] = useState<Array<{
    id: string;
    member_type: string;
    member_name: string;
    is_owner: boolean;
  }>>([]);
  const [mentions, setMentions] = useState<Array<{
    id: string;
    name: string;
  }>>([]);
  const streamingMsgIdRef = useRef<string | null>(null);

  // Handle streaming updates from WebSocket
  const handleStreamingUpdate = useCallback((update: Update) => {
    switch (update.type) {
      case 'message.new': {
        const payload = update.payload as Record<string, unknown>;
        const senderRole = payload.role as string | undefined;

        // For user messages: the optimistic temp message was already added on HTTP success.
        // The real persisted message arrives here — replace the temp with the real one.
        // For assistant messages: start a new message bubble.
        if (senderRole === 'user') {
          // Replace temp message with the real one
          const realId = (payload.id as string) || '';
          if (realId) {
            setMessages((prev) =>
              prev.map((m) => (m.id.startsWith('temp-') ? { ...m, id: realId } : m))
            );
          }
          return;
        }

        // Assistant message.new — start streaming
        setMessages((prev) => [
          ...prev,
          {
            id: (payload.id as string) || `stream-${Date.now()}`,
            conversation_id: convId,
            seq: update.seq,
            sender_role: 'assistant',
            sender_id: (payload.sender_id as string) || '',
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
        const delta = (payload.delta as string) || '';
        setStreamingContent((prev) => (prev || '') + delta);
        break;
      }
      case 'message.thinking': {
        const payload = update.payload as Record<string, unknown>;
        const delta = (payload.delta as string) || '';
        setMessages((prev) => {
          if (prev.length === 0) return prev;
          const last = prev[prev.length - 1];
          return [
            ...prev.slice(0, -1),
            { ...last, reason_content: last.reason_content + delta },
          ];
        });
        break;
      }
      case 'message.tool_call': {
        const payload = update.payload as Record<string, unknown>;
        const toolCall = {
          name: (payload.tool_name as string) || '',
          input: (payload.input as Record<string, unknown>) || {},
          output: (payload.content as string) || '',
          status: (payload.status as string) || 'done',
        };
        setMessages((prev) => {
          if (prev.length === 0) return prev;
          const last = prev[prev.length - 1];
          const existingCalls = (last.metadata?.tool_calls as Array<unknown> | undefined) || [];
          return [
            ...prev.slice(0, -1),
            { ...last, metadata: { ...last.metadata, tool_calls: [...existingCalls, toolCall] } },
          ];
        });
        break;
      }
      case 'message.done': {
        setIsStreaming(false);
        setStreamingContent(null);
        setSending(false);
        // Refetch messages to get the final persisted state
        api.get<MessageType[]>(`/conversations/${convId}/messages`)
          .then(setMessages)
          .catch((err) => console.error('[Chat] message.done refetch failed', err));
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

  // Fetch members when ConvInfoPanel opens
  useEffect(() => {
    if (showConvInfo) {
      api.get<Array<{ id: string; member_type: string; member_name: string; is_owner: boolean }>>(
        `/conversations/${convId}/members`
      )
        .then(setMembers)
        .catch(() => {});
    }
  }, [showConvInfo, convId]);

  // Initial load
  useEffect(() => {
    api.get<MessageType[]>(`/conversations/${convId}/messages`)
      .then(setMessages)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [convId, message]);

  const handleSend = useCallback(async (content: string, mentionedMembers?: string[]) => {
    setSending(true);

    // Optimistic: show temp message immediately
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
      const resp = await api.post<{ conversation_id: string; message_id: string; seq: number }>(
        `/conversations/${convId}/messages`,
        { content, mentioned_members: mentionedMembers }
      );

      // Remove optimistic temp message
      setMessages((prev) => prev.filter((m) => m.id !== tempMsg.id));

      // Route real message through applyUpdates pipeline
      const update: Update = {
        seq: resp.seq,
        type: 'message.new',
        payload: {
          id: resp.message_id,
          conversation_id: resp.conversation_id,
          content,
          role: 'user',
          sender_id: '',
          reason_content: '',
          metadata: {},
          finish_reason: null,
          error_message: null,
          duration_ms: null,
          token_prompt: 0,
          token_completion: 0,
          created_at: new Date().toISOString(),
        },
      };
      dispatcher.applyUpdates([update]);
      setMentions([]);
      setSending(false);
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

  const handleMemberMention = useCallback((member: { id: string; name: string }) => {
    setMentions((prev) => {
      if (prev.find((m) => m.id === member.id)) return prev;
      return [...prev, member];
    });
  }, []);

  const handleRemoveMention = useCallback((id: string) => {
    setMentions((prev) => prev.filter((m) => m.id !== id));
  }, []);

  const getMessageContextMenu = useCallback((msg: MessageType): MenuProps['items'] => {
    return [
      {
        key: 'copy',
        icon: <CopyOutlined />,
        label: 'Copy',
        onClick: () => {
          navigator.clipboard?.writeText(msg.content);
        },
      },
      { type: 'divider' },
      {
        key: 'reply',
        icon: <ForkOutlined />,
        label: 'Reply',
        // TODO: Implement reply functionality
      },
      { type: 'divider' },
      {
        key: 'branch',
        icon: <ForkOutlined />,
        label: 'Create Branch Conversation',
        // TODO: Implement branch conversation creation
      },
    ];
  }, []);

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  // Merge streaming content into the last message for display
  const displayMessages = [...messages];
  const showStreamingInline = isStreaming && streamingContent !== null;
  if (showStreamingInline) {
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
    <div style={{ display: 'flex', flexDirection: 'row', flex: 1, minWidth: 0, overflow: 'hidden' }}>
      {/* Main chat area */}
      <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0, minHeight: 0, position: 'relative' }}>
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
          <MessageList messages={displayMessages} isStreaming={isStreaming} messageContextMenuItems={getMessageContextMenu} />
        )}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          isLoading={isStreaming || sending}
          mentions={mentions}
          onRemoveMention={handleRemoveMention}
        />
      </div>

      {/* ConvInfo Panel */}
      {showConvInfo && (
        <ConvInfoPanel
          onClose={() => setShowConvInfo(false)}
          members={members.map((m) => ({
            id: m.id,
            name: m.member_name,
            type: m.member_type as 'user' | 'agent',
            color: m.is_owner ? 'var(--accent)' : 'var(--bg-elevated)',
            role: m.is_owner ? 'owner' : 'agent',
          }))}
          stats={stats}
          model="Sonnet"
          onMemberMention={handleMemberMention}
        />
      )}
    </div>
  );
}
