'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Input, Button, Space, Typography, Spin, Tag, Tooltip, Select, Divider } from 'antd';
import { PlusOutlined, ClockCircleOutlined, StopOutlined } from '@ant-design/icons';
import { api } from '@/lib/api';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';
import type { components } from '@/types/api';
import { useTranslations } from 'next-intl';

const { Text } = Typography;

type CronTask = components['schemas']['CronTask'];

interface CronTaskPanelProps {
  conversationId: string;
  collapsible?: boolean;
}

const ROLE_OPTIONS = [
  { value: 'user', label: 'User' },
  { value: 'assistant', label: 'Assistant' },
  { value: 'tool', label: 'Tool' },
  { value: 'system', label: 'System' },
];

const roleColors: Record<string, string> = {
  user: 'blue',
  assistant: 'purple',
  tool: 'orange',
  system: 'default',
};

export function CronTaskPanel({ conversationId, collapsible = false }: CronTaskPanelProps) {
  const t = useTranslations('chat');
  const [tasks, setTasks] = useState<CronTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [newContent, setNewContent] = useState('');
  const [newSchedule, setNewSchedule] = useState('');
  const [newSenderRole, setNewSenderRole] = useState<string>('user');
  const [collapsed, setCollapsed] = useState(collapsible);
  const [statusFilter, setStatusFilter] = useState<string>('active');
  const topic = `conv:${conversationId}`;

  const fetchTasks = useCallback(async () => {
    try {
      const params = new URLSearchParams();
      params.set('status', statusFilter);
      const data = await api.get<CronTask[]>(`/conversations/${conversationId}/cron-tasks?${params.toString()}`);
      setTasks(data);
    } catch {
      // Silently fail
    } finally {
      setLoading(false);
    }
  }, [conversationId, statusFilter]);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  // Real-time: refetch on cron task events
  const fetchTasksRef = useRef(fetchTasks);
  fetchTasksRef.current = fetchTasks;

  const handleUpdate = useCallback((update: Update) => {
    if (update.type === 'cron_task.sync') {
      const payload = update.payload as Record<string, unknown>;
      // Only refetch if the update belongs to this conversation
      if (payload.conversation_id === conversationId) {
        fetchTasksRef.current();
      }
    }
  }, [conversationId]);

  useSubscribe(topic, handleUpdate);

  const handleCreate = async () => {
    if (!newContent.trim() || !newSchedule.trim()) return;
    try {
      const task = await api.post<CronTask>(`/conversations/${conversationId}/cron-tasks`, {
        content: newContent.trim(),
        schedule: newSchedule.trim(),
        sender_role: newSenderRole,
      });
      // Refetch to ensure consistent state
      fetchTasks();
      setNewContent('');
      setNewSchedule('');
    } catch {
      // Silently fail
    }
  };

  const handleCancel = async (taskId: string) => {
    try {
      await api.post(`/conversations/${conversationId}/cron-tasks/${taskId}/cancel`);
      fetchTasks();
    } catch {
      // Silently fail
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'green';
      case 'pending':
        return 'orange';
      case 'cancelled':
        return 'default';
      case 'completed':
        return 'blue';
      case 'failed':
        return 'red';
      default:
        return 'default';
    }
  };

  const activeTasks = tasks.filter((t) => t.status === 'active' || t.status === 'pending');

  if (loading) {
    return <Spin size="small" style={{ display: 'block', textAlign: 'center', padding: 16 }} />;
  }

  if (collapsed && !collapsible) {
    return null;
  }

  return (
    <div style={{ padding: '8px 12px' }}>
      {/* Header */}
      {collapsible && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            cursor: 'pointer',
            padding: '4px 0',
            marginBottom: 12,
          }}
          onClick={() => setCollapsed(!collapsed)}
        >
          <Text strong style={{ fontSize: 13 }}>
            {t('cronTasks') || 'Scheduled Tasks'}
            {activeTasks.length > 0 && (
              <Text type="secondary" style={{ marginLeft: 6, fontSize: 12 }}>
                {activeTasks.length}
              </Text>
            )}
          </Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {collapsed ? '▶' : '▼'}
          </Text>
        </div>
      )}

      {!collapsed && (
        <>
          {/* Create form */}
          <div style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Input.TextArea
                value={newContent}
                onChange={(e) => setNewContent(e.target.value)}
                placeholder={t('addCronTask') || 'Message content to send when the task fires...'}
                autoSize={{ minRows: 2, maxRows: 4 }}
                size="small"
              />
              <Space style={{ width: '100%' }}>
                <Input
                  value={newSchedule}
                  onChange={(e) => setNewSchedule(e.target.value)}
                  placeholder={t('schedulePlaceholder') || 'e.g. 0 9 * * * or once:5m'}
                  onPressEnter={(e) => {
                    if (e.shiftKey) return;
                    handleCreate();
                  }}
                  size="small"
                  style={{ flex: 1, minWidth: 0 }}
                />
                <Select
                  value={newSenderRole}
                  onChange={setNewSenderRole}
                  size="small"
                  style={{ width: 110 }}
                  options={ROLE_OPTIONS}
                />
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={handleCreate}
                  disabled={!newContent.trim() || !newSchedule.trim()}
                  size="small"
                />
              </Space>
            </Space>
          </div>

          {tasks.length === 0 && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              {t('noCronTasks') || 'No scheduled tasks yet'}
            </Text>
          )}

          {/* Task list */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {tasks.map((task) => (
              <div
                key={task.id}
                style={{
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: 10,
                  padding: '8px 10px',
                  borderRadius: 6,
                  backgroundColor: 'var(--ant-color-fill-quaternary)',
                }}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4, flexWrap: 'wrap' }}>
                    <Tag color={getStatusColor(task.status)} style={{ fontSize: 10, margin: 0, lineHeight: '18px' }}>
                      {task.status}
                    </Tag>
                    <Tag color={roleColors[task.sender_role] || 'default'} style={{ fontSize: 10, margin: 0, lineHeight: '18px' }}>
                      {task.sender_role}
                    </Tag>
                    <Text style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {task.content}
                    </Text>
                  </div>
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    <ClockCircleOutlined /> {task.schedule}
                    {task.next_run_at && (
                      <> · {t('nextRun') || 'Next run'}: {new Date(task.next_run_at).toLocaleString()}</>
                    )}
                  </Text>
                </div>
                {(task.status === 'active' || task.status === 'pending') && (
                  <Tooltip title={t('cancelTask') || 'Cancel'}>
                    <Button
                      type="text"
                      danger
                      size="small"
                      icon={<StopOutlined />}
                      onClick={() => handleCancel(task.id)}
                      style={{ padding: '0 4px', flexShrink: 0 }}
                    />
                  </Tooltip>
                )}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
