'use client';

import { Layout, Menu, Space } from 'antd';
import Link from 'next/link';
import { usePathname, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { ThemeSwitch } from './ThemeSwitch';

const { Header } = Layout;

interface TopBarProps {
  currentLocale: string;
  projectName?: string;
}

export function TopBar({ currentLocale, projectName }: TopBarProps) {
  const t = useTranslations('app');
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const projectId = pathname.match(/\/project\/([^/]+)/)?.[1] || '';

  const items = [
    {
      key: `/${currentLocale}/project/${projectId}/chat`,
      label: <Link href={`/${currentLocale}/project/${projectId}/chat`}>{t('chat')}</Link>,
    },
  ];

  return (
    <Header
      style={{
        height: 52,
        lineHeight: '52px',
        padding: '0 16px',
        display: 'flex',
        alignItems: 'center',
        borderBottom: '1px solid #f0f0f0',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <Link href={`/${currentLocale}/`} style={{ fontSize: 16, fontWeight: 600, color: 'inherit', textDecoration: 'none' }}>
            {t('title')}
          </Link>
          {projectName && (
            <span style={{ color: '#999' }}>›</span>
          )}
          {projectName && (
            <span style={{ fontWeight: 500 }}>{projectName}</span>
          )}
          {projectId && (
            <Menu
              mode="horizontal"
              selectedKeys={[pathname]}
              items={items}
              style={{ border: 'none', minWidth: 0, background: 'transparent' }}
            />
          )}
        </div>
        <Space size={12}>
          <ThemeSwitch />
        </Space>
      </div>
    </Header>
  );
}
