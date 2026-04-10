'use client';

import { useState } from 'react';
import { Button } from 'antd';
import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

interface ChatInputProps {
  onSend: (content: string) => Promise<void>;
  onStop?: () => Promise<void>;
  isLoading?: boolean;
}

export function ChatInput({ onSend, onStop, isLoading }: ChatInputProps) {
  const [text, setText] = useState('');
  const t = useTranslations('chat');

  const handleSend = async () => {
    if (!text.trim() || isLoading) return;
    const content = text.trim();
    setText('');
    await onSend(content);
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
