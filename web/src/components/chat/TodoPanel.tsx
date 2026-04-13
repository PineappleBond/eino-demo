'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Checkbox, Input, Button, Space, Typography, Spin } from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import { api } from '@/lib/api';
import { useSubscribe } from '@/providers/UpdateProvider';
import type { Update } from '@/lib/updateDispatcher';
import type { components } from '@/types/api';
import { useTranslations } from 'next-intl';

const { Text } = Typography;

type Todo = components['schemas']['Todo'];

interface TodoPanelProps {
  conversationId: string;
  collapsible?: boolean;
}

export function TodoPanel({ conversationId, collapsible = false }: TodoPanelProps) {
  const t = useTranslations('chat');
  const [todos, setTodos] = useState<Todo[]>([]);
  const [loading, setLoading] = useState(true);
  const [newContent, setNewContent] = useState('');
  const [collapsed, setCollapsed] = useState(collapsible);
  const topic = `conv:${conversationId}`;

  const fetchTodos = useCallback(async () => {
    try {
      const data = await api.get<Todo[]>(`/conversations/${conversationId}/todos`);
      setTodos(data);
    } catch {
      // Silently fail
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    fetchTodos();
  }, [fetchTodos]);

  // Real-time: refetch on todo events
  const fetchTodosRef = useRef(fetchTodos);
  fetchTodosRef.current = fetchTodos;

  const handleTodoUpdate = useCallback((update: Update) => {
    if (update.type === 'todo.sync') {
      const payload = update.payload as Record<string, unknown>;
      // Only refetch if the update belongs to this conversation
      if (payload.conversation_id === conversationId) {
        fetchTodosRef.current();
      }
    }
  }, [conversationId]);

  useSubscribe(topic, handleTodoUpdate);

  const handleCreate = async () => {
    if (!newContent.trim()) return;
    try {
      const todo = await api.post<Todo>(`/conversations/${conversationId}/todos`, {
        content: newContent.trim(),
      });
      setTodos((prev) => [...prev, todo]);
      setNewContent('');
    } catch {
      // Silently fail
    }
  };

  const handleToggle = async (todo: Todo) => {
    try {
      const updated = await api.patch<Todo>(`/todos/${todo.id}`, {
        completed: !todo.completed,
      });
      setTodos((prev) => prev.map((t) => (t.id === updated.id ? updated : t)));
    } catch {
      // Silently fail
    }
  };

  const handleDelete = async (todoId: string) => {
    try {
      await api.delete(`/todos/${todoId}`);
      setTodos((prev) => prev.filter((t) => t.id !== todoId));
    } catch {
      // Silently fail
    }
  };

  const doneCount = todos.filter((t) => t.completed).length;

  if (loading) {
    return <Spin size="small" style={{ display: 'block', textAlign: 'center', padding: 16 }} />;
  }

  if (collapsed && !collapsible) {
    return null;
  }

  return (
    <div style={{ padding: '0 8px' }}>
      {/* Header with progress */}
      {collapsible && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            cursor: 'pointer',
            padding: '4px 0',
            marginBottom: 8,
          }}
          onClick={() => setCollapsed(!collapsed)}
        >
          <Text strong style={{ fontSize: 13 }}>
            Todos
            {todos.length > 0 && (
              <Text type="secondary" style={{ marginLeft: 6, fontSize: 12 }}>
                {doneCount}/{todos.length}
              </Text>
            )}
          </Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {collapsed ? '▶' : '▼'}
          </Text>
        </div>
      )}

      {!collapsed && (
        <>
          <div style={{ marginBottom: 8 }}>
            <Space.Compact style={{ width: '100%' }}>
              <Input
                value={newContent}
                onChange={(e) => setNewContent(e.target.value)}
                placeholder={t('addTodo') || 'Add a todo...'}
                onPressEnter={handleCreate}
                size="small"
              />
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={handleCreate}
                disabled={!newContent.trim()}
                size="small"
              />
            </Space.Compact>
          </div>

          {todos.length === 0 && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              {t('noTodos') || 'No todos yet'}
            </Text>
          )}

          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {todos.map((todo) => (
              <div
                key={todo.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '2px 0',
                }}
              >
                <Checkbox
                  checked={todo.completed}
                  onChange={() => handleToggle(todo)}
                  style={{ flexShrink: 0 }}
                />
                <Text
                  delete={todo.completed}
                  style={{ flex: 1, fontSize: 13 }}
                >
                  {todo.content}
                </Text>
                <Button
                  type="text"
                  danger
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => handleDelete(todo.id)}
                  style={{ padding: '0 2px', flexShrink: 0 }}
                />
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
