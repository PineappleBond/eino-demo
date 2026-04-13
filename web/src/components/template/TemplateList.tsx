'use client';

import { useEffect, useState } from 'react';
import { message } from 'antd';
import { TemplateCard } from './TemplateCard';
import { api, TemplateInfo } from '@/lib/api';

export function TemplateList({ locale }: { locale: string }) {
  const [templates, setTemplates] = useState<TemplateInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [messageApi, contextHolder] = message.useMessage();

  useEffect(() => {
    api.get<TemplateInfo[]>('/templates')
      .then(setTemplates)
      .catch((err) => setErrorMsg(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (errorMsg) {
      messageApi.error(errorMsg);
    }
  }, [errorMsg, messageApi]);

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '48px 0' }}>
        <div className="typing-indicator" style={{ justifyContent: 'center' }}>
          <div className="typing-dot" /><div className="typing-dot" /><div className="typing-dot" />
        </div>
      </div>
    );
  }

  return (
    <>
      {contextHolder}
      <div className="template-grid">
        {templates.map((tpl) => (
          <TemplateCard key={tpl.id} {...tpl} locale={locale} />
        ))}
      </div>
    </>
  );
}
