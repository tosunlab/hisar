export interface KeyValueArea {
  get(allKey: string[]): Promise<Record<string, unknown>>;
  set(allEntry: Record<string, unknown>): Promise<void>;
  remove(allKey: string | string[]): Promise<void>;
}

export class ExtensionStorage {
  constructor(
    private readonly area: KeyValueArea,
    private readonly createID: () => string,
  ) {}

  async getInstanceID(): Promise<string> {
    const stored = await this.area.get(["instanceID"]);
    if (typeof stored.instanceID === "string" && stored.instanceID) return stored.instanceID;
    const instanceID = this.createID();
    await this.area.set({ instanceID });
    return instanceID;
  }

  async getToken(): Promise<string | undefined> {
    const stored = await this.area.get(["token"]);
    return typeof stored.token === "string" && stored.token ? stored.token : undefined;
  }

  async setToken(token: string): Promise<void> {
    await this.area.set({ token });
  }

  async removeToken(): Promise<void> {
    await this.area.remove("token");
  }

  async setAppFeatures(allFeature: string[]): Promise<void> {
    await this.area.set({ appFeatures: allFeature });
  }

  async getAllNeverOrigin(): Promise<string[]> {
    const stored = await this.area.get(["allNeverOrigin"]);
    return Array.isArray(stored.allNeverOrigin) ? stored.allNeverOrigin.filter((origin) => typeof origin === "string") : [];
  }

  async addNeverOrigin(origin: string): Promise<void> {
    const allNeverOrigin = await this.getAllNeverOrigin();
    if (allNeverOrigin.includes(origin)) return;
    await this.area.set({ allNeverOrigin: [...allNeverOrigin, origin] });
  }

  async isNeverOrigin(origin: string): Promise<boolean> {
    return (await this.getAllNeverOrigin()).includes(origin);
  }

  // The extension remembers when each item was last filled, by its vault and item IDs only, to put recently used logins
  // first; it keeps the latest maximumLastUsedCount.
  async getAllLastUsed(): Promise<Record<string, number>> {
    const stored = (await this.area.get(["allLastUsed"])).allLastUsed;
    if (!stored || typeof stored !== "object") return {};
    return Object.fromEntries(Object.entries(stored as Record<string, unknown>).filter((entry): entry is [string, number] => typeof entry[1] === "number"));
  }

  async recordUse(key: string, time: number): Promise<void> {
    const allLastUsed = { ...(await this.getAllLastUsed()), [key]: time };
    const allKept = Object.entries(allLastUsed)
      .sort((first, second) => second[1] - first[1])
      .slice(0, maximumLastUsedCount);
    await this.area.set({ allLastUsed: Object.fromEntries(allKept) });
  }
}

const maximumLastUsedCount = 200;
