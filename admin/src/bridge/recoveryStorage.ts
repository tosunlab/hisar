export interface RecoveryMetadata {
  operationId: string;
  kind: string;
  origin: string;
  accountId: string;
  email: string;
  name: string;
  phase: string;
  revision: number;
  kitSaved?: boolean;
  problems?: string[];
}

export interface RecoveryRecord {
  metadata: RecoveryMetadata;
  envelope: string;
}

const databaseName = "hisar.admin.recovery";
const storeName = "pending";

function openDatabase(): Promise<IDBDatabase> {
  const { promise, resolve, reject } = Promise.withResolvers<IDBDatabase>();
  const request = indexedDB.open(databaseName, 1);
  request.onupgradeneeded = () => request.result.createObjectStore(storeName, { keyPath: "metadata.operationId" });
  request.onsuccess = () => resolve(request.result);
  request.onerror = () => reject(new Error("The encrypted recovery journal could not be opened. Browser storage must be available before account creation."));
  request.onblocked = () => reject(new Error("Close other Hisar setup tabs so the encrypted recovery journal can be opened."));
  return promise;
}

async function transact<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const database = await openDatabase();
  const { promise, resolve, reject } = Promise.withResolvers<T>();
  const transaction = database.transaction(storeName, mode, mode === "readwrite" ? { durability: "strict" } : undefined);
  const request = action(transaction.objectStore(storeName));
  transaction.oncomplete = () => {
    database.close();
    resolve(request.result);
  };
  transaction.onabort = transaction.onerror = () => {
    database.close();
    reject(new Error("The encrypted recovery journal could not be saved. Keep this page open and retry."));
  };
  return promise;
}

export async function stageRecovery(metadata: RecoveryMetadata, envelope: string): Promise<void> {
  if (!Number.isSafeInteger(metadata.revision) || metadata.revision < 1
    || envelope.length > 1024 * 1024 || new TextEncoder().encode(JSON.stringify(metadata)).length > 64 * 1024) {
    throw new Error("The recovery record is invalid or exceeds this browser's recovery storage limit.");
  }
  const database = await openDatabase();
  const { promise, resolve, reject } = Promise.withResolvers<void>();
  const transaction = database.transaction(storeName, "readwrite", { durability: "strict" });
  const store = transaction.objectStore(storeName);
  let failure = "The encrypted recovery journal could not be saved. Keep this page open and retry.";
  const existing = store.get(metadata.operationId) as IDBRequest<RecoveryRecord | undefined>;
  existing.onsuccess = () => {
    const previous = existing.result?.metadata;
    if (previous) {
      if (previous.revision !== metadata.revision - 1 || previous.origin !== metadata.origin || previous.accountId !== metadata.accountId) {
        failure = "Recovery changed in another page. Review the current operation and retry.";
        transaction.abort();
        return;
      }
      store.put({ metadata, envelope } satisfies RecoveryRecord);
    } else if (metadata.revision !== 1) {
      failure = "This recovery operation was removed or replaced. Reload its current state before continuing.";
      transaction.abort();
    } else {
      const count = store.count();
      count.onsuccess = () => {
        if (count.result >= 100) {
          failure = "This browser holds 100 recovery operations. Finish or discard an uncommitted operation before creating another.";
          transaction.abort();
        } else {
          store.add({ metadata, envelope } satisfies RecoveryRecord);
        }
      };
    }
  };
  transaction.oncomplete = () => { database.close(); resolve(); };
  transaction.onabort = transaction.onerror = () => { database.close(); reject(new Error(failure)); };
  return promise;
}

export async function loadRecovery(operationId: string): Promise<RecoveryRecord | null> {
  return (await transact<RecoveryRecord | undefined>("readonly", (store) => store.get(operationId))) ?? null;
}

export async function listRecovery(): Promise<RecoveryRecord[]> {
  const records = await transact<RecoveryRecord[]>("readonly", (store) => store.getAll(undefined, 101));
  if (records.length > 100) throw new Error("This browser's recovery journal exceeds its operation limit.");
  return records;
}

export async function removeRecovery(operationId: string, expectedRevision: number): Promise<void> {
  const database = await openDatabase();
  const { promise, resolve, reject } = Promise.withResolvers<void>();
  const transaction = database.transaction(storeName, "readwrite", { durability: "strict" });
  const store = transaction.objectStore(storeName);
  const existing = store.get(operationId) as IDBRequest<RecoveryRecord | undefined>;
  existing.onsuccess = () => {
    if (!existing.result || existing.result.metadata.revision !== expectedRevision) {
      transaction.abort();
      return;
    }
    store.delete(operationId);
  };
  transaction.oncomplete = () => { database.close(); resolve(); };
  transaction.onabort = transaction.onerror = () => {
    database.close();
    reject(new Error("Recovery changed in another page. Review its current state before removing it."));
  };
  return promise;
}
