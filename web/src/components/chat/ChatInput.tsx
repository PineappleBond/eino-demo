'use client';

import { useState, useRef, useEffect, useCallback } from 'react';
import { Button } from 'antd';
import { SendOutlined, StopOutlined, CloseOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

const TEXTAREA_MAX_HEIGHT = 120;

interface MentionMember {
  id: string;
  name: string;
}

interface ChatInputProps {
  onSend: (content: string, mentionedMembers?: string[]) => Promise<void>;
  onStop?: () => Promise<void>;
  /** True while the agent is actively streaming — shows the stop button. */
  isStreaming?: boolean;
  /** True while the POST request is in-flight — disables send button. */
  isSending?: boolean;
  mentions?: MentionMember[];
  onRemoveMention?: (id: string) => void;
}

export function ChatInput({ onSend, onStop, isStreaming, isSending, mentions, onRemoveMention }: ChatInputProps) {
  const [text, setText] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const t = useTranslations('chat');

  const adjustHeight = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, TEXTAREA_MAX_HEIGHT) + 'px';
  }, []);

  useEffect(() => {
    adjustHeight();
  }, [text, adjustHeight]);

  const handleSend = async () => {
    if (!text.trim() || isSending) return;
    const content = text.trim();
    const mentionedIds = mentions?.map((m) => m.id);
    setText('');
    await onSend(content, mentionedIds);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="chat-input-area">
      <div className="chat-input-inner">
        {mentions && mentions.length > 0 && (
          <div className="chat-input-mentions">
            {mentions.map((m) => (
              <span key={m.id} className="mention-tag">
                @{m.name}
                {onRemoveMention && (
                  <span
                    className="mention-tag-remove"
                    onClick={() => onRemoveMention(m.id)}
                  >
                    <CloseOutlined style={{ fontSize: 10 }} />
                  </span>
                )}
              </span>
            ))}
          </div>
        )}
        <div className="chat-input-box">
          <textarea
            ref={textareaRef}
            className="chat-input-textarea"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('placeholder')}
            aria-label={t('placeholder')}
            rows={1}
            style={{ height: 'auto', overflowY: 'auto' }}
          />
          <div className="chat-input-actions">
            {isStreaming && (
              <Button
                type="primary"
                danger
                icon={<StopOutlined />}
                onClick={onStop}
                size="small"
                style={{ marginRight: 8 }}
              >
                {t('stop')}
              </Button>
            )}
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={handleSend}
              disabled={!text.trim() || isSending}
              size="small"
            >
              {t('send')}
            </Button>
          </div>
        </div>
        <div className="chat-input-hint">
          <kbd>Enter</kbd> to send, <kbd>Shift+Enter</kbd> for new line
        </div>
      </div>
    </div>
  );
}
