import { APIError, send } from "./api";

const pageHeader = "Hisar-Collection-Pages";
const changedMessage = "the collection changed during traversal; reload it before continuing";
const pageBytes = 8 << 20;

export interface CollectionRecord {
  key: string;
  field: string;
  parent?: string;
  value: unknown;
}

interface CollectionPage {
  version: string;
  records: CollectionRecord[];
  nextCursor?: string;
  snapshot?: string;
  planVersion?: string;
}

async function readJSON(response: Response, limit: number): Promise<{ value: unknown; bytes: number }> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error("The server returned no collection response.");
  const decoder = new TextDecoder();
  const parts: string[] = [];
  let bytes = 0;
  for (;;) {
    const chunk = await reader.read();
    if (chunk.done) break;
    bytes += chunk.value.byteLength;
    if (bytes > limit) {
      await reader.cancel();
      throw new Error("The server response exceeds the collection byte budget.");
    }
    if (chunk.value.byteLength > 0) parts.push(decoder.decode(chunk.value, { stream: true }));
  }
  parts.push(decoder.decode());
  return { value: JSON.parse(parts.join("")) as unknown, bytes };
}

// A partial attempt remains local. A changed snapshot restarts the complete traversal at most twice.
export async function readCollection<T>(path: string, initial: () => T, consume: (result: T, record: CollectionRecord) => void): Promise<T> {
  for (let attempt = 0; attempt < 3; attempt++) {
    const result = initial();
    let cursor = "";
    let lastKey = "";
    let snapshot: string | undefined;
    let planVersion: string | undefined;
    try {
      for (;;) {
        const response = await send("GET", cursor ? `${path}?${new URLSearchParams({ cursor })}` : path, { collectionPages: true });
        const version = response.headers.get(pageHeader);
        if (version && version !== "1") throw new Error("The server returned an unsupported collection version.");
        const parsed = await readJSON(response, version ? pageBytes : 64 << 20);
        const body = parsed.value;
        const claimsPages = body !== null && typeof body === "object" && ("version" in body || "records" in body || "nextCursor" in body);
        if (!version && !claimsPages) {
          if (cursor) throw new Error("The collection protocol changed during traversal.");
          return body as T;
        }
        if (parsed.bytes > pageBytes) throw new Error("The collection page exceeds the byte budget.");
        const page = body as CollectionPage;
        if (page.version !== "1" || !Array.isArray(page.records) || page.records.length > 100 || (page.nextCursor !== undefined && (typeof page.nextCursor !== "string" || page.nextCursor.length > 4096))) {
          throw new Error("The server returned an invalid collection page.");
        }
        if (!lastKey) {
          snapshot = page.snapshot;
          planVersion = page.planVersion;
        } else if (snapshot !== page.snapshot || planVersion !== page.planVersion) {
          throw new Error("The collection snapshot or deletion plan changed between pages.");
        }
        for (const record of page.records) {
          if (typeof record.key !== "string" || record.key <= lastKey || typeof record.field !== "string" || !("value" in record)) throw new Error("The server returned an unordered collection record.");
          consume(result, record);
          lastKey = record.key;
        }
        if (!page.nextCursor) return result;
        if (page.records.length === 0 || page.nextCursor === cursor) throw new Error("The collection cursor made no progress.");
        cursor = page.nextCursor;
      }
    } catch (error) {
      if (!(error instanceof APIError) || error.status !== 409 || error.message !== changedMessage) throw error;
    }
  }
  throw new APIError(409, changedMessage);
}
