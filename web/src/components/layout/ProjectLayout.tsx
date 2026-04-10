'use client';

import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { Layout } from 'antd';
import { TopBar } from '@/components/layout/Header';
import { ChatSider } from '@/components/layout/ChatSider';

const { Content, Sider } = Layout;

export function ProjectLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const convIdFromPath = pathname.split('/').pop() || '';
  const [selectedConv, setSelectedConv] = useState(convIdFromPath || '');

  return (
    <Layout style={{ height: '100vh' }}>
      <TopBar currentLocale={(pathname.match(/\/([^/]+)\//) || [])[1] || 'en'} />
      <Layout>
        <Sider
          width={240}
          theme="light"
          style={{ borderRight: '1px solid #f0f0f0', background: '#fff' }}
        >
          <ChatSider onSelect={setSelectedConv} selectedKey={selectedConv} />
        </Sider>
        <Content style={{ background: '#fafafa' }}>{children}</Content>
      </Layout>
    </Layout>
  );
}
