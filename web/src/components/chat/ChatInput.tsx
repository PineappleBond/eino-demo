'use client';

import { useState } from 'react';
import { Input, Button, Space } from 'antd';
import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

const { TextArea } = Input;

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
    <div style={{ padding: '12px 16px', borderTop: '1px solid #f0f0f0', background: '#fff' }}>
      <Space.Compact style={{ width: '100%' }}>
        <TextArea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t('placeholder')}
          autoSize={{ minRows: 1, maxRows: 4 }}
          disabled={isLoading}
          style={{ resize: 'none' }}
        />
        {isLoading ? (
          <Button
            type="primary"
            danger
            icon={<StopOutlined />}
            onClick={onStop}
          >
            {t('stop')}
          </Button>
        ) : (
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={handleSend}
            disabled={!text.trim()}
          >
            {t('send')}
          </Button>
        )}
      </Space.Compact>
    </div>
  );
}
