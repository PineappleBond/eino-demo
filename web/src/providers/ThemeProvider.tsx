'use client';

import { ConfigProvider, theme as antdTheme, App } from 'antd';
import { ReactNode, useState, useEffect } from 'react';
import { getLatestTheme } from '@/hooks/useTheme';

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeMode, setThemeMode] = useState<'light' | 'dark'>('light');

  useEffect(() => {
    setThemeMode(getLatestTheme());
  }, []);

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
