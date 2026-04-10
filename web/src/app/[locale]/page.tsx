import { getTranslations } from 'next-intl/server';
import { TopBar } from '@/components/layout/Header';
import { TemplateList } from '@/components/template/TemplateList';

export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const t = await getTranslations('home');

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <TopBar currentLocale={locale} />
      <div style={{ flex: 1, overflowY: 'auto', background: 'var(--bg-primary)' }}>
        <div style={{ maxWidth: 1200, margin: '0 auto', padding: '32px 24px' }}>
          <h2 style={{ margin: '0 0 8px', color: 'var(--text-primary)', fontSize: 24, fontWeight: 600 }}>
            {t('title')}
          </h2>
          <p style={{ color: 'var(--text-secondary)', fontSize: 15, marginBottom: 32 }}>
            {t('subtitle')}
          </p>
          <TemplateList locale={locale} />
        </div>
      </div>
    </div>
  );
}
