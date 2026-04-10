import { getTranslations } from 'next-intl/server';
import { TemplateList } from '@/components/template/TemplateList';

export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const t = await getTranslations('home');

  return (
    <div style={{ padding: '24px 24px 0', maxWidth: 1200, margin: '0 auto', width: '100%' }}>
      <h1>{t('title')}</h1>
      <p style={{ color: '#666', marginBottom: 24 }}>{t('subtitle')}</p>
      <TemplateList locale={(await params).locale} />
    </div>
  );
}
