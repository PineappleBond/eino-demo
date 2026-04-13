'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Button, Input, App, Empty, Divider, Dropdown, Popconfirm, Modal } from 'antd';
import type { MenuProps } from 'antd';
import {
  PlusOutlined,
  SearchOutlined,
  SettingOutlined,
  FileOutlined,
  EditOutlined,
  FolderOutlined,
  CompressOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import { api, Conversation } from '@/lib/api';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';

// Tree node extends Conversation with recursive children from the API
interface ConversationNode extends Conversation {
  children?: ConversationNode[];
}

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
  const [conversations, setConversations] = useState<ConversationNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [contextMenuConv, setContextMenuConv] = useState<Conversation | null>(null);
  const [renameConv, setRenameConv] = useState<Conversation | null>(null);
  const [renameModalOpen, setRenameModalOpen] = useState(false);
  const [renameValue, setRenameValue] = useState('');

  // Accordion: only one parent expanded at a time
  const [expandedConvId, setExpandedConvId] = useState<string | null>(null);

  useEffect(() => {
    api.get<ConversationNode[]>(`/projects/${projectId}/conversations`)
      .then(setConversations)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [projectId, message]);

  // ─── Subscribe to conversation lifecycle events ───

  const fetchRef = useRef(() => {
    api.get<ConversationNode[]>(`/projects/${projectId}/conversations`)
      .then(setConversations)
      .catch((err) => message.error(err.message));
  });

  // Accordion: only one parent expanded at a time
  const toggleExpand = useCallback((convId: string) => {
    setExpandedConvId((prev) => (prev === convId ? null : convId));
  }, []);

  // Auto-expand the parent of the currently selected conversation
  useEffect(() => {
    if (!selectedKey) return;
    const findParentOf = (nodes: ConversationNode[], target: string): string | null => {
      for (const node of nodes) {
        if (node.children) {
          for (const child of node.children) {
            if (child.id === target) return node.id;
            const deeper = findParentOf([child] as ConversationNode[], target);
            if (deeper) return node.id;
          }
        }
      }
      return null;
    };
    const parentId = findParentOf(conversations, selectedKey);
    if (parentId) {
      setExpandedConvId(parentId);
    }
  }, [selectedKey, conversations]);

  useSubscribe(`project:${projectId}`, (update: Update) => {
    switch (update.type) {
      case 'conversation.created':
      case 'conversation.deleted':
      case 'conversation.archived':
      case 'conversation.compacted':
        fetchRef.current();
        break;
    }
  });

  const handleNew = async () => {
    try {
      const res = await api.post<{ id: string }>(`/projects/${projectId}/conversations`, {});
      router.push(`/${locale}/project/${projectId}/chat/${res.id}`);
    } catch (err) {
      message.error(t('failedCreate'));
    }
  };

  const handleArchive = async (convId: string) => {
    try {
      await api.put(`/conversations/${convId}`, { status: 'archived' });
      setConversations((prev) =>
        prev.map((c) => c.id === convId ? { ...c, status: 'archived' } : c)
      );
    } catch (err) {
      message.error(t('failedArchive'));
    }
  };

  const handleDelete = async (convId: string) => {
    try {
      await api.delete(`/conversations/${convId}`);
      setConversations((prev) => prev.filter((c) => c.id !== convId));
    } catch (err) {
      message.error(t('failedDelete'));
    }
  };

  const handleCompact = async (convId: string) => {
    try {
      await api.post(`/conversations/${convId}/compact`);
    } catch {
      // Compact may not be available
    }
  };

  const openRenameModal = (conv: Conversation) => {
    setRenameConv(conv);
    setRenameValue(conv.title || t('untitled'));
    setRenameModalOpen(true);
    closeContextMenu();
  };

  const handleRename = async () => {
    if (!renameConv) return;
    const trimmed = renameValue.trim();
    if (!trimmed) {
      message.error(t('renameEmpty') || 'Title cannot be empty');
      return;
    }
    try {
      await api.patch(`/conversations/${renameConv.id}`, { title: trimmed });
      setConversations((prev) =>
        prev.map((c) => c.id === renameConv.id ? { ...c, title: trimmed } : c)
      );
      setRenameModalOpen(false);
      setRenameConv(null);
    } catch (err: any) {
      message.error(err.message || 'Failed to rename');
    }
  };

  const handleContextMenu = useCallback((e: React.MouseEvent, conv: Conversation) => {
    e.preventDefault();
    e.stopPropagation();
    setContextMenuConv(conv);
  }, []);

  const closeContextMenu = useCallback(() => setContextMenuConv(null), []);

  // ─── Recursive tree rendering ───

  const renderConversationNode = (node: ConversationNode, depth: number): React.ReactNode => {
    const isActive = node.id === selectedKey;
    const hasChildren = (node.children_count || 0) > 0;
    const isExpanded = expandedConvId === node.id;
    const indent = depth * 16;
    const paddingR = Math.max(10 - Math.min(indent, 8), 4);
    const paddingL = 10 + indent;
    const fontSize = Math.max(13 - depth, 11);

    const linkContent = (
      <div
        className={`sider-item ${isActive ? 'active' : ''}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: `8px ${paddingR}px 8px ${paddingL}px`,
          borderRadius: 'var(--radius-sm)',
          textDecoration: 'none',
          fontSize,
          background: isActive ? 'var(--accent-soft)' : 'transparent',
          transition: 'all 0.12s ease',
          cursor: 'pointer',
        }}
      >
        {/* Expand/collapse arrow */}
        {hasChildren && (
          <span
            style={{
              fontSize: 9,
              color: isActive ? 'var(--accent)' : 'var(--text-tertiary)',
              width: 12,
              flexShrink: 0,
              cursor: 'pointer',
            }}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              toggleExpand(node.id);
            }}
          >
            {isExpanded ? '▼' : '▶'}
          </span>
        )}
        {!hasChildren && <span style={{ width: 12, flexShrink: 0 }} />}

        {/* Title */}
        <span
          style={{
            flex: 1,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            color: isActive ? 'var(--accent)' : depth > 0 ? 'var(--text-tertiary)' : 'var(--text-secondary)',
            fontWeight: isActive ? 600 : 400,
          }}
        >
          {node.title || t('untitled')}
        </span>

        {/* Children count badge (when collapsed, root level only) */}
        {hasChildren && !isExpanded && depth === 0 && (
          <span
            style={{
              fontSize: 9,
              padding: '1px 5px',
              borderRadius: 8,
              background: 'var(--bg-hover)',
              color: 'var(--text-tertiary)',
              flexShrink: 0,
            }}
          >
            {node.children_count}
          </span>
        )}
      </div>
    );

    const link = (
      <Link href={`/${locale}/project/${projectId}/chat/${node.id}`}>
        {linkContent}
      </Link>
    );

    // Add context menu for root-level conversations only
    const withMenu = depth === 0 ? (
      <Dropdown
        menu={{ items: contextMenuConv?.id === node.id ? getContextMenuItems() : [] }}
        trigger={['contextMenu']}
        onOpenChange={(open) => { if (!open) closeContextMenu(); }}
      >
        {link}
      </Dropdown>
    ) : link;

    return (
      <div key={node.id} style={{ marginBottom: depth === 0 ? 2 : 1 }}>
        {withMenu}
        {/* Render expanded children */}
        {isExpanded && node.children?.map((child) =>
          renderConversationNode(child as ConversationNode, depth + 1)
        )}
      </div>
    );
  };

  const getContextMenuItems = useCallback((): MenuProps['items'] => {
    if (!contextMenuConv) return [];
    return [
      {
        key: 'rename',
        icon: <EditOutlined />,
        label: t('rename'),
        onClick: () => {
          openRenameModal(contextMenuConv);
        },
      },
      {
        key: 'archive',
        icon: <FolderOutlined />,
        label: t('archive'),
        onClick: () => {
          handleArchive(contextMenuConv.id);
          closeContextMenu();
        },
      },
      {
        key: 'compact',
        icon: <CompressOutlined />,
        label: t('compactContext'),
        onClick: () => {
          handleCompact(contextMenuConv.id);
          closeContextMenu();
        },
      },
      { type: 'divider' },
      {
        key: 'delete',
        danger: true,
        icon: <DeleteOutlined />,
        label: (
          <Popconfirm
            title={t('deleteConfirm')}
            onConfirm={() => {
              handleDelete(contextMenuConv.id);
              closeContextMenu();
            }}
            onCancel={closeContextMenu}
            okText={t('delete')}
            cancelText={t('cancel')}
          >
            <span>{t('delete')}</span>
          </Popconfirm>
        ),
      },
    ];
  }, [contextMenuConv, closeContextMenu]);

  const activeRoots = conversations.filter(
    (c) => c.status !== 'archived' && (c.title ?? t('untitled')).toLowerCase().includes(searchQuery.toLowerCase())
  );
  const archivedRoots = conversations.filter(
    (c) => c.status === 'archived' && (c.title ?? t('untitled')).toLowerCase().includes(searchQuery.toLowerCase())
  );

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
          placeholder={t('search')}
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
            <div className="typing-indicator" style={{ justifyContent: 'center' }}>
              <div className="typing-dot" /><div className="typing-dot" /><div className="typing-dot" />
            </div>
          </div>
        ) : activeRoots.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '24px 0' }} />
        ) : (
          activeRoots.map((node) => renderConversationNode(node, 0))
        )}

        {archivedRoots.length > 0 && (
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
            {archivedRoots.map((node) => renderConversationNode(node, 0))}
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
            router.push(`/${locale}/settings`);
          }}
        >
          <SettingOutlined style={{ fontSize: 14 }} />
          {tApp('settings')}
        </div>
      </div>

      {/* Rename Modal */}
      <Modal
        title={t('rename')}
        open={renameModalOpen}
        onOk={handleRename}
        onCancel={() => { setRenameModalOpen(false); setRenameConv(null); }}
        okText={t('save') || 'Save'}
        cancelText={t('cancel')}
      >
        <Input
          value={renameValue}
          onChange={(e) => setRenameValue(e.target.value)}
          onPressEnter={handleRename}
          autoFocus
          style={{ marginTop: 8 }}
        />
      </Modal>
    </div>
  );
}
