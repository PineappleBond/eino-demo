'use client';

import { useEffect, useState, useRef, useCallback } from 'react';
import { Form, Select, Button, Divider, Typography, Spin, App } from 'antd';
import { useTranslations } from 'next-intl';
import { useTheme } from '@/hooks/useTheme';
import { api, Settings as SettingsType } from '@/lib/api';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';

const { Text } = Typography;

const fieldLabelStyle = {
  fontFamily: 'var(--font-label)',
  fontSize: 10,
  fontWeight: 700,
  letterSpacing: 1.2,
  textTransform: 'uppercase' as const,
  color: 'var(--text-tertiary)',
};

interface SettingsFormProps {
  onSaved?: () => void;
  children?: React.ReactNode;
}

/**
 * Shared settings form component used by both the Settings page
 * and the SettingsDrawer. Handles API fetch, save, and theme sync.
 */
export function SettingsForm({ onSaved, children }: SettingsFormProps) {
  const t = useTranslations('settings');
  const { theme, setTheme } = useTheme();
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settings, setSettings] = useState<SettingsType | null>(null);

  useEffect(() => {
    api.get<SettingsType>('/settings')
      .then((data) => {
        setSettings(data);
        form.setFieldsValue(data);
      })
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [form, message]);

  // Sync settings when another tab updates them
  const fetchSettingsRef = useRef(() => {
    api.get<SettingsType>('/settings')
      .then((data) => {
        setSettings(data);
        form.setFieldsValue(data);
        // Apply theme change immediately
        if (data.theme && data.theme !== theme) {
          setTheme(data.theme);
        }
      })
      .catch(() => {
        // Silently fail — settings will refresh on next save
      });
  });

  const handleSettingsChange = useCallback((update: Update) => {
    if (update.type === 'settings.changed') {
      fetchSettingsRef.current();
    }
  }, []);

  useSubscribe('system', handleSettingsChange);

  const handleSave = async () => {
    setSaving(true);
    try {
      const values = form.getFieldsValue();
      const updated = await api.put<SettingsType>('/settings', values);
      setSettings(updated);
      if (values.theme && values.theme !== theme) {
        setTheme(values.theme);
      }
      message.success(t('saved'));
      onSaved?.();
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to save');
      if (settings) form.setFieldsValue(settings);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  const formItems = (
    <>
      <Form.Item
        label={<Text style={fieldLabelStyle}>{t('model')}</Text>}
        name="model_tier"
      >
        <Select
          options={[
            { value: 'haiku', label: t('model_desc.haiku') },
            { value: 'sonnet', label: t('model_desc.sonnet') },
            { value: 'opus', label: t('model_desc.opus') },
          ]}
        />
      </Form.Item>
      <Form.Item
        label={<Text style={fieldLabelStyle}>{t('language')}</Text>}
        name="locale"
      >
        <Select
          options={[
            { value: 'en', label: t('english') },
            { value: 'zh', label: t('chinese') },
          ]}
        />
      </Form.Item>
      <Form.Item
        label={<Text style={fieldLabelStyle}>{t('theme')}</Text>}
        name="theme"
      >
        <Select
          options={[
            { value: 'light', label: t('light') },
            { value: 'dark', label: t('dark') },
          ]}
        />
      </Form.Item>
    </>
  );

  return (
    <>
      <Form form={form} layout="vertical" disabled={saving}>
        {formItems}
        {children}
      </Form>
      <Divider style={{ borderColor: 'var(--border-subtle)' }} />
      <Button type="primary" block loading={saving} onClick={handleSave}>
        {t('saved')}
      </Button>
    </>
  );
}
