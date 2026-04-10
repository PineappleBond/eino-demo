import { getLatestSeq, setLatestSeq } from '../store/indexedDB';
import { deriveTopic } from '../store/topic';

type UpdateType =
  | 'message.new' | 'message.delta' | 'message.done'
  | 'message.tool_call' | 'message.thinking' | 'message.error' | 'message.stop'
  | 'conversation.created' | 'conversation.deleted'
  | 'conversation.compacting' | 'conversation.compacted' | 'conversation.archived'
  | 'project.created' | 'project.deleted'
  | 'settings.changed'
  | 'empty';

interface Update {
  seq: number;
  type: UpdateType;
  payload: Record<string, unknown>;
}

type Subscriber = (update: Update) => void;

class UpdateDispatcher {
  private subscribers = new Map<string, Set<Subscriber>>();
  private processing = false;
  private queue: Update[][] = [];

  /**
   * Subscribe to a topic. Returns unsubscribe function.
   */
  subscribe(topic: string, fn: Subscriber): () => void {
    if (!this.subscribers.has(topic)) {
      this.subscribers.set(topic, new Set());
    }
    this.subscribers.get(topic)!.add(fn);
    return () => {
      this.subscribers.get(topic)?.delete(fn);
    };
  }

  /**
   * Main entry point. ALL incoming updates (WS push or HTTP response) go through this.
   * Thread-safe: concurrent calls are serialized via queue.
   */
  async applyUpdates(updates: Update[]): Promise<void> {
    if (this.processing) {
      this.queue.push(updates);
      return;
    }
    this.processing = true;

    try {
      await this._processBatch(updates);

      while (this.queue.length > 0) {
        const next = this.queue.shift()!;
        await this._processBatch(next);
      }
    } finally {
      this.processing = false;
    }
  }

  private async _processBatch(updates: Update[]): Promise<void> {
    const persistable = updates.filter((u) => u.seq > 0);
    const ephemeral = updates.filter((u) => u.seq === 0);

    if (persistable.length > 0) {
      const localMaxSeq = await getLatestSeq();
      const minPersistable = Math.min(...persistable.map((u) => u.seq));

      if (minPersistable !== localMaxSeq + 1) {
        // Seq gap — abort, trigger HTTP pull
        console.warn(`[UpdateDispatcher] seq gap: local=${localMaxSeq}, incoming min=${minPersistable}`);
        return;
      }

      const maxSeq = Math.max(...persistable.map((u) => u.seq));
      await setLatestSeq(maxSeq);
    }

    // Notify subscribers by topic
    for (const update of updates) {
      const topic = deriveTopic(update.payload);
      const subs = this.subscribers.get(topic);
      if (subs) {
        for (const fn of subs) {
          fn(update);
        }
      }
      // Also broadcast settings changes to system topic
      if (update.type === 'settings.changed') {
        const systemSubs = this.subscribers.get('system');
        if (systemSubs) {
          for (const fn of systemSubs) {
            fn(update);
          }
        }
      }
    }
  }
}

export const dispatcher = new UpdateDispatcher();
export type { Update, UpdateType };
