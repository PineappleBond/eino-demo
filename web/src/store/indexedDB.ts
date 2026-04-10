const DB_NAME = 'eino-demo';
const DB_VERSION = 1;
const SETTINGS_STORE = 'settings';

interface SettingsRow {
  key: string;
  value: unknown;
}

let dbPromise: Promise<IDBDatabase> | null = null;

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;

  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(SETTINGS_STORE)) {
        db.createObjectStore(SETTINGS_STORE, { keyPath: 'key' });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });

  return dbPromise;
}

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
