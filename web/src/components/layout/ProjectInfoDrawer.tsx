'use client';

import { useState, useEffect } from 'react';
import { Drawer, Form, Input, Button, Divider, App, Spin, Descriptions, Typography } from 'antd';
import { useTranslations } from 'next-intl';
import { api } from '@/lib/api';

const { Text } = Typography;

interface ProjectInfo {
  id: string;
  name: string;
  template_id: string;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

function extractWorkspaceDir(config: Record<string, unknown>): string {
  if (typeof config.workspace_dir === 'string') {
    return config.workspace_dir;
  }
  return '';
}

interface ProjectInfoDrawerProps {
  open: boolean;
  projectId: string;
  onClose: () => void;
  onNameChange?: (name: string) => void;
}

export function ProjectInfoDrawer({ open, projectId, onClose, onNameChange }: ProjectInfoDrawerProps) {
  const t = useTranslations('project');
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [project, setProject] = useState<ProjectInfo | null>(null);

  useEffect(() => {
    if (open && projectId) {
      setLoading(true);
      api.get<ProjectInfo>(`/projects/${projectId}`)
        .then((data) => {
          setProject(data);
          form.setFieldsValue({
            name: data.name,
            workspace_dir: extractWorkspaceDir(data.config),
          });
        })
        .catch((err) => message.error(err.message))
        .finally(() => setLoading(false));
    }
  }, [open, projectId, message]);

  const handleSave = async () => {
    if (!project) return;
    setSaving(true);
    try {
      const values = form.getFieldsValue();
      const config: Record<string, unknown> = {
        ...(project.config || {}),
        workspace_dir: (values.workspace_dir as string) || undefined,
      };
      const updated = await api.put<ProjectInfo>(`/projects/${projectId}`, {
        name: values.name,
        config,
      });
      setProject(updated);
      onNameChange?.(updated.name);
      message.success('Project updated');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to update');
      if (project) {
        form.setFieldsValue({
          name: project.name,
          workspace_dir: extractWorkspaceDir(project.config),
        });
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer
      title={t('info')}
      open={open}
      onClose={onClose}
      width={420}
      closable
      destroyOnClose
      styles={{
        body: { padding: '20px' },
      }}
    >
      <Form form={form} layout="vertical" disabled={saving || loading}>
        {/* Info section */}
        <Descriptions
          column={1}
          size="small"
          style={{ marginBottom: 20 }}
          styles={{ label: { color: 'var(--text-tertiary)', fontWeight: 500 } }}
        >
          <Descriptions.Item label="Template">{project?.template_id || <Spin size="small" />}</Descriptions.Item>
          <Descriptions.Item label="Created">{project ? new Date(project.created_at).toLocaleDateString() : <Spin size="small" />}</Descriptions.Item>
          <Descriptions.Item label="Updated">{project ? new Date(project.updated_at).toLocaleDateString() : <Spin size="small" />}</Descriptions.Item>
        </Descriptions>

        <Divider style={{ borderColor: 'var(--border-subtle)' }} />

        {/* Editable section */}
        <Form.Item
          label={<Text style={{ fontFamily: 'var(--font-label)', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, textTransform: 'uppercase', color: 'var(--text-tertiary)' }}>Name</Text>}
          name="name"
        >
          <Input />
        </Form.Item>

        <Form.Item
          label={<Text style={{ fontFamily: 'var(--font-label)', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, textTransform: 'uppercase', color: 'var(--text-tertiary)' }}>Workspace Directory</Text>}
          name="workspace_dir"
        >
          <Input placeholder="/path/to/project" />
        </Form.Item>

        <Divider style={{ borderColor: 'var(--border-subtle)' }} />
        <Button type="primary" block loading={saving} onClick={handleSave}>
          {t('save')}
        </Button>
      </Form>
    </Drawer>
  );
}
