'use client';

import { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { Layout, Menu, Typography, Spin } from 'antd';
import {
  MessageOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
} from '@ant-design/icons';
import { api, Conversation } from '@/lib/api';
import { useRouter } from 'next/navigation';
import { message } from 'antd';
import { useTranslations } from 'next-intl';

const { Sider } = Layout;
const { Title } = Typography;

export function ChatSider({
  onSelect,
  selectedKey,
}: {
  onSelect: (convId: string) => void;
  selectedKey: string;
}) {
  const params = useParams();
  const projectId = params.id as string;
  const router = useRouter();
  const t = useTranslations('chat');
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.get<Conversation[]>(`/projects/${projectId}/conversations`)
      .then(setConversations)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [projectId]);

  const handleNew = async () => {
    try {
      const res = await api.post<{ id: string }>(`/projects/${projectId}/conversations`, { title: 'New Conversation' });
      setConversations((prev) => [...prev, { id: res.id, title: 'New Conversation', status: 'active' } as Conversation]);
      onSelect(res.id);
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to create conversation');
    }
  };

  const handleDelete = async (convId: string) => {
    try {
      await api.delete(`/conversations/${convId}`);
      setConversations((prev) => prev.filter((c) => c.id !== convId));
      message.success('Conversation deleted');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  if (loading) {
    return <Spin style={{ padding: 24 }} />;
  }

  return (
    <Sider width={240} theme="light" style={{ borderRight: '1px solid #f0f0f0' }}>
      <div style={{ padding: '16px 12px', borderBottom: '1px solid #f0f0f0' }}>
        <Title level={5} style={{ margin: 0 }}>{t('title')}</Title>
      </div>
      <Menu
        mode="inline"
        selectedKeys={selectedKey ? [selectedKey] : []}
        style={{ border: 'none' }}
        items={[
          ...conversations.map((conv) => ({
            key: conv.id,
            icon: <MessageOutlined />,
            label: conv.title || 'Untitled',
            onClick: () => onSelect(conv.id),
            extra: (
              <DeleteOutlined
                style={{ color: '#999', cursor: 'pointer' }}
                onClick={(e) => { e.stopPropagation(); handleDelete(conv.id); }}
              />
            ),
          })),
          {
            key: '__new__',
            icon: <PlusOutlined />,
            label: t('new'),
            onClick: handleNew,
          },
        ]}
      />
    </Sider>
  );
}
