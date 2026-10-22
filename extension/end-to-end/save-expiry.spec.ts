import { expect, test, type Worker } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness } from "./harness";

async function installWorkerClock(worker: Worker): Promise<void> {
  await worker.evaluate(() => {
    let now = Date.now();
    let nextID = -1;
    const timers = new Map<number, { deadline: number; run: () => void }>();
    const originalClear = globalThis.clearTimeout;
    Date.now = () => now;
    globalThis.setTimeout = ((handler: () => void, delay = 0) => {
      const id = nextID--;
      timers.set(id, { deadline: now + delay, run: handler });
      return id;
    }) as typeof setTimeout;
    globalThis.clearTimeout = ((id: number) => {
      if (id < 0) timers.delete(id);
      else originalClear(id);
    }) as typeof clearTimeout;
    Object.assign(globalThis, {
      advanceExpiryClock(milliseconds: number) {
        now += milliseconds;
        for (const [id, timer] of timers) {
          if (timer.deadline > now) continue;
          timers.delete(id);
          timer.run();
        }
      },
    });
  });
}

async function advance(worker: Worker, milliseconds: number): Promise<void> {
  await worker.evaluate((amount) => {
    const advanceClock: unknown = Reflect.get(globalThis, "advanceExpiryClock");
    if (typeof advanceClock !== "function") throw new Error("Worker clock was not installed");
    advanceClock(amount);
  }, milliseconds);
}

for (const replace of [false, true]) {
  test(`actively expires the installed offer without popup or navigation${replace ? " after replacement" : ""}`, async () => {
    const harness = await launchHarness();
    try {
      await harness.pairWithoutPopup();
      const worker = harness.context.serviceWorkers()[0] ?? await harness.context.waitForEvent("serviceworker");
      await installWorkerClock(worker);
      const page = await harness.context.newPage();
      await page.goto(`${harness.site.origin}/login`);
      const tabId = await harness.getTabId(page.url());
      await page.evaluate(() => document.querySelector("form")!.addEventListener("submit", event => event.preventDefault()));
      await page.locator("#username").fill("alex@example.com");
      await page.locator("#password").fill("expiry-first-password");
      await page.locator("h1").click();
      await page.evaluate(() => document.querySelector("form")!.requestSubmit());
      const badge = () => harness.evaluateInExtension(id => chrome.action.getBadgeText({ tabId: id }), tabId);
      await expect.poll(badge).toBe("+");
      if (replace) {
        await advance(worker, 119999);
        await page.locator("#password").fill("expiry-replacement-password");
        await page.evaluate(() => document.querySelector("form")!.requestSubmit());
        await expect.poll(() => Object.values(harness.readState().pendingSaves ?? {}).some(context => context.password === "expiry-replacement-password")).toBe(true);
        await expect.poll(badge).toBe("+");
        await advance(worker, 1);
        expect(await badge()).toBe("+");
        await advance(worker, 119999);
      } else {
        await advance(worker, 120000);
      }
      await expect.poll(badge).toBe("");
      const popup = await harness.context.newPage();
      await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
      await expect(popup.getByTestId("save-confirmation")).toHaveCount(0);
      expect(harness.readState().allLogin[0].password).toBe("old password");
    } finally {
      await harness.close();
    }
  });
}

test("worker restart removes the orphaned pending badge before accepting a new capture", async () => {
  const harness = await launchHarness();
  try {
    await harness.pairWithoutPopup();
    const page = await harness.context.newPage();
    await page.goto(`${harness.site.origin}/login`);
    const tabId = await harness.getTabId(page.url());
    await page.evaluate(() => document.querySelector("form")!.addEventListener("submit", event => event.preventDefault()));
    await page.locator("#username").fill("alex@example.com");
    await page.locator("#password").fill("before-restart-password");
    await page.locator("h1").click();
    await page.evaluate(() => document.querySelector("form")!.requestSubmit());
    const badge = () => harness.evaluateInExtension(id => chrome.action.getBadgeText({ tabId: id }), tabId);
    await expect.poll(badge).toBe("+");
    const oldAlarms = await harness.evaluateInExtension(() => chrome.alarms.getAll(), undefined);
    expect(oldAlarms).toHaveLength(1);
    const session = await harness.context.newCDPSession(page);
    await session.send("ServiceWorker.enable");
    await session.send("ServiceWorker.stopAllWorkers");
    await harness.evaluateInExtension(id => chrome.runtime.sendMessage({ type: "popup.getPendingSaveOffer", tabId: id }), tabId);
    await expect.poll(badge).toBe("");
    expect(await harness.evaluateInExtension(() => chrome.alarms.getAll(), undefined)).toEqual([]);
    await page.locator("#password").fill("after-restart-password");
    await page.evaluate(() => document.querySelector("form")!.requestSubmit());
    await expect.poll(badge).toBe("+");
    const alarms = await harness.evaluateInExtension(() => chrome.alarms.getAll(), undefined);
    expect(alarms).toHaveLength(1);
    expect(alarms[0].name).not.toBe(oldAlarms[0].name);
    const popup = await harness.context.newPage();
    await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
    await expect(popup.getByTestId("save-confirmation")).toBeVisible();
    await popup.getByTestId("save-confirm").click();
    await expect.poll(() => harness.readState().allLogin[0].password).toBe("after-restart-password");
    await session.detach();
  } finally {
    await harness.close();
  }
});
