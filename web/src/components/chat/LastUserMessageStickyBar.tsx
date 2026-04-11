'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { UserOutlined, ArrowDownOutlined, ArrowUpOutlined, CloseOutlined, RollbackOutlined } from '@ant-design/icons';
import { Message } from '@/lib/api';

interface LastUserMessageStickyBarProps {
  lastUserMessage: Message;
  messageElement: HTMLDivElement | null;
  containerRef: React.RefObject<HTMLDivElement | null>;
}

export function LastUserMessageStickyBar({
  lastUserMessage,
  messageElement,
  containerRef,
}: LastUserMessageStickyBarProps) {
  const t = useTranslations('chat');
  const [isIntersecting, setIsIntersecting] = useState(true);
  const [isExpanded, setIsExpanded] = useState(false);
  const [isManuallyClosed, setIsManuallyClosed] = useState(false);
  const [needsExpand, setNeedsExpand] = useState(false);

  // IntersectionObserver: detect when the message scrolls out of view.
  // Takes the actual DOM element (not a ref) so it re-subscribes when the element changes.
  useEffect(() => {
    const root = containerRef.current;
    if (!messageElement || !root) return;
    if (typeof IntersectionObserver === 'undefined') return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        setIsIntersecting(entry.isIntersecting);
        if (entry.isIntersecting) {
          setIsManuallyClosed(false);
        }
      },
      { root, threshold: 0 }
    );

    observer.observe(messageElement);
    return () => observer.disconnect();
  }, [messageElement, containerRef]);

  // Detect if content overflows the collapsed height
  const contentRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    setNeedsExpand(el.scrollHeight > (isExpanded ? 250 : 100));
  }, [lastUserMessage.content, isExpanded]);

  const handleBackToMessage = useCallback(() => {
    messageElement?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }, [messageElement]);

  const handleClose = useCallback(() => {
    setIsManuallyClosed(true);
  }, []);

  const handleToggleExpand = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  // Don't render if message is visible, manually closed, or has no content
  if (isIntersecting || isManuallyClosed || !lastUserMessage.content?.trim()) return null;

  const timeStr = lastUserMessage.created_at
    ? new Date(lastUserMessage.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '';

  return (
    <div className="message-sticky-bar">
      <div className="message-sticky-bubble">
        <div className="message-sticky-content">
          <div
            ref={contentRef}
            className={isExpanded ? 'message-sticky-expanded' : 'message-sticky-collapsed'}
            style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
          >
            {lastUserMessage.content}
            {!isExpanded && needsExpand && (
              <div className="message-sticky-gradient">
                <button className="message-sticky-expand-btn" type="button" onClick={handleToggleExpand}>
                  {t('expandAll')} <ArrowDownOutlined style={{ fontSize: 10 }} />
                </button>
              </div>
            )}
          </div>
        </div>
        {isExpanded && needsExpand && (
          <div style={{ textAlign: 'right', marginTop: 4 }}>
            <button className="message-sticky-close-btn" type="button" onClick={handleToggleExpand} style={{ fontSize: 10 }}>
              <ArrowUpOutlined style={{ fontSize: 10 }} /> {t('collapse')}
            </button>
          </div>
        )}
        <div className="message-sticky-action-row">
          <span><UserOutlined /> {t('you')}{timeStr && ` · ${timeStr}`}</span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="message-sticky-back-btn" type="button" onClick={handleBackToMessage}>
              <RollbackOutlined style={{ marginRight: 2 }} /> {t('backToMessage')}
            </button>
            <button className="message-sticky-close-btn" type="button" onClick={handleClose}>
              <CloseOutlined />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
