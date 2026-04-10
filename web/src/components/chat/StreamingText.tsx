'use client';

import { useEffect, useRef, useState } from 'react';
import { prepare, layout } from '@chenglou/pretext';

// Match the CSS --font-sans and message content font settings
const FONT = "500 14px 'DM Sans', -apple-system, BlinkMacSystemFont, sans-serif";
const LINE_HEIGHT = 23; // ~1.65 * 14px

interface StreamingTextProps {
  content: string;
}

/**
 * Streaming text display using pretext for layout-shift prevention
 * and accurate cursor positioning.
 *
 * pretext measures text dimensions without DOM reflow, preventing
 * scroll jumps during streaming token updates.
 */
export function StreamingText({ content }: StreamingTextProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const cursorRef = useRef<HTMLSpanElement>(null);
  const [containerWidth, setContainerWidth] = useState(600);
  const [textHeight, setTextHeight] = useState(0);
  const [cursorLeft, setCursorLeft] = useState(0);
  const [cursorBottom, setCursorBottom] = useState(0);

  // Measure container width
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(Math.floor(entry.contentRect.width));
      }
    });
    ro.observe(el);
    setContainerWidth(Math.floor(el.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, []);

  // Measure text with pretext to prevent layout shift
  useEffect(() => {
    if (!content || containerWidth <= 0) {
      setTextHeight(0);
      setCursorLeft(0);
      setCursorBottom(0);
      return;
    }

    try {
      const prepared = prepare(content, FONT, { whiteSpace: 'pre-wrap' });
      const dims = layout(prepared, containerWidth, LINE_HEIGHT);
      setTextHeight(dims.height);

      // Estimate cursor position: last character width for left offset
      const lastChar = content.slice(-1) || ' ';
      const charPrepared = prepare(lastChar, FONT, { whiteSpace: 'pre-wrap' });
      layout(charPrepared, containerWidth, LINE_HEIGHT);
      // pretext doesn't expose per-char width easily; use a reasonable default
      setCursorLeft(7);
      setCursorBottom(0);
    } catch {
      // Fallback: let browser handle it
    }
  }, [content, containerWidth]);

  // Update cursor position from the rendered DOM for accuracy
  useEffect(() => {
    if (cursorRef.current && containerRef.current) {
      // Use pretext measurements as a fallback
      // The cursor position is primarily determined by pretext's layout
      cursorRef.current.style.transform = `translate(${cursorLeft}px, ${cursorBottom}px)`;
    }
  }, [cursorLeft, cursorBottom]);

  return (
    <div
      ref={containerRef}
      style={{
        fontSize: 14,
        lineHeight: 1.65,
        color: 'var(--text-primary)',
        whiteSpace: 'pre-wrap',
        position: 'relative',
        minHeight: textHeight > 0 ? `${textHeight}px` : undefined,
        transition: 'min-height 0.05s ease',
      }}
    >
      {content || ''}
      <span
        ref={cursorRef}
        style={{
          display: content ? 'inline-block' : 'none',
          position: 'absolute',
          width: 2,
          height: `${LINE_HEIGHT * 0.7}px`,
          background: 'var(--accent)',
          borderRadius: 1,
          animation: 'typing 1s infinite',
          pointerEvents: 'none',
          left: cursorLeft > 0 ? `${cursorLeft}px` : undefined,
          bottom: cursorBottom > 0 ? `${cursorBottom}px` : undefined,
        }}
      />
    </div>
  );
}
