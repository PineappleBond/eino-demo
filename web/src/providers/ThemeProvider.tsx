'use client';

import { ConfigProvider, theme as antdTheme, App } from 'antd';
import { ReactNode, useState, useEffect } from 'react';
import { getLatestTheme, setTheme } from '@/hooks/useTheme';
import { api } from '@/lib/api';

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeMode, setThemeMode] = useState<'light' | 'dark'>('light');

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

  return (
    <ConfigProvider
      theme={{
        algorithm: themeMode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#6c5ce7',
          borderRadius: 6,
        },
      }}
    >
      <App>{children}</App>
    </ConfigProvider>
  );
}
