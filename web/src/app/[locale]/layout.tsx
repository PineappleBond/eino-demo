import type { ReactNode } from 'react';
import { NextIntlClientProvider } from 'next-intl';
import { getMessages } from 'next-intl/server';
import { AntdRegistry } from '@ant-design/nextjs-registry';
import { ThemeProvider } from '@/providers/ThemeProvider';
import { AuthProvider } from '@/providers/AuthProvider';
import { WSProvider } from '@/providers/WSProvider';
import { UpdateProvider } from '@/providers/UpdateProvider';
import { AuthGuard } from '@/components/auth/AuthGuard';

export default async function RootLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const messages = await getMessages();

  return (
    <html lang={locale}>
      <body style={{ margin: 0 }}>
        <NextIntlClientProvider messages={messages}>
          <AntdRegistry>
            <ThemeProvider>
              <AuthProvider>
                <WSProvider>
                  <UpdateProvider>
                    <AuthGuard>{children}</AuthGuard>
                  </UpdateProvider>
                </WSProvider>
              </AuthProvider>
            </ThemeProvider>
          </AntdRegistry>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
