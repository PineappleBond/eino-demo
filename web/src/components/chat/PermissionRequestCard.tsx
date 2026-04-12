'use client';

import { Button, Card, Typography, Space, Tag } from 'antd';
import { LockOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';
import type { components } from '@/types/api';

const { Text, Paragraph } = Typography;

const safetyColors: Record<number, string> = { 1: 'green', 2: 'blue', 3: 'orange', 4: 'red' };
const safetyLabels: Record<number, string> = { 1: '低', 2: '中', 3: '高', 4: '严重' };

interface PermissionRequestCardProps {
  permission: components['schemas']['PermissionPendingPayload'];
  onAnswer: (decision: string) => Promise<void>;
  loading?: boolean;
}

export function PermissionRequestCard({ permission, onAnswer, loading = false }: PermissionRequestCardProps) {
  const t = useTranslations('permission');

  return (
    <Card
      size="small"
      style={{
        border: '1px solid var(--border-subtle, #d9d9d9)',
        background: 'var(--bg-secondary, #fafafa)',
        margin: '8px 0',
      }}
      title={
        <Space>
          <LockOutlined />
          <Text strong>{t('title')}</Text>
          <Tag color={safetyColors[permission.safety_level] || 'default'}>
            {t('risk')} {permission.safety_level} ({safetyLabels[permission.safety_level] || '?'})
          </Tag>
        </Space>
      }
    >
      <Paragraph>
        <Text strong>{permission.tool_name}</Text>{' '}
        {permission.tool_desc && `(${permission.tool_desc})`}
        <br />
        {permission.action}: <Text code>{permission.content}</Text>
      </Paragraph>

      {permission.safety_reason && (
        <Paragraph>
          <Text type="secondary">{permission.safety_reason}</Text>
        </Paragraph>
      )}

      {permission.args_summary && (
        <Paragraph>
          <Text code style={{ fontSize: 12 }}>
            {permission.args_summary}
          </Text>
        </Paragraph>
      )}

      <Space direction="vertical" style={{ width: '100%' }}>
        <Button
          block
          onClick={() => onAnswer('approved')}
          loading={loading}
          icon={<CheckOutlined />}
        >
          {t('approve')}
        </Button>
        <Button
          block
          onClick={() => onAnswer('approved_exact')}
          loading={loading}
        >
          {t('approveAndRemember')}
        </Button>
        <Button
          block
          onClick={() => onAnswer('approved_wildcard')}
          loading={loading}
        >
          {t('approveAndWildcard')}
        </Button>
        <Button
          block
          danger
          onClick={() => onAnswer('denied')}
          loading={loading}
          icon={<CloseOutlined />}
        >
          {t('deny')}
        </Button>
      </Space>
    </Card>
  );
}
