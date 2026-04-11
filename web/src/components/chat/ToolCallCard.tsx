'use client';

import { Collapse, Typography } from 'antd';
import { CodeOutlined } from '@ant-design/icons';
import { useTranslations } from 'next-intl';

const { Text } = Typography;

interface ToolCallCardProps {
  name: string;
  input: Record<string, unknown>;
  output: string;
  status: 'running' | 'done' | 'error';
  duration?: string;
}

const statusColors = {
  done: 'var(--green)',
  error: 'var(--red)',
  running: 'var(--blue)',
};

export function ToolCallCard({ name, input, output, status, duration }: ToolCallCardProps) {
  const t = useTranslations('tools');
  const statusColor = statusColors[status];

  const statusLabel = status === 'running' ? t('running') : status === 'done' ? t('done') : t('error');

  return (
    <Collapse
      size="small"
      style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: 'var(--radius-md)',
        marginBottom: 12,
      }}
      items={[
        {
          key: '1',
          label: (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <CodeOutlined style={{ color: statusColor }} />
              <Text strong style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-primary)' }}>{name}</Text>
              <Text style={{ color: statusColor, fontSize: 12, textTransform: 'capitalize' }}>
                {statusLabel}
              </Text>
              {duration && (
                <Text style={{ color: 'var(--text-tertiary)', fontSize: 12, fontFamily: 'var(--font-mono)' }}>
                  {duration}
                </Text>
              )}
            </div>
          ),
          children: (
            <div style={{ fontSize: 13 }}>
              <details>
                <summary style={{ cursor: 'pointer', marginBottom: 4, color: 'var(--text-secondary)' }}>{t('input')}</summary>
                <pre style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  padding: '8px 12px',
                  borderRadius: 'var(--radius-sm)',
                  overflow: 'auto',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 12,
                  lineHeight: 1.5,
                  color: 'var(--text-primary)',
                }}>
                  {JSON.stringify(input, null, 2)}
                </pre>
              </details>
              <details style={{ marginTop: 8 }}>
                <summary style={{ cursor: 'pointer', marginBottom: 4, color: 'var(--text-secondary)' }}>{t('output')}</summary>
                <pre style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  padding: '8px 12px',
                  borderRadius: 'var(--radius-sm)',
                  overflow: 'auto',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 12,
                  lineHeight: 1.5,
                  color: 'var(--text-primary)',
                }}>
                  {output}
                </pre>
              </details>
            </div>
          ),
        },
      ]}
    />
  );
}
