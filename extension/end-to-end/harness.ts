import { createHash, randomBytes } from "node:crypto";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { chromium, type BrowserContext, type Frame, type Page } from "@playwright/test";
import { createInitialState, readState, writeState, type FakeHostState } from "../tools/fake-native-host.mjs";
import { extensionID, registerFakeHost } from "../tools/register-fake-host.mjs";
import { startTestSite, type TestSite } from "./test-site";

const distDirectory = resolve(import.meta.dirname, "../dist");
const isHeaded = process.env.HISAR_HEADED === "1";

interface CdpNode {
  nodeId: number;
  backendNodeId: number;
  nodeName: string;
  nodeValue?: string;
  attributes?: string[];
  children?: CdpNode[];
  shadowRoots?: CdpNode[];
}

function collectText(node: CdpNode): string {
  const allChild = [...(node.shadowRoots ?? []), ...(node.children ?? [])];
  return (node.nodeValue ?? "") + allChild.map(collectText).join("");
}

function findAllNode(node: CdpNode, predicate: (candidate: CdpNode) => boolean, allMatch: CdpNode[] = []): CdpNode[] {
  if (predicate(node)) allMatch.push(node);
  for (const child of [...(node.shadowRoots ?? []), ...(node.children ?? [])]) findAllNode(child, predicate, allMatch);
  return allMatch;
}

export interface HarnessOptions {
  chromePath?: string;
  deviceScaleFactor?: number;
  allLogin?: FakeHostState["allLogin"];
}

export interface Harness {
  context: BrowserContext;
  evaluateInExtension: <Result, Argument>(callback: (argument: Argument) => Result | Promise<Result>, argument: Argument) => Promise<Result>;
  site: TestSite;
  stateFile: string;
  updateState: (change: (state: FakeHostState) => void) => void;
  readState: () => FakeHostState;
  getTabId: (url: string) => Promise<number>;
  pair: (tabId: number) => Promise<Page>;
  pairWithoutPopup: () => Promise<void>;
  readShadowHost: (
    page: Page,
    tagName: string,
  ) => Promise<{ text: string; clickButton: (label: string) => Promise<void>; typeInto: (field: string, text: string) => Promise<void> } | undefined>;
  findInlineMenuFrame: (page: Page) => Frame | undefined;
  readInlineMenuSource: (page: Page) => Promise<string | undefined>;
  close: () => Promise<void>;
}

