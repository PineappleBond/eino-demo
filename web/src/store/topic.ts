/**
 * Derive topic from Update payload.
 * Backend has no topic concept — frontend extracts from payload fields.
 */
export function deriveTopic(payload: Record<string, unknown>): string {
  if (payload.conversation_id) {
    return `conv:${payload.conversation_id}`;
  }
  if (payload.project_id) {
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
