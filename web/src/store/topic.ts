import type { components } from '@/types/api';

/**
 * Union of all Update payload types.
 */
export type UpdatePayload =
  | components['schemas']['MessageNewPayload']
  | components['schemas']['MessageDeltaPayload']
  | components['schemas']['MessageDonePayload']
  | components['schemas']['MessageToolCallPayload']
  | components['schemas']['MessageThinkingPayload']
  | components['schemas']['MessageErrorPayload']
  | components['schemas']['MessageStopPayload']
  | components['schemas']['ConversationCreatedPayload']
  | components['schemas']['ConversationDeletedPayload']
  | components['schemas']['ConversationCompressedPayload']
  | components['schemas']['ConversationCompactingPayload']
  | components['schemas']['ConversationCompactedPayload']
  | components['schemas']['ConversationArchivedPayload']
  | components['schemas']['ProjectCreatedPayload']
  | components['schemas']['ProjectDeletedPayload']
  | components['schemas']['SettingsChangedPayload']
  | components['schemas']['EmptyPayload'];

/**
 * Derive topic from Update payload.
 * Backend has no topic concept — frontend extracts from payload fields.
 */
export function deriveTopic(payload: UpdatePayload): string {
  if ('conversation_id' in payload && typeof payload.conversation_id === 'string') {
    return `conv:${payload.conversation_id}`;
  }
  // conversation.compacted uses old_conv_id / new_conv_id
  if ('new_conv_id' in payload && typeof payload.new_conv_id === 'string') {
    return `conv:${payload.new_conv_id}`;
  }
  if ('old_conv_id' in payload && typeof payload.old_conv_id === 'string') {
    return `conv:${payload.old_conv_id}`;
  }
  if ('project_id' in payload && typeof payload.project_id === 'string') {
    return `project:${payload.project_id}`;
  }
  return 'system';
}

/**
 * Get topic from current pathname.
 * Called by useSubscribe in page components.
 */
export function topicFromPath(pathname: string): string {
  // /[locale]/project/[id]/chat/[convId] → conv:{convId}
  const convMatch = pathname.match(/\/[a-z]{2}\/project\/([^/]+)\/chat\/([^/]+)/);
  if (convMatch) {
    return `conv:${convMatch[2]}`;
  }

  // /[locale]/project/[id]/chat → system (project-level)
  const chatMatch = pathname.match(/\/[a-z]{2}\/project\/([^/]+)\/chat/);
  if (chatMatch) {
    return `project:${chatMatch[1]}`;
  }

  // /[locale]/settings → system
  if (pathname.includes('/settings')) {
    return 'system';
  }

  // /[locale]/ → system
  return 'system';
}
