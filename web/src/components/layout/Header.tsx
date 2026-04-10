'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { Layout, Menu, Tooltip, Badge } from 'antd';
import { DisconnectOutlined, CheckCircleOutlined, LoadingOutlined } from '@ant-design/icons';
import { ThemeSwitch } from './ThemeSwitch';
import { useWS } from '@/providers/WSProvider';
import { useAuth } from '@/providers/AuthProvider';

const { Header: AntHeader } = Layout;

const navItems = [
  { key: 'home', href: '/[locale]/', labelKey: 'app.home' },
  { key: 'chat', href: '/[locale]/chat/', labelKey: 'app.chat' },
  { key: 'settings', href: '/[locale]/settings', labelKey: 'app.settings' },
] as const;

export function Header({ currentLocale }: { currentLocale: string }) {
  const t = useTranslations('app');
  const { connected, reconnecting } = useWS();
  const { disconnect } = useAuth();

  const items = navItems.map(({ key, href, labelKey }) => ({
    key,
    label: (
      <Link href={href.replace('[locale]', currentLocale)}>
        {t(labelKey.replace('app.', ''))}
      </Link>
    ),
  }));

  return (
    <AntHeader style={{ padding: '0 24px', borderBottom: '1px solid #f0f0f0' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: '100%', maxWidth: 1200, margin: '0 auto' }}>
        <Menu
          mode="horizontal"
          selectedKeys={[]}
          items={items}
          style={{ border: 'none', flex: 1, minWidth: 0 }}
        />
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginLeft: 24 }}>
          <ThemeSwitch />
          {connected && (
            <Tooltip title="Disconnect">
              <Badge color="#52c41a">
                <DisconnectOutlined
                  style={{ cursor: 'pointer', fontSize: 16 }}
                  onClick={disconnect}
                />
              </Badge>
            </Tooltip>
          )}
          {reconnecting && (
            <Tooltip title="Reconnecting...">
              <LoadingOutlined style={{ color: '#faad14' }} />
            </Tooltip>
          )}
        </div>
      </div>
    </AntHeader>
  );
}
