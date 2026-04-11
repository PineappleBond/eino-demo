'use client';

import { Avatar, Typography } from 'antd';
import { UserOutlined, RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { useTranslations } from 'next-intl';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

const { Text } = Typography;

export function MessageBubble({ message }: { message: Message }) {
  const t = useTranslations('chat');
  const isUser = message.sender_role === 'user';

  return (
    <div
      className="message-group"
    >
      {/* Sender info */}
      <div className="message-sender">
        <Avatar
          size={24}
          className="message-avatar"
          icon={isUser ? <UserOutlined /> : <RobotOutlined />}
          style={{
            fontSize: 11,
            background: isUser ? 'var(--accent)' : 'var(--bg-elevated)',
          }}
        />
        <span className="message-name">
          {isUser ? t('you') : t('assistant')}
        </span>
        {message.created_at && (
          <span className="message-time">
            {new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </span>
        )}
      </div>

      {/* Content */}
      <div className="message-content">
        {isUser ? (
          <Text style={{ fontSize: 14, lineHeight: 1.65, color: 'var(--text-primary)', whiteSpace: 'pre-wrap' }}>
            {message.content}
          </Text>
        ) : (
          <div style={{ fontSize: 14, lineHeight: 1.65, color: 'var(--text-primary)' }}>
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeHighlight]}
              components={{
                code: ({ children, ...props }) => {
                  const isBlock = props.className?.includes('language-') || String(children).includes('\n');
                  if (isBlock) {
                    return (
                      <pre style={{ margin: '8px 0' }}>
                        <code {...props}>{children}</code>
                      </pre>
                    );
                  }
                  return <code {...props}>{children}</code>;
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
