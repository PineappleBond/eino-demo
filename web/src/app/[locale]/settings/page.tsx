'use client';

import { Card, Typography } from 'antd';
import { useTranslations } from 'next-intl';
import { useParams } from 'next/navigation';
import { TopBar } from '@/components/layout/Header';
import { SettingsForm } from '@/components/settings/SettingsForm';

const { Text } = Typography;

export default function SettingsPage() {
  const t = useTranslations('settings');
  const params = useParams();
  const locale = (params.locale as string) || 'en';

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <TopBar currentLocale={locale} />
      <div style={{ flex: 1, overflowY: 'auto', background: 'var(--bg-primary)', padding: 24 }}>
        <Card
          title={<span style={{ color: 'var(--text-primary)' }}>{t('title')}</span>}
          style={{ maxWidth: 520, margin: '0 auto', background: 'var(--bg-secondary)', borderColor: 'var(--border-subtle)' }}
        >
          <SettingsForm />
        </Card>
      </div>
    </div>
  );
}
