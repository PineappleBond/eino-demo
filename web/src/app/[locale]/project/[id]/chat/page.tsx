'use client';

import { useTranslations } from 'next-intl';
import { Result, Typography } from 'antd';
import { CommentOutlined } from '@ant-design/icons';

const { Text } = Typography;

export default function ChatPage() {
  const t = useTranslations('chat');

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
      <Result
        icon={<CommentOutlined style={{ fontSize: 48, color: 'var(--text-tertiary)' }} />}
        title={<span style={{ color: 'var(--text-primary)' }}>{t('title')}</span>}
        subTitle={<Text style={{ color: 'var(--text-secondary)' }}>{t('noConversations')}</Text>}
      />
    </div>
  );
}
