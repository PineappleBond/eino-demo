'use client';

import { useEffect, useState } from 'react';
import { Card, Form, Select, message, Spin } from 'antd';
import { api, Settings as SettingsType } from '@/lib/api';
import { useTranslations } from 'next-intl';
import { useTheme } from '@/hooks/useTheme';
import { TopBar } from '@/components/layout/Header';
import { useParams } from 'next/navigation';

export default function SettingsPage() {
  const t = useTranslations('settings');
  const params = useParams();
  const locale = (params.locale as string) || 'en';
  const { setTheme } = useTheme();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settings, setSettings] = useState<SettingsType | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [form] = Form.useForm();
  const [messageApi, contextHolder] = message.useMessage();

  useEffect(() => {
    api.get<SettingsType>('/settings')
      .then(setSettings)
      .catch((err) => setErrorMsg(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (errorMsg) {
      messageApi.error(errorMsg);
    }
  }, [errorMsg, messageApi]);

  const handleChange = async (field: string, value: string) => {
    setSaving(true);
    try {
      const payload: Record<string, string> = { [field]: value };
      const updated = await api.put<SettingsType>('/settings', payload);
      setSettings(updated);
      if (field === 'theme') {
        setTheme(value as 'light' | 'dark');
      }
      messageApi.success(t('saved'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to save');
      if (settings) form.setFieldsValue(settings);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  return (
    <>
      <TopBar currentLocale={locale} />
      {contextHolder}
      <div style={{ padding: 24, maxWidth: 520, margin: '0 auto', width: '100%' }}>
        <Card title={t('title')}>
          <Form form={form} layout="vertical" disabled={saving}>
            <Form.Item label={t('model')} name="model_tier">
              <Select
                options={[
                  { value: 'haiku', label: t('model_desc.haiku') },
                  { value: 'sonnet', label: t('model_desc.sonnet') },
                  { value: 'opus', label: t('model_desc.opus') },
                ]}
                onChange={(v) => handleChange('model_tier', v)}
              />
            </Form.Item>
            <Form.Item label={t('language')} name="locale">
              <Select
                options={[
                  { value: 'en', label: 'English' },
                  { value: 'zh', label: '中文' },
                ]}
                onChange={(v) => handleChange('locale', v)}
              />
            </Form.Item>
            <Form.Item label={t('theme')} name="theme">
              <Select
                options={[
                  { value: 'light', label: 'Light' },
                  { value: 'dark', label: 'Dark' },
                ]}
                onChange={(v) => handleChange('theme', v)}
              />
            </Form.Item>
          </Form>
        </Card>
      </div>
    </>
  );
}
