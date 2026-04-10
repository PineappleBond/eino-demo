'use client';

import { useEffect, useState } from 'react';
import { Row, Col, Spin, message } from 'antd';
import { TemplateCard } from './TemplateCard';
import { api, TemplateInfo } from '@/lib/api';

export function TemplateList({ locale }: { locale: string }) {
  const [templates, setTemplates] = useState<TemplateInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.get<TemplateInfo[]>('/templates')
      .then(setTemplates)
      .catch((err) => message.error(err.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <Spin size="large" style={{ display: 'block', textAlign: 'center', padding: '48px 0' }} />;
  }

  return (
    <Row gutter={[24, 24]}>
      {templates.map((tpl) => (
        <Col xs={24} sm={12} lg={8} key={tpl.id}>
          <TemplateCard {...tpl} locale={locale} />
        </Col>
      ))}
    </Row>
  );
}
