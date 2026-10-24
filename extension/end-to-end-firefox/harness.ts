import { createHash, randomBytes } from "node:crypto";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect } from "@playwright/test";
import { Builder, By } from "selenium-webdriver";
import { Context, Options, ServiceBuilder, type Driver } from "selenium-webdriver/firefox.js";
import { Command } from "selenium-webdriver/lib/command.js";
import { createInitialState, readState, writeState, type FakeHostState } from "../tools/fake-native-host.mjs";
import { installTestFirefox } from "../tools/install-test-firefox.mjs";
import { firefoxExtensionID, registerFakeFirefoxHost } from "../tools/register-fake-host.mjs";
import { startTestSite, type TestSite } from "../end-to-end/test-site";

const distDirectory = resolve(import.meta.dirname, "../dist-firefox");
const isHeaded = process.env.HISAR_HEADED === "1";
// Firefox gives each install a random moz-extension UUID; the pref fixes it so tests can open the extension's pages.
const extensionUUID = "4e1b7c2a-9d0f-4c3e-8a5b-6f2d1e0c9b7a";
export const extensionBaseUrl = `moz-extension://${extensionUUID}/`;

interface BrowsingContextNode {
  context: string;
  url: string;
  children?: BrowsingContextNode[] | null;
}

interface Bidi {
  send(command: { method: string; params: Record<string, unknown> }): Promise<{ result?: Record<string, unknown>; error?: string; message?: string }>;
}

export interface FirefoxHarness {
  driver: Driver;
  site: TestSite;
  readState: () => FakeHostState;
  updateState: (change: (state: FakeHostState) => void) => void;
  pairWithoutPopup: () => Promise<void>;
  // openTab opens url in a new tab and returns its window handle; the tab is then the current one.
  openTab: (url: string) => Promise<string>;
  getTabId: (url: string) => Promise<number>;
  // waitForContentScripts waits until the content script listens in every frame of the tab at url: it starts once a page
  // is idle, so a field focused before then gets no menu.
  waitForContentScripts: (url: string) => Promise<void>;
  evaluateInExtension: <Result>(script: string, ...allArgument: unknown[]) => Promise<Result>;
  // findFrame returns the browsing context of the frame whose URL contains part, in the current tab, even inside a
  // closed shadow root.
  findFrame: (part: string) => Promise<string | undefined>;
  waitForFrame: (part: string) => Promise<string>;
  readFrame: (frame: string, expression: string) => Promise<unknown>;
  // clickInFrame clicks the element with a real pointer, which the inline menu and passkey prompt require.
  clickInFrame: (frame: string, selector: string, index?: number) => Promise<void>;
  readClipboard: () => Promise<string>;
  writeClipboard: (text: string) => Promise<void>;
  close: () => Promise<void>;
}

