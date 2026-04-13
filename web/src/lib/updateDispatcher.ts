import { getLatestSeq, setLatestSeq } from '../store/indexedDB';
import { deriveTopic } from '../store/topic';
import type { components } from '@/types/api';

type Update = components['schemas']['Update'];
type UpdateType = Update['type'];

type Subscriber = (update: Update) => void;
type GapCallback = (localMaxSeq: number, incomingMinSeq: number) => void;
type MissedCallback = (update: Update, topic: string) => void;

class UpdateDispatcher {
  private subscribers = new Map<string, Set<Subscriber>>();
  private processing = false;
  private queue: Update[][] = [];
  private maxServerSeq: number | null = null;
  private onGapDetected: GapCallback | null = null;
  private onMissed: MissedCallback | null = null;

  /**
   * Register a callback for updates whose topic has no active subscriber.
   * Used to show a notification when the user is on a different conversation.
   */
  setMissedCallback(fn: MissedCallback): void {
    this.onMissed = fn;
  }

  /**
   * Register a callback that fires when a seq gap is detected.
   * The callback should trigger HTTP pull for the missing range.
   */
  setGapCallback(fn: GapCallback): void {
    this.onGapDetected = fn;
  }

  /**
   * Set the max_seq from server's connected frame.
   * Used for large-gap detection: if local latest_seq is far behind, trigger HTTP pull.
   */
  setMaxServerSeq(seq: number): void {
    this.maxServerSeq = seq;
  }

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
        // Seq gap detected — trigger HTTP pull for missing range, but still
        // process ephemeral (streaming) updates so the UI stays responsive.
        console.warn(
          `[UpdateDispatcher] seq gap: local=${localMaxSeq}, incoming min=${minPersistable}`
        );
        this.onGapDetected?.(localMaxSeq, minPersistable - 1);

        // Skip persistable updates but still notify subscribers for ephemeral ones
        for (const update of ephemeral) {
          this._notifySubscribers(update);
        }
        return;
      }

      const maxSeq = Math.max(...persistable.map((u) => u.seq));
      await setLatestSeq(maxSeq);
    }

    // Notify all subscribers
    for (const update of updates) {
      this._notifySubscribers(update);
    }
  }

  private _notifySubscribers(update: Update): void {
    if (update.type === 'empty') {
      return;
    }
    const topic = deriveTopic(update.payload);
    const subs = this.subscribers.get(topic);
    if (subs) {
      for (const fn of subs) {
        fn(update);
      }
    } else {
      // No subscriber for this topic — treat as missed (user is on a different page)
      this.onMissed?.(update, topic);
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

export const dispatcher = new UpdateDispatcher();
export type { Update, UpdateType };
