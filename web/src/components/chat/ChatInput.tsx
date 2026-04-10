'use client';

import { useState } from 'react';
import { Button, Input, Typography } from 'antd';
import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

const { TextArea } = Input;
const { Text } = Typography;

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
    <div style={{
      borderTop: '1px solid var(--border-subtle)',
      background: 'var(--bg-secondary)',
      padding: '12px 24px 16px',
      flexShrink: 0,
    }}>
      <div style={{ maxWidth: 720, margin: '0 auto' }}>
        <div style={{
          display: 'flex',
          alignItems: 'flex-end',
          gap: 8,
          background: 'var(--bg-tertiary)',
          border: '1px solid var(--border-default)',
          borderRadius: 'var(--radius-lg)',
          padding: '8px 12px',
          transition: 'border-color 0.15s ease',
        }}>
          <TextArea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('placeholder')}
            autoSize={{ minRows: 1, maxRows: 4 }}
            disabled={isLoading}
            style={{
              flex: 1,
              resize: 'none',
              background: 'transparent',
              border: 'none',
              outline: 'none',
              color: 'var(--text-primary)',
              fontFamily: 'var(--font-sans)',
              fontSize: 14,
              lineHeight: 1.5,
              boxShadow: 'none',
              padding: 0,
            }}
          />
          <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0 }}>
            {isLoading ? (
              <Button
                type="primary"
                danger
                icon={<StopOutlined />}
                onClick={onStop}
                size="small"
                style={{ borderRadius: 'var(--radius-sm)' }}
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
                style={{ borderRadius: 'var(--radius-sm)' }}
              >
                {t('send')}
              </Button>
            )}
          </div>
        </div>
        <Text style={{
          fontSize: 11,
          color: 'var(--text-tertiary)',
          marginTop: 6,
          display: 'flex',
          alignItems: 'center',
          gap: 4,
        }}>
          <kbd style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            padding: '1px 5px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-subtle)',
            borderRadius: 3,
          }}>
            Enter
          </kbd>
          {' '}to send,{' '}
          <kbd style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            padding: '1px 5px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-subtle)',
            borderRadius: 3,
          }}>
            Shift+Enter
          </kbd>
          {' '}for new line
        </Text>
      </div>
    </div>
  );
}