export async function launchFirefoxHarness(options: { allLogin?: FakeHostState["allLogin"] } = {}): Promise<FirefoxHarness> {
  const temporaryDirectory = mkdtempSync(join(tmpdir(), "hisar-firefox-end-to-end-"));
  const site = await startTestSite();
  const manifestsDirectory = join(temporaryDirectory, "manifests");
  const { stateFile } = registerFakeFirefoxHost({
    manifestsDirectory,
    stateFile: join(temporaryDirectory, "state.json"),
    hostDirectory: join(temporaryDirectory, "host"),
    shouldResetState: true,
  });
  const state = createInitialState();
  state.pairingCode = "246810";
  state.allLogin = options.allLogin?.map((login) => ({ ...login, url: login.url || site.origin })) ?? [
    { vaultID: "vault-personal", vaultName: "Personal", itemID: "item-test-site", title: "Test site", username: "alex@example.com", password: "old password", url: site.origin },
  ];
  writeState(stateFile, state);

  const { firefoxPath, geckodriverPath } = await installTestFirefox();
  const firefoxOptions = new Options().setBinary(firefoxPath);
  if (!isHeaded) firefoxOptions.addArguments("-headless");
  firefoxOptions.setPreference("extensions.webextensions.uuids", JSON.stringify({ [firefoxExtensionID]: extensionUUID }));
  firefoxOptions.enableBidi();
  // Privileged scripts point Firefox at the test's host manifests and read the clipboard.
  const service = new ServiceBuilder(geckodriverPath).addArguments("--allow-system-access");
  const driver = (await new Builder().forBrowser("firefox").setFirefoxOptions(firefoxOptions).setFirefoxService(service).build()) as Driver;

  // Firefox reads the user's real NativeMessagingHosts whatever HOME says, so the directory it looks in is replaced before
  // the extension first connects.
  await driver.setContext(Context.CHROME);
  await driver.executeScript(
    `const directory = Cc["@mozilla.org/file/local;1"].createInstance(Ci.nsIFile);
    directory.initWithPath(arguments[0]);
    try { Services.dirsvc.undefine("XREUserNativeManifests"); } catch {}
    Services.dirsvc.set("XREUserNativeManifests", directory);`,
    manifestsDirectory,
  );
  await driver.setContext(Context.CONTENT);
  // A temporary install keeps browser_specific_settings' ID, which the host manifest allows; geckodriver takes the
  // unpacked directory by path.
  await driver.execute(new Command("install addon").setParameter("path", distDirectory).setParameter("temporary", true));
  await driver.get(`${extensionBaseUrl}inline-menu.html`);
  const extensionHandle = await driver.getWindowHandle();
  const bidi = (await driver.getBidi()) as unknown as Bidi;

  const send = async (method: string, params: Record<string, unknown>): Promise<Record<string, unknown>> => {
    const response = await bidi.send({ method, params });
    if (response.error) throw new Error(`${method}: ${response.error} ${response.message ?? ""}`);
    return response.result ?? {};
  };

  const evaluateInExtension: FirefoxHarness["evaluateInExtension"] = async (script, ...allArgument) => {
    const current = await driver.getWindowHandle();
    await driver.switchTo().window(extensionHandle);
    try {
      return await driver.executeAsyncScript(`const done = arguments[arguments.length - 1]; (async () => { ${script} })(...arguments).then(done, (error) => done({ error: String(error) }));`, ...allArgument);
    } finally {
      await driver.switchTo().window(current);
    }
  };

  // Firefox's match patterns cannot name the test site's port, so the tab is found by its exact address.
  const getTabId: FirefoxHarness["getTabId"] = async (url) => {
    const tabId = await evaluateInExtension<number | undefined>("return (await chrome.tabs.query({})).find((tab) => tab.url === arguments[0])?.id;", url);
    if (tabId === undefined) throw new Error(`no tab at ${url}`);
    return tabId;
  };

  const findFrame: FirefoxHarness["findFrame"] = async (part) => {
    const result = await send("browsingContext.getTree", { root: await driver.getWindowHandle() });
    const flatten = (node: BrowsingContextNode): BrowsingContextNode[] => [node, ...(node.children ?? []).flatMap(flatten)];
    return (result.contexts as BrowsingContextNode[]).flatMap(flatten).find((node) => node.url.includes(part))?.context;
  };

  const readFrame: FirefoxHarness["readFrame"] = async (frame, expression) => {
    const result = await send("script.evaluate", { expression, target: { context: frame }, awaitPromise: true });
    return (result.result as { value?: unknown } | undefined)?.value;
  };

  return {
    driver,
    site,
    readState: () => readState(stateFile),
    updateState: (change) => {
      const current = readState(stateFile);
      change(current);
      writeState(stateFile, current);
    },
    async pairWithoutPopup() {
      const instanceID = "instance-firefox-end-to-end";
      const token = randomBytes(32).toString("hex");
      const current = readState(stateFile);
      current.allPairing.push({ instanceID, browser: "Firefox", tokenHash: createHash("sha256").update(token).digest("hex"), pairedAt: Date.now() });
      writeState(stateFile, current);
      await evaluateInExtension("await chrome.storage.local.set(arguments[0]);", { instanceID, token });
    },
    async openTab(url) {
      await driver.switchTo().newWindow("tab");
      await driver.get(url);
      return driver.getWindowHandle();
    },
    getTabId,
    async waitForContentScripts(url) {
      const tabId = await getTabId(url);
      const readAllLoaded = () =>
        evaluateInExtension<boolean[]>(
          "return (await chrome.scripting.executeScript({ target: { tabId: arguments[0], allFrames: true }, func: () => globalThis.isHisarContentScriptLoaded === true })).map((injection) => injection.result);",
          tabId,
        );
      await expect.poll(async () => (await readAllLoaded()).every(Boolean), { timeout: 15_000 }).toBe(true);
    },
    evaluateInExtension,
    findFrame,
    async waitForFrame(part) {
      await expect.poll(() => findFrame(part), { timeout: 15_000 }).toBeDefined();
      return (await findFrame(part))!;
    },
    readFrame,
    async clickInFrame(frame, selector, index = 0) {
      const result = await send("browsingContext.locateNodes", { context: frame, locator: { type: "css", value: selector } });
      const node = (result.nodes as { sharedId: string }[])[index];
      if (!node) throw new Error(`no ${selector} in the frame`);
      // The click may fill the page and close its frame before the action ends, which Firefox 140 reports as no such frame;
      // the caller checks what the click did.
      await send("input.performActions", {
        context: frame,
        actions: [
          {
            type: "pointer",
            id: "mouse",
            parameters: { pointerType: "mouse" },
            actions: [
              { type: "pointerMove", x: 0, y: 0, origin: { type: "element", element: { sharedId: node.sharedId } } },
              { type: "pointerDown", button: 0 },
              { type: "pointerUp", button: 0 },
            ],
          },
        ],
      }).catch((error: Error) => {
        if (!error.message.includes("no such frame")) throw error;
      });
    },
    async readClipboard() {
      await driver.setContext(Context.CHROME);
      try {
        return await driver.executeScript<string>(
          `const transferable = Cc["@mozilla.org/widget/transferable;1"].createInstance(Ci.nsITransferable);
          transferable.init(null);
          transferable.addDataFlavor("text/plain");
          Services.clipboard.getData(transferable, Services.clipboard.kGlobalClipboard, null);
          const data = {};
          try { transferable.getTransferData("text/plain", data); } catch { return ""; }
          return data.value.QueryInterface(Ci.nsISupportsString).data;`,
        );
      } finally {
        await driver.setContext(Context.CONTENT);
      }
    },
    async writeClipboard(text) {
      await driver.setContext(Context.CHROME);
      try {
        await driver.executeScript(`Cc["@mozilla.org/widget/clipboardhelper;1"].getService(Ci.nsIClipboardHelper).copyString(arguments[0]);`, text);
      } finally {
        await driver.setContext(Context.CONTENT);
      }
    },
    async close() {
      await driver.quit().catch(() => undefined);
      await site.close();
      rmSync(temporaryDirectory, { recursive: true, force: true });
    },
  };
}

export { By };
