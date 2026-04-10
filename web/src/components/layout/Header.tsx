'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { Layout, Space } from 'antd';
import { ThemeSwitch } from './ThemeSwitch';

const { Header: AntHeader } = Layout;

export function Header({ currentLocale }: { currentLocale: string }) {
  const t = useTranslations('app');

  return (
    <AntHeader style={{ background: '#fff', padding: '0 24px', borderBottom: '1px solid #f0f0f0', position: 'sticky', top: 0, zIndex: 1000 }}>
      <Space style={{ width: '100%', justifyContent: 'space-between', maxWidth: 1200, margin: '0 auto' }}>
        <Link href={`/${currentLocale}/`} style={{ fontSize: 18, fontWeight: 600, color: 'inherit', textDecoration: 'none' }}>
          {t('title')}
        </Link>
        <Space>
          <Link href={`/${currentLocale}/`}>{t('home')}</Link>
          <Link href={`/${currentLocale}/chat/`}>{t('chat')}</Link>
          <Link href={`/${currentLocale}/settings`}>{t('settings')}</Link>
          <ThemeSwitch />
        </Space>
      </Space>
    </AntHeader>
  );
}
