'use client';

import { Layout, Menu } from 'antd';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

const { Header } = Layout;

interface AppHeaderProps {
  currentLocale: string;
}

const navItems = (locale: string, t: (key: string) => string) => [
  {
    key: `/${locale}/`,
    label: <Link href={`/${locale}/`}>{t('home')}</Link>,
  },
  {
    key: `/${locale}/chat/`,
    label: <Link href={`/${locale}/chat/`}>{t('chat')}</Link>,
  },
  {
    key: `/${locale}/settings`,
    label: <Link href={`/${locale}/settings`}>{t('settings')}</Link>,
  },
];

export function AppHeader({ currentLocale }: AppHeaderProps) {
  const t = useTranslations('app');
  const pathname = usePathname();

  return (
    <Header
      style={{
        padding: '0 24px',
        display: 'flex',
        alignItems: 'center',
      }}
    >
      <Link
        href={`/${currentLocale}/`}
        style={{ fontSize: 18, fontWeight: 600, color: 'inherit', marginRight: 48, flexShrink: 0 }}
      >
        {t('title')}
      </Link>
      <Menu
        theme="dark"
        mode="horizontal"
        selectedKeys={[pathname]}
        items={navItems(currentLocale, t)}
        style={{ flex: 1, border: 'none', lineHeight: '64px' }}
      />
    </Header>
  );
}
