'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { Button, Card, Typography } from 'antd';
import { useRouter, useParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { dispatcher } from '@/lib/updateDispatcher';
import { getConversations } from '@/store/indexedDB';
import { api } from '@/lib/api';
import type { components } from '@/types/api';

const { Text } = Typography;

type PermissionEntry = components['schemas']['HumanInPermission'];

/**
 * Global notification component that listens for missed updates.
 * A "missed update" is a WS push whose target conversation has no active subscriber
 * (i.e. the user is viewing a different page/conversation).
 *
 * Placed in the root layout so it's always mounted.
 *
 * Behavior:
 * - On mount (on a project page): fetch /projects/{id}/permissions?status=pending, render all
 * - On WS missed update events: re-fetch the same endpoint, re-render
 * - WS payload is only used to know which conversation to navigate to; data always comes from API
 */
export function MissedUpdateNotification() {
  const [entries, setEntries] = useState<PermissionEntry[]>([]);
  const router = useRouter();
  const params = useParams<{ locale?: string; id?: string }>();
  const t = useTranslations('notification');
  const projectId = params.id || null;
  const navigatingRef = useRef<Set<string>>(new Set());

  // Sync pending permissions from the API for the current project.
  const syncFromAPI = useCallback(async () => {
    if (!projectId) return;
    try {
      const perms = await api.get<PermissionEntry[]>(
        `/projects/${projectId}/permissions?status=pending`
      );
      setEntries(perms);
    } catch {
      // Ignore fetch errors; stale state is fine
    }
  }, [projectId]);

  // On mount: initial sync from API
  useEffect(() => {
    if (!projectId) return;
    syncFromAPI();
  }, [projectId, syncFromAPI]);

  // On WS missed update events: re-sync from API instead of rendering payload directly.
  const handleMissed = useCallback(
    (_update: components['schemas']['Update'], _topic: string) => {
      // Only re-sync for interrupt-type updates that require user action.
      // We re-fetch the API so the UI always reflects server state.
      const interruptTypes = new Set([
        'human_in_the_loop.created',
        'permission.pending',
      ]);
      if (!interruptTypes.has(_update.type)) return;

      // Re-sync after a short delay to let the server persist the new record.
      setTimeout(syncFromAPI, 300);
    },
    [syncFromAPI]
  );

  // Register callback on mount
  useEffect(() => {
    dispatcher.setMissedCallback(handleMissed);
  }, [handleMissed]);

  const handleNavigate = useCallback(
    async (entry: PermissionEntry) => {
      const convId = entry.conversation_id;
      if (navigatingRef.current.has(convId)) return;
      navigatingRef.current.add(convId);

      // Try IndexedDB first, then API to get project_id
      let targetProjectId: string | null = null;
      try {
        const convs = await getConversations();
        const conv = convs.find((c) => c.id === convId) as { project_id?: string } | undefined;
        targetProjectId = conv?.project_id || null;
      } catch { /* ignore */ }

      if (!targetProjectId) {
        try {
          const conv = await api.get<{ project_id: string }>(`/conversations/${convId}`);
          targetProjectId = conv.project_id;
        } catch { /* ignore */ }
      }

      const locale = params.locale || 'en';
      if (targetProjectId) {
        router.push(`/${locale}/project/${targetProjectId}/chat/${convId}`);
      } else {
        router.push(`/${locale}`);
      }
    },
    [router, params.locale]
  );

  const handleDismiss = useCallback((convId: string) => {
    setEntries((prev) => prev.filter((e) => e.conversation_id !== convId));
  }, []);

  if (entries.length === 0) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 16,
        right: 16,
        zIndex: 9999,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        maxWidth: 360,
      }}
    >
      {entries.map((entry) => (
        <Card
          key={entry.id}
          size="small"
          style={{
            border: '1px solid #faad14',
            background: '#fffbe6',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
          }}
          extra={
            <Button
              type="text"
              size="small"
              onClick={() => handleDismiss(entry.conversation_id)}
              style={{ padding: 0 }}
            >
              ×
            </Button>
          }
        >
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Text strong>
              {t('newPermission') || 'New permission request'}
              {entry.tool_name ? ` — ${entry.tool_name}` : ''}
            </Text>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Button
                type="primary"
                size="small"
                onClick={() => handleNavigate(entry)}
              >
                {t('navigate') || 'Go to conversation'}
              </Button>
              <Button
                size="small"
                onClick={() => handleDismiss(entry.conversation_id)}
              >
                {t('dismiss') || 'Close'}
              </Button>
            </div>
          </div>
        </Card>
      ))}
    </div>
  );
}
