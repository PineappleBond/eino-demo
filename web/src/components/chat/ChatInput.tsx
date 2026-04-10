'use client';

import { useState } from 'react';
import { Button } from 'antd';
import { SendOutlined, StopOutlined, CloseOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

interface MentionMember {
  id: string;
  name: string;
}

interface ChatInputProps {
  onSend: (content: string, mentionedMembers?: string[]) => Promise<void>;
  onStop?: () => Promise<void>;
  isLoading?: boolean;
  mentions?: MentionMember[];
  onRemoveMention?: (id: string) => void;
}

export function ChatInput({ onSend, onStop, isLoading, mentions, onRemoveMention }: ChatInputProps) {
  const [text, setText] = useState('');
  const t = useTranslations('chat');

  const handleSend = async () => {
    if (!text.trim() || isLoading) return;
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
            className="chat-input-textarea"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('placeholder')}
            rows={1}
            disabled={isLoading}
          />
          <div className="chat-input-actions">
            {isLoading ? (
              <Button
                type="primary"
                danger
                icon={<StopOutlined />}
                onClick={onStop}
                size="small"
              >
                {t('stop')}
              </Button>
            ) : (
              <Button
                type="primary"
                icon={<SendOutlined />}
                onClick={handleSend}
                disabled={!text.trim()}
                size="small"
              >
                {t('send')}
              </Button>
            )}
          </div>
        </div>
        <div className="chat-input-hint">
          <kbd>Enter</kbd> to send, <kbd>Shift+Enter</kbd> for new line
        </div>
      </div>
    </div>
  );
}
