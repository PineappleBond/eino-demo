'use client';

import { useRouter } from 'next/navigation';
import { Card, Tag, Button, Space, message, Typography } from 'antd';
import { RocketOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';
import { api, TemplateInfo } from '@/lib/api';

const { Text } = Typography;

interface TemplateCardProps extends TemplateInfo {
  locale: string;
}

const difficultyColors: Record<string, string> = {
  beginner: 'green',
  intermediate: 'orange',
  advanced: 'red',
};

export function TemplateCard({ id, name, description, tags, difficulty, locale }: TemplateCardProps) {
  const router = useRouter();
  const t = useTranslations('home');
  const [messageApi, contextHolder] = message.useMessage();

  const handleStart = async () => {
    try {
      const res = await api.post<{ id: string }>(`/templates/${id}/projects`, { name });
      messageApi.success(`Project "${name}" created`);
      router.push(`/${locale}/project/${res.id}/chat`);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to create project');
    }
  };

  return (
    <>
      {contextHolder}
      <Card
        hoverable
        actions={[
          <Button
            type="primary"
            icon={<RocketOutlined />}
            onClick={handleStart}
            style={{ borderRadius: 'var(--radius-sm)' }}
          >
            {t('start')}
          </Button>,
        ]}
      >
        <Card.Meta
          title={<Text style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{name}</Text>}
          description={
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              <Text style={{ color: 'var(--text-secondary)' }}>{description}</Text>
              <Space wrap>
                {tags.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
                <Tag color={difficultyColors[difficulty]}>
                  {t(`difficulty.${difficulty}`)}
                </Tag>
              </Space>
            </Space>
          }
        />
      </Card>
    </>
  );
}
