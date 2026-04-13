'use client';

import { Drawer } from 'antd';
import { useTranslations } from 'next-intl';
import { SettingsForm } from '@/components/settings/SettingsForm';

interface SettingsDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function SettingsDrawer({ open, onClose }: SettingsDrawerProps) {
  const t = useTranslations('settings');

  return (
    <Drawer
      title={t('title')}
      open={open}
      onClose={onClose}
      width={420}
      closable
      destroyOnClose
      styles={{
        body: { padding: '20px' },
      }}
    >
      <SettingsForm onSaved={onClose} />
    </Drawer>
  );
}
