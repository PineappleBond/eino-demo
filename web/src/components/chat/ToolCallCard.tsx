'use client';

import { Collapse } from 'antd';
import { CodeOutlined } from '@ant-design/icons';

interface ToolCallCardProps {
  name: string;
  input: Record<string, unknown>;
  output: string;
  status: 'running' | 'done' | 'error';
  duration?: string;
}

export function ToolCallCard({ name, input, output, status, duration }: ToolCallCardProps) {
  const statusColor = status === 'done' ? '#52c41a' : status === 'error' ? '#ff4d4f' : '#1677ff';

  return (
    <Collapse
      size="small"
      items={[
        {
          key: '1',
          label: (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <CodeOutlined style={{ color: statusColor }} />
              <strong>{name}</strong>
              <span style={{ color: statusColor, fontSize: 12, textTransform: 'capitalize' }}>
                {status}
              </span>
              {duration && <span style={{ color: '#999', fontSize: 12 }}>{duration}</span>}
            </div>
          ),
          children: (
            <div style={{ fontSize: 13 }}>
              <details>
                <summary style={{ cursor: 'pointer', marginBottom: 4 }}>Input</summary>
                <pre style={{ background: '#f5f5f5', padding: 8, borderRadius: 4, overflow: 'auto' }}>
                  {JSON.stringify(input, null, 2)}
                </pre>
              </details>
              <details>
                <summary style={{ cursor: 'pointer', marginBottom: 4 }}>Output</summary>
                <pre style={{ background: '#f5f5f5', padding: 8, borderRadius: 4, overflow: 'auto' }}>
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
