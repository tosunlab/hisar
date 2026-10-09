import type { NativePort } from "./native-client";

export class FakePort implements NativePort {
  readonly allSent: Record<string, unknown>[] = [];
  private readonly allMessageListener: ((message: unknown) => void)[] = [];
  private readonly allDisconnectListener: (() => void)[] = [];
  isDisconnected = false;

  constructor(private readonly answer?: (request: Record<string, unknown>) => unknown) {}

  readonly onMessage = { addListener: (listener: (message: unknown) => void) => this.allMessageListener.push(listener) };
  readonly onDisconnect = { addListener: (listener: () => void) => this.allDisconnectListener.push(listener) };

  postMessage(message: unknown): void {
    if (this.isDisconnected) throw new Error("Attempting to use a disconnected port object");
    const request = message as Record<string, unknown>;
    this.allSent.push(request);
    if (!this.answer) return;
    const response = this.answer(request);
    if (response !== undefined) queueMicrotask(() => this.deliver(response));
  }

  disconnect(): void {
    this.isDisconnected = true;
  }

  deliver(message: unknown): void {
    for (const listener of this.allMessageListener) listener(message);
  }

  simulateDisconnect(): void {
    this.isDisconnected = true;
    for (const listener of this.allDisconnectListener) listener();
  }
}

export function createMemoryArea(): {
  data: Record<string, unknown>;
  get(allKey: string[]): Promise<Record<string, unknown>>;
  set(allEntry: Record<string, unknown>): Promise<void>;
  remove(allKey: string | string[]): Promise<void>;
} {
  const data: Record<string, unknown> = {};
  return {
    data,
    async get(allKey) {
      return Object.fromEntries(allKey.filter((key) => key in data).map((key) => [key, data[key]]));
    },
    async set(allEntry) {
      Object.assign(data, structuredClone(allEntry));
    },
    async remove(allKey) {
      for (const key of Array.isArray(allKey) ? allKey : [allKey]) delete data[key];
    },
  };
}
