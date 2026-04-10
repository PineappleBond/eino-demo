'use client';

import { useEffect, useState, useRef } from 'react';
import { Row, Col, Spin, message } from 'antd';
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
    return <Spin size="large" style={{ display: 'block', textAlign: 'center', padding: '48px 0' }} />;
  }

  return (
    <>
      {contextHolder}
      <Row gutter={[24, 24]}>
        {templates.map((tpl) => (
          <Col xs={24} sm={12} lg={8} key={tpl.id}>
            <TemplateCard {...tpl} locale={locale} />
          </Col>
        ))}
      </Row>
    </>
  );
}
