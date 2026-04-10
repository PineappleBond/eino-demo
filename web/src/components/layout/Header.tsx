'use client';

import { useState } from 'react';
import { Layout, Space, Dropdown, Button } from 'antd';
import {
  SettingOutlined,
  SunOutlined,
  MoonOutlined,
  GlobalOutlined,
  DownOutlined,
} from '@ant-design/icons';
import { useTranslations } from 'next-intl';
import { useTheme } from '@/hooks/useTheme';
import { SettingsDrawer } from './SettingsDrawer';

const { Header } = Layout;

interface TopBarProps {
  currentLocale: string;
  projectName?: string;
  projectId?: string;
}

export function TopBar({ currentLocale, projectName, projectId }: TopBarProps) {
  const t = useTranslations('app');
  const { theme, setTheme } = useTheme();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const toggleTheme = () => setTheme(theme === 'dark' ? 'light' : 'dark');

  const toggleLocale = () => {
    document.cookie = `NEXT_LOCALE=${currentLocale === 'en' ? 'zh' : 'en'};path=/;max-age=31536000`;
    window.location.reload();
  };

  return (
    <>
      <Header
        style={{
          height: 52,
          padding: '0 16px',
          background: 'var(--bg-secondary)',
          borderBottom: '1px solid var(--border-subtle)',
          lineHeight: 'normal',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: '100%' }}>
          {/* Left: Project dropdown or Title */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            {projectName ? (
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'info',
                      label: t('home'),
                    },
                  ],
                }}
              >
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '6px 12px',
                  background: 'var(--bg-tertiary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: 'var(--radius-md)',
                  cursor: 'pointer',
                  transition: 'all 0.15s ease',
                  minWidth: 200,
                }}>
                  <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', flex: 1 }}>
                    {projectName}
                  </span>
                  <DownOutlined style={{ fontSize: 10, color: 'var(--text-tertiary)' }} />
                </div>
              </Dropdown>
            ) : (
              <span style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-primary)' }}>
                {t('title')}
              </span>
            )}
          </div>

          {/* Right: Action buttons */}
          <Space size={4}>
            <Button
              type="text"
              icon={<SettingOutlined />}
              onClick={() => setSettingsOpen(true)}
              style={{
                width: 32,
                height: 32,
                color: 'var(--text-secondary)',
                borderRadius: 'var(--radius-sm)',
              }}
            />
            <Button
              type="text"
              icon={theme === 'dark' ? <SunOutlined /> : <MoonOutlined />}
              onClick={toggleTheme}
              style={{
                width: 32,
                height: 32,
                color: 'var(--text-secondary)',
                borderRadius: 'var(--radius-sm)',
              }}
            />
            <Button
              type="text"
              icon={<GlobalOutlined />}
              onClick={toggleLocale}
              style={{
                width: 32,
                height: 32,
                minWidth: 32,
                color: 'var(--text-secondary)',
                borderRadius: 'var(--radius-sm)',
                fontSize: 12,
              }}
            >
              {currentLocale.toUpperCase()}
            </Button>
          </Space>
        </div>
      </Header>
      <SettingsDrawer open={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </>
  );
}
