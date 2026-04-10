import { getTranslations } from 'next-intl/server';
import { TemplateList } from '@/components/template/TemplateList';
import { TopBar } from '@/components/layout/Header';

export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const t = await getTranslations('home');

  return (
    <>
      <TopBar currentLocale={locale} />
      <div style={{ maxWidth: 1200, margin: '0 auto', padding: '24px 16px', width: '100%' }}>
        <h1 style={{ margin: '0 0 8px', fontSize: 24 }}>{t('title')}</h1>
        <p style={{ color: '#666', marginBottom: 24 }}>{t('subtitle')}</p>
        <TemplateList locale={locale} />
      </div>
    </>
  );
}
