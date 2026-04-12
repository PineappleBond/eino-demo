'use client';

import { useState, useEffect } from 'react';
import { Drawer, Form, Input, Button, Divider, App, Spin, Descriptions, Typography } from 'antd';
import { useTranslations } from 'next-intl';
import { api, Project } from '@/lib/api';

const { Text } = Typography;

function extractWorkspaceDir(config?: Record<string, never>): string {
  if (config && typeof config.workspace_dir === 'string') {
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
  const [project, setProject] = useState<Project | null>(null);

  useEffect(() => {
    if (open && projectId) {
      setLoading(true);
      api.get<Project>(`/projects/${projectId}`)
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
      const config = {
        ...(project.config || {}),
        workspace_dir: (values.workspace_dir as string) || undefined,
      } as unknown as Record<string, never>;
      const updated = await api.put<Project>(`/projects/${projectId}`, {
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
          <Descriptions.Item label="Created">{project?.created_at ? new Date(project.created_at).toLocaleDateString() : <Spin size="small" />}</Descriptions.Item>
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
