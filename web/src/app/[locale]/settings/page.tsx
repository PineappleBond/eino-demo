'use client';

import { useEffect, useState } from 'react';
import { Card, Form, Select, Spin, App, Button, Divider, Typography } from 'antd';
import { api, Settings as SettingsType } from '@/lib/api';
import { useTranslations } from 'next-intl';
import { useTheme } from '@/hooks/useTheme';
import { TopBar } from '@/components/layout/Header';
import { useParams } from 'next/navigation';

const { Text } = Typography;

export default function SettingsPage() {
  const t = useTranslations('settings');
  const params = useParams();
  const locale = (params.locale as string) || 'en';
  const { theme, setTheme } = useTheme();
  const { message } = App.useApp();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settings, setSettings] = useState<SettingsType | null>(null);
  const [form] = Form.useForm();

  useEffect(() => {
    api.get<SettingsType>('/settings')
      .then((data) => {
        setSettings(data);
        form.setFieldsValue(data);
      })
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, [form, message]);

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

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <TopBar currentLocale={locale} />
      <div style={{ flex: 1, overflowY: 'auto', background: 'var(--bg-primary)', padding: 24 }}>
        <Card
          title={<span style={{ color: 'var(--text-primary)' }}>{t('title')}</span>}
          style={{ maxWidth: 520, margin: '0 auto', background: 'var(--bg-secondary)', borderColor: 'var(--border-subtle)' }}
        >
          <Form form={form} layout="vertical" disabled={saving}>
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
          <Button type="primary" block loading={saving} onClick={handleSave}>
            {t('saved')}
          </Button>
        </Card>
      </div>
    </div>
  );
}
