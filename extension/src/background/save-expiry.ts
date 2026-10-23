import type { ScheduleSaveExpiry } from "./save";

const alarmPrefix = "hisar-save-expiry/";

interface ExpiryBrowser {
  alarms: {
    create(name: string, info: { when: number }): Promise<void>;
    clear(name: string): Promise<boolean>;
    getAll(): Promise<{ name: string }[]>;
    onAlarm: { addListener(listener: (alarm: { name: string }) => void): void };
  };
  tabs: { query(query: object): Promise<{ id?: number }[]> };
  action: {
    getBadgeText(details: { tabId: number }): Promise<string>;
    setBadgeText(details: { tabId: number; text: string }): Promise<void>;
  };
}

export class SaveExpiryScheduler {
  readonly ready: Promise<void>;
  private readonly allExpiry = new Map<string, () => void>();

  constructor(private readonly browser: ExpiryBrowser) {
    browser.alarms.onAlarm.addListener(alarm => this.allExpiry.get(alarm.name)?.());
    this.ready = this.reconcileRestart();
  }

  readonly schedule: ScheduleSaveExpiry = (tabId, generation, deadline, expire) => {
    const name = `${alarmPrefix}${tabId}/${generation}`;
    const timer = setTimeout(expire, Math.max(0, deadline - Date.now()));
    this.allExpiry.set(name, expire);
    let isCancelled = false;
    void this.browser.alarms.create(name, { when: deadline }).then(() => {
      if (isCancelled) void this.browser.alarms.clear(name);
    });
    return () => {
      isCancelled = true;
      clearTimeout(timer);
      this.allExpiry.delete(name);
      void this.browser.alarms.clear(name);
    };
  };

  private async reconcileRestart(): Promise<void> {
    const [allAlarm, allTab] = await Promise.all([this.browser.alarms.getAll(), this.browser.tabs.query({})]);
    await Promise.all([
      ...allAlarm.filter(alarm => alarm.name.startsWith(alarmPrefix)).map(alarm => this.browser.alarms.clear(alarm.name)),
      ...allTab.map(async tab => {
        if (tab.id === undefined) return;
        try {
          if (await this.browser.action.getBadgeText({ tabId: tab.id }) === "+") {
            await this.browser.action.setBadgeText({ tabId: tab.id, text: "" });
          }
        } catch {
          // A tab can close while the worker reconciles its badge.
        }
      }),
    ]);
  }
}
