'use client';

import { useEffect, useReducer, useCallback } from 'react';
import { useParams } from 'next/navigation';
import { Spin, Result, App } from 'antd';
import type { MenuProps } from 'antd';
import {
  CopyOutlined,
  ForkOutlined,
} from '@ant-design/icons';
import { api, Message as MessageType } from '@/lib/api';
import { MessageList } from '@/components/chat/MessageList';
import { ConvInfoPanel } from '@/components/chat/ConvInfoPanel';
import { ChatInput } from '@/components/chat/ChatInput';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';
import type { components } from '@/types/api';
import { dispatcher } from '@/lib/updateDispatcher';
import { saveMessages, getMessages } from '@/store/indexedDB';

// ─── State types ───

interface ChatState {
  messages: MessageType[];
  streamingMessageId: string | null;
  isStreaming: boolean;
  loading: boolean;
  sending: boolean;
  showConvInfo: boolean;
  members: Array<{
    id: string;
    member_type: string;
    member_name: string;
    is_owner: boolean;
  }>;
  mentions: Array<{ id: string; name: string }>;
}

type ChatAction =
  | { type: 'SET_LOADING'; payload: boolean }
  | { type: 'SET_MESSAGES'; payload: MessageType[] }
  | { type: 'ADD_MESSAGE'; payload: MessageType }
  | { type: 'SET_STREAMING_MESSAGE_ID'; payload: string | null }
  | { type: 'APPEND_STREAMING_CONTENT'; payload: { delta: string; messageId: string } }
  | { type: 'APPEND_REASON_TO_MESSAGE'; payload: { delta: string; messageId: string } }
  | { type: 'ADD_TOOL_CALL_TO_LAST_MESSAGE'; payload: Record<string, unknown> }
  | { type: 'ADD_TEMP_MESSAGE'; payload: MessageType }
  | { type: 'REMOVE_TEMP_MESSAGE'; payload: string }
  | { type: 'REPLACE_TEMP_MESSAGE'; payload: { tempPrefix: string; message: MessageType } }
  | { type: 'SET_IS_STREAMING'; payload: boolean }
  | { type: 'SET_SENDING'; payload: boolean }
  | { type: 'TOGGLE_CONV_INFO' }
  | { type: 'SET_MEMBERS'; payload: ChatState['members'] }
  | { type: 'ADD_MENTION'; payload: { id: string; name: string } }
  | { type: 'REMOVE_MENTION'; payload: string }
  | { type: 'CLEAR_MENTIONS' }
  | { type: 'STOP_STREAMING' };

function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case 'SET_LOADING':
      return { ...state, loading: action.payload };
    case 'SET_MESSAGES':
      return { ...state, messages: action.payload };
    case 'ADD_MESSAGE':
      if (state.messages.some((m) => m.id === action.payload.id)) return state;
      return { ...state, messages: [...state.messages, action.payload] };
    case 'APPEND_STREAMING_CONTENT': {
      const idx = state.messages.findIndex((m) => m.id === action.payload.messageId);
      if (idx === -1) return { ...state, streamingMessageId: action.payload.messageId };
      const msgs = [...state.messages];
      const msg = msgs[idx];
      msgs[idx] = { ...msg, content: (msg.content || '') + action.payload.delta };
      return { ...state, messages: msgs, streamingMessageId: action.payload.messageId };
    }
    case 'SET_STREAMING_MESSAGE_ID':
      return { ...state, streamingMessageId: action.payload };
    case 'APPEND_REASON_TO_MESSAGE': {
      const idx = state.messages.findIndex((m) => m.id === action.payload.messageId);
      if (idx === -1) return state;
      const msgs = [...state.messages];
      const msg = msgs[idx];
      msgs[idx] = { ...msg, reason_content: (msg.reason_content || '') + action.payload.delta };
      return { ...state, messages: msgs };
    }
    case 'ADD_TOOL_CALL_TO_LAST_MESSAGE': {
      if (state.messages.length === 0) return state;
      const msgs = [...state.messages];
      const last = msgs[msgs.length - 1];
      const existingCalls = ((last.metadata as Record<string, unknown> | undefined)?.tool_calls as Array<unknown> | undefined) || [];
      msgs[msgs.length - 1] = {
        ...last,
        metadata: { ...(last.metadata as object), tool_calls: [...existingCalls, action.payload] },
      };
      return { ...state, messages: msgs };
    }
    case 'ADD_TEMP_MESSAGE':
      return { ...state, messages: [...state.messages, action.payload] };
    case 'REMOVE_TEMP_MESSAGE':
      return { ...state, messages: state.messages.filter((m) => m.id !== action.payload) };
    case 'REPLACE_TEMP_MESSAGE': {
      const { tempPrefix, message } = action.payload;
      return {
        ...state,
        messages: state.messages.map((m) => (m.id.startsWith(tempPrefix) ? { ...m, id: message.id } : m)),
      };
    }
    case 'SET_IS_STREAMING':
      return { ...state, isStreaming: action.payload };
    case 'SET_SENDING':
      return { ...state, sending: action.payload };
    case 'TOGGLE_CONV_INFO':
      return { ...state, showConvInfo: !state.showConvInfo };
    case 'SET_MEMBERS':
      return { ...state, members: action.payload };
    case 'ADD_MENTION':
      if (state.mentions.find((m) => m.id === action.payload.id)) return state;
      return { ...state, mentions: [...state.mentions, action.payload] };
    case 'REMOVE_MENTION':
      return { ...state, mentions: state.mentions.filter((m) => m.id !== action.payload) };
    case 'CLEAR_MENTIONS':
      return { ...state, mentions: [] };
    case 'STOP_STREAMING':
      return { ...state, isStreaming: false, streamingMessageId: null, sending: false };
    default:
      return state;
  }
}

