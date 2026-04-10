'use client';

import { useState, useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Button, Input, App, Empty, Divider } from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  InboxOutlined,
  SearchOutlined,
  SettingOutlined,
  FileOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import { api, Conversation } from '@/lib/api';
import { useTranslations } from 'next-intl';

const ARCHIVED_KEY = '__archived__';

interface ChatSiderProps {
  selectedKey: string;
}

export function ChatSider({ selectedKey }: ChatSiderProps) {
  const params = useParams();
  const router = useRouter();
  const projectId = params.id as string;
  const locale = params.locale as string;
  const t = useTranslations('chat');
  const tApp = useTranslations('app');
  const { message } = App.useApp();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');

  useEffect(() => {
    api.get<Conversation[]>(`/projects/${projectId}/conversations`)
      .then(setConversations)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [projectId, message]);

  const handleNew = async () => {
    try {
      const res = await api.post<{ id: string }>(`/projects/${projectId}/conversations`, {});
      router.push(`/${locale}/project/${projectId}/chat/${res.id}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to create conversation');
    }
  };

  const handleDelete = async (convId: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    try {
      await api.delete(`/conversations/${convId}`);
      setConversations((prev) => prev.filter((c) => c.id !== convId));
      message.success('Conversation deleted');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const filtered = conversations.filter((c) =>
    c.title?.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const activeConvs = filtered.filter((c) => c.status !== 'archived');
  const archivedConvs = filtered.filter((c) => c.status === 'archived');

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Header */}
      <div style={{
        padding: '12px 12px 8px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
        <span style={{
          fontFamily: 'var(--font-label)',
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: 1.5,
          textTransform: 'uppercase',
          color: 'var(--text-tertiary)',
        }}>
          {t('title')}
        </span>
        <Button
          type="text"
          size="small"
          icon={<PlusOutlined style={{ fontSize: 14 }} />}
          onClick={handleNew}
          style={{
            color: 'var(--text-tertiary)',
            width: 20,
            height: 20,
            padding: 0,
          }}
        />
      </div>

      {/* Search */}
      <div style={{ padding: '0 8px 8px' }}>
        <Input
          prefix={<SearchOutlined style={{ fontSize: 12, color: 'var(--text-tertiary)' }} />}
          placeholder="Search..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          size="small"
          style={{
            background: 'var(--bg-tertiary)',
            border: '1px solid var(--border-default)',
            color: 'var(--text-primary)',
            borderRadius: 'var(--radius-sm)',
          }}
        />
      </div>

      {/* List */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px' }}>
        {loading ? (
          <div style={{ textAlign: 'center', padding: 24, color: 'var(--text-tertiary)' }}>
            Loading...
          </div>
        ) : activeConvs.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '24px 0' }} />
        ) : (
          activeConvs.map((conv) => (
            <div key={conv.id} style={{ marginBottom: 2 }}>
              <Link
                href={`/${locale}/project/${projectId}/chat/${conv.id}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '8px 10px',
                  borderRadius: 'var(--radius-sm)',
                  textDecoration: 'none',
                  fontSize: 13,
                  background: conv.id === selectedKey ? 'var(--accent-soft)' : 'transparent',
                  transition: 'all 0.12s ease',
                }}
                onMouseEnter={(e) => {
                  if (conv.id !== selectedKey) {
                    (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (conv.id !== selectedKey) {
                    (e.currentTarget as HTMLElement).style.background = 'transparent';
                  }
                }}
              >
                <span style={{
                  flex: 1,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  color: conv.id === selectedKey ? 'var(--accent)' : 'var(--text-secondary)',
                  fontWeight: conv.id === selectedKey ? 600 : 400,
                }}>
                  {conv.title || 'Untitled'}
                </span>
                <DeleteOutlined
                  style={{
                    color: 'var(--text-tertiary)',
                    fontSize: 12,
                    cursor: 'pointer',
                    opacity: 0.5,
                    transition: 'opacity 0.1s',
                  }}
                  onClick={(e) => handleDelete(conv.id, e)}
                />
              </Link>
            </div>
          ))
        )}

        {archivedConvs.length > 0 && (
          <>
            <Divider style={{ margin: '8px 0', borderColor: 'var(--border-subtle)' }} />
            <div style={{
              fontFamily: 'var(--font-label)',
              fontSize: 10,
              fontWeight: 700,
              letterSpacing: 1.5,
              textTransform: 'uppercase',
              color: 'var(--text-tertiary)',
              padding: '4px 10px',
            }}>
              {t('archived')}
            </div>
            {archivedConvs.map((conv) => (
              <Link
                key={conv.id}
                href={`/${locale}/project/${projectId}/chat/${conv.id}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '8px 10px',
                  borderRadius: 'var(--radius-sm)',
                  textDecoration: 'none',
                  fontSize: 13,
                  color: 'var(--text-tertiary)',
                  transition: 'all 0.12s ease',
                  marginBottom: 2,
                }}
                onMouseEnter={(e) => {
                  (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)';
                }}
                onMouseLeave={(e) => {
                  (e.currentTarget as HTMLElement).style.background = 'transparent';
                }}
              >
                <InboxOutlined style={{ fontSize: 12 }} />
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {conv.title || 'Untitled'}
                </span>
              </Link>
            ))}
          </>
        )}
      </div>

      {/* Footer */}
      <div style={{
        borderTop: '1px solid var(--border-subtle)',
        padding: '8px',
        display: 'flex',
        flexDirection: 'column',
      }}>
        <Link
          href={`/${locale}`}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '8px 10px',
            borderRadius: 'var(--radius-sm)',
            textDecoration: 'none',
            fontSize: 13,
            color: 'var(--text-secondary)',
            transition: 'all 0.12s ease',
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)';
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.background = 'transparent';
          }}
        >
          <FileOutlined style={{ fontSize: 14 }} />
          {tApp('home')}
        </Link>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '8px 10px',
            borderRadius: 'var(--radius-sm)',
            fontSize: 13,
            color: 'var(--text-secondary)',
            cursor: 'pointer',
            transition: 'all 0.12s ease',
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)';
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.background = 'transparent';
          }}
          onClick={() => {
            // Trigger settings from parent
            const event = new CustomEvent('open-settings');
            window.dispatchEvent(event);
          }}
        >
          <SettingOutlined style={{ fontSize: 14 }} />
          {tApp('settings')}
        </div>
      </div>
    </div>
  );
}
