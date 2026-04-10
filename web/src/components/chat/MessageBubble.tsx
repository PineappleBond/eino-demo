'use client';

import { Avatar, Space, Typography } from 'antd';
import { UserOutlined, RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

const { Text } = Typography;

export function MessageBubble({ message }: { message: Message }) {
  const isUser = message.sender_role === 'user';

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: isUser ? 'flex-end' : 'flex-start',
      padding: '8px 16px',
      maxWidth: '75%',
      alignSelf: isUser ? 'flex-end' : 'flex-start',
    }}>
      <Space align="start" style={{ width: '100%', flexDirection: isUser ? 'row-reverse' : 'row' }}>
        <Avatar icon={isUser ? <UserOutlined /> : <RobotOutlined />} />
        <div style={{
          background: isUser ? '#1677ff' : '#fff',
          color: isUser ? '#fff' : 'inherit',
          borderRadius: 12,
          padding: '8px 12px',
          maxWidth: '100%',
          overflowWrap: 'break-word',
        }}>
          {isUser ? (
            <Text style={{ color: isUser ? '#fff' : 'inherit', whiteSpace: 'pre-wrap' }}>
              {message.content}
            </Text>
          ) : (
            <div style={{ fontSize: 14, lineHeight: 1.6 }}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {message.content}
              </ReactMarkdown>
            </div>
          )}
        </div>
      </Space>
      {/* Show tool call cards below assistant messages if metadata contains tool calls */}
      {!isUser && (() => {
        const toolCalls = message.metadata?.tool_calls as Array<{ name: string; input: Record<string, unknown>; output: string; status: string }> | undefined;
        if (!toolCalls || !Array.isArray(toolCalls)) return null;
        return (
          <div style={{ marginTop: 8, width: '100%' }}>
            {toolCalls.map((tc, i) => (
              <ToolCallCard
                key={i}
                name={tc.name}
                input={tc.input}
                output={tc.output}
                status={tc.status as 'running' | 'done' | 'error'}
              />
            ))}
          </div>
        );
      })()}
    </div>
  );
}
