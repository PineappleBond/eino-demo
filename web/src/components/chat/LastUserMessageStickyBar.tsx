'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { UserOutlined, ArrowDownOutlined, ArrowUpOutlined, CloseOutlined, RollbackOutlined } from '@ant-design/icons';
import { Message } from '@/lib/api';

interface LastUserMessageStickyBarProps {
  lastUserMessage: Message;
  lastUserMsgIdx: number;
  containerRef: React.RefObject<HTMLDivElement | null>;
  messageContainerRef: React.RefObject<HTMLDivElement | null>;
}

export function LastUserMessageStickyBar({
  lastUserMessage,
  lastUserMsgIdx,
  containerRef,
  messageContainerRef,
}: LastUserMessageStickyBarProps) {
  const t = useTranslations('chat');
  const [isStuck, setIsStuck] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const [needsExpand, setNeedsExpand] = useState(false);

  // Detect when the bar becomes sticky by checking scroll position
  useEffect(() => {
    const container = containerRef.current;
    const messages = messageContainerRef.current;
    if (!container || !messages) return;

    const checkStuck = () => {
      // The bar's position in the scroll container
      const msgEl = messages.children[lastUserMsgIdx] as HTMLElement | undefined;
      if (!msgEl) return;
      const barTop = msgEl.offsetTop + msgEl.offsetHeight; // bar is right after message

      // Bar is "stuck" when the scroll has passed the bar's natural position
      const isStuckNow = container.scrollTop > barTop - 4; // 4px tolerance for top offset
      setIsStuck(isStuckNow);
    };

    // Initial check after layout settles
    const timer = setTimeout(checkStuck, 300);

    container.addEventListener('scroll', checkStuck, { passive: true });
    window.addEventListener('resize', checkStuck, { passive: true });
    return () => {
      clearTimeout(timer);
      container.removeEventListener('scroll', checkStuck);
      window.removeEventListener('resize', checkStuck);
    };
  }, [containerRef, messageContainerRef, lastUserMsgIdx]);

  // Detect if content overflows the collapsed height
  const contentRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    setNeedsExpand(el.scrollHeight > (isExpanded ? 250 : 100));
  }, [lastUserMessage.content, isExpanded]);

  const handleBackToMessage = useCallback(() => {
    const messages = messageContainerRef.current;
    if (!messages) return;
    const msgEl = messages.children[lastUserMsgIdx] as HTMLElement | undefined;
    if (msgEl) {
      msgEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  }, [messageContainerRef, lastUserMsgIdx]);

  const handleClose = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    // Hide the bar by hiding its content (keep it in DOM for layout)
    (e.currentTarget.closest('.message-sticky-bar') as HTMLElement | null)?.style.setProperty('display', 'none');
  }, []);

  const handleToggleExpand = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  // Don't render if message has no content
  if (!lastUserMessage.content?.trim()) return null;

  const timeStr = lastUserMessage.created_at
    ? new Date(lastUserMessage.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '';

  return (
    <div
      className="message-sticky-bar"
      style={{
        opacity: isStuck ? 1 : 0,
        pointerEvents: isStuck ? 'auto' : 'none',
        transition: 'opacity 0.2s ease',
      }}
    >
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
