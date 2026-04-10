'use client';

import { ReactNode } from 'react';
import { useAuth } from '@/providers/AuthProvider';
import { AuthModal } from '@/components/auth/AuthModal';

export function AuthGuard({ children }: { children: ReactNode }) {
  const { isConnected } = useAuth();

  return (
    <>
      <AuthModal open={!isConnected} />
      {children}
    </>
  );
}
