'use client';

import { Modal, Input, Button, Space } from 'antd';
import { useTranslations } from 'next-intl';
import { useAuth } from '@/providers/AuthProvider';
import { useWS } from '@/providers/WSProvider';
import { useState } from 'react';

interface AuthModalProps {
  open: boolean;
}

export function AuthModal({ open }: AuthModalProps) {
  const t = useTranslations('auth');
  const { setToken } = useAuth();
  const { reconnect } = useWS();
  const [inputValue, setInputValue] = useState('');

  const handleConnect = () => {
    const trimmed = inputValue.trim();
    if (trimmed) {
      setToken(trimmed);
      reconnect();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && inputValue.trim()) {
      handleConnect();
    }
  };

  return (
    <Modal
      open={open}
      title="Authentication"
      closable={false}
      footer={null}
      maskClosable={false}
      width={400}
    >
      <Space direction="vertical" style={{ width: '100%', marginTop: 16 }}>
        <Input.Password
          placeholder={t('tokenPlaceholder')}
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          autoFocus
        />
        <Button
          type="primary"
          block
          onClick={handleConnect}
          disabled={!inputValue.trim()}
        >
          {t('connect')}
        </Button>
      </Space>
    </Modal>
  );
}