const initialState: ChatState = {
  messages: [],
  streamingMessageId: null,
  isStreaming: false,
  loading: true,
  sending: false,
  showConvInfo: false,
  members: [],
  mentions: [],
};

export default function ConvChatPage() {
  const params = useParams();
  const convId = params.convId as string;
  const { message } = App.useApp();
  const t = useTranslations('chat');

  const [state, dispatch] = useReducer(chatReducer, initialState);

  // ─── Streaming update handler ───

  const handleStreamingUpdate = useCallback((update: Update) => {
    switch (update.type) {
      case 'message.new': {
        const payload = update.payload as components['schemas']['MessageNewPayload'];
        const senderRole = payload.role as string | undefined;

        if (senderRole === 'user') {
          const realId = payload.message_id || '';
          if (!realId) return;

          // Temp message (if any) is already removed by the sync dispatch in
          // handleSend before applyUpdates processes. Always add the real
          // message with full content from the payload.
          dispatch({
            type: 'ADD_MESSAGE',
            payload: {
              id: realId,
              conversation_id: payload.conversation_id || '',
              seq: (payload as Record<string, unknown>).seq as number | undefined,
              sender_role: 'user',
              sender_id: payload.sender_id || '',
              content: (payload.content as string | undefined) || '',
              reason_content: '',
              metadata: (payload.metadata as Record<string, unknown> | undefined) || {},
              finish_reason: null,
              error_message: null,
              duration_ms: null,
              token_prompt: 0,
              token_completion: 0,
              created_at: new Date().toISOString(),
            },
          });
          return;
        }

        // Assistant message.new — start streaming
        const msgId = payload.message_id || `stream-${Date.now()}`;
        const alreadyExists = state.messages.some((m) => m.id === msgId);

        if (!alreadyExists) {
          const newMsg: MessageType = {
            id: msgId,
            conversation_id: convId,
            seq: update.seq,
            sender_role: 'assistant',
            sender_id: payload.sender_id || '',
            content: '',
            reason_content: '',
            metadata: {},
            finish_reason: null,
            error_message: null,
            duration_ms: null,
            token_prompt: 0,
            token_completion: 0,
            created_at: new Date().toISOString(),
          };
          dispatch({ type: 'ADD_MESSAGE', payload: newMsg });
        }
        dispatch({ type: 'SET_STREAMING_MESSAGE_ID', payload: msgId });
        dispatch({ type: 'SET_IS_STREAMING', payload: true });
        break;
      }
      case 'message.delta': {
        const payload = update.payload as components['schemas']['MessageDeltaPayload'];
        const messageId = (payload as Record<string, unknown>).message_id as string | undefined;
        dispatch({ type: 'APPEND_STREAMING_CONTENT', payload: { delta: payload.delta || '', messageId: messageId || '' } });
        break;
      }
      case 'message.thinking': {
        const payload = update.payload as components['schemas']['MessageThinkingPayload'];
        const messageId = (payload as Record<string, unknown>).message_id as string | undefined;
        dispatch({ type: 'APPEND_REASON_TO_MESSAGE', payload: { delta: payload.delta || '', messageId: messageId || '' } });
        break;
      }
      case 'message.tool_call': {
        const payload = update.payload as components['schemas']['MessageToolCallPayload'];
        const toolCall = {
          name: payload.tool_name || '',
          input: {} as Record<string, unknown>,
          output: payload.content || '',
          status: 'done',
        };
        dispatch({ type: 'ADD_TOOL_CALL_TO_LAST_MESSAGE', payload: toolCall });
        break;
      }
      case 'message.done': {
        dispatch({ type: 'SET_IS_STREAMING', payload: false });
        dispatch({ type: 'SET_SENDING', payload: false });
        // Refetch and persist to IndexedDB
        api.get<MessageType[]>(`/conversations/${convId}/messages`)
          .then((msgs) => {
            dispatch({ type: 'SET_MESSAGES', payload: msgs });
            saveMessages(msgs.map((m) => ({ id: m.id, data: m as Record<string, unknown> }))).catch(() => {});
          })
          .catch((err) => console.error('[Chat] message.done refetch failed', err));
        break;
      }
      case 'message.stop':
        dispatch({ type: 'STOP_STREAMING' });
        break;
      case 'message.error': {
        const payload = update.payload as components['schemas']['MessageErrorPayload'];
        message.error(payload.error || 'Stream error');
        dispatch({ type: 'STOP_STREAMING' });
        break;
      }
      case 'conversation.compacting':
        message.loading('Compacting conversation...', 0);
        break;
      case 'conversation.compacted': {
        message.destroy();
        const payload = update.payload as components['schemas']['ConversationCompactedPayload'];
        const newConvId = payload.new_conv_id as string | undefined;
        if (newConvId) {
          window.location.href = window.location.pathname.replace(/\/chat\/[^/]+$/, `/chat/${newConvId}`);
        }
        break;
      }
    }
  }, [convId, message]);

  useSubscribe(`conv:${convId}`, handleStreamingUpdate);

  // ─── Fetch members when ConvInfoPanel opens ───

  useEffect(() => {
    if (!state.showConvInfo) return;
    api.get<Array<{ id: string; member_type: string; member_name: string; is_owner: boolean }>>(
      `/conversations/${convId}/members`
    )
      .then((data) => dispatch({ type: 'SET_MEMBERS', payload: data }))
      .catch(() => {});
  }, [state.showConvInfo, convId]);

  // ─── Initial load (try IndexedDB first, fallback to HTTP) ───

  useEffect(() => {
    let cancelled = false;
    const loadMessages = async () => {
      // Try IndexedDB first for offline support
      try {
        const cached = await getMessages(convId);
        if (!cancelled && cached.length > 0) {
          dispatch({ type: 'SET_MESSAGES', payload: cached as MessageType[] });
          dispatch({ type: 'SET_LOADING', payload: false });
        }
      } catch {
        // IndexedDB not available, fall through to HTTP
      }

      // Always refresh from server for latest data
      api.get<MessageType[]>(`/conversations/${convId}/messages`)
        .then((msgs) => {
          if (cancelled) return;
          dispatch({ type: 'SET_MESSAGES', payload: msgs });
          // Persist to IndexedDB
          saveMessages(msgs.map((m) => ({ id: m.id, data: m as Record<string, unknown> }))).catch(() => {});
        })
        .catch((err) => {
          if (cancelled) return;
          message.error(err.message);
        })
        .finally(() => {
          if (!cancelled) dispatch({ type: 'SET_LOADING', payload: false });
        });
    };
    loadMessages();
    return () => { cancelled = true; };
  }, [convId, message]);

  // ─── Send message ───

  const handleSend = useCallback(async (content: string, mentionedMembers?: string[]) => {
    dispatch({ type: 'SET_SENDING', payload: true });

    const tempId = `temp-${Date.now()}`;
    const tempMsg: MessageType = {
      id: tempId,
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
    dispatch({ type: 'ADD_TEMP_MESSAGE', payload: tempMsg });

    try {
      const resp = await api.post<{ conversation_id: string; message_id: string; seq: number }>(
        `/conversations/${convId}/messages`,
        { content, mentioned_members: mentionedMembers }
      );

      dispatch({ type: 'REMOVE_TEMP_MESSAGE', payload: tempId });
      dispatch({ type: 'CLEAR_MENTIONS' });

      // Route real message through applyUpdates pipeline for seq tracking
      // and notification of other subscribers. Note: applyUpdates may skip
      // this update if there's a seq gap (localMaxSeq + 1 !== resp.seq), in
      // which case handleStreamingUpdate's ADD_MESSAGE never fires. We
      // dispatch ADD_MESSAGE unconditionally below to ensure the user's
      // message always appears; the reducer's dedup check prevents doubles.
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
      } as never;
      dispatcher.applyUpdates([update]);

      // Ensure the real message is added regardless of seq continuity
      dispatch({
        type: 'ADD_MESSAGE',
        payload: {
          id: resp.message_id,
          conversation_id: convId,
          seq: resp.seq,
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
        },
      });
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to send');
      dispatch({ type: 'REMOVE_TEMP_MESSAGE', payload: tempId });
    } finally {
      dispatch({ type: 'SET_SENDING', payload: false });
    }
  }, [convId, message]);

  // ─── Stop streaming ───

  const handleStop = useCallback(async () => {
    try {
      await api.post(`/conversations/${convId}/stop`);
      message.info('Stopped');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to stop');
    } finally {
      // Fallback: reset state even if message.stop Update is missed
      dispatch({ type: 'STOP_STREAMING' });
    }
  }, [convId, message]);

  // ─── Mentions ───

  const handleMemberMention = useCallback((member: { id: string; name: string }) => {
    dispatch({ type: 'ADD_MENTION', payload: member });
  }, []);

  const handleRemoveMention = useCallback((id: string) => {
    dispatch({ type: 'REMOVE_MENTION', payload: id });
  }, []);

  // ─── Context menu ───

  const getMessageContextMenu = useCallback((msg: MessageType): MenuProps['items'] => {
    return [
      {
        key: 'copy',
        icon: <CopyOutlined />,
        label: t('copy'),
        onClick: () => {
          navigator.clipboard?.writeText(msg.content);
        },
      },
      { type: 'divider' },
      {
        key: 'reply',
        icon: <ForkOutlined />,
        label: t('reply'),
      },
      { type: 'divider' },
      {
        key: 'branch',
        icon: <ForkOutlined />,
        label: t('createBranch'),
      },
    ];
  }, [t]);

  // ─── Render ───

  if (state.loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  const displayMessages = state.messages;

  // Stats for ConvInfoPanel
  const stats = state.messages.reduce((acc, m) => ({
    messages: acc.messages + 1,
    tokenPrompt: acc.tokenPrompt + (m.token_prompt || 0),
    tokenCompletion: acc.tokenCompletion + (m.token_completion || 0),
    toolCalls: acc.toolCalls + (((m.metadata as Record<string, unknown> | undefined)?.tool_calls as Array<unknown> | undefined)?.length || 0),
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
          title={state.showConvInfo ? 'Hide conversation info' : 'Show conversation info'}
          onClick={() => dispatch({ type: 'TOGGLE_CONV_INFO' })}
        >
          ℹ
        </button>

        {displayMessages.length === 0 && !state.isStreaming ? (
          <Result
            subTitle={t('noMessages')}
            style={{ flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center' }}
          />
        ) : (
          <MessageList
            messages={displayMessages}
            isStreaming={state.isStreaming}
            messageContextMenuItems={getMessageContextMenu}
          />
        )}
        <ChatInput
          onSend={handleSend}
          onStop={handleStop}
          isLoading={state.isStreaming || state.sending}
          mentions={state.mentions}
          onRemoveMention={handleRemoveMention}
        />
      </div>

      {/* ConvInfo Panel */}
      {state.showConvInfo && (
        <ConvInfoPanel
          onClose={() => dispatch({ type: 'TOGGLE_CONV_INFO' })}
          members={state.members.map((m) => ({
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
