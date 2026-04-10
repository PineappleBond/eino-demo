'use client';

import { ConfigProvider, theme as antdTheme } from 'antd';
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
      }}
    >
      {children}
    </ConfigProvider>
  );
}
