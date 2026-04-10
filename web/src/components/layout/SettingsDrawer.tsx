'use client';

import { useState, useEffect } from 'react';
import { Drawer, App, Button, Form, Select, Divider, Typography } from 'antd';
import { useTranslations } from 'next-intl';
import { useTheme } from '@/hooks/useTheme';
import { api, Settings as SettingsType } from '@/lib/api';

const { Text } = Typography;

interface SettingsDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function SettingsDrawer({ open, onClose }: SettingsDrawerProps) {
  const t = useTranslations('settings');
  const { theme, setTheme } = useTheme();
  const { message: messageApi } = App.useApp();
  const [settings, setSettings] = useState<SettingsType | null>(null);
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    if (open) {
      api.get<SettingsType>('/settings')
        .then((data) => {
          setSettings(data);
          form.setFieldsValue(data);
        })
        .catch((err) => messageApi.error(err.message));
    }
  }, [open, form, messageApi]);

  const handleSave = async () => {
    setLoading(true);
    try {
      const values = form.getFieldsValue();
      const updated = await api.put<SettingsType>('/settings', values);
      setSettings(updated);
      if (values.theme && values.theme !== theme) {
        setTheme(values.theme);
      }
      messageApi.success(t('saved'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to save');
      if (settings) form.setFieldsValue(settings);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Drawer
      title={t('title')}
      open={open}
      onClose={onClose}
      width={420}
      closable
      destroyOnClose
      styles={{
        body: { padding: '20px' },
      }}
    >
      <Form form={form} layout="vertical" disabled={loading}>
        <Form.Item
          label={<Text style={{ fontFamily: 'var(--font-label)', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, textTransform: 'uppercase', color: 'var(--text-tertiary)' }}>{t('model')}</Text>}
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
          label={<Text style={{ fontFamily: 'var(--font-label)', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, textTransform: 'uppercase', color: 'var(--text-tertiary)' }}>{t('language')}</Text>}
          name="locale"
        >
          <Select
            options={[
              { value: 'en', label: 'English' },
              { value: 'zh', label: '中文' },
            ]}
          />
        </Form.Item>
        <Form.Item
          label={<Text style={{ fontFamily: 'var(--font-label)', fontSize: 10, fontWeight: 700, letterSpacing: 1.2, textTransform: 'uppercase', color: 'var(--text-tertiary)' }}>{t('theme')}</Text>}
          name="theme"
        >
          <Select
            options={[
              { value: 'light', label: 'Light' },
              { value: 'dark', label: 'Dark' },
            ]}
          />
        </Form.Item>
      </Form>
      <Divider style={{ borderColor: 'var(--border-subtle)' }} />
      <Button type="primary" block loading={loading} onClick={handleSave}>
        {t('saved')}
      </Button>
    </Drawer>
  );
}
