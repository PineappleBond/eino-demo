'use client';

import { Avatar } from 'antd';
import { UserOutlined, RobotOutlined, CloseOutlined } from '@ant-design/icons';

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
}

export function ConvInfoPanel({ onClose, members, stats, model, template }: ConvInfoPanelProps) {
  return (
    <div className="right-panel">
      <div className="right-panel-header">
        <span className="right-panel-title">Conversation Info</span>
        <span className="panel-close" onClick={onClose}>
          <CloseOutlined style={{ fontSize: 14 }} />
        </span>
      </div>

      {stats && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">Statistics</div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">Messages</span>
            <span className="right-panel-stat-value">{stats.messages}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">Tokens (prompt)</span>
            <span className="right-panel-stat-value">{stats.tokenPrompt.toLocaleString()}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">Tokens (completion)</span>
            <span className="right-panel-stat-value">{stats.tokenCompletion.toLocaleString()}</span>
          </div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">Tool calls</span>
            <span className="right-panel-stat-value">{stats.toolCalls}</span>
          </div>
        </div>
      )}

      {members && members.length > 0 && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">Members</div>
          {members.map((m) => (
            <div key={m.id} className="right-panel-member">
              <Avatar
                size={22}
                className="right-panel-member-avatar"
                icon={m.type === 'user' ? <UserOutlined /> : <RobotOutlined />}
                style={{ background: m.color }}
              />
              <span className="right-panel-member-name">{m.name}</span>
              <span className="right-panel-member-role">{m.role}</span>
            </div>
          ))}
        </div>
      )}

      {model && (
        <div className="right-panel-section">
          <div className="right-panel-section-title">Model</div>
          <div className="right-panel-stat">
            <span className="right-panel-stat-label">Active model</span>
            <span className="right-panel-stat-value" style={{ color: 'var(--accent)' }}>{model}</span>
          </div>
          {template && (
            <div className="right-panel-stat">
              <span className="right-panel-stat-label">Template</span>
              <span className="right-panel-stat-value">{template}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
