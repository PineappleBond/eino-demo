'use client';

import { Avatar, Dropdown } from 'antd';
import { UserOutlined, RobotOutlined, CloseOutlined, CheckSquareOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useTranslations } from 'next-intl';
import { TodoPanel } from './TodoPanel';

interface Member {
  id: string;
  name: string;
  type: 'user' | 'agent';
  color: string;
  role: string;
}

interface ConvInfoPanelProps {
  onClose: () => void;
  members?: Member[];
  stats?: {
    messages: number;
    tokenPrompt: number;
    tokenCompletion: number;
    toolCalls: number;
  };
  model?: string;
  template?: string;
  onMemberMention?: (member: Member) => void;
  conversationId?: string;
}

export function ConvInfoPanel({ onClose, members, stats, model, template, onMemberMention, conversationId }: ConvInfoPanelProps) {
  const t = useTranslations('conv');
  const getMemberContextMenu = (member: Member): MenuProps['items'] => [
    {
      key: 'mention',
      icon: <span style={{ fontSize: 14, fontWeight: 'bold' }}>@</span>,
      label: `@${member.name}`,
      onClick: () => onMemberMention?.(member),
    },
  ];

  return (
    <div className="right-panel">
      <div className="right-panel-header">
        <span className="right-panel-title">{t('title')}</span>
        <span className="panel-close" onClick={onClose} aria-label="Close info panel" role="button" tabIndex={0}>
          <CloseOutlined style={{ fontSize: 14 }} />
        </span>
      </div>

      <div className="right-panel-content">
      {stats && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">{t('statistics')}</div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">{t('messages')}</span>
            <span className="right-panel-stat-value">{stats.messages}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">{t('tokensPrompt')}</span>
            <span className="right-panel-stat-value">{stats.tokenPrompt.toLocaleString()}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">{t('tokensCompletion')}</span>
            <span className="right-panel-stat-value">{stats.tokenCompletion.toLocaleString()}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">{t('toolCalls')}</span>
            <span className="right-panel-stat-value">{stats.toolCalls}</span>
          </div>
        </div>
      )}

      {members && members.length > 0 && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">{t('members')}</div>
          {members.map((m) => (
            <Dropdown
              key={m.id}
              menu={{ items: getMemberContextMenu(m) }}
              trigger={['contextMenu']}
            >
              <div className="right-panel-member">
                <Avatar
                  size={22}
                  className="right-panel-member-avatar"
                  icon={m.type === 'user' ? <UserOutlined /> : <RobotOutlined />}
                  style={{ background: m.color }}
                />
                <span className="right-panel-member-name">{m.name}</span>
                <span className="right-panel-member-role">{m.role}</span>
              </div>
            </Dropdown>
          ))}
        </div>
      )}

      {model && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">{t('model')}</div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">{t('activeModel')}</span>
            <span className="right-panel-stat-value" style={{ color: 'var(--accent)' }}>{model}</span>
          </div>
          {template && (
            <div className="right-panel-stat">
              <span className="right-panel-stat-label">{t('template')}</span>
              <span className="right-panel-stat-value">{template}</span>
            </div>
          )}
        </div>
      )}

      {conversationId && (
        <div className="right-panel-section">
          <div className="right-panel-section-title" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <CheckSquareOutlined />
            <span>Todos</span>
          </div>
          <TodoPanel conversationId={conversationId} collapsible />
        </div>
      )}
      </div>
    </div>
  );
}
