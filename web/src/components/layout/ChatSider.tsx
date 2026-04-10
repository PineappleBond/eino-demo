'use client';

import { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { Menu, Typography, Spin, Button, message } from 'antd';
import {
  MessageOutlined,
  PlusOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import { api, Conversation } from '@/lib/api';
import { useTranslations } from 'next-intl';

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
  const t = useTranslations('chat');
  const [messageApi, contextHolder] = message.useMessage();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.get<Conversation[]>(`/projects/${projectId}/conversations`)
      .then(setConversations)
      .catch((err) => messageApi.error(err.message))
      .finally(() => setLoading(false));
  }, [projectId]);

  const handleNew = async () => {
    try {
      const res = await api.post<{ id: string }>(`/projects/${projectId}/conversations`, { title: 'New Conversation' });
      setConversations((prev) => [...prev, { id: res.id, title: 'New Conversation', status: 'active' } as Conversation]);
      onSelect(res.id);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to create conversation');
    }
  };

  const handleDelete = async (convId: string) => {
    try {
      await api.delete(`/conversations/${convId}`);
      setConversations((prev) => prev.filter((c) => c.id !== convId));
      messageApi.success('Conversation deleted');
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  if (loading) {
    return <Spin style={{ padding: 24 }} />;
  }

  return (
    <>
      {contextHolder}
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#fff' }}>
      <div style={{ padding: '16px 12px', borderBottom: '1px solid #f0f0f0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Title level={5} style={{ margin: 0 }}>{t('title')}</Title>
        <Button type="text" icon={<PlusOutlined />} onClick={handleNew} />
      </div>
      <Menu
        mode="inline"
        selectedKeys={selectedKey ? [selectedKey] : []}
        style={{ flex: 1, border: 'none', overflowY: 'auto' }}
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
        ]}
      />
    </div>
    </>
  );
}
