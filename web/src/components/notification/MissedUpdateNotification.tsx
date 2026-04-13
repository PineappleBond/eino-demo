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
type HITLEntry = components['schemas']['HumanInTheLoop'];

interface HitlCardEntry {
  id: string;
  conversation_id: string;
  question: string;
  choices: unknown;
  answer_type: string;
  checkpoint_id?: string;
  interrupt_id?: string;
}

let nextKey = 0;

/**
 * Global notification component that listens for missed updates.
 * A "missed update" is a WS push whose target conversation has no active subscriber
 * (i.e. the user is viewing a different page/conversation).
 *
 * Placed in the root layout so it's always mounted.
 *
 * Behavior:
 * - On mount (on a project page): fetch permissions + HITLs from API, render all
 * - On WS missed update events: re-fetch the same endpoints, re-render
 * - WS payload is only used to know a new event arrived; data always comes from API
 */
export function MissedUpdateNotification() {
  const [permissions, setPermissions] = useState<PermissionEntry[]>([]);
  const [hitls, setHitls] = useState<HitlCardEntry[]>([]);
  const router = useRouter();
  const params = useParams<{ locale?: string; id?: string }>();
  const t = useTranslations('notification');
  const projectId = params.id || null;
  const navigatingRef = useRef<Set<string>>(new Set());

  // Sync pending permissions from the API for the current project.
  const syncPermissionsFromAPI = useCallback(async () => {
    if (!projectId) return;
    try {
      const perms = await api.get<PermissionEntry[]>(
        `/projects/${projectId}/permissions?status=pending`
      );
      setPermissions(perms);
    } catch {
      // Ignore fetch errors; stale state is fine
    }
  }, [projectId]);

  // Sync pending HITLs from the API for the current project.
  const syncHitlsFromAPI = useCallback(async () => {
    if (!projectId) return;
    try {
      const hitlData = await api.get<components['schemas']['HumanInTheLoop'][]>(
        `/projects/${projectId}/hitls?status=pending`
      );
      setHitls(hitlData.map((h) => ({
        id: h.id,
        conversation_id: h.conversation_id,
        question: h.question,
        choices: h.choices,
        answer_type: h.answer_type,
        checkpoint_id: h.checkpoint_id,
        interrupt_id: h.interrupt_id,
      })));
    } catch {
      // Ignore fetch errors
    }
  }, [projectId]);

  const syncAll = useCallback(() => {
    syncPermissionsFromAPI();
    syncHitlsFromAPI();
  }, [syncPermissionsFromAPI, syncHitlsFromAPI]);

  // On mount: initial sync from API
  useEffect(() => {
    if (!projectId) return;
    syncAll();
  }, [projectId, syncAll]);

  // On WS missed update events: re-sync from API instead of rendering payload directly.
  const handleMissed = useCallback(
    (_update: components['schemas']['Update'], _topic: string) => {
      // Only re-sync for interrupt-type updates that require user action.
      const interruptTypes = new Set([
        'human_in_the_loop.created',
        'permission.pending',
      ]);
      if (!interruptTypes.has(_update.type)) return;

      // Re-sync after a short delay to let the server persist the new record.
      setTimeout(syncAll, 300);
    },
    [syncAll]
  );

  // Register callback on mount
  useEffect(() => {
    dispatcher.setMissedCallback(handleMissed);
  }, [handleMissed]);

  const handleNavigate = useCallback(
    async (conversationId: string) => {
      if (navigatingRef.current.has(conversationId)) return;
      navigatingRef.current.add(conversationId);

      // Try IndexedDB first, then API to get project_id
      let targetProjectId: string | null = null;
      try {
        const convs = await getConversations();
        const conv = convs.find((c) => c.id === conversationId) as { project_id?: string } | undefined;
        targetProjectId = conv?.project_id || null;
      } catch { /* ignore */ }

      if (!targetProjectId) {
        try {
          const conv = await api.get<{ project_id: string }>(`/conversations/${conversationId}`);
          targetProjectId = conv.project_id;
        } catch { /* ignore */ }
      }

      const locale = params.locale || 'en';
      if (targetProjectId) {
        router.push(`/${locale}/project/${targetProjectId}/chat/${conversationId}`);
      } else {
        router.push(`/${locale}`);
      }
    },
    [router, params.locale]
  );

  const dismissPermission = useCallback((permId: string) => {
    setPermissions((prev) => prev.filter((e) => e.id !== permId));
  }, []);

  const dismissHitl = useCallback((hitlId: string) => {
    setHitls((prev) => prev.filter((e) => e.id !== hitlId));
  }, []);

  if (permissions.length === 0 && hitls.length === 0) return null;

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
      {permissions.map((entry) => (
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
              onClick={() => dismissPermission(entry.id)}
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
                onClick={() => handleNavigate(entry.conversation_id)}
              >
                {t('navigate') || 'Go to conversation'}
              </Button>
              <Button
                size="small"
                onClick={() => dismissPermission(entry.id)}
              >
                {t('dismiss') || 'Close'}
              </Button>
            </div>
          </div>
        </Card>
      ))}

      {hitls.map((entry) => (
        <Card
          key={entry.id}
          size="small"
          style={{
            border: '1px solid #4096ff',
            background: '#e6f4ff',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
          }}
          extra={
            <Button
              type="text"
              size="small"
              onClick={() => dismissHitl(entry.id)}
              style={{ padding: 0 }}
            >
              ×
            </Button>
          }
        >
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Text strong>
              {t('newQuestion') || 'New question waiting'}
            </Text>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Button
                type="primary"
                size="small"
                onClick={() => handleNavigate(entry.conversation_id)}
              >
                {t('navigate') || 'Go to conversation'}
              </Button>
              <Button
                size="small"
                onClick={() => dismissHitl(entry.id)}
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
