'use client';

import { Avatar } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { useTranslations } from 'next-intl';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

export function MessageBubble({ message }: { message: Message }) {
  const t = useTranslations('chat');
  const isUser = message.sender_role === 'user';

  const timeStr = message.created_at
    ? new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '';

  const toolCalls = message.metadata?.tool_calls as Array<{
    name: string;
    input: Record<string, unknown>;
    output: string;
    status: string;
  }> | undefined;
  const hasToolCalls = toolCalls && Array.isArray(toolCalls) && toolCalls.length > 0;

  if (isUser) {
    return (
      <div className="message-user">
        <div className="message-bubble">
          <div className="message-bubble-text" style={{ whiteSpace: 'pre-wrap' }}>
            {message.content}
          </div>
          {timeStr && <div className="message-time">{timeStr}</div>}
          {hasToolCalls && (
            <div className="message-tool-calls">
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
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="message-assistant">
      <div className="message-avatar-wrapper">
        <Avatar
          size={28}
          className="message-avatar"
          icon={<RobotOutlined />}
          style={{ fontSize: 14, background: 'var(--bg-elevated)', color: 'var(--text-secondary)' }}
        />
        <div className="message-content-area">
          <span className="message-sender-name">{t('assistant')}</span>
          <div className="message-bubble">
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
          {timeStr && <div className="message-time">{timeStr}</div>}
          {hasToolCalls && (
            <div className="message-tool-calls">
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
          )}
        </div>
      </div>
    </div>
  );
}
