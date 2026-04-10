'use client';

import { ReactNode, useEffect, useState } from 'react';
import { useAuth } from '@/providers/AuthProvider';
import { AuthModal } from '@/components/auth/AuthModal';

export function AuthGuard({ children }: { children: ReactNode }) {
  const { isConnected } = useAuth();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  return (
    <>
      {mounted && <AuthModal open={!isConnected} />}
      {children}
    </>
  );
}
