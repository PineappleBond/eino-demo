'use client';

import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { Layout } from 'antd';
import { ChatSider } from '@/components/layout/ChatSider';

const { Content } = Layout;

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const convIdFromPath = pathname.split('/').pop() || '';
  const [selectedConv, setSelectedConv] = useState(convIdFromPath || '');

  return (
    <Layout style={{ height: 'calc(100vh - 64px)' }}>
      <ChatSider onSelect={setSelectedConv} selectedKey={selectedConv} />
      <Content style={{ background: '#f5f5f5' }}>{children}</Content>
    </Layout>
  );
}
