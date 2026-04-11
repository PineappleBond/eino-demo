'use client';

import { useState, useRef } from 'react';
import { Avatar, Collapse, Tag } from 'antd';
import { RobotOutlined, BulbOutlined, ToolOutlined, InfoCircleOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import { useTranslations } from 'next-intl';
import { Message } from '@/lib/api';
import { ToolCallCard } from './ToolCallCard';

const SYSTEM_MSG_MAX_LINES = 5;

function SystemMessage({ content }: { content: string }) {
  const [expanded, setExpanded] = useState(false);
  const contentRef = useRef<HTMLSpanElement>(null);
  const isLong = content.length > 200;

  return (
    <div className="message-system">
      <InfoCircleOutlined style={{ fontSize: 12, color: 'var(--text-tertiary)', flexShrink: 0, marginTop: 2 }} />
      <div
        ref={contentRef}
        className="message-system-text"
        style={{
          display: '-webkit-box',
          WebkitLineClamp: expanded ? 'unset' : SYSTEM_MSG_MAX_LINES,
          WebkitBoxOrient: 'vertical',
          overflow: 'hidden',
          position: 'relative',
        }}
      >
        {content}
      </div>
      {isLong && (
        <button
          className="message-system-toggle-btn"
          type="button"
          onClick={() => setExpanded(!expanded)}
          style={{ flexShrink: 0, marginTop: 2 }}
        >
          {expanded ? '收起' : '展开'}
        </button>
      )}
    </div>
  );
}

export function MessageBubble({ message }: { message: Message }) {
  const t = useTranslations('chat');
  const isUser = message.sender_role === 'user';
  const isTool = message.sender_role === 'tool';
  const isSystem = message.sender_role === 'system';

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

  // Tool message: render tool_calling from the dedicated field
  if (isTool) {
    const toolCalling = message.tool_calling;
    const toolName = message.metadata?.tool_name as string | undefined || message.sender_id || 'tool';

    return (
      <div className="message-tool">
        <div className="message-bubble">
          <Tag icon={<ToolOutlined />} color="blue">
            {toolName}
          </Tag>
          {toolCalling && (
            <ToolCallCard
              name={toolName}
              input={toolCalling.input ?? {}}
              output={toolCalling.output ?? ''}
              status="done"
            />
          )}
        </div>
        {timeStr && <div className="message-time">{timeStr}</div>}
      </div>
    );
  }

  // System message: centered, muted style for compressed context / system notices
  if (isSystem) {
    return <SystemMessage content={message.content} />;
  }

  if (isUser) {
    return (
      <div className="message-user">
        <div className="message-bubble">
          <div className="message-bubble-text" style={{ whiteSpace: 'pre-wrap' }}>
            {message.content}
          </div>
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
        {timeStr && <div className="message-time">{timeStr}</div>}
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
          {message.reason_content && (
            <Collapse
              size="small"
              ghost
              items={[
                {
                  key: 'thinking',
                  label: (
                    <span style={{ fontSize: 12, opacity: 0.6 }}>
                      <BulbOutlined style={{ marginRight: 4 }} />
                      {t('thinking')}
                    </span>
                  ),
                  children: (
                    <div
                      style={{
                        fontSize: 13,
                        lineHeight: 1.6,
                        whiteSpace: 'pre-wrap',
                        color: 'var(--text-secondary)',
                        maxHeight: 300,
                        overflow: 'auto',
                      }}
                    >
                      {message.reason_content}
                    </div>
                  ),
                },
              ]}
            />
          )}
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
