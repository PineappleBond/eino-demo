'use client';

import { ConfigProvider, theme as antdTheme, App } from 'antd';
import { StyleProvider, createCache } from '@ant-design/cssinjs';
import { ReactNode, useState, useEffect, useRef, useCallback } from 'react';
import { getLatestTheme, setTheme } from '@/hooks/useTheme';
import { api } from '@/lib/api';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeMode, setThemeMode] = useState<'light' | 'dark'>('light');
  const cacheRef = useRef(createCache());

  // Sync theme from API on mount (server is source of truth), fall back to localStorage.
  useEffect(() => {
    const localTheme = getLatestTheme();
    // Apply immediately to prevent flash
    document.documentElement.setAttribute('data-theme', localTheme);
    setThemeMode(localTheme);

    // Sync from API if authenticated
    api.get<{ theme?: string }>('/settings')
      .then((data) => {
        const apiTheme = data.theme as 'light' | 'dark';
        if (apiTheme && apiTheme !== localTheme) {
          setTheme(apiTheme);
          setThemeMode(apiTheme);
        }
      })
      .catch(() => {
        // Not authenticated or API unavailable — use localStorage
      });
  }, []);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', themeMode);
  }, [themeMode]);

  // Listen for settings changes from other tabs to sync theme
  const handleSettingsChange = useCallback((update: Update) => {
    if (update.type === 'settings.changed') {
      const payload = update.payload as Record<string, unknown>;
      if (payload.theme === 'light' || payload.theme === 'dark') {
        const newTheme = payload.theme as 'light' | 'dark';
        if (newTheme !== themeMode) {
          setTheme(newTheme);
          setThemeMode(newTheme);
        }
      }
    }
  }, [themeMode]);

  useSubscribe('system', handleSettingsChange);

  // Dynamic Ant Design theme tokens — these change when themeMode changes
  const isDark = themeMode === 'dark';

  return (
    <StyleProvider cache={cacheRef.current}>
      <ConfigProvider
        theme={{
          algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
          token: {
            colorPrimary: '#6c5ce7',
            borderRadius: 6,
            // Global text colors
            colorText: isDark ? '#e8e8ed' : '#1a1a2e',
            colorTextSecondary: isDark ? '#b8b8c0' : '#555568',
            colorTextTertiary: isDark ? '#b0b0b8' : '#7a7a8e',
            colorTextQuaternary: isDark ? '#b0b0b8' : '#7a7a8e',
            // Background colors
            colorBgContainer: isDark ? '#161618' : '#ffffff',
            colorBgElevated: isDark ? '#262629' : '#ffffff',
            colorBgLayout: isDark ? '#0e0e10' : '#ffffff',
            colorBgSpotlight: isDark ? '#2a2a2e' : '#1a1a2e',
            // Border colors
            colorBorder: isDark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(0, 0, 0, 0.10)',
            colorBorderSecondary: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
            // Misc
            colorTextLightSolid: '#ffffff',
          },
          components: {
            // Typography
            Typography: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
            },
            // Tag
            Tag: {
              borderRadius: 6,
              fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
              fontSize: 11,
            },
            // Collapse
            Collapse: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextHeading: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgContainer: 'transparent',
              colorBorder: 'transparent',
            },
            // Result
            Result: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextDescription: isDark ? '#b8b8c0' : '#555568',
            },
            // Empty
            Empty: {
              colorTextDescription: isDark ? '#b0b0b8' : '#7a7a8e',
            },
            // Spin
            Spin: {
              colorPrimary: '#6c5ce7',
            },
            // Dropdown menu
            Dropdown: {
              motionDurationMid: '0.15s',
              borderRadiusLG: 8,
            },
            // Menu (used in Dropdowns)
            Menu: {
              colorItemText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorItemTextHover: isDark ? '#e8e8ed' : '#1a1a2e',
              colorItemBg: isDark ? '#1e1e21' : '#ffffff',
              colorItemTextSelected: '#6c5ce7',
              colorItemBgSelected: isDark ? 'rgba(108, 92, 231, 0.12)' : 'rgba(108, 92, 231, 0.10)',
              colorItemBgHover: isDark ? '#2a2a2e' : '#e8e8ec',
              colorSplit: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
            },
            // Popconfirm / Popover
            Popover: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgElevated: isDark ? '#262629' : '#ffffff',
              colorBorder: isDark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(0, 0, 0, 0.10)',
            },
            // Modal
            Modal: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgContainer: isDark ? '#161618' : '#ffffff',
              colorBorderSecondary: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
              colorTextHeading: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextDescription: isDark ? '#b8b8c0' : '#555568',
            },
            // Drawer
            Drawer: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgElevated: isDark ? '#161618' : '#ffffff',
              colorBorder: isDark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(0, 0, 0, 0.10)',
              colorTextHeading: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextDescription: isDark ? '#b8b8c0' : '#555568',
              colorSplit: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
            },
            // Input
            Input: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgContainer: isDark ? '#1e1e21' : '#ffffff',
              colorBorder: isDark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(0, 0, 0, 0.10)',
              colorTextPlaceholder: isDark ? '#b0b0b8' : '#7a7a8e',
              activeBg: isDark ? '#1e1e21' : '#ffffff',
              hoverBg: isDark ? '#262629' : '#f0f0f2',
            },
            // Select
            Select: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgContainer: isDark ? '#1e1e21' : '#ffffff',
              colorBorder: isDark ? 'rgba(255, 255, 255, 0.10)' : 'rgba(0, 0, 0, 0.10)',
              colorTextPlaceholder: isDark ? '#b0b0b8' : '#7a7a8e',
              colorBgElevated: isDark ? '#1e1e21' : '#ffffff',
              colorTextDescription: isDark ? '#b8b8c0' : '#555568',
            },
            // Button
            Button: {
              colorTextDisabled: isDark ? '#b0b0b8' : '#7a7a8e',
              colorTextLightSolid: '#ffffff',
            },
            // Card
            Card: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextHeading: isDark ? '#e8e8ed' : '#1a1a2e',
              colorBgContainer: isDark ? '#161618' : '#ffffff',
              colorBorder: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
              colorTextDescription: isDark ? '#b8b8c0' : '#555568',
              colorBorderSecondary: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
            },
            // Divider
            Divider: {
              colorSplit: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
            },
            // Descriptions
            Descriptions: {
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
              colorTextDescription: isDark ? '#b0b0b8' : '#7a7a8e',
            },
            // Switch
            Switch: {
              colorPrimary: '#6c5ce7',
              colorTextQuaternary: isDark ? '#b0b0b8' : '#7a7a8e',
            },
            // Message
            Message: {
              contentBg: isDark ? '#262629' : '#ffffff',
              colorText: isDark ? '#e8e8ed' : '#1a1a2e',
            },
            // Layout
            Layout: {
              bodyBg: isDark ? '#0e0e10' : '#ffffff',
              headerBg: isDark ? '#161618' : '#ffffff',
              siderBg: isDark ? '#161618' : '#ffffff',
              triggerBg: isDark ? '#1e1e21' : '#ffffff',
            },
          },
        }}
      >
        <App>{children}</App>
      </ConfigProvider>
    </StyleProvider>
  );
}
