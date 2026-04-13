'use client';

import { useState, useEffect } from 'react';
import { Modal, Button, Input, Typography, List, Divider } from 'antd';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useTranslations } from 'next-intl';

const { Text, Title } = Typography;

interface HitlChoice {
  title: string;
  desc?: string;
}

interface HitlModalProps {
  open: boolean;
  question: string;
  choices: HitlChoice[];
  answerType: 'single' | 'multi' | 'text';
  onAnswer: (answer: string) => void;
  onCancel?: () => void;
}

export function HitlModal({ open, question, choices, answerType, onAnswer, onCancel }: HitlModalProps) {
  const t = useTranslations('chat');
  const tHitl = useTranslations('hitl');
  const hasChoices = choices.length > 0;
  const [selectedTitle, setSelectedTitle] = useState<string>('');
  const [selectedTitles, setSelectedTitles] = useState<string[]>([]);
  const [textValue, setTextValue] = useState('');

  useEffect(() => {
    if (open) {
      setSelectedTitle('');
      setSelectedTitles([]);
      setTextValue('');
    }
  }, [open, choices]);

  const handleSubmit = () => {
    // If text input has content, prefer it (free text override)
    if (textValue.trim()) {
      onAnswer(textValue.trim());
    } else if (answerType === 'multi') {
      onAnswer(selectedTitles.join(', '));
    } else if (answerType === 'single' && hasChoices) {
      onAnswer(selectedTitle);
    } else {
      onAnswer(textValue);
    }
  };

  const hasValidAnswer = hasChoices
    ? textValue.trim().length > 0 || (answerType === 'multi' ? selectedTitles.length > 0 : selectedTitle !== '')
    : textValue.trim().length > 0;

  const handleSelect = (title: string) => {
    if (answerType === 'multi') {
      setSelectedTitles((prev) =>
        prev.includes(title) ? prev.filter((t) => t !== title) : [...prev, title]
      );
    } else {
      setSelectedTitle(title);
    }
    // Clear text when a choice is selected
    setTextValue('');
  };

  return (
    <Modal
      open={open}
      title={t('hitlQuestion') || 'Question'}
      closable={false}
      maskClosable={false}
      width={560}
      footer={[
        <Button key="cancel" onClick={onCancel} disabled={!hasValidAnswer}>
          {t('cancel')}
        </Button>,
        <Button key="submit" type="primary" onClick={handleSubmit} disabled={!hasValidAnswer}>
          {t('submit') || 'Submit'}
        </Button>,
      ]}
    >
      <Title level={5} style={{ marginTop: 0, marginBottom: 16 }}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{question}</ReactMarkdown>
      </Title>

      {hasChoices && (
        <List
          dataSource={choices}
          renderItem={(item) => (
            <List.Item
              onClick={() => handleSelect(item.title)}
              style={{
                cursor: 'pointer',
                padding: '12px 16px',
                borderRadius: 8,
                border:
                  selectedTitle === item.title || selectedTitles.includes(item.title)
                    ? '2px solid #1677ff'
                    : '1px solid #f0f0f0',
                background:
                  selectedTitle === item.title || selectedTitles.includes(item.title)
                    ? '#e6f4ff'
                    : 'transparent',
                transition: 'all 0.2s',
              }}
            >
              <List.Item.Meta
                title={<Text strong style={{ fontSize: 15 }}>{item.title}</Text>}
                description={item.desc ? <Text type="secondary">{item.desc}</Text> : null}
              />
            </List.Item>
          )}
          style={{ marginBottom: 12 }}
        />
      )}

      {hasChoices && <Divider style={{ margin: '12px 0', color: '#999', fontSize: 12 }}>{tHitl('orCustomAnswer')}</Divider>}

      <Input.TextArea
        value={textValue}
        onChange={(e) => {
          setTextValue(e.target.value);
          // Clear selections when user types
          if (e.target.value) {
            setSelectedTitle('');
            setSelectedTitles([]);
          }
        }}
        placeholder={t('typeAnswer') || 'Type your answer...'}
        autoSize={{ minRows: 2, maxRows: 4 }}
        onPressEnter={(e) => {
          if (e.ctrlKey || e.metaKey) {
            handleSubmit();
          }
        }}
      />
    </Modal>
  );
}
