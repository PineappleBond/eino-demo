import { getTranslations } from 'next-intl/server';
import { Header } from '@/components/layout/Header';
import { TemplateList } from '@/components/template/TemplateList';

export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const t = await getTranslations('home');

  return (
    <>
      <Header currentLocale={locale} />
      <div style={{ maxWidth: 1200, margin: '0 auto', padding: '24px 16px' }}>
        <h1>{t('title')}</h1>
        <p style={{ color: '#666', marginBottom: 24 }}>{t('subtitle')}</p>
        <TemplateList locale={locale} />
      </div>
    </>
  );
}
