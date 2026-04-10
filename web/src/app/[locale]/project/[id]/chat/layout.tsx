'use client';

import { useState } from 'react';
import { usePathname, useParams } from 'next/navigation';
import { Layout } from 'antd';
import { TopBar } from '@/components/layout/Header';
import { ChatSider } from '@/components/layout/ChatSider';

const { Sider, Content } = Layout;

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const params = useParams();
  const locale = params.locale as string;
  const projectId = params.id as string;
  const convIdFromPath = pathname.split('/').pop() || '';
  const [selectedConv, setSelectedConv] = useState(convIdFromPath);

  return (
    <Layout style={{ height: '100vh' }}>
      <TopBar currentLocale={locale} projectName="Project" projectId={projectId} />
      <Layout>
        <Sider
          width={240}
          theme="dark"
          style={{
            background: 'var(--bg-secondary)',
            borderRight: '1px solid var(--border-subtle)',
            overflow: 'hidden',
            flexShrink: 0,
          }}
        >
          <ChatSider selectedKey={selectedConv} />
        </Sider>
        <Content style={{
          background: 'var(--bg-primary)',
          display: 'flex',
          flexDirection: 'column',
          minWidth: 0,
          width: '100%',
          padding: 0,
          overflow: 'hidden',
        }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  );
}
