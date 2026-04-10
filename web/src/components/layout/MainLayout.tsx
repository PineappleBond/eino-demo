'use client';

import { ReactNode } from 'react';
import { Layout } from 'antd';
import { AppHeader } from './Header';

const { Content } = Layout;

export function MainLayout({ children, locale }: { children: ReactNode; locale: string }) {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <AppHeader currentLocale={locale} />
      <Content>{children}</Content>
    </Layout>
  );
}
