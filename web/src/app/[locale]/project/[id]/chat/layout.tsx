'use client';

import { useState, useEffect } from 'react';
import { usePathname, useParams } from 'next/navigation';
import { Layout, Spin } from 'antd';
import { TopBar } from '@/components/layout/Header';
import { ChatSider } from '@/components/layout/ChatSider';
import { api } from '@/lib/api';

const { Sider, Content } = Layout;

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const params = useParams();
  const locale = params.locale as string;
  const projectId = params.id as string;
  const convIdFromPath = pathname.split('/').pop() || '';
  const [selectedConv, setSelectedConv] = useState(convIdFromPath);

  // Sync selected conversation when route changes
  useEffect(() => {
    const currentConvId = pathname.split('/').pop() || '';
    setSelectedConv(currentConvId);
  }, [pathname]);
  const [projectName, setProjectName] = useState('Project');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (projectId) {
      api.get<{ name: string }>(`/projects/${projectId}`)
        .then((p) => setProjectName(p.name || 'Project'))
        .catch(() => setProjectName('Project'))
        .finally(() => setLoading(false));
    }
  }, [projectId]);

  if (loading) {
    return <Spin size="large" style={{ display: 'flex', justifyContent: 'center', padding: '80px 0' }} />;
  }

  return (
    <Layout style={{ height: '100vh' }}>
      <TopBar currentLocale={locale} projectName={projectName} projectId={projectId} />
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
        <div style={{
          background: 'var(--bg-primary)',
          display: 'flex',
          flexDirection: 'column',
          minWidth: 0,
          flex: 1,
          overflow: 'hidden',
        }}>
          {children}
        </div>
      </Layout>
    </Layout>
  );
}
