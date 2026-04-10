'use client';

import { useEffect, useRef, useState, memo } from 'react';
import { Typography } from 'antd';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { prepare, layout } from '@chenglou/pretext';

const { Text } = Typography;

interface StreamingTextProps {
  content: string;
  isStreaming: boolean;
  font?: string;
  lineHeight?: number;
}

const CURSOR_FONT = '14px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';

/**
 * Streaming text with pretext-based cursor positioning.
 * Renders markdown content with a blinking cursor at the end while streaming.
 */
export const StreamingText = memo(function StreamingText({
  content,
  isStreaming,
  font = CURSOR_FONT,
  lineHeight = 22.4,
}: StreamingTextProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [cursorOffset, setCursorOffset] = useState<{ top: number; left: number }>({ top: 0, left: 0 });
  const prevContentRef = useRef('');

  // Update cursor position using pretext when content changes
  useEffect(() => {
    if (!isStreaming || !content) return;

    // Measure text dimensions using pretext for accurate cursor placement
    try {
      const prepared = prepare(content, font, { whiteSpace: 'pre-wrap' });
      const containerWidth = containerRef.current?.clientWidth ?? 600;
      const { height, lineCount } = layout(prepared, containerWidth, lineHeight);

      // Cursor goes at the end: bottom of the last line
      setCursorOffset({
        top: height - lineHeight,
        left: 0, // Cursor at start of the (potentially partial) last line
      });
    } catch {
      // Fallback if pretext fails
    }

    prevContentRef.current = content;
  }, [content, isStreaming, font, lineHeight]);

  if (!content && !isStreaming) {
    return <Text type="secondary">Thinking...</Text>;
  }

  return (
    <div style={{ position: 'relative' }}>
      <div ref={containerRef} style={{ fontSize: 14, lineHeight: 1.6, position: 'relative' }}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {content}
        </ReactMarkdown>
      </div>
      {isStreaming && (
        <span
          style={{
            position: 'absolute',
            top: cursorOffset.top,
            left: cursorOffset.left,
            display: 'inline-block',
            width: 2,
            height: lineHeight * 0.8,
            background: '#1677ff',
            animation: 'blink 1s step-end infinite',
          }}
        />
      )}
      <style>{`
        @keyframes blink {
          0%, 100% { opacity: 1; }
          50% { opacity: 0; }
        }
      `}</style>
    </div>
  );
});
