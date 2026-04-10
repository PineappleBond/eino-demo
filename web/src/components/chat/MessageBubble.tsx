'use client';

import { Avatar, Typography } from 'antd';
import { UserOutlined, RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

const { Text } = Typography;

export function MessageBubble({ message }: { message: Message }) {
  const isUser = message.sender_role === 'user';

  return (
    <div className="slide-up" style={{ marginBottom: 24 }}>
      {/* Sender info */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <Avatar
          size={24}
          icon={isUser ? <UserOutlined /> : <RobotOutlined />}
          style={{
            fontSize: 11,
            fontWeight: 700,
            background: isUser ? 'var(--accent)' : 'var(--bg-elevated)',
            flexShrink: 0,
          }}
        />
        <Text style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)' }}>
          {isUser ? 'You' : 'Assistant'}
        </Text>
        {message.created_at && (
          <Text style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
            {new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </Text>
        )}
      </div>

      {/* Content */}
      <div style={{ paddingLeft: 32 }}>
        {isUser ? (
          <Text style={{ fontSize: 14, lineHeight: 1.65, color: 'var(--text-primary)', whiteSpace: 'pre-wrap' }}>
            {message.content}
          </Text>
        ) : (
          <div style={{ fontSize: 14, lineHeight: 1.65, color: 'var(--text-primary)' }}>
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                code: ({ children, ...props }) => {
                  const isBlock = props.className?.includes('language-') || String(children).includes('\n');
                  if (isBlock) {
                    return (
                      <pre style={{
                        background: 'var(--bg-secondary)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: 'var(--radius-md)',
                        padding: '14px 16px',
                        margin: '8px 0',
                        overflowX: 'auto',
                        fontFamily: 'var(--font-mono)',
                        fontSize: 12.5,
                        lineHeight: 1.6,
                      }}>
                        <code {...props}>{children}</code>
                      </pre>
                    );
                  }
                  return (
                    <code style={{
                      fontFamily: 'var(--font-mono)',
                      background: 'var(--bg-elevated)',
                      padding: '2px 6px',
                      borderRadius: 4,
                      fontSize: 13,
                    }} {...props}>
                      {children}
                    </code>
                  );
                },
                pre: ({ children }) => <>{children}</>,
              }}
            >
              {message.content}
            </ReactMarkdown>
          </div>
        )}

        {/* Tool calls */}
        {(() => {
          const toolCalls = message.metadata?.tool_calls as Array<{
            name: string;
            input: Record<string, unknown>;
            output: string;
            status: string;
          }> | undefined;
          if (!toolCalls || !Array.isArray(toolCalls)) return null;
          return (
            <div style={{ marginTop: 12 }}>
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
    </div>
  );
}