export async function launchHarness(options: HarnessOptions = {}): Promise<Harness> {
  const temporaryDirectory = mkdtempSync(join(tmpdir(), "hisar-end-to-end-"));
  const site = await startTestSite();
  const { stateFile } = registerFakeHost({
    userDataDirectory: join(temporaryDirectory, "profile"),
    stateFile: join(temporaryDirectory, "state.json"),
    hostDirectory: join(temporaryDirectory, "host"),
    shouldResetState: true,
  });
  const state = createInitialState();
  state.pairingCode = "246810";
  state.allLogin = options.allLogin?.map((login) => ({ ...login, url: login.url || site.origin })) ?? [
    {
      vaultID: "vault-personal",
      vaultName: "Personal",
      itemID: "item-test-site",
      title: "Test site",
      username: "alex@example.com",
      password: "old password",
      url: site.origin,
    },
  ];
  writeState(stateFile, state);
  const context = await chromium.launchPersistentContext(join(temporaryDirectory, "profile"), {
    ...(options.chromePath ? { executablePath: options.chromePath } : { channel: "chromium" }),
    headless: !isHeaded,
    ignoreDefaultArgs: ["--disable-extensions"],
    deviceScaleFactor: options.deviceScaleFactor,
    // Branded Chrome ignores --load-extension, so it loads the build over the DevTools protocol instead.
    args: options.chromePath
      ? ["--enable-unsafe-extension-debugging"]
      : [`--disable-extensions-except=${distDirectory}`, `--load-extension=${distDirectory}`],
  });
  if (options.chromePath) {
    const session = await context.browser()!.newBrowserCDPSession();
    await session.send("Extensions.loadUnpacked" as never, { path: distDirectory } as never);
  }
  const extensionPage = await context.newPage();
  await extensionPage.goto(`chrome-extension://${extensionID}/inline-menu.html`);
  const evaluateInExtension: Harness["evaluateInExtension"] = (callback, argument) =>
    extensionPage.evaluate(callback as (argument: unknown) => unknown, argument) as never;

  // The save bar and the inline icon live in closed shadow roots, which only the DevTools protocol can see into.
  const readShadowHost: Harness["readShadowHost"] = async (page, tagName) => {
    const session = await context.newCDPSession(page);
    try {
      const { root } = (await session.send("DOM.getDocument", { depth: -1, pierce: true })) as { root: CdpNode };
      const [host] = findAllNode(root, (node) => node.nodeName === tagName.toUpperCase());
      if (!host) return undefined;
      const allButton = findAllNode(host, (node) => node.nodeName === "BUTTON");
      const clickNode = async (node: CdpNode): Promise<void> => {
        const clickSession = await context.newCDPSession(page);
        const { model } = (await clickSession.send("DOM.getBoxModel", { backendNodeId: node.backendNodeId })) as {
          model: { content: number[] };
        };
        const [left, top, , , right, bottom] = model.content;
        await page.mouse.click((left + right) / 2, (top + bottom) / 2);
        await clickSession.detach();
      };
      return {
        text: collectText(host),
        clickButton: async (label) => {
          const button = allButton.find((candidate) => collectText(candidate).trim() === label || label === "*");
          if (!button) throw new Error(`no ${label} button in ${tagName}`);
          await clickNode(button);
        },
        // typeInto clicks the field whose data-field is field and types text over its value; in a list, typing picks the
        // option that starts with it.
        typeInto: async (field, text) => {
          const [node] = findAllNode(host, (candidate) => {
            const attributes = candidate.attributes ?? [];
            return attributes.some((value, index) => index % 2 === 0 && value === "data-field" && attributes[index + 1] === field);
          });
          if (!node) throw new Error(`no ${field} field in ${tagName}`);
          await clickNode(node);
          if (node.nodeName === "SELECT") {
            await page.keyboard.press("Escape");
            await page.keyboard.type(text);
            return;
          }
          await page.keyboard.press("ControlOrMeta+A");
          await page.keyboard.type(text);
        },
      };
    } finally {
      await session.detach();
    }
  };

  const getTabId = (url: string): Promise<number> =>
    extensionPage.evaluate(async (targetUrl) => {
      const [tab] = await chrome.tabs.query({ url: targetUrl });
      return tab.id as number;
    }, url);

  return {
    context,
    evaluateInExtension,
    site,
    stateFile,
    updateState: (change) => {
      const current = readState(stateFile);
      change(current);
      writeState(stateFile, current);
    },
    readState: () => readState(stateFile),
    getTabId,
    async pair(tabId) {
      const popup = await context.newPage();
      await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
      await popup.getByTestId("start-pairing").click();
      await popup.getByTestId("pairing-code").fill("246810");
      await popup.getByTestId("finish-pairing").click();
      await popup.getByTestId("unlocked-view").waitFor();
      return popup;
    },
    async pairWithoutPopup() {
      const instanceID = "instance-end-to-end";
      const token = randomBytes(32).toString("hex");
      const current = readState(stateFile);
      current.allPairing.push({ instanceID, browser: "Chrome", tokenHash: createHash("sha256").update(token).digest("hex"), pairedAt: Date.now() });
      writeState(stateFile, current);
      await evaluateInExtension((stored) => chrome.storage.local.set(stored), { instanceID, token });
    },
    readShadowHost,
    async readInlineMenuSource(page) {
      const session = await context.newCDPSession(page);
      try {
        const { root } = (await session.send("DOM.getDocument", { depth: -1, pierce: true })) as { root: CdpNode };
        const [host] = findAllNode(root, (node) => node.nodeName === "HISAR-INLINE-MENU");
        const [iframe] = host ? findAllNode(host, (node) => node.nodeName === "IFRAME") : [];
        const attributes = iframe?.attributes ?? [];
        const sourceIndex = attributes.indexOf("src");
        return sourceIndex >= 0 ? attributes[sourceIndex + 1] : undefined;
      } finally {
        await session.detach();
      }
    },
    findInlineMenuFrame: (page) => page.frames().find((frame) => frame.url().includes("/inline-menu.html")),
    async close() {
      await context.close();
      await site.close();
      rmSync(temporaryDirectory, { recursive: true, force: true });
    },
  };
}
