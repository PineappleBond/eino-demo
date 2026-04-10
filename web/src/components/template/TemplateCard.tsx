'use client';

import { useRouter } from 'next/navigation';
import { message } from 'antd';
import { useTranslations } from 'next-intl';
import { api, TemplateInfo } from '@/lib/api';

export function TemplateCard({ id, name, description, tags, locale }: TemplateInfo & { locale: string }) {
  const router = useRouter();
  const t = useTranslations('home');
  const [messageApi, contextHolder] = message.useMessage();

  const handleStart = async () => {
    try {
      const res = await api.post<{ id: string }>(`/templates/${id}/projects`, { name });
      messageApi.success(`Project "${name}" created`);
      router.push(`/${locale}/project/${res.id}/chat`);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : 'Failed to create project');
    }
  };

  return (
    <>
      {contextHolder}
      <div className="template-card" onClick={handleStart}>
        <div className="template-number">TEMPLATE {id.toUpperCase()}</div>
        <div className="template-name">{name}</div>
        <div className="template-desc">{description}</div>
        <div className="template-tags">
          {tags.map((tag) => (
            <span key={tag} className="template-tag">{tag}</span>
          ))}
        </div>
      </div>
    </>
  );
}
