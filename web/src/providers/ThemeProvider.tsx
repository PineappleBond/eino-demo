'use client';

import { ConfigProvider, theme as antdTheme, App } from 'antd';
import { StyleProvider, createCache } from '@ant-design/cssinjs';
import { ReactNode, useState, useEffect, useRef } from 'react';
import { getLatestTheme, setTheme } from '@/hooks/useTheme';
import { api } from '@/lib/api';

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

  return (
    <StyleProvider cache={cacheRef.current}>
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
    </StyleProvider>
  );
}
