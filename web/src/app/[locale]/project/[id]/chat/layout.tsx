'use client';

import { useState } from 'react';
import { useParams, useRouter, usePathname } from 'next/navigation';
import { Layout } from 'antd';
import { ChatSider } from '@/components/layout/ChatSider';
import { Header } from '@/components/layout/Header';

const { Content } = Layout;

export default function ChatLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const convIdFromPath = pathname.split('/').pop() || '';
  const [selectedConv, setSelectedConv] = useState(convIdFromPath || '');
  const { locale } = params as unknown as { locale: string };

  const handleSelect = (convId: string) => {
    setSelectedConv(convId);
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header currentLocale={locale} />
      <Layout>
        <ChatSider onSelect={handleSelect} selectedKey={selectedConv} />
        <Content style={{ padding: 0, background: '#f5f5f5' }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  );
}
