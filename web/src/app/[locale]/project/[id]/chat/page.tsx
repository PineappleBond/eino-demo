'use client';

import { useTranslations } from 'next-intl';
import { Result } from 'antd';
import { CommentOutlined } from '@ant-design/icons';

export default function ChatPage() {
  const t = useTranslations('chat');

  return (
    <Result
      icon={<CommentOutlined />}
      title={t('title')}
      subTitle={t('noConversations')}
      style={{ padding: '80px 0' }}
    />
  );
}
