'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { message, Empty } from 'antd';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';
import { api, Project } from '@/lib/api';

interface ProjectListProps {
  locale: string;
}

export function ProjectList({ locale }: ProjectListProps) {
  const t = useTranslations('home');
  const router = useRouter();
  const [messageApi, contextHolder] = message.useMessage();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchProjects = useCallback(() => {
    api.get<Project[]>('/projects')
      .then(setProjects)
      .catch(() => {
        // Silently fail — projects are optional on the home page
      })
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    fetchProjects();
  }, [fetchProjects]);

  // Listen for project lifecycle events on the system topic
  const fetchRef = useRef(fetchProjects);
  fetchRef.current = fetchProjects;

  const handleProjectUpdate = useCallback((update: Update) => {
    switch (update.type) {
      case 'project.created':
        fetchRef.current();
        break;
      case 'project.deleted': {
        const payload = update.payload as Record<string, unknown>;
        const deletedId = payload?.id as string | undefined;
        if (deletedId) {
          setProjects((prev) => prev.filter((p) => p.id !== deletedId));
        } else {
          fetchRef.current();
        }
        break;
      }
    }
  }, []);

  useSubscribe('system', handleProjectUpdate);

  if (loading || projects.length === 0) {
    return null;
  }

  return (
    <>
      {contextHolder}
      <div style={{ marginBottom: 32 }}>
        <h3 style={{ margin: '0 0 16px', color: 'var(--text-primary)', fontSize: 16, fontWeight: 600 }}>
          {t('myProjects') || 'My Projects'}
        </h3>
        <div className="template-grid">
          {projects.map((project) => (
            <div
              key={project.id}
              className="template-card"
              onClick={() => router.push(`/${locale}/project/${project.id}/chat`)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  router.push(`/${locale}/project/${project.id}/chat`);
                }
              }}
            >
              <div className="template-number">PROJECT</div>
              <div className="template-name">{project.name}</div>
              <div className="template-desc">
                {project.template_id}
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
