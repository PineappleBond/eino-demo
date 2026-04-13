'use client';

import { Result, Button } from 'antd';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

export default function NotFound() {
  const t = useTranslations('notFound');
  return (
    <div style={{ display: 'flex', justifyContent: 'center', padding: '80px 24px' }}>
      <Result
        status="404"
        title={t('title')}
        subTitle={t('message')}
        extra={
          <Button type="primary">
            <Link href="/">{t('backHome')}</Link>
          </Button>
        }
      />
    </div>
  );
}
