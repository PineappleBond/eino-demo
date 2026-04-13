'use client';

import { Switch } from 'antd';
import { SunOutlined, MoonOutlined } from '@ant-design/icons';
import { useEffect, useState } from 'react';
import { getLatestTheme, setTheme } from '@/hooks/useTheme';

export function ThemeSwitch() {
  const [dark, setDark] = useState(false);

  useEffect(() => {
    setDark(getLatestTheme() === 'dark');
  }, []);

  const onChange = (checked: boolean) => {
    const theme = checked ? 'dark' : 'light';
    setTheme(theme);
    setDark(checked);
    window.location.reload();
  };

  return (
    <Switch
      checked={dark}
      onChange={onChange}
      checkedChildren={<MoonOutlined />}
      unCheckedChildren={<SunOutlined />}
    />
  );
}
