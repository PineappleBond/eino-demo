'use client';

import { useState, type CSSProperties } from 'react';
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
import { ProjectInfoDrawer } from './ProjectInfoDrawer';

const { Header } = Layout;

const actionBtnStyle: CSSProperties = {
  width: 36,
  height: 36,
  minWidth: 36,
  color: 'var(--text-secondary)',
  borderRadius: 'var(--radius-sm)',
};

interface TopBarProps {
  currentLocale: string;
  projectName?: string;
  projectId?: string;
}

export function TopBar({ currentLocale, projectName, projectId }: TopBarProps) {
  const t = useTranslations('app');
  const { theme, setTheme } = useTheme();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [projectInfoOpen, setProjectInfoOpen] = useState(false);
  const [displayName, setDisplayName] = useState(projectName || '');

  const toggleTheme = () => setTheme(theme === 'dark' ? 'light' : 'dark');

  const toggleLocale = () => {
    const newLocale = currentLocale === 'en' ? 'zh' : 'en';
    document.cookie = `NEXT_LOCALE=${newLocale};path=/;max-age=31536000`;
    const pathname = window.location.pathname;
    const newPathname = pathname.replace(`/${currentLocale}`, `/${newLocale}`);
    window.location.href = newPathname;
  };

  const handleSettingsClick = () => {
    if (projectId) {
      setProjectInfoOpen(true);
    } else {
      setSettingsOpen(true);
    }
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
            {displayName ? (
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'home',
                      label: t('home'),
                      onClick: () => window.location.href = `/${currentLocale}`,
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
                    {displayName}
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
              onClick={handleSettingsClick}
              style={actionBtnStyle}
              title={projectId ? 'Project Info' : 'Settings'}
            />
            <Button
              type="text"
              icon={theme === 'dark' ? <SunOutlined /> : <MoonOutlined />}
              onClick={toggleTheme}
              style={actionBtnStyle}
            />
            <Button
              type="text"
              icon={<GlobalOutlined />}
              onClick={toggleLocale}
              style={{ ...actionBtnStyle, width: 52, minWidth: 52, gap: 4 }}
            >
              <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: 0.5 }}>
                {currentLocale.toUpperCase()}
              </span>
            </Button>
          </Space>
        </div>
      </Header>

      {/* Global settings drawer (home page) */}
      <SettingsDrawer open={settingsOpen} onClose={() => setSettingsOpen(false)} />

      {/* Project info drawer (project pages) */}
      {projectId && (
        <ProjectInfoDrawer
          open={projectInfoOpen}
          projectId={projectId}
          onClose={() => setProjectInfoOpen(false)}
          onNameChange={setDisplayName}
        />
      )}
    </>
  );
}
