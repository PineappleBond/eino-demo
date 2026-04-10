const DB_NAME = 'eino-demo';
const DB_VERSION = 2;
const SETTINGS_STORE = 'settings';
const MESSAGES_STORE = 'messages';
const CONVERSATIONS_STORE = 'conversations';
const PROJECTS_STORE = 'projects';
const USERS_STORE = 'users';

interface SettingsRow {
  key: string;
  value: unknown;
}

interface IndexedRow<T = Record<string, unknown>> {
  id: string;
  data: T;
  updated_at: number;
}

let dbPromise: Promise<IDBDatabase> | null = null;

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;

  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = (event) => {
      const db = req.result;
      const oldVersion = event.oldVersion;

      // v1: settings store
      if (oldVersion < 1) {
        if (!db.objectStoreNames.contains(SETTINGS_STORE)) {
          db.createObjectStore(SETTINGS_STORE, { keyPath: 'key' });
        }
      }

      // v2: entity stores
      if (oldVersion < 2) {
        if (!db.objectStoreNames.contains(MESSAGES_STORE)) {
          const msgStore = db.createObjectStore(MESSAGES_STORE, { keyPath: 'id' });
          msgStore.createIndex('conversation_id', 'data.conversation_id', { unique: false });
        }
        if (!db.objectStoreNames.contains(CONVERSATIONS_STORE)) {
          db.createObjectStore(CONVERSATIONS_STORE, { keyPath: 'id' });
        }
        if (!db.objectStoreNames.contains(PROJECTS_STORE)) {
          db.createObjectStore(PROJECTS_STORE, { keyPath: 'id' });
        }
        if (!db.objectStoreNames.contains(USERS_STORE)) {
          db.createObjectStore(USERS_STORE, { keyPath: 'id' });
        }
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });

  return dbPromise;
}

// ─── Settings ───

async function getSetting<T>(key: string): Promise<T | null> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(SETTINGS_STORE, 'readonly');
    const store = tx.objectStore(SETTINGS_STORE);
    const req = store.get(key);
    req.onsuccess = () => resolve(req.result ? (req.result as SettingsRow).value as T : null);
    req.onerror = () => reject(req.error);
  });
}

async function setSetting(key: string, value: unknown): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(SETTINGS_STORE, 'readwrite');
    const store = tx.objectStore(SETTINGS_STORE);
    store.put({ key, value });
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

// ─── Generic entity helpers ───

async function getEntity<T>(storeName: string, id: string): Promise<T | null> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readonly');
    const store = tx.objectStore(storeName);
    const req = store.get(id);
    req.onsuccess = () => resolve(req.result ? (req.result as IndexedRow<T>).data : null);
    req.onerror = () => reject(req.error);
  });
}

async function getEntities<T>(storeName: string): Promise<T[]> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readonly');
    const store = tx.objectStore(storeName);
    const req = store.getAll();
    req.onsuccess = () => resolve((req.result as IndexedRow<T>[]).map((r) => r.data));
    req.onerror = () => reject(req.error);
  });
}

async function getEntitiesByIndex<T>(
  storeName: string,
  indexName: string,
  value: unknown
): Promise<T[]> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readonly');
    const store = tx.objectStore(storeName);
    const index = store.index(indexName);
    const req = index.getAll(value as IDBValidKey);
    req.onsuccess = () => resolve((req.result as IndexedRow<T>[]).map((r) => r.data));
    req.onerror = () => reject(req.error);
  });
}

async function putEntity<T>(storeName: string, id: string, data: T): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readwrite');
    const store = tx.objectStore(storeName);
    store.put({ id, data, updated_at: Date.now() });
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function putEntities<T>(storeName: string, items: { id: string; data: T }[]): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readwrite');
    const store = tx.objectStore(storeName);
    const now = Date.now();
    for (const item of items) {
      store.put({ id: item.id, data: item.data, updated_at: now });
    }
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function deleteEntity(storeName: string, id: string): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(storeName, 'readwrite');
    const store = tx.objectStore(storeName);
    store.delete(id);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

// ─── Messages ───

export async function getMessages(conversationId?: string): Promise<Record<string, unknown>[]> {
  if (conversationId) {
    return getEntitiesByIndex(MESSAGES_STORE, 'conversation_id', conversationId);
  }
  return getEntities(MESSAGES_STORE);
}

export async function saveMessages(
  items: { id: string; data: Record<string, unknown> }[]
): Promise<void> {
  return putEntities(MESSAGES_STORE, items);
}

export async function deleteMessage(id: string): Promise<void> {
  return deleteEntity(MESSAGES_STORE, id);
}

// ─── Conversations ───

export async function getConversations(): Promise<Record<string, unknown>[]> {
  return getEntities(CONVERSATIONS_STORE);
}

export async function saveConversations(
  items: { id: string; data: Record<string, unknown> }[]
): Promise<void> {
  return putEntities(CONVERSATIONS_STORE, items);
}

export async function deleteConversation(id: string): Promise<void> {
  return deleteEntity(CONVERSATIONS_STORE, id);
}

// ─── Projects ───

export async function getProjects(): Promise<Record<string, unknown>[]> {
  return getEntities(PROJECTS_STORE);
}

export async function saveProjects(
  items: { id: string; data: Record<string, unknown> }[]
): Promise<void> {
  return putEntities(PROJECTS_STORE, items);
}

export async function deleteProject(id: string): Promise<void> {
  return deleteEntity(PROJECTS_STORE, id);
}

// ─── Users ───

export async function getUser(id: string): Promise<Record<string, unknown> | null> {
  return getEntity(USERS_STORE, id);
}

export async function saveUsers(
  items: { id: string; data: Record<string, unknown> }[]
): Promise<void> {
  return putEntities(USERS_STORE, items);
}

// ─── Seq helpers ───

/**
 * Get the latest_seq cursor for seq continuity checking.
 */
export async function getLatestSeq(): Promise<number> {
  const val = await getSetting<number>('latest_seq');
  return val ?? 0;
}

/**
 * Set the latest_seq cursor after processing persistable updates.
 */
export async function setLatestSeq(seq: number): Promise<void> {
  await setSetting('latest_seq', seq);
}
