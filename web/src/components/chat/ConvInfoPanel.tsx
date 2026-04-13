'use client';

import { Avatar, Dropdown, Select, Tag } from 'antd';
import { UserOutlined, RobotOutlined, CloseOutlined, CheckSquareOutlined, ClockCircleOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useTranslations } from 'next-intl';
import { TodoPanel } from './TodoPanel';
import { CronTaskPanel } from './CronTaskPanel';
import type { components } from '@/types/api';

type Member = components['schemas']['Member'];

// Role label colors matching member type
const typeColors: Record<string, string> = {
  user: '#5B8FF9',
  agent: '#61DDAA',
};

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
  onMemberMention?: (member: { id: string; name: string }) => void;
  conversationId?: string;
  mode?: string;
  onModeChange?: (mode: string) => void;
}

export function ConvInfoPanel({ onClose, members, stats, model, template, onMemberMention, conversationId, mode, onModeChange }: ConvInfoPanelProps) {
  const t = useTranslations('conv');
  const tMode = useTranslations('conv.modeOptions');

  const getMemberDisplay = (member: Member) => {
    const name = member.member_name || 'Unknown';
    const icon = member.member_type === 'user' ? <UserOutlined /> : <RobotOutlined />;
    const color = typeColors[member.member_type] || '#999';
    const role = member.is_owner ? 'owner' : member.member_type;
    return { name, icon, color, role };
  };

  const getMemberContextMenu = (member: Member): MenuProps['items'] => {
    const display = getMemberDisplay(member);
    return [
      {
        key: 'mention',
        icon: <span style={{ fontSize: 14, fontWeight: 'bold' }}>@</span>,
        label: `@${display.name}`,
        onClick: () => onMemberMention?.({ id: member.id, name: display.name }),
      },
    ];
  };

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
          {members.map((m) => {
            const display = getMemberDisplay(m);
            return (
              <Dropdown
                key={m.id}
                menu={{ items: getMemberContextMenu(m) }}
                trigger={['contextMenu']}
              >
                <div className="right-panel-member">
                  <Avatar
                    size={22}
                    className="right-panel-member-avatar"
                    icon={display.icon}
                    style={{ background: display.color }}
                  />
                  <span className="right-panel-member-name">{display.name}</span>
                  <span className="right-panel-member-role">{display.role}</span>
                </div>
              </Dropdown>
            );
          })}
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

      {mode && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">{t('mode')}</div>
          <Select
            value={mode}
            onChange={(value) => onModeChange?.(value)}
            style={{ width: '100%' }}
            size="small"
            options={[
              { value: 'ask_before_edits', label: tMode('ask_before_edits') },
              { value: 'edit_automatically', label: tMode('edit_automatically') },
              { value: 'bypass_permissions', label: tMode('bypass_permissions') },
              {
                value: 'plan_mode',
                label: (
                  <span>
                    {tMode('plan_mode')}
                    <Tag color="orange" style={{ marginLeft: 6, fontSize: 10, lineHeight: '18px', padding: '0 4px' }}>
                      WIP
                    </Tag>
                  </span>
                ),
                disabled: true,
              },
            ]}
          />
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

      {conversationId && (
        <div className="right-panel-section">
          <div className="right-panel-section-title" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ClockCircleOutlined />
            <span>Scheduled</span>
          </div>
          <CronTaskPanel conversationId={conversationId} collapsible />
        </div>
      )}
      </div>
    </div>
  );
}
